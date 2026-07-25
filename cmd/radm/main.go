package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sort"

	"github.com/urfave/cli/v3"

	"github.com/nicerobot/tools.admin/internal/app"
	"github.com/nicerobot/tools.admin/internal/app/commands/cleanupruns"
	"github.com/nicerobot/tools.admin/internal/app/commands/createpr"
	dependabotcmd "github.com/nicerobot/tools.admin/internal/app/commands/dependabot"
	"github.com/nicerobot/tools.admin/internal/app/commands/snapshot"
)

const (
	argUsage    = ``
	description = `GitHub admin automation tools.

Available Commands:
  snapshot       - Snapshot live repo settings into per-repo settings override files
  create-pr      - Commit a snapshot onto a branch and open a pull request
  cleanup-runs   - Delete old GitHub Actions workflow runs
  dependabot-sweep - Merge green, non-major Dependabot pull requests

Each command renders a structured JSON result on stdout. Configuration comes from
flags and the standard GitHub Actions environment (GH_TOKEN, GITHUB_API_URL,
GITHUB_REPOSITORY).

Version:
  Use --version flag (built-in urfave/cli support)`
	envName   = "RADM"
	envPrefix = envName + "_"
	name      = `radm`
	usage     = `GitHub admin automation tools.`
)

var (
	appCreator    = createApp
	loggerConfig  app.LoggerConfig
	loggerCreator = productionLogger
)

// productionLogger builds the application logger from the parsed logging flags.
// It is invoked from the root Before hook, after flag parsing has populated
// loggerConfig, so --log-level and --log-format take effect.
func productionLogger(_ *cli.Command) *slog.Logger {
	logger := app.NewLogger(os.Stderr, loggerConfig)
	return &logger
}

// version is the application version.
// Set via ldflags: -X main.version=1.0.0
var version = "dev"

// osExit is indirected so tests can observe the process exit code.
var osExit = os.Exit

func main() { osExit(run(os.Args)) }

// run builds and executes the CLI, returning the process exit code. Keeping the
// exit code as a return value (rather than calling os.Exit here) makes the whole
// run path testable.
func run(args []string) int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	if err := appCreator(loggerCreator).Run(ctx, args); err != nil {
		slog.Error("Application error", "error", err)
		return 1
	}
	return 0
}

// createApp constructs the definition of the CLI.
func createApp(getLogger app.GetLoggerFunc) *cli.Command {
	cliApp := &cli.Command{
		Name:                  name,
		Usage:                 usage,
		ArgsUsage:             argUsage,
		Description:           description,
		Version:               version,
		EnableShellCompletion: true,
		Commands: []*cli.Command{
			snapshot.Command(),
			createpr.Command(),
			cleanupruns.Command(),
			dependabotcmd.Command(),
		},
		Before: func(ctx context.Context, c *cli.Command) (context.Context, error) {
			c.Root().Metadata[app.LoggerMetadataKey] = getLogger(c)
			return ctx, nil
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "log-level",
				Sources:     cli.EnvVars(envPrefix + "LOG_LEVEL"),
				Value:       "info",
				Usage:       "Set the logging level (debug, info, warn, error)",
				Destination: (*string)(&loggerConfig.LogLevel),
			},
			&cli.StringFlag{
				Name:        "log-format",
				Sources:     cli.EnvVars(envPrefix + "LOG_FORMAT"),
				Value:       "text",
				Usage:       "Set the log output format (text, json)",
				Destination: (*string)(&loggerConfig.LogFormat),
			},
		},
	}

	sort.Sort(cli.FlagsByName(cliApp.Flags))

	return cliApp
}
