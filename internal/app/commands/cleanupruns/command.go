package cleanupruns

import (
	"github.com/urfave/cli/v3"

	"github.com/nicerobot/tools.admin/internal/app"
	domain "github.com/nicerobot/tools.admin/internal/domain/cleanupruns"
)

const (
	name        = `cleanup-runs`
	usage       = `Delete old workflow runs.`
	argUsage    = ``
	description = `Delete old GitHub Actions workflow runs. For the target owner (or the current
repository from GITHUB_REPOSITORY when --owner is omitted) and repo (or every
repo under the owner when --repo is omitted), this lists completed runs older
than --days, groups them by workflow, keeps the newest --keep per workflow, and
deletes the rest. With --dry-run nothing is deleted; the result reports what
would be.`
)

const (
	ownerFlag  = "owner"
	repoFlag   = "repo"
	daysFlag   = "days"
	keepFlag   = "keep"
	dryRunFlag = "dry-run"
)

const (
	defaultDays = 30
	defaultKeep = 5
)

var (
	cfg       domain.Config
	runAction = domain.Run
)

// Command returns the CLI command definition.
func Command() *cli.Command {
	return &cli.Command{
		Name:        name,
		Usage:       usage,
		ArgsUsage:   argUsage,
		Description: description,
		Action:      app.Default(&cfg, runAction),
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        ownerFlag,
				Sources:     cli.EnvVars("RADM_OWNER"),
				Value:       "",
				Usage:       "GitHub user or organization (default: current repo from GITHUB_REPOSITORY)",
				Destination: (*string)(&cfg.Owner),
			},
			&cli.StringFlag{
				Name:        repoFlag,
				Sources:     cli.EnvVars("RADM_REPO"),
				Value:       "",
				Usage:       "Single repo (omit for all repos)",
				Destination: (*string)(&cfg.Repo),
			},
			&cli.IntFlag{
				Name:        daysFlag,
				Sources:     cli.EnvVars("RADM_DAYS"),
				Usage:       "Delete runs older than N days",
				Value:       defaultDays,
				Destination: (*int)(&cfg.Days),
			},
			&cli.IntFlag{
				Name:        keepFlag,
				Sources:     cli.EnvVars("RADM_KEEP"),
				Usage:       "Keep at least N runs per workflow",
				Value:       defaultKeep,
				Destination: (*int)(&cfg.Keep),
			},
			&cli.BoolFlag{
				Name:        dryRunFlag,
				Sources:     cli.EnvVars("RADM_DRY_RUN"),
				Usage:       "Print what would be deleted without deleting",
				Destination: (*bool)(&cfg.IsDryRun),
			},
		},
	}
}
