package github

import (
	"strings"

	"github.com/nicerobot/tools.admin/internal/repo"
)

// User is the subset of GET /users/{owner} that drives account-type detection.
type User struct {
	Login string `json:"login"`
	Type  string `json:"type"`
}

// AccountType returns the user's account type as a domain value.
func (u User) AccountType() repo.AccountType { return repo.AccountType(u.Type) }

// Repository is the subset of a GitHub repository object that snapshot diffs
// against the org/account defaults.
type Repository struct {
	DefaultBranch             string `json:"default_branch"`
	Description               string `json:"description"`
	Homepage                  string `json:"homepage"`
	Name                      string `json:"name"`
	HasDiscussions            bool   `json:"has_discussions"`
	HasIssues                 bool   `json:"has_issues"`
	HasProjects               bool   `json:"has_projects"`
	HasWiki                   bool   `json:"has_wiki"`
	IsPrivate                 bool   `json:"private"`
	IsTemplate                bool   `json:"is_template"`
	CanSquashMerge            bool   `json:"allow_squash_merge"`
	CanMergeCommit            bool   `json:"allow_merge_commit"`
	CanRebaseMerge            bool   `json:"allow_rebase_merge"`
	CanAutoMerge              bool   `json:"allow_auto_merge"`
	ShouldDeleteBranchOnMerge bool   `json:"delete_branch_on_merge"`
	IsArchived                bool   `json:"archived"`
	IsFork                    bool   `json:"fork"`
}

// Visibility derives the public/private visibility from the private flag.
func (r Repository) Visibility() repo.Visibility {
	if r.IsPrivate {
		return repo.VisibilityPrivate
	}
	return repo.VisibilityPublic
}

// WorkflowRun is the subset of a GitHub Actions run object that cleanup-runs
// groups, sorts, and prunes.
type WorkflowRun struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	CreatedAt  string `json:"created_at"`
	ID         int64  `json:"id"`
	WorkflowID int64  `json:"workflow_id"`
}

// PullRequest is the subset of a pull-request object the sweeper needs: who
// opened it, what commit it points at, and the prose Dependabot writes the
// version transitions into. The list endpoint populates every field here, so
// the sweeper never spends a request per pull request to fetch detail.
type PullRequest struct {
	Title   string    `json:"title"`
	Body    string    `json:"body"`
	User    UserRef   `json:"user"`
	Head    CommitRef `json:"head"`
	Number  int       `json:"number"`
	IsDraft bool      `json:"draft"`
}

// UserRef is the login-bearing actor reference on a pull request.
type UserRef struct {
	Login string `json:"login"`
}

// CommitRef is a pull request's head reference.
type CommitRef struct {
	SHA string `json:"sha"`
	Ref string `json:"ref"`
}

// Author returns the pull request's author login as a domain type.
func (p PullRequest) Author() repo.Author { return repo.Author(p.User.Login) }

// HeadSHA returns the pull request's head commit as a domain type.
func (p PullRequest) HeadSHA() repo.SHA { return repo.SHA(p.Head.SHA) }

// CheckRun is the subset of a check-run object the sweeper gates merges on.
type CheckRun struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}

// IsComplete reports whether the check run has finished, however it finished.
func (c CheckRun) IsComplete() bool { return c.Status == checkStatusCompleted }

// IsPassing reports whether a completed check run should not block a merge.
// neutral and skipped are non-blocking by GitHub's own merge semantics, so
// treating them as passing matches what a required-checks gate would do.
func (c CheckRun) IsPassing() bool {
	switch c.Conclusion {
	case checkSuccess, checkNeutral, checkSkipped:
		return true
	default:
		return false
	}
}

const (
	checkStatusCompleted = "completed"
	checkSuccess         = "success"
	checkNeutral         = "neutral"
	checkSkipped         = "skipped"
)

// RateLimit is the core API budget, read from the rate_limit endpoint (which is
// itself not metered) so a sweep can stop before it starts failing.
type RateLimit struct {
	Remaining int `json:"remaining"`
	Limit     int `json:"limit"`
}

// rateLimitResources groups the per-resource budgets the rate_limit endpoint
// reports; only the core budget governs the sweep.
type rateLimitResources struct {
	Core RateLimit `json:"core"`
}

// rateLimitEnvelope decodes the resources wrapper the rate_limit endpoint returns.
type rateLimitEnvelope struct {
	Resources rateLimitResources `json:"resources"`
}

// SearchItem is one open Dependabot pull request as returned by the search
// endpoint: enough to locate it, not enough to judge it.
type SearchItem struct {
	RepositoryURL string `json:"repository_url"`
	Title         string `json:"title"`
	Number        int    `json:"number"`
}

// RepoName derives the repository name from the search result's repository URL,
// whose final path segment is the repo. An empty or malformed URL yields an
// empty name, which the sweeper reports as a skip rather than acting on.
func (s SearchItem) RepoName() repo.Name {
	idx := strings.LastIndex(s.RepositoryURL, "/")
	if idx < 0 || idx+1 >= len(s.RepositoryURL) {
		return ""
	}
	return repo.Name(s.RepositoryURL[idx+1:])
}
