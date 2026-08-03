package dependabot

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path"

	"github.com/nicerobot/tools.admin/internal/constants"
	"github.com/nicerobot/tools.admin/internal/domain"
	"github.com/nicerobot/tools.admin/internal/github"
	"github.com/nicerobot/tools.admin/internal/repo"
)

// Skip reasons. Each names precisely why a pull request was left alone, so the
// result distinguishes "nothing to do" from "declined, and here is why".
const (
	skipMajor      repo.SkipReason = "major version bump; awaiting review"
	skipUnknown    repo.SkipReason = "version transition could not be parsed"
	skipNotForward repo.SkipReason = "version transition does not provably move forward"
	skipDraft      repo.SkipReason = "pull request is a draft"
	skipPending    repo.SkipReason = "checks still running"
	skipFailing    repo.SkipReason = "checks not passing"
	skipNoChecks   repo.SkipReason = "no checks reported"
	skipUnresolved repo.SkipReason = "repository could not be resolved from search result"
	skipDryRun     repo.SkipReason = "would merge (dry run)"
	skipRefused    repo.SkipReason = "merge refused by GitHub"
	skipFetch      repo.SkipReason = "pull request could not be fetched"
)

// excludedBy renders the skip reason for a held repository, naming the pattern
// that held it so a sweep result says why, not merely that.
func excludedBy(pattern repo.ExcludePattern) repo.SkipReason {
	return repo.SkipReason("excluded by pattern " + string(pattern))
}

// PullResult is one pull request's outcome. Reason is empty exactly when the
// pull request was merged.
type PullResult struct {
	Repo     string `json:"repo"`
	Title    string `json:"title"`
	Reason   string `json:"reason,omitempty"`
	Number   int    `json:"number"`
	IsMerged bool   `json:"merged"`
}

// Result is the outcome of a sweep: what it walked, what it merged, what it
// declined, and whether it stopped before finishing.
type Result struct {
	Pulls          []PullResult `json:"pulls"`
	OwnersScanned  int          `json:"owners_scanned"`
	Considered     int          `json:"considered"`
	Merged         int          `json:"merged"`
	Skipped        int          `json:"skipped"`
	IsStoppedEarly bool         `json:"stopped_early"`
	IsDryRun       bool         `json:"dry_run"`
}

// githubAPI is the GitHub surface the sweep needs.
type githubAPI interface {
	SearchDependabotPulls(owner repo.Owner) ([]github.SearchItem, error)
	GetPullRequest(owner repo.Owner, name repo.Name, number repo.PullNumber) (github.PullRequest, error)
	ListCheckRuns(owner repo.Owner, name repo.Name, sha repo.SHA) ([]github.CheckRun, error)
	MergePullRequest(owner repo.Owner, name repo.Name, number repo.PullNumber, method repo.MergeMethod) error
	RateRemaining() (repo.RateFloor, error)
}

// dependencies are the sweep's injected collaborators.
type dependencies struct {
	github githubAPI
}

// deps builds the production collaborators, indirected so tests substitute
// in-memory fakes.
var deps = osDeps

func osDeps() (dependencies, error) {
	client, err := github.NewFromEnv(os.Getenv)
	if err != nil {
		return dependencies{}, err
	}
	return dependencies{github: client}, nil
}

// Run executes the sweep, returning a structured Result the app tier renders.
func Run(_ context.Context, logger *slog.Logger, cfg Config, _ ...domain.Argument) (Result, error) {
	d, err := deps()
	if err != nil {
		return Result{}, err
	}
	return run(d, logger, cfg)
}

func run(d dependencies, logger *slog.Logger, cfg Config) (Result, error) {
	if len(cfg.Owners) == 0 {
		return Result{}, constants.ErrNoOwners.With(nil)
	}
	result := Result{IsDryRun: bool(cfg.IsDryRun)}
	for _, owner := range cfg.Owners {
		ok, err := hasBudget(d, cfg.RateFloor)
		if err != nil {
			return Result{}, err
		}
		if !ok {
			result.IsStoppedEarly = true
			break
		}
		result = sweepOwner(d, cfg, owner, result)
	}
	return summarize(result, logger), nil
}

// hasBudget reports whether the remaining API budget is above the floor.
func hasBudget(d dependencies, floor repo.RateFloor) (bool, error) {
	remaining, err := d.github.RateRemaining()
	if err != nil {
		return false, err
	}
	return remaining > floor, nil
}

// sweepOwner folds one owner's pull requests into the running result. A search
// failure for one owner is recorded and the sweep moves on, so a single bad
// owner cannot abort the fleet.
func sweepOwner(d dependencies, cfg Config, owner repo.Owner, result Result) Result {
	result.OwnersScanned++
	items, err := d.github.SearchDependabotPulls(owner)
	if err != nil {
		return append1(result, PullResult{Repo: string(owner), Reason: err.Error()})
	}
	for _, item := range items {
		result = append1(result, considerPull(d, cfg, owner, item))
	}
	return result
}

