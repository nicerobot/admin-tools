package snapshot

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nicerobot/tools.admin/internal/constants"
	"github.com/nicerobot/tools.admin/internal/github"
	"github.com/nicerobot/tools.admin/internal/overrides"
	"github.com/nicerobot/tools.admin/internal/repo"
)

type fakeGH struct {
	accountType repo.AccountType
	repos       []github.Repository
	exists      map[string]bool
	accountErr  error
	listErr     error
	existsErr   error
	existsCalls []repo.Name
}

func (f *fakeGH) GetAccountType(repo.Owner) (repo.AccountType, error) {
	return f.accountType, f.accountErr
}

func (f *fakeGH) ListRepos(repo.Owner) ([]github.Repository, error) {
	return f.repos, f.listErr
}

func (f *fakeGH) RepoExists(_ repo.Owner, n repo.Name) (bool, error) {
	f.existsCalls = append(f.existsCalls, n)
	if f.existsErr != nil {
		return false, f.existsErr
	}
	return f.exists[string(n)], nil
}

type harness struct {
	gh        *fakeGH
	existing  []string
	globErr   error
	loadData  string
	loadErr   error
	mkdirErr  error
	writeErr  error
	removeErr error
	wrote     map[string]string
	removed   []string
}

func (h *harness) install(t *testing.T) {
	t.Helper()
	orig := deps
	t.Cleanup(func() { deps = orig })
	h.wrote = map[string]string{}
	deps = func() (dependencies, error) {
		return dependencies{
			github:    h.gh,
			readFile:  h.readFile,
			mkdir:     func(overrides.ReposDir) error { return h.mkdirErr },
			writeFile: h.writeFile,
			glob:      h.globFn,
			remove:    h.removeFn,
		}, nil
	}
}

func (h *harness) readFile(string) ([]byte, error) {
	if h.loadErr != nil {
		return nil, h.loadErr
	}
	return []byte(h.loadData), nil
}

func (h *harness) writeFile(name overrides.OutFile, data []byte) error {
	if h.writeErr != nil {
		return h.writeErr
	}
	h.wrote[string(name)] = string(data)
	return nil
}

func (h *harness) globFn(string) ([]string, error) {
	if h.globErr != nil {
		return nil, h.globErr
	}
	matches := make([]string, len(h.existing))
	for i, n := range h.existing {
		matches[i] = ".github/repos/" + n + ".yml"
	}
	return matches, nil
}

func (h *harness) removeFn(path string) error {
	if h.removeErr != nil {
		return h.removeErr
	}
	h.removed = append(h.removed, path)
	return nil
}

func (h *harness) run(t *testing.T, owner repo.Owner) (Result, error) {
	t.Helper()
	h.install(t)
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	return Run(context.Background(), logger, Config{SettingsPath: ".github"}, string(owner))
}

func matchingRepo(name string) github.Repository {
	return github.Repository{
		Name:                      name,
		IsPrivate:                 true,
		DefaultBranch:             "main",
		CanSquashMerge:            true,
		CanMergeCommit:            true,
		CanRebaseMerge:            true,
		ShouldDeleteBranchOnMerge: true,
	}
}

func TestWritesOverrideFiles(t *testing.T) {
	repo1 := matchingRepo("repo1")
	repo1.HasIssues = true
	h := &harness{gh: &fakeGH{accountType: "User", repos: []github.Repository{repo1, matchingRepo("repo2")}}}
	res, err := h.run(t, "nicerobot")
	require.NoError(t, err)
	assert.Equal(t, []string{".github/repos/repo1.yml", ".github/repos/repo2.yml"}, res.Wrote)
	assert.Contains(t, h.wrote[".github/repos/repo1.yml"], "has_issues: true")
	assert.NotContains(t, h.wrote[".github/repos/repo2.yml"], "has_issues")
}

func TestRemovesStaleVerified404(t *testing.T) {
	h := &harness{
		gh: &fakeGH{
			accountType: "User",
			repos:       []github.Repository{matchingRepo("alive")},
			exists:      map[string]bool{"dead": false},
		},
		existing: []string{"alive", "dead"},
	}
	res, err := h.run(t, "nicerobot")
	require.NoError(t, err)
	assert.Equal(t, []repo.Name{"dead"}, h.gh.existsCalls)
	assert.Equal(t, []string{".github/repos/dead.yml"}, res.Removed)
	assert.Equal(t, []string{".github/repos/dead.yml"}, h.removed)
}

func TestAbortsWhenStaleRepoExists(t *testing.T) {
	h := &harness{
		gh: &fakeGH{
			accountType: "User",
			repos:       []github.Repository{matchingRepo("visible")},
			exists:      map[string]bool{"hidden": true},
		},
		existing: []string{"visible", "hidden"},
	}
	_, err := h.run(t, "nicerobot")
	require.ErrorIs(t, err, constants.ErrStaleRepoExists)
	assert.Empty(t, h.wrote) // aborts before any write
	assert.Empty(t, h.removed)
}

func TestAbortsOnVerificationError(t *testing.T) {
	h := &harness{
		gh: &fakeGH{
			accountType: "User",
			repos:       []github.Repository{matchingRepo("alive")},
			existsErr:   errors.New("server error"),
		},
		existing: []string{"alive", "maybe"},
	}
	_, err := h.run(t, "nicerobot")
	require.Error(t, err)
	assert.NotErrorIs(t, err, constants.ErrStaleRepoExists)
	assert.Empty(t, h.wrote)
}

func TestNoStaleSkipsVerification(t *testing.T) {
	h := &harness{
		gh:       &fakeGH{accountType: "User", repos: []github.Repository{matchingRepo("repo1")}},
		existing: []string{"repo1"},
	}
	res, err := h.run(t, "nicerobot")
	require.NoError(t, err)
	assert.Empty(t, h.gh.existsCalls)
	assert.Empty(t, res.Removed)
	require.Len(t, res.Wrote, 1)
}

