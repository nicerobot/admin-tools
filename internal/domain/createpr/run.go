package createpr

import (
	"context"
	"log/slog"
	"path/filepath"

	"github.com/nicerobot/tools.admin/internal/domain"
	"github.com/nicerobot/tools.admin/internal/gitcmd"
	"github.com/nicerobot/tools.admin/internal/repo"
)

const (
	commitMessage = "chore: snapshot live repo settings"
	prTitle       = "chore: snapshot live repo settings"
	prBody        = "Auto-generated snapshot of current GitHub repo settings vs org/account defaults."
)

// Result is the outcome of the create-pr command: the branch operated on, and
// what was done. When nothing is staged, HasChanges is false and every
// subsequent step is false.
type Result struct {
	Branch          string `json:"branch"`
	HasChanges      bool   `json:"changed"`
	IsCommitted     bool   `json:"committed"`
	IsPushed        bool   `json:"pushed"`
	IsPRCreated     bool   `json:"pr_created"`
	IsPRAlreadyOpen bool   `json:"pr_already_open"`
}

// gitService is the git/gh surface create-pr needs.
type gitService interface {
	ConfigureBotIdentity() error
	CheckoutBranch(branch repo.Branch) error
	StageDirectory(path gitcmd.StagePath) error
	HasStagedChanges() (bool, error)
	Commit(message gitcmd.CommitMessage) error
	ForcePush(branch repo.Branch) error
	PrExists(branch repo.Branch) (bool, error)
	CreatePR(title gitcmd.PRTitle, body gitcmd.PRBody, head repo.Branch, base repo.Base) error
}

// dependencies are create-pr's injected collaborators.
type dependencies struct {
	git gitService
}

// deps builds the production collaborators. It is indirected through a variable
// so tests substitute a fake git service.
var deps = osDeps

// osDeps wires the OS-backed git/gh runner.
func osDeps() dependencies {
	return dependencies{git: gitcmd.New(gitcmd.OSRun)}
}

// Run executes the create-pr command, returning a structured Result the app tier
// renders. It orchestrates the gitcmd package and holds no presentation logic.
func Run(_ context.Context, logger *slog.Logger, cfg Config, _ ...domain.Argument) (Result, error) {
	return run(deps(), logger, cfg)
}

func run(d dependencies, logger *slog.Logger, cfg Config) (Result, error) {
	if err := prepare(d, cfg); err != nil {
		return Result{}, err
	}
	staged, err := d.git.HasStagedChanges()
	if err != nil {
		return Result{}, err
	}
	if !staged {
		logger.Info("No changes to commit.", "branch", cfg.Branch)
		return Result{Branch: string(cfg.Branch)}, nil
	}
	return commitAndPR(d, logger, cfg)
}

func prepare(d dependencies, cfg Config) error {
	if err := d.git.ConfigureBotIdentity(); err != nil {
		return err
	}
	if err := d.git.CheckoutBranch(cfg.Branch); err != nil {
		return err
	}
	return d.git.StageDirectory(gitcmd.StagePath(filepath.Join(string(cfg.SettingsPath), "repos")))
}

func commitAndPR(d dependencies, logger *slog.Logger, cfg Config) (Result, error) {
	if err := d.git.Commit(commitMessage); err != nil {
		return Result{}, err
	}
	if err := d.git.ForcePush(cfg.Branch); err != nil {
		return Result{}, err
	}
	exists, err := d.git.PrExists(cfg.Branch)
	if err != nil {
		return Result{}, err
	}
	res := Result{Branch: string(cfg.Branch), HasChanges: true, IsCommitted: true, IsPushed: true}
	if exists {
		res.IsPRAlreadyOpen = true
		logger.Info("PR already open.", "branch", cfg.Branch)
		return res, nil
	}
	if err := d.git.CreatePR(prTitle, prBody, cfg.Branch, cfg.Base); err != nil {
		return Result{}, err
	}
	res.IsPRCreated = true
	logger.Info("PR created.", "branch", cfg.Branch, "base", cfg.Base)
	return res, nil
}