// considerPull judges and, when it qualifies, merges one pull request.
func considerPull(d dependencies, cfg Config, owner repo.Owner, item github.SearchItem) PullResult {
	name := item.RepoName()
	out := PullResult{Repo: string(name), Number: item.Number, Title: item.Title}
	if name == "" {
		return declined(out, skipUnresolved)
	}
	// Checked before any per-pull request: a held repo costs the sweep nothing.
	if pattern, held := excluded(cfg.Excludes, owner, name); held {
		return declined(out, excludedBy(pattern))
	}
	pull, err := d.github.GetPullRequest(owner, name, repo.PullNumber(item.Number))
	if err != nil {
		return declined(out, skipFetch)
	}
	if reason, ok := gate(d, cfg, owner, name, pull); !ok {
		return declined(out, reason)
	}
	return merge(d, cfg, owner, name, out)
}

// excluded reports the first pattern holding owner/name, if any. A malformed
// pattern never matches: path.Match's error is treated as no match, so a typo
// silently widens nothing.
func excluded(
	patterns []repo.ExcludePattern,
	owner repo.Owner,
	name repo.Name,
) (repo.ExcludePattern, bool) {
	slug := string(owner) + "/" + string(name)
	for _, pattern := range patterns {
		if ok, err := path.Match(string(pattern), slug); err == nil && ok {
			return pattern, true
		}
	}
	return "", false
}

// gate applies every precondition in increasing order of cost: cheap local
// judgements before the request that lists check runs.
func gate(
	d dependencies,
	cfg Config,
	owner repo.Owner,
	name repo.Name,
	pull github.PullRequest,
) (repo.SkipReason, bool) {
	if pull.IsDraft {
		return skipDraft, false
	}
	if reason, ok := gateBump(cfg, pullBody(pull.Body)); !ok {
		return reason, false
	}
	return gateChecks(d, owner, name, pull.HeadSHA())
}

// gateBump enforces the semver policy: major bumps await human review unless
// explicitly allowed, an unparseable transition is never merged, and neither is
// one that cannot be shown to move forward — staying inside a major says
// nothing about direction, and a downgrade proposed as an update merges just as
// quietly as a real one.
func gateBump(cfg Config, body pullBody) (repo.SkipReason, bool) {
	switch classify(body) {
	case bumpWithinMajor:
		return "", true
	case bumpMajor:
		return skipMajor, bool(cfg.IsMajorAllowed)
	case bumpNotForward:
		return skipNotForward, false
	case bumpUnknown:
		// Named rather than left to the default: an unjudgeable transition is
		// declined deliberately, and so is any classification a later member
		// adds — a new bump kind must be judged here before it can be merged.
		fallthrough
	default:
		return skipUnknown, false
	}
}

// gateChecks requires at least one check run, all complete, all passing. A
// commit with no checks is skipped rather than merged: absence of evidence is
// not evidence of green.
func gateChecks(d dependencies, owner repo.Owner, name repo.Name, sha repo.SHA) (repo.SkipReason, bool) {
	runs, err := d.github.ListCheckRuns(owner, name, sha)
	if err != nil {
		return skipFetch, false
	}
	if len(runs) == 0 {
		return skipNoChecks, false
	}
	return verdict(runs)
}

// verdict reduces a commit's check runs to a single merge decision.
func verdict(runs []github.CheckRun) (repo.SkipReason, bool) {
	for _, run := range runs {
		if !run.IsComplete() {
			return skipPending, false
		}
		if !run.IsPassing() {
			return skipFailing, false
		}
	}
	return "", true
}

// merge performs the merge, or reports what a real run would have done.
func merge(d dependencies, cfg Config, owner repo.Owner, name repo.Name, out PullResult) PullResult {
	if bool(cfg.IsDryRun) {
		return declined(out, skipDryRun)
	}
	err := d.github.MergePullRequest(owner, name, repo.PullNumber(out.Number), cfg.MergeMethod)
	if err != nil {
		return declined(out, mergeRefusal(err))
	}
	out.IsMerged = true
	return out
}

// mergeRefusal classifies a merge failure. GitHub refuses a merge it considers
// unsafe (head moved, branch behind, not mergeable) with an HTTP status; that is
// a skip, not a sweep failure, because the next sweep will retry it.
func mergeRefusal(err error) repo.SkipReason {
	if errors.Is(err, constants.ErrHTTPStatus) {
		return skipRefused
	}
	return repo.SkipReason(err.Error())
}

// declined stamps a skip reason onto a result.
func declined(out PullResult, reason repo.SkipReason) PullResult {
	out.Reason = string(reason)
	return out
}

// append1 adds one pull result and keeps the running tallies consistent.
func append1(result Result, out PullResult) Result {
	result.Pulls = append(result.Pulls, out)
	result.Considered++
	if out.IsMerged {
		result.Merged++
		return result
	}
	result.Skipped++
	return result
}

func summarize(result Result, logger *slog.Logger) Result {
	if result.Pulls == nil {
		result.Pulls = []PullResult{}
	}
	logger.Info("Dependabot sweep complete.",
		"owners", result.OwnersScanned,
		"considered", result.Considered,
		"merged", result.Merged,
		"skipped", result.Skipped,
		"stopped_early", result.IsStoppedEarly,
		"dry_run", result.IsDryRun)
	return result
}
