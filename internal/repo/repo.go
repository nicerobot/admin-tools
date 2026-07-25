// Package repo holds the shared repository/GitHub scalar vocabulary that flows
// through tools.admin. Every function and method parameter that carries one of
// these domain concepts uses a named type here instead of a bare string/int/bool,
// so the type system documents intent and prevents accidental argument
// transposition. It is a pure implementation package: it knows nothing about the
// CLI or about being "a command", and is freely imported by the client, the
// settings/overrides packages, and the per-command domains.
package repo

// Owner is a GitHub user or organization login.
type Owner string

// Name is a repository's short name (no owner prefix).
type Name string

// AccountType is the GitHub account type returned by GET /users/{owner}.
type AccountType string

// AccountTypeOrganization is the value GitHub reports for organization accounts.
const AccountTypeOrganization AccountType = "Organization"

// CommentSource names whose defaults a repo override file is diffed against.
type CommentSource string

const (
	// CommentSourceOrg is used when the owner is an organization.
	CommentSourceOrg CommentSource = "org"
	// CommentSourceAccount is used when the owner is a user account.
	CommentSourceAccount CommentSource = "account"
)

// Visibility is a repository's public/private visibility.
type Visibility string

const (
	// VisibilityPublic is a public repository.
	VisibilityPublic Visibility = "public"
	// VisibilityPrivate is a private repository.
	VisibilityPrivate Visibility = "private"
)

// Days is a retention window expressed in whole days.
type Days int

// KeepCount is the minimum number of runs to retain per workflow.
type KeepCount int

// DryRun toggles delete-vs-report behavior for cleanup-runs.
type DryRun bool

// Branch is a git branch name.
type Branch string

// Base is a pull-request base branch name.
type Base string

// SettingsPath is the path to the settings directory (e.g. ".github").
type SettingsPath string

// CreatedBefore is the upper-bound date (YYYY-MM-DD) for run listing.
type CreatedBefore string

// RunID is a GitHub Actions workflow-run identifier.
type RunID int64

// IsFork marks a repository as a fork.
type IsFork bool

// PullNumber is a pull-request number within a repository.
type PullNumber int

// Author is a pull-request author's login.
type Author string

// AuthorDependabot is the login Dependabot opens pull requests under.
const AuthorDependabot Author = "dependabot[bot]"

// MergeMethod is the strategy used to merge a pull request.
type MergeMethod string

// MergeMethodSquash collapses a pull request to a single commit on merge.
const MergeMethodSquash MergeMethod = "squash"

// SHA is a git commit identifier.
type SHA string

// SkipReason explains why the sweeper declined to merge a pull request. Every
// skip carries one, so a sweep never silently drops work.
type SkipReason string

// MajorAllowed permits merging pull requests that cross a major version
// boundary. It is false by fleet policy: major bumps await human review.
type MajorAllowed bool

// RateFloor is the number of API requests the sweeper refuses to spend below,
// so a sweep degrades into a reported early stop rather than rate-limit errors.
type RateFloor int

// ExcludePattern is an "owner/name" glob naming repositories a sweep must not
// touch. It exists because an owner's _admin/manifest.yaml can hold a repo back
// from automation, and a tool that enumerates repositories from the API alone
// has no way to see that hold.
type ExcludePattern string
