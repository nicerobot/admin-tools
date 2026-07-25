package dependabot

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nicerobot/tools.admin/internal/constants"
	"github.com/nicerobot/tools.admin/internal/github"
	"github.com/nicerobot/tools.admin/internal/repo"
)

type mergeCall struct {
	owner  repo.Owner
	name   repo.Name
	method repo.MergeMethod
	number repo.PullNumber
}

type fakeGH struct {
	pulls     map[string]github.PullRequest
	checks    map[string][]github.CheckRun
	searchErr error
	getErr    error
	checksErr error
	mergeErr  error
	rateErr   error
	items     []github.SearchItem
	merges    []mergeCall
	remaining repo.RateFloor
}

func (f *fakeGH) SearchDependabotPulls(repo.Owner) ([]github.SearchItem, error) {
	return f.items, f.searchErr
}

func (f *fakeGH) GetPullRequest(_ repo.Owner, n repo.Name, _ repo.PullNumber) (github.PullRequest, error) {
	if f.getErr != nil {
		return github.PullRequest{}, f.getErr
	}
	return f.pulls[string(n)], nil
}

func (f *fakeGH) ListCheckRuns(_ repo.Owner, n repo.Name, _ repo.SHA) ([]github.CheckRun, error) {
	if f.checksErr != nil {
		return nil, f.checksErr
	}
	return f.checks[string(n)], nil
}

func (f *fakeGH) MergePullRequest(
	o repo.Owner,
	n repo.Name,
	num repo.PullNumber,
	m repo.MergeMethod,
) error {
	if f.mergeErr != nil {
		return f.mergeErr
	}
	f.merges = append(f.merges, mergeCall{owner: o, name: n, method: m, number: num})
	return nil
}

func (f *fakeGH) RateRemaining() (repo.RateFloor, error) {
	return f.remaining, f.rateErr
}

// item builds a search result for the canonical test repository.
func item(number int) github.SearchItem {
	return github.SearchItem{
		RepositoryURL: "https://api.github.com/repos/acme/widget",
		Number:        number,
		Title:         "bump widget",
	}
}

func passing(names ...string) []github.CheckRun {
	out := make([]github.CheckRun, 0, len(names))
	for _, n := range names {
		out = append(out, github.CheckRun{Name: n, Status: "completed", Conclusion: "success"})
	}
	return out
}

func withinMajor() github.PullRequest {
	return github.PullRequest{Body: "Bumps foo from 1.1.0 to 1.2.0.", Head: github.CommitRef{SHA: "abc"}}
}

func sweep(t *testing.T, gh *fakeGH, cfg Config) (Result, error) {
	t.Helper()
	orig := deps
	t.Cleanup(func() { deps = orig })
	deps = func() (dependencies, error) { return dependencies{github: gh}, nil }
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelWarn}))
	return Run(context.Background(), logger, cfg)
}

func baseCfg() Config {
	return Config{
		Owners:      []repo.Owner{"acme"},
		MergeMethod: repo.MergeMethodSquash,
		RateFloor:   10,
	}
}

func TestRunRequiresOwners(t *testing.T) {
	_, err := sweep(t, &fakeGH{remaining: 100}, Config{})
	assert.New(t).ErrorIs(err, constants.ErrNoOwners)
}

func TestRunMergesGreenWithinMajor(t *testing.T) {
	want, must := assert.New(t), require.New(t)

	gh := &fakeGH{
		remaining: 100,
		items:     []github.SearchItem{item(7)},
		pulls:     map[string]github.PullRequest{"widget": withinMajor()},
		checks:    map[string][]github.CheckRun{"widget": passing("go", "docs")},
	}

	result, err := sweep(t, gh, baseCfg())
	must.NoError(err)

	want.Equal(1, result.Merged)
	want.Equal(0, result.Skipped)
	want.Equal(1, result.Considered)
	want.Equal(1, result.OwnersScanned)
	want.False(result.IsStoppedEarly)
	must.Len(result.Pulls, 1)
	want.True(result.Pulls[0].IsMerged)
	want.Empty(result.Pulls[0].Reason)
	want.Equal(
		[]mergeCall{{owner: "acme", name: "widget", method: repo.MergeMethodSquash, number: 7}},
		gh.merges,
	)
}

