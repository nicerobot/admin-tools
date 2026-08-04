package cleanupruns

import (
	"context"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/nicerobot/tools.admin/internal/constants"
	"github.com/nicerobot/tools.admin/internal/domain"
	"github.com/nicerobot/tools.admin/internal/github"
	"github.com/nicerobot/tools.admin/internal/repo"
)

const dateLayout = "2006-01-02"

// RepoResult is one repo's pruning outcome.
type RepoResult struct {
	Name    string `json:"name"`
	Deleted int    `json:"deleted"`
	Kept    int    `json:"kept"`
}

// Result is the outcome of the cleanup-runs command: whether it was a dry run,
// how many repos were scanned, the totals, and the per-repo breakdown for the
// repos that had runs to prune.
type Result struct {
	Repos        []RepoResult `json:"repos"`
	ReposScanned int          `json:"repos_scanned"`
	Deleted      int          `json:"deleted"`
	Kept         int          `json:"kept"`
	IsDryRun     bool         `json:"dry_run"`
}

// githubAPI is the GitHub surface cleanup-runs needs.
type githubAPI interface {
	ListRepos(owner repo.Owner) ([]github.Repository, error)
	ListWorkflowRuns(owner repo.Owner, name repo.Name, before repo.CreatedBefore) ([]github.WorkflowRun, error)
	DeleteWorkflowRun(owner repo.Owner, name repo.Name, id repo.RunID) error
}

// dependencies are cleanup-runs' injected collaborators.
type dependencies struct {
	github githubAPI
	getenv func(key string) string
	now    func() time.Time
}

// deps builds the production collaborators. It is indirected through a variable
// so tests substitute in-memory fakes.
var deps = osDeps

// osDeps wires the OS-backed GitHub client (from GH_TOKEN/GITHUB_API_URL), the
// environment lookup, and the wall clock.
func osDeps() (dependencies, error) {
	client, err := github.NewFromEnv(os.Getenv)
	if err != nil {
		return dependencies{}, err
	}
	return dependencies{github: client, getenv: os.Getenv, now: time.Now}, nil
}

// Run executes the cleanup-runs command, returning a structured Result the app
// tier renders. It orchestrates the github package and holds no presentation
// logic.
func Run(_ context.Context, logger *slog.Logger, cfg Config, _ ...domain.Argument) (Result, error) {
	d, err := deps()
	if err != nil {
		return Result{}, err
	}
	return run(d, logger, cfg)
}

func run(d dependencies, logger *slog.Logger, cfg Config) (Result, error) {
	owner, name, err := resolveTarget(cfg.Owner, cfg.Repo, d.getenv)
	if err != nil {
		return Result{}, err
	}
	names, err := targetRepos(d, owner, name)
	if err != nil {
		return Result{}, err
	}
	cutoff := cutoffDate(d.now, cfg.Days)
	repos, err := pruneRepos(d, owner, names, cutoff, cfg.Keep, cfg.IsDryRun)
	if err != nil {
		return Result{}, err
	}
	result := summarize(names, repos, cfg.IsDryRun)
	logger.Info("Cleanup complete.",
		"repos", result.ReposScanned, "deleted", result.Deleted, "kept", result.Kept, "dry_run", bool(cfg.IsDryRun))
	return result, nil
}

func resolveTarget(owner repo.Owner, name repo.Name, env func(string) string) (repo.Owner, repo.Name, error) {
	if owner != "" {
		return owner, name, nil
	}
	o, n, ok := strings.Cut(env("GITHUB_REPOSITORY"), "/")
	if !ok || o == "" || n == "" {
		return "", "", constants.ErrNoTarget.With(nil)
	}
	return repo.Owner(o), repo.Name(n), nil
}

func cutoffDate(now func() time.Time, days repo.Days) repo.CreatedBefore {
	return repo.CreatedBefore(now().UTC().AddDate(0, 0, -int(days)).Format(dateLayout))
}

func targetRepos(d dependencies, owner repo.Owner, name repo.Name) ([]repo.Name, error) {
	if name != "" {
		return []repo.Name{name}, nil
	}
	repos, err := d.github.ListRepos(owner)
	if err != nil {
		return nil, err
	}
	names := make([]repo.Name, 0, len(repos))
	for _, r := range repos {
		names = append(names, repo.Name(r.Name))
	}
	sort.Slice(names, func(i, j int) bool { return names[i] < names[j] })
	return names, nil
}

func pruneRepos(
	d dependencies,
	owner repo.Owner,
	names []repo.Name,
	cutoff repo.CreatedBefore,
	keep repo.KeepCount,
	isDryRun repo.DryRun,
) ([]RepoResult, error) {
	results := make([]RepoResult, 0)
	for _, name := range names {
		r, ok, err := pruneRepo(d, owner, name, cutoff, keep, isDryRun)
		if err != nil {
			return nil, err
		}
		if ok {
			results = append(results, r)
		}
	}
	return results, nil
}

func pruneRepo(
	d dependencies,
	owner repo.Owner,
	name repo.Name,
	cutoff repo.CreatedBefore,
	keep repo.KeepCount,
	isDryRun repo.DryRun,
) (RepoResult, bool, error) {
	// EVERY completed run, not just the ones past the cutoff. The keep floor
	// means "this workflow always retains N runs of history", which can only be
	// judged against the full history — asking the API for the old ones alone
	// and then keeping N of THOSE protects the oldest runs and guarantees the
	// newest are the ones considered for deletion, which is backwards.
	runs, err := d.github.ListWorkflowRuns(owner, name, "")
	if err != nil {
		return RepoResult{}, false, err
	}
	toDelete, kept := partition(runs, keep, cutoff)
	if len(toDelete) == 0 {
		return RepoResult{}, false, nil
	}
	if err := deleteRuns(d, owner, name, toDelete, isDryRun); err != nil {
		return RepoResult{}, false, err
	}
	return RepoResult{Name: string(name), Deleted: len(toDelete), Kept: kept}, true, nil
}

func deleteRuns(
	d dependencies,
	owner repo.Owner,
	name repo.Name,
	runs []github.WorkflowRun,
	isDryRun repo.DryRun,
) error {
	if bool(isDryRun) {
		return nil
	}
	for _, r := range runs {
		if err := d.github.DeleteWorkflowRun(owner, name, repo.RunID(r.ID)); err != nil {
			return err
		}
	}
	return nil
}

func summarize(names []repo.Name, repos []RepoResult, isDryRun repo.DryRun) Result {
	deleted, kept := 0, 0
	for _, r := range repos {
		deleted += r.Deleted
		kept += r.Kept
	}
	return Result{
		IsDryRun:     bool(isDryRun),
		ReposScanned: len(names),
		Deleted:      deleted,
		Kept:         kept,
		Repos:        repos,
	}
}
