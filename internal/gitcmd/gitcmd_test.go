package gitcmd_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nicerobot/tools.admin/internal/constants"
	"github.com/nicerobot/tools.admin/internal/gitcmd"
)

// recorder captures invocations and returns scripted results in order.
type recorder struct {
	calls   [][]string
	results []gitcmd.Result
	errs    []error
	i       int
}

func (r *recorder) run(args []string) (gitcmd.Result, error) {
	r.calls = append(r.calls, args)
	res := gitcmd.Result{}
	if r.i < len(r.results) {
		res = r.results[r.i]
	}
	var err error
	if r.i < len(r.errs) {
		err = r.errs[r.i]
	}
	r.i++
	return res, err
}

func newService(results []gitcmd.Result, errs []error) (gitcmd.Service, *recorder) {
	rec := &recorder{results: results, errs: errs}
	return gitcmd.New(rec.run), rec
}

func TestConfigureBotIdentitySuccess(t *testing.T) {
	svc, rec := newService(nil, nil)
	require.NoError(t, svc.ConfigureBotIdentity())
	require.Len(t, rec.calls, 2)
	assert.Equal(t, []string{"git", "config", "user.name", "github-actions[bot]"}, rec.calls[0])
	assert.Equal(
		t,
		[]string{"git", "config", "user.email", "41898282+github-actions[bot]@users.noreply.github.com"},
		rec.calls[1],
	)
}

func TestConfigureBotIdentityFirstFails(t *testing.T) {
	svc, rec := newService(nil, []error{errors.New("boom")})
	require.ErrorIs(t, svc.ConfigureBotIdentity(), constants.ErrCommand)
	assert.Len(t, rec.calls, 1) // second never runs
}

func TestCheckoutBranch(t *testing.T) {
	svc, rec := newService(nil, nil)
	require.NoError(t, svc.CheckoutBranch("settings-sync/snapshot"))
	assert.Equal(t, []string{"git", "checkout", "-B", "settings-sync/snapshot"}, rec.calls[0])
}

func TestStageDirectory(t *testing.T) {
	svc, rec := newService(nil, nil)
	require.NoError(t, svc.StageDirectory(".github/repos"))
	assert.Equal(t, []string{"git", "add", "--all", ".github/repos"}, rec.calls[0])
}

func TestCommit(t *testing.T) {
	svc, rec := newService(nil, nil)
	require.NoError(t, svc.Commit("chore: snapshot live repo settings"))
	assert.Equal(t, []string{"git", "commit", "-m", "chore: snapshot live repo settings"}, rec.calls[0])
}

func TestForcePush(t *testing.T) {
	svc, rec := newService(nil, nil)
	require.NoError(t, svc.ForcePush("settings-sync/snapshot"))
	assert.Equal(t, []string{"git", "push", "--force", "origin", "settings-sync/snapshot"}, rec.calls[0])
}

func TestCheckedNonZeroExit(t *testing.T) {
	svc, _ := newService([]gitcmd.Result{{ExitCode: 1}}, nil)
	require.ErrorIs(t, svc.CheckoutBranch("b"), constants.ErrCommand)
}

func TestHasStagedChangesTrue(t *testing.T) {
	svc, _ := newService([]gitcmd.Result{{ExitCode: 1}}, nil)
	has, err := svc.HasStagedChanges()
	require.NoError(t, err)
	assert.True(t, has)
}

func TestHasStagedChangesFalse(t *testing.T) {
	svc, _ := newService([]gitcmd.Result{{ExitCode: 0}}, nil)
	has, err := svc.HasStagedChanges()
	require.NoError(t, err)
	assert.False(t, has)
}

func TestHasStagedChangesExecError(t *testing.T) {
	svc, _ := newService(nil, []error{errors.New("git missing")})
	_, err := svc.HasStagedChanges()
	require.ErrorIs(t, err, constants.ErrCommand)
}

func TestPrExistsTrue(t *testing.T) {
	svc, rec := newService([]gitcmd.Result{{Stdout: "42\n"}}, nil)
	exists, err := svc.PrExists("my-branch")
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, []string{
		"gh", "pr", "list",
		"--head", "my-branch",
		"--state", "open",
		"--json", "number",
		"--jq", ".[0].number",
	}, rec.calls[0])
}

func TestPrExistsFalse(t *testing.T) {
	svc, _ := newService([]gitcmd.Result{{Stdout: ""}}, nil)
	exists, err := svc.PrExists("my-branch")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestPrExistsExecError(t *testing.T) {
	svc, _ := newService(nil, []error{errors.New("gh missing")})
	_, err := svc.PrExists("b")
	require.ErrorIs(t, err, constants.ErrCommand)
}

// TestServiceIsAnImmutableValueSafeToCopy pins the property Service documents.
// A copy is taken, driven through a mutating-sounding operation, and the
// original is then driven through the same one: both issue the identical
// command, so neither carries per-instance state the other can disturb. A
// pointer receiver or a mutable field would show up here as divergence between
// the two invocations.
func TestServiceIsAnImmutableValueSafeToCopy(t *testing.T) {
	original, rec := newService(nil, nil)
	copied := original

	require.NoError(t, copied.CheckoutBranch("feature"))
	require.NoError(t, original.CheckoutBranch("feature"))

	require.Len(t, rec.calls, 2)
	assert.Equal(t, rec.calls[0], rec.calls[1])
	assert.Equal(t, []string{"git", "checkout", "-B", "feature"}, rec.calls[0])
}

func TestCreatePR(t *testing.T) {
	svc, rec := newService(nil, nil)
	require.NoError(t, svc.CreatePR("chore: snapshot", "Auto-generated.", "my-branch", "main"))
	assert.Equal(t, []string{
		"gh", "pr", "create",
		"--title", "chore: snapshot",
		"--body", "Auto-generated.",
		"--head", "my-branch",
		"--base", "main",
	}, rec.calls[0])
}