func TestRunSkipReasons(t *testing.T) {
	tests := []struct {
		name           string
		wantReason     repo.SkipReason
		checks         []github.CheckRun
		pull           github.PullRequest
		isMajorAllowed repo.MajorAllowed
		isDryRun       repo.DryRun
	}{
		{
			name:       "draft",
			pull:       github.PullRequest{IsDraft: true, Body: "Bumps foo from 1.1.0 to 1.2.0."},
			checks:     passing("go"),
			wantReason: skipDraft,
		},
		{
			name:       "major bump declined by policy",
			pull:       github.PullRequest{Body: "Bumps foo from 1.1.0 to 2.0.0."},
			checks:     passing("go"),
			wantReason: skipMajor,
		},
		{
			name:       "unparseable transition",
			pull:       github.PullRequest{Body: "Bumps several things together."},
			checks:     passing("go"),
			wantReason: skipUnknown,
		},
		{
			name:       "no checks reported",
			pull:       withinMajor(),
			checks:     nil,
			wantReason: skipNoChecks,
		},
		{
			name:       "checks still running",
			pull:       withinMajor(),
			checks:     []github.CheckRun{{Name: "go", Status: "in_progress"}},
			wantReason: skipPending,
		},
		{
			name: "checks failing",
			pull: withinMajor(),
			checks: []github.CheckRun{
				{Name: "go", Status: "completed", Conclusion: "failure"},
			},
			wantReason: skipFailing,
		},
		{
			name:       "dry run reports without merging",
			pull:       withinMajor(),
			checks:     passing("go"),
			isDryRun:   true,
			wantReason: skipDryRun,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want, must := assert.New(t), require.New(t)

			gh := &fakeGH{
				remaining: 100,
				items:     []github.SearchItem{item(1)},
				pulls:     map[string]github.PullRequest{"widget": tt.pull},
				checks:    map[string][]github.CheckRun{"widget": tt.checks},
			}
			cfg := baseCfg()
			cfg.IsMajorAllowed, cfg.IsDryRun = tt.isMajorAllowed, tt.isDryRun

			result, err := sweep(t, gh, cfg)
			must.NoError(err)

			must.Len(result.Pulls, 1)
			want.Equal(string(tt.wantReason), result.Pulls[0].Reason)
			want.False(result.Pulls[0].IsMerged)
			want.Equal(1, result.Skipped)
			want.Empty(gh.merges)
		})
	}
}

func TestRunAllowMajorMergesMajorBump(t *testing.T) {
	want, must := assert.New(t), require.New(t)

	gh := &fakeGH{
		remaining: 100,
		items:     []github.SearchItem{item(1)},
		pulls: map[string]github.PullRequest{
			"widget": {Body: "Bumps foo from 1.1.0 to 2.0.0.", Head: github.CommitRef{SHA: "abc"}},
		},
		checks: map[string][]github.CheckRun{"widget": passing("go")},
	}
	cfg := baseCfg()
	cfg.IsMajorAllowed = true

	result, err := sweep(t, gh, cfg)
	must.NoError(err)
	want.Equal(1, result.Merged)
	want.Len(gh.merges, 1)
}

func TestRunNeverMergesUnparseableEvenWithAllowMajor(t *testing.T) {
	want, must := assert.New(t), require.New(t)

	gh := &fakeGH{
		remaining: 100,
		items:     []github.SearchItem{item(1)},
		pulls:     map[string]github.PullRequest{"widget": {Body: "no versions here"}},
		checks:    map[string][]github.CheckRun{"widget": passing("go")},
	}
	cfg := baseCfg()
	cfg.IsMajorAllowed = true

	result, err := sweep(t, gh, cfg)
	must.NoError(err)
	want.Equal(string(skipUnknown), result.Pulls[0].Reason)
	want.Empty(gh.merges)
}

func TestRunStopsAtRateFloor(t *testing.T) {
	want, must := assert.New(t), require.New(t)

	gh := &fakeGH{remaining: 5, items: []github.SearchItem{item(1)}}
	cfg := baseCfg()
	cfg.Owners = []repo.Owner{"acme", "other"}

	result, err := sweep(t, gh, cfg)
	must.NoError(err)

	want.True(result.IsStoppedEarly)
	want.Equal(0, result.OwnersScanned)
	want.Equal(0, result.Considered)
	want.Empty(result.Pulls)
}

