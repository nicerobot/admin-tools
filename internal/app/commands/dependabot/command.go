package dependabot

import (
	"context"

	"github.com/urfave/cli/v3"

	"github.com/nicerobot/tools.admin/internal/app"
	domain "github.com/nicerobot/tools.admin/internal/domain/dependabot"
	"github.com/nicerobot/tools.admin/internal/repo"
)

const (
	name        = `dependabot-sweep`
	usage       = `Merge green, non-major Dependabot pull requests.`
	argUsage    = ``
	description = `Sweep every --owner for open Dependabot pull requests and merge the ones that
qualify: not a draft, every check run complete and passing, and no dependency
crossing a major version boundary.

Major bumps are never merged unless --allow-major is given; a pull request whose
version transition cannot be parsed is never merged at all. Every pull request
left alone is reported with the reason it was skipped, so a sweep never reads as
complete when it was not.

Discovery costs one search request per owner rather than one per repository, and
the sweep stops cleanly once the remaining API budget reaches --rate-floor,
reporting stopped_early rather than failing partway.

Merging goes through the pull-request API, which carries no plan gate, so the
same mechanism works on public and private repositories alike.`
)

const (
	ownerFlag      = "owner"
	methodFlag     = "merge-method"
	rateFloorFlag  = "rate-floor"
	allowMajorFlag = "allow-major"
	dryRunFlag     = "dry-run"
)

const defaultRateFloor = 200

var (
	cfg       domain.Config
	runAction = domain.Run
	owners    []string
	method    = string(repo.MergeMethodSquash)
)

// Command returns the CLI command definition.
func Command() *cli.Command {
	return &cli.Command{
		Name:        name,
		Usage:       usage,
		ArgsUsage:   argUsage,
		Description: description,
		Before:      bindOwners,
		Action:      app.Default(&cfg, runAction),
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:        ownerFlag,
				Usage:       "GitHub user or organization to sweep (repeatable)",
				Destination: &owners,
			},
			&cli.StringFlag{
				Name:        methodFlag,
				Usage:       "Merge strategy: squash, merge, or rebase",
				Value:       string(repo.MergeMethodSquash),
				Destination: &method,
			},
			&cli.IntFlag{
				Name:        rateFloorFlag,
				Usage:       "Stop sweeping once fewer than N API requests remain",
				Value:       defaultRateFloor,
				Destination: (*int)(&cfg.RateFloor),
			},
			&cli.BoolFlag{
				Name:        allowMajorFlag,
				Usage:       "Also merge major version bumps (off by fleet policy)",
				Destination: (*bool)(&cfg.IsMajorAllowed),
			},
			&cli.BoolFlag{
				Name:        dryRunFlag,
				Usage:       "Report what would be merged without merging",
				Destination: (*bool)(&cfg.IsDryRun),
			},
		},
	}
}

// bindOwners converts the repeatable string flags into their domain types. The
// flag tier cannot bind []repo.Owner directly, so the conversion happens here
// rather than leaking string slices into the domain Config.
func bindOwners(ctx context.Context, _ *cli.Command) (context.Context, error) {
	cfg.Owners = make([]repo.Owner, 0, len(owners))
	for _, o := range owners {
		cfg.Owners = append(cfg.Owners, repo.Owner(o))
	}
	cfg.MergeMethod = repo.MergeMethod(method)
	return ctx, nil
}
