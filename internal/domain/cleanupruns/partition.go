package cleanupruns

import (
	"sort"

	"github.com/nicerobot/tools.admin/internal/github"
	"github.com/nicerobot/tools.admin/internal/repo"
)

// The retention policy: which of a repository's runs may go. Separated from the
// orchestration in run.go because it is the part with a contract worth stating
// on its own — two independent conditions, either of which alone destroys
// history the other exists to protect.

// partition splits a repo's runs into those to delete and a count of those
// retained. A run is deleted only when it fails BOTH tests: it is beyond its
// workflow's keep floor, and it is older than the cutoff.
//
// Both conditions are load-bearing and were not both applied before. The floor
// alone would delete a repo's entire history the moment it exceeded N runs; the
// cutoff alone would empty a workflow that simply has not run this month,
// leaving no history to compare a regression against.
func partition(
	runs []github.WorkflowRun, keep repo.KeepCount, cutoff repo.CreatedBefore,
) ([]github.WorkflowRun, int) {
	var toDelete []github.WorkflowRun
	kept := 0
	for _, group := range groupByWorkflow(runs) {
		sortNewestFirst(group)
		floor := min(len(group), int(keep))
		kept += floor
		for _, run := range group[floor:] {
			if olderThan(run, cutoff) {
				toDelete = append(toDelete, run)
				continue
			}
			kept++
		}
	}
	return toDelete, kept
}

// olderThan reports whether a run predates the cutoff date. CreatedAt is
// RFC3339 and the cutoff is YYYY-MM-DD, so the prefix ordering a lexical
// comparison gives is the chronological one, and a run created ON the cutoff
// day sorts after it and is retained.
func olderThan(run github.WorkflowRun, cutoff repo.CreatedBefore) bool {
	return run.CreatedAt < string(cutoff)
}

func groupByWorkflow(runs []github.WorkflowRun) [][]github.WorkflowRun {
	index := map[int64]int{}
	var groups [][]github.WorkflowRun
	for _, r := range runs {
		i, ok := index[r.WorkflowID]
		if !ok {
			i = len(groups)
			index[r.WorkflowID] = i
			groups = append(groups, nil)
		}
		groups[i] = append(groups[i], r)
	}
	return groups
}

func sortNewestFirst(runs []github.WorkflowRun) {
	sort.SliceStable(runs, func(i, j int) bool { return runs[i].CreatedAt > runs[j].CreatedAt })
}