func TestOrgCommentSource(t *testing.T) {
	h := &harness{gh: &fakeGH{accountType: repo.AccountTypeOrganization, repos: []github.Repository{matchingRepo("p")}}}
	res, err := h.run(t, "myorg")
	require.NoError(t, err)
	assert.Equal(t, string(repo.CommentSourceOrg), res.CommentSource)
	assert.Contains(t, h.wrote[".github/repos/p.yml"], "from org defaults")
}

func TestAccountCommentSource(t *testing.T) {
	h := &harness{gh: &fakeGH{accountType: "User", repos: []github.Repository{matchingRepo("p")}}}
	res, err := h.run(t, "nicerobot")
	require.NoError(t, err)
	assert.Equal(t, string(repo.CommentSourceAccount), res.CommentSource)
}

func TestForkMarked(t *testing.T) {
	r := matchingRepo("forked")
	r.IsFork = true
	h := &harness{gh: &fakeGH{accountType: "User", repos: []github.Repository{r}}}
	_, err := h.run(t, "nicerobot")
	require.NoError(t, err)
	assert.Contains(t, h.wrote[".github/repos/forked.yml"], "_fork: true")
}

func TestEmptyReposCleansVerifiedStale(t *testing.T) {
	h := &harness{
		gh:       &fakeGH{accountType: "User", exists: map[string]bool{"orphan": false}},
		existing: []string{"orphan"},
	}
	res, err := h.run(t, "nicerobot")
	require.NoError(t, err)
	assert.Equal(t, []string{".github/repos/orphan.yml"}, res.Removed)
}

func TestLoadError(t *testing.T) {
	h := &harness{gh: &fakeGH{}, loadErr: errors.New("missing")}
	_, err := h.run(t, "nicerobot")
	require.ErrorIs(t, err, constants.ErrSettingsNotFound)
}

func TestAccountTypeError(t *testing.T) {
	h := &harness{gh: &fakeGH{accountErr: errors.New("boom")}}
	_, err := h.run(t, "nicerobot")
	require.Error(t, err)
}

func TestListReposError(t *testing.T) {
	h := &harness{gh: &fakeGH{accountType: "User", listErr: errors.New("boom")}}
	_, err := h.run(t, "nicerobot")
	require.Error(t, err)
}

func TestListExistingError(t *testing.T) {
	h := &harness{
		gh:      &fakeGH{accountType: "User", repos: []github.Repository{matchingRepo("r")}},
		globErr: errors.New("boom"),
	}
	_, err := h.run(t, "nicerobot")
	require.ErrorIs(t, err, constants.ErrListRepoFiles)
}

func TestWriteError(t *testing.T) {
	h := &harness{
		gh:       &fakeGH{accountType: "User", repos: []github.Repository{matchingRepo("r")}},
		writeErr: errors.New("disk"),
	}
	_, err := h.run(t, "nicerobot")
	require.ErrorIs(t, err, constants.ErrWriteFile)
}

func TestMkdirError(t *testing.T) {
	h := &harness{
		gh:       &fakeGH{accountType: "User", repos: []github.Repository{matchingRepo("r")}},
		mkdirErr: errors.New("denied"),
	}
	_, err := h.run(t, "nicerobot")
	require.ErrorIs(t, err, constants.ErrWriteFile)
}

func TestRemoveError(t *testing.T) {
	h := &harness{
		gh:        &fakeGH{accountType: "User", exists: map[string]bool{"gone": false}},
		existing:  []string{"gone"},
		removeErr: errors.New("perm"),
	}
	_, err := h.run(t, "nicerobot")
	require.ErrorIs(t, err, constants.ErrRemoveFile)
}

// TestOSDeps exercises the production wiring: a present token yields a client,
// an absent one surfaces ErrNoAuth.
func TestOSDeps(t *testing.T) {
	t.Setenv("GH_TOKEN", "tok")
	d, err := osDeps()
	require.NoError(t, err)
	assert.NotNil(t, d.github)
	assert.NotNil(t, d.readFile)
	assert.NotNil(t, d.mkdir)
	assert.NotNil(t, d.writeFile)
	assert.NotNil(t, d.glob)
	assert.NotNil(t, d.remove)

	t.Setenv("GH_TOKEN", "")
	_, err = osDeps()
	require.ErrorIs(t, err, constants.ErrNoAuth)
}

func TestRunBuildsDepsError(t *testing.T) {
	orig := deps
	t.Cleanup(func() { deps = orig })
	deps = func() (dependencies, error) { return dependencies{}, constants.ErrNoAuth.With(nil) }
	logger := slog.New(slog.NewTextHandler(&strings.Builder{}, nil))
	_, err := Run(context.Background(), logger, Config{SettingsPath: ".github"}, "x")
	require.ErrorIs(t, err, constants.ErrNoAuth)
}

// TestMissingOwnerArgument proves a Run with no positional owner fails with
// ErrMissingArgument before deps() is consulted: deps is stubbed to fail with
// ErrNoAuth, yet the missing-argument sentinel is what surfaces.
func TestMissingOwnerArgument(t *testing.T) {
	orig := deps
	t.Cleanup(func() { deps = orig })
	deps = func() (dependencies, error) { return dependencies{}, constants.ErrNoAuth.With(nil) }
	logger := slog.New(slog.NewTextHandler(&strings.Builder{}, nil))
	_, err := Run(context.Background(), logger, Config{SettingsPath: ".github"})
	require.ErrorIs(t, err, constants.ErrMissingArgument)
}
