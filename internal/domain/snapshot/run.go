package snapshot

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"github.com/nicerobot/tools.admin/internal/constants"
	"github.com/nicerobot/tools.admin/internal/domain"
	"github.com/nicerobot/tools.admin/internal/github"
	"github.com/nicerobot/tools.admin/internal/overrides"
	"github.com/nicerobot/tools.admin/internal/repo"
	"github.com/nicerobot/tools.admin/internal/settings"
)

// Result is the outcome of the snapshot command: the owner and the comment
// source the overrides were diffed against, the override files written, and the
// stale files confirmed gone and removed.
type Result struct {
	Owner         string   `json:"owner"`
	CommentSource string   `json:"comment_source"`
	Wrote         []string `json:"wrote"`
	Removed       []string `json:"removed"`
}

// githubAPI is the GitHub surface snapshot needs.
type githubAPI interface {
	GetAccountType(owner repo.Owner) (repo.AccountType, error)
	ListRepos(owner repo.Owner) ([]github.Repository, error)
	RepoExists(owner repo.Owner, name repo.Name) (bool, error)
}

// dependencies are snapshot's injected collaborators.
type dependencies struct {
	github    githubAPI
	readFile  settings.ReadFileFunc
	mkdir     overrides.MkdirAllFunc
	writeFile overrides.WriteFileFunc
	glob      overrides.GlobFunc
	remove    overrides.RemoveFunc
}

// deps builds the production collaborators. It is indirected through a variable
// so tests substitute in-memory fakes.
var deps = osDeps

// osDeps wires the OS-backed GitHub client (from GH_TOKEN/GITHUB_API_URL) and
// filesystem seams.
func osDeps() (dependencies, error) {
	client, err := github.NewFromEnv(os.Getenv)
	if err != nil {
		return dependencies{}, err
	}
	return dependencies{
		github:    client,
		readFile:  os.ReadFile,
		mkdir:     overrides.OSMkdir,
		writeFile: overrides.OSWriteFile,
		glob:      filepath.Glob,
		remove:    os.Remove,
	}, nil
}

// Run executes the snapshot command, returning a structured Result the app tier
// renders. It orchestrates the implementation packages and holds no presentation
// logic.
func Run(_ context.Context, logger *slog.Logger, cfg Config, _ ...domain.Argument) (Result, error) {
	d, err := deps()
	if err != nil {
		return Result{}, err
	}
	return run(d, logger, cfg)
}

func run(d dependencies, logger *slog.Logger, cfg Config) (Result, error) {
	s, err := settings.Load(d.readFile, cfg.SettingsPath)
	if err != nil {
		return Result{}, err
	}
	at, err := d.github.GetAccountType(cfg.Owner)
	if err != nil {
		return Result{}, err
	}
	repos, err := d.github.ListRepos(cfg.Owner)
	if err != nil {
		return Result{}, err
	}
	source := commentSource(at)
	reposDir := overrides.ReposDir(filepath.Join(string(cfg.SettingsPath), "repos"))
	files, live := computeFiles(repos, s.Repository, cfg.Owner, source)
	gone, err := verifyStale(d, cfg.Owner, reposDir, live)
	if err != nil {
		return Result{}, err
	}
	wrote, removed, err := apply(d, reposDir, files, gone)
	if err != nil {
		return Result{}, err
	}
	logger.Info("Snapshot complete.", "owner", cfg.Owner, "wrote", len(wrote), "removed", len(removed))
	return Result{Owner: string(cfg.Owner), CommentSource: string(source), Wrote: wrote, Removed: removed}, nil
}

func commentSource(at repo.AccountType) repo.CommentSource {
	if at == repo.AccountTypeOrganization {
		return repo.CommentSourceOrg
	}
	return repo.CommentSourceAccount
}

func computeFiles(
	repos []github.Repository,
	defaults settings.RepositoryDefaults,
	owner repo.Owner,
	source repo.CommentSource,
) ([]overrides.File, map[string]bool) {
	files := make([]overrides.File, 0, len(repos))
	live := make(map[string]bool, len(repos))
	for _, r := range repos {
		live[r.Name] = true
		files = append(files, overrides.Compute(r, defaults, owner, source))
	}
	return files, live
}

func verifyStale(
	d dependencies,
	owner repo.Owner,
	reposDir overrides.ReposDir,
	live map[string]bool,
) ([]string, error) {
	existing, err := overrides.ListExisting(reposDir, d.glob)
	if err != nil {
		return nil, err
	}
	gone := make([]string, 0)
	for _, name := range staleNames(existing, live) {
		exists, err := d.github.RepoExists(owner, repo.Name(name))
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, constants.ErrStaleRepoExists.With(nil, "repo", string(owner)+"/"+name)
		}
		gone = append(gone, name)
	}
	return gone, nil
}

func staleNames(existing []string, live map[string]bool) []string {
	stale := make([]string, 0)
	for _, name := range existing {
		if !live[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	return stale
}

func apply(
	d dependencies,
	reposDir overrides.ReposDir,
	files []overrides.File,
	gone []string,
) ([]string, []string, error) {
	wrote, err := writeAll(d, reposDir, files)
	if err != nil {
		return nil, nil, err
	}
	removed, err := removeGone(d, reposDir, gone)
	if err != nil {
		return nil, nil, err
	}
	return wrote, removed, nil
}

func writeAll(d dependencies, reposDir overrides.ReposDir, files []overrides.File) ([]string, error) {
	wrote := make([]string, 0, len(files))
	for _, f := range files {
		out, err := overrides.Write(f, reposDir, d.mkdir, d.writeFile)
		if err != nil {
			return nil, err
		}
		wrote = append(wrote, string(out))
	}
	return wrote, nil
}

func removeGone(d dependencies, reposDir overrides.ReposDir, gone []string) ([]string, error) {
	removed := make([]string, 0, len(gone))
	for _, name := range gone {
		path := filepath.Join(string(reposDir), name+".yml")
		if err := d.remove(path); err != nil {
			return nil, constants.ErrRemoveFile.With(err, "file", path)
		}
		removed = append(removed, path)
	}
	return removed, nil
}