func TestRunRateErrorFailsSweep(t *testing.T) {
	sentinel := errors.New("rate unavailable")
	_, err := sweep(t, &fakeGH{rateErr: sentinel}, baseCfg())
	assert.New(t).ErrorIs(err, sentinel)
}

func TestRunRecordsSearchFailureAndContinues(t *testing.T) {
	want, must := assert.New(t), require.New(t)

	gh := &fakeGH{remaining: 100, searchErr: errors.New("search exploded")}
	cfg := baseCfg()
	cfg.Owners = []repo.Owner{"acme", "other"}

	result, err := sweep(t, gh, cfg)
	must.NoError(err)

	want.Equal(2, result.OwnersScanned)
	want.Equal(2, result.Skipped)
	must.Len(result.Pulls, 2)
	want.Contains(result.Pulls[0].Reason, "search exploded")
}

func TestRunSkipsUnresolvableRepository(t *testing.T) {
	want, must := assert.New(t), require.New(t)

	gh := &fakeGH{
		remaining: 100,
		items:     []github.SearchItem{{RepositoryURL: "", Number: 1, Title: "orphan"}},
	}

	result, err := sweep(t, gh, baseCfg())
	must.NoError(err)
	must.Len(result.Pulls, 1)
	want.Equal(string(skipUnresolved), result.Pulls[0].Reason)
}

func TestRunSkipsWhenPullCannotBeFetched(t *testing.T) {
	want, must := assert.New(t), require.New(t)

	gh := &fakeGH{
		remaining: 100,
		items:     []github.SearchItem{item(1)},
		getErr:    errors.New("boom"),
	}

	result, err := sweep(t, gh, baseCfg())
	must.NoError(err)
	want.Equal(string(skipFetch), result.Pulls[0].Reason)
}

func TestRunSkipsWhenChecksCannotBeListed(t *testing.T) {
	want, must := assert.New(t), require.New(t)

	gh := &fakeGH{
		remaining: 100,
		items:     []github.SearchItem{item(1)},
		pulls:     map[string]github.PullRequest{"widget": withinMajor()},
		checksErr: errors.New("boom"),
	}

	result, err := sweep(t, gh, baseCfg())
	must.NoError(err)
	want.Equal(string(skipFetch), result.Pulls[0].Reason)
}

func TestRunClassifiesMergeRefusal(t *testing.T) {
	tests := []struct {
		name       string
		mergeErr   error
		wantReason string
	}{
		{
			name:       "github refusal is a skip, retried next sweep",
			mergeErr:   constants.ErrHTTPStatus.With(nil, "status", 405),
			wantReason: string(skipRefused),
		},
		{
			name:       "other failures surface verbatim",
			mergeErr:   errors.New("transport died"),
			wantReason: "transport died",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want, must := assert.New(t), require.New(t)

			gh := &fakeGH{
				remaining: 100,
				items:     []github.SearchItem{item(1)},
				pulls:     map[string]github.PullRequest{"widget": withinMajor()},
				checks:    map[string][]github.CheckRun{"widget": passing("go")},
				mergeErr:  tt.mergeErr,
			}

			result, err := sweep(t, gh, baseCfg())
			must.NoError(err)
			want.Equal(tt.wantReason, result.Pulls[0].Reason)
			want.Equal(0, result.Merged)
			want.Equal(1, result.Skipped)
		})
	}
}

func TestRunEmptyResultCarriesEmptySlice(t *testing.T) {
	want, must := assert.New(t), require.New(t)

	result, err := sweep(t, &fakeGH{remaining: 100}, baseCfg())
	must.NoError(err)
	want.NotNil(result.Pulls)
	want.Empty(result.Pulls)
}

func TestRunPropagatesDependencyFailure(t *testing.T) {
	sentinel := errors.New("no token")
	orig := deps
	t.Cleanup(func() { deps = orig })
	deps = func() (dependencies, error) { return dependencies{}, sentinel }

	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelWarn}))
	_, err := Run(context.Background(), logger, baseCfg())
	assert.New(t).ErrorIs(err, sentinel)
}

func TestOSDepsRequiresToken(t *testing.T) {
	t.Setenv("GH_TOKEN", "")
	_, err := osDeps()
	assert.New(t).ErrorIs(err, constants.ErrNoAuth)
}

