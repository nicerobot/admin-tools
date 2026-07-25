package dependabot

import "github.com/nicerobot/tools.admin/internal/repo"

// Config holds the flags for the dependabot-sweep command. Owners is the set of
// owners to walk and must be non-empty; Excludes holds "owner/name" globs the
// sweep must skip, mirroring the holds an owner declares in _admin/manifest.yaml
// that the GitHub API cannot reveal. Its fields are bound by the CLI tier and
// read by Run; it carries no behavior.
type Config struct {
	MergeMethod    repo.MergeMethod
	Owners         []repo.Owner
	Excludes       []repo.ExcludePattern
	RateFloor      repo.RateFloor
	IsMajorAllowed repo.MajorAllowed
	IsDryRun       repo.DryRun
}
