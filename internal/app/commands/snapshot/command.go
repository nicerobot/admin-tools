package snapshot

import (
	"github.com/urfave/cli/v3"

	"github.com/nicerobot/tools.admin/internal/app"
	domain "github.com/nicerobot/tools.admin/internal/domain/snapshot"
)

const (
	name        = `snapshot`
	usage       = `Snapshot live repo settings.`
	argUsage    = `<owner>`
	description = `Snapshot live GitHub repo settings into per-repo settings override files.

For <owner> (a GitHub user or organization, the positional argument), this lists
every repository, diffs each against the org or account defaults from
<settings-path>/settings.yml, and writes a <settings-path>/repos/<name>.yml
override for it. Override files for repositories that no longer exist are
removed — but only after confirming each is truly gone, so a token that cannot
see every repo never deletes a file it merely failed to list.`
)

const settingsPathFlag = "settings-path"

const defaultSettingsPath = ".github"

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
		},
	}
}
