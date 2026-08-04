package cleanupruns

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nicerobot/tools.admin/internal/github"
)

// TestOldRunsAreDeletedEvenWhenFewerThanTheKeepFloor is the regression test for
// a cleanup that had quietly stopped cleaning.
//
// The API was asked only for runs older than the cutoff, and the keep floor was
// then applied to THAT set. So a repository with hundreds of recent runs and a
// handful of old ones kept every old one — the floor was satisfied by the very
// runs the job existed to remove, and it deleted nothing. Worse, the floor
// protected the OLDEST runs rather than guaranteeing recent history survived,
// which is the opposite of what it is for.
//
// Observed in the field: gomatic/ui.scry held 79 runs, 5 of them past the
// cutoff, and reported deleted=0 kept=0. Runs accumulated fleet-wide for as
// long as this shipped.
func TestOldRunsAreDeletedEvenWhenFewerThanTheKeepFloor(t *testing.T) {
	runs := []github.WorkflowRun{}
	// Twelve recent runs of one workflow: well past the keep floor, none old.
	for id := int64(1); id <= 12; id++ {
		runs = append(runs, mkRun(id, 1, "2025-01-20T00:00:00Z"))
	}
	// Three old runs of that same workflow — fewer than the floor of 5.
	for id := int64(13); id <= 15; id++ {
		runs = append(runs, mkRun(id, 1, "2024-11-01T00:00:00Z"))
	}

	gh := &fakeGH{runs: map[string][]github.WorkflowRun{"repo1": runs}}
	res, err := runCleanup(t, gh, nil, Config{Owner: "nicerobot", Repo: "repo1", Days: 30, Keep: 5})

	require.NoError(t, err)
	assert.Equal(t, 3, res.Deleted, "every run past the floor AND past the cutoff goes")
	assert.Equal(t, 12, res.Kept, "the recent history survives untouched")
	for _, call := range gh.deletes {
		assert.GreaterOrEqual(t, call.id, int64(13), "no recent run may be deleted")
	}
}

// TestRecentRunsSurviveEvenBeyondTheKeepFloor pins the other half of the
// contract. The floor is a MINIMUM to retain, not a maximum: a workflow that
// ran fifty times this week keeps all fifty, because none of them is old.
func TestRecentRunsSurviveEvenBeyondTheKeepFloor(t *testing.T) {
	runs := []github.WorkflowRun{}
	for id := int64(1); id <= 50; id++ {
		runs = append(runs, mkRun(id, 1, "2025-01-25T00:00:00Z"))
	}

	gh := &fakeGH{runs: map[string][]github.WorkflowRun{"repo1": runs}}
	res, err := runCleanup(t, gh, nil, Config{Owner: "nicerobot", Repo: "repo1", Days: 30, Keep: 5})

	require.NoError(t, err)
	assert.Empty(t, gh.deletes)
	assert.Equal(t, 0, res.Deleted)
}
