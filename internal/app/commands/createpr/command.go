package createpr

import (
	"github.com/urfave/cli/v3"

	"github.com/nicerobot/tools.admin/internal/app"
	domain "github.com/nicerobot/tools.admin/internal/domain/createpr"
)

const (
	name        = `create-pr`
	usage       = `Create a PR from a snapshot.`
	argUsage    = ``
	description = `Commit the snapshot under <settings-path>/repos onto a branch and open a pull
request into the base branch. The github-actions[bot] identity is configured,
the branch is force-created, and the repos directory is staged; if nothing is
staged the command is a no-op. Otherwise it commits, force-pushes, and opens a
PR — unless one is already open for the branch.`
)

const (
	settingsPathFlag = "settings-path"
	branchFlag       = "branch"
	baseFlag         = "base"
)

const (
	defaultSettingsPath = ".github"
	defaultBranch       = "settings-sync/snapshot"
	defaultBase         = "main"
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
				Name:        settingsPathFlag,
				Sources:     cli.EnvVars("RADM_SETTINGS_PATH"),
				Usage:       "Path to settings directory",
				Value:       defaultSettingsPath,
				Destination: (*string)(&cfg.SettingsPath),
			},
			&cli.StringFlag{
				Name:        branchFlag,
				Sources:     cli.EnvVars("RADM_BRANCH"),
				Usage:       "Branch name",
				Value:       defaultBranch,
				Destination: (*string)(&cfg.Branch),
			},
			&cli.StringFlag{
				Name:        baseFlag,
				Sources:     cli.EnvVars("RADM_BASE"),
				Usage:       "Base branch",
				Value:       defaultBase,
				Destination: (*string)(&cfg.Base),
			},
		},
	}
}
