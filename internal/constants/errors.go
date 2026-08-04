package constants

import errs "github.com/gomatic/go-error"

// Keep these constants sorted alphabetically.
const (
	// ErrCommand means an external git/gh command failed.
	ErrCommand errs.Const = "command failed"
	// ErrDecodeResponse means a GitHub API response body failed to decode.
	ErrDecodeResponse errs.Const = "failed to decode GitHub API response"
	// ErrHTTPStatus means a GitHub API call returned an unexpected status.
	ErrHTTPStatus errs.Const = "unexpected GitHub API status"
	// ErrResponseTooLarge means a GitHub API response exceeded the in-memory ceiling.
	ErrResponseTooLarge errs.Const = "GitHub API response exceeds the size limit"
	// ErrInvalidSettings means the org settings.yml file failed to parse.
	ErrInvalidSettings errs.Const = "settings file is not valid YAML"
	// ErrInvalidValue means a value (e.g. an output format) is not recognized.
	ErrInvalidValue errs.Const = "invalid value"
	// ErrListRepoFiles means the repos directory could not be listed.
	ErrListRepoFiles errs.Const = "failed to list repo files"
	// ErrMissingArgument means a required positional argument was not supplied.
	ErrMissingArgument errs.Const = "missing required argument"
	// ErrNoAuth means the GH_TOKEN environment variable is unset.
	ErrNoAuth errs.Const = "GH_TOKEN environment variable must be set"
	// ErrNoTarget means cleanup-runs has no owner and GITHUB_REPOSITORY is unset.
	ErrNoTarget errs.Const = "--owner is required when GITHUB_REPOSITORY is unset"
	// ErrRateFloor means the sweep stopped because the API rate budget fell to
	// the configured floor, leaving the remaining repos unswept.
	ErrRateFloor errs.Const = "GitHub API rate budget exhausted; sweep stopped early"
	// ErrNoOwners means the sweep was given no owners to walk.
	ErrNoOwners errs.Const = "at least one --owner is required"
	// ErrRemoveFile means a stale override file could not be removed.
	ErrRemoveFile errs.Const = "failed to remove override file"
	// ErrSettingsNotFound means the org settings.yml file is absent.
	ErrSettingsNotFound errs.Const = "settings file not found"
	// ErrStaleRepoExists means a stale-candidate repo still exists, signalling
	// the token lacks access to all repos; snapshot aborts before any write.
	ErrStaleRepoExists errs.Const = "repo exists but was not returned by list_repos; aborting to prevent data loss"
	// ErrWriteFile means a repos/<name>.yml file could not be written.
	ErrWriteFile errs.Const = "failed to write override file"
)