func TestOSDepsBuildsClient(t *testing.T) {
	t.Setenv("GH_TOKEN", "token")
	d, err := osDeps()
	require.New(t).NoError(err)
	assert.New(t).NotNil(d.github)
}

func TestExcluded(t *testing.T) {
	tests := []struct {
		name        string
		owner       repo.Owner
		repoName    repo.Name
		wantPattern repo.ExcludePattern
		patterns    []repo.ExcludePattern
		wantHeld    bool
	}{
		{
			name:        "dotted family glob holds a repo the API cannot reveal as held",
			patterns:    []repo.ExcludePattern{"robertcnix/fmt.*"},
			owner:       "robertcnix",
			repoName:    "fmt.api",
			wantPattern: "robertcnix/fmt.*",
			wantHeld:    true,
		},
		{
			name:     "same glob does not reach another owner",
			patterns: []repo.ExcludePattern{"robertcnix/fmt.*"},
			owner:    "gomatic",
			repoName: "fmt.api",
		},
		{
			name:     "prefix glob does not match a differently-punctuated sibling",
			patterns: []repo.ExcludePattern{"robertcnix/fmt.*"},
			owner:    "robertcnix",
			repoName: "fmt-user.plugin",
		},
		{
			name:        "exact slug",
			patterns:    []repo.ExcludePattern{"uplang/up.go"},
			owner:       "uplang",
			repoName:    "up.go",
			wantPattern: "uplang/up.go",
			wantHeld:    true,
		},
		{
			name:        "first matching pattern is the one reported",
			patterns:    []repo.ExcludePattern{"acme/*", "acme/widget"},
			owner:       "acme",
			repoName:    "widget",
			wantPattern: "acme/*",
			wantHeld:    true,
		},
		{
			name:     "owner glob does not span the separator",
			patterns: []repo.ExcludePattern{"*"},
			owner:    "acme",
			repoName: "widget",
		},
		{
			name:     "malformed pattern never matches",
			patterns: []repo.ExcludePattern{"acme/[", "acme/widget"},
			owner:    "acme",
			repoName: "other",
		},
		{
			name:     "no patterns holds nothing",
			patterns: nil,
			owner:    "acme",
			repoName: "widget",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := assert.New(t)
			pattern, held := excluded(tt.patterns, tt.owner, tt.repoName)
			want.Equal(tt.wantHeld, held)
			want.Equal(tt.wantPattern, pattern)
		})
	}
}

// TestRunSkipsHeldRepositoryBeforeSpendingRequests is the case that motivated
// the flag: a repo its owner holds in _admin/manifest.yaml must be skipped
// without the sweep fetching the pull request or its checks, and the result must
// name the pattern responsible.
func TestRunSkipsHeldRepositoryBeforeSpendingRequests(t *testing.T) {
	want, must := assert.New(t), require.New(t)

	gh := &fakeGH{
		remaining: 100,
		items:     []github.SearchItem{item(1)},
		pulls:     map[string]github.PullRequest{"widget": withinMajor()},
		checks:    map[string][]github.CheckRun{"widget": passing("go")},
		getErr:    errors.New("GetPullRequest must not be called for a held repo"),
		checksErr: errors.New("ListCheckRuns must not be called for a held repo"),
	}
	cfg := baseCfg()
	cfg.Excludes = []repo.ExcludePattern{"acme/wid*"}

	result, err := sweep(t, gh, cfg)
	must.NoError(err)

	must.Len(result.Pulls, 1)
	want.Equal("excluded by pattern acme/wid*", result.Pulls[0].Reason)
	want.False(result.Pulls[0].IsMerged)
	want.Equal(1, result.Skipped)
	want.Empty(gh.merges)
}

func TestRunSweepsRepositoriesNoPatternHolds(t *testing.T) {
	want, must := assert.New(t), require.New(t)

	gh := &fakeGH{
		remaining: 100,
		items:     []github.SearchItem{item(1)},
		pulls:     map[string]github.PullRequest{"widget": withinMajor()},
		checks:    map[string][]github.CheckRun{"widget": passing("go")},
	}
	cfg := baseCfg()
	cfg.Excludes = []repo.ExcludePattern{"other/*", "acme/different"}

	result, err := sweep(t, gh, cfg)
	must.NoError(err)
	want.Equal(1, result.Merged)
	want.Len(gh.merges, 1)
}
