package dependabot

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/nicerobot/tools.admin/internal/app"
	domain "github.com/nicerobot/tools.admin/internal/domain/dependabot"
	"github.com/nicerobot/tools.admin/internal/repo"
)

func TestDependabotSweepCommand(t *testing.T) {
	tests := []struct {
		name         string
		wantContains string
		args         []string
		wantCfg      domain.Config
	}{
		{
			name: "defaults squash, keeps major bumps out, no owners",
			args: []string{"app", "dependabot-sweep"},
			wantCfg: domain.Config{
				Owners:      []repo.Owner{},
				MergeMethod: repo.MergeMethodSquash,
				RateFloor:   defaultRateFloor,
			},
			wantContains: `"considered": 0`,
		},
		{
			name: "binds every flag",
			args: []string{
				"app", "dependabot-sweep",
				"--owner", "acme", "--owner", "other",
				"--merge-method", "rebase",
				"--rate-floor", "50",
				"--allow-major",
				"--dry-run",
			},
			wantCfg: domain.Config{
				Owners:         []repo.Owner{"acme", "other"},
				MergeMethod:    "rebase",
				RateFloor:      50,
				IsMajorAllowed: true,
				IsDryRun:       true,
			},
			wantContains: `"dry_run": true`,
		},
		{
			name: "repeated owners accumulate in order",
			args: []string{"app", "dependabot-sweep", "--owner", "a", "--owner", "b", "--owner", "c"},
			wantCfg: domain.Config{
				Owners:      []repo.Owner{"a", "b", "c"},
				MergeMethod: repo.MergeMethodSquash,
				RateFloor:   defaultRateFloor,
			},
			wantContains: `"considered": 0`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want, must := assert.New(t), require.New(t)

			origRun, origCfg, origOwners, origMethod := runAction, cfg, owners, method
			t.Cleanup(func() { runAction, cfg, owners, method = origRun, origCfg, origOwners, origMethod })
			cfg, owners, method = domain.Config{}, nil, string(repo.MergeMethodSquash)

			var gotCfg domain.Config
			runAction = func(_ context.Context, _ *slog.Logger, c domain.Config, _ ...string) (domain.Result, error) {
				gotCfg = c
				return domain.Result{IsDryRun: bool(c.IsDryRun), Pulls: []domain.PullResult{}}, nil
			}

			var stdout bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelWarn}))
			appCmd := &cli.Command{
				Name:     "app",
				Writer:   &stdout,
				Commands: []*cli.Command{Command()},
				Metadata: map[string]any{app.LoggerMetadataKey: logger},
			}

			must.NoError(appCmd.Run(context.Background(), tt.args))
			want.Equal(tt.wantCfg, gotCfg)
			want.Contains(stdout.String(), tt.wantContains)
		})
	}
}

func TestBindOwnersTrimsWrappedYAMLScalars(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []repo.Owner
	}{
		{
			name: "comma-space list from a folded YAML scalar",
			args: []string{"app", "dependabot-sweep", "--owner", "acme, other,  third"},
			want: []repo.Owner{"acme", "other", "third"},
		},
		{
			name: "empty entries are dropped, not swept as blank owners",
			args: []string{"app", "dependabot-sweep", "--owner", "acme,,  ,other"},
			want: []repo.Owner{"acme", "other"},
		},
		{
			name: "padded merge method is trimmed",
			args: []string{"app", "dependabot-sweep", "--owner", "acme", "--merge-method", " rebase "},
			want: []repo.Owner{"acme"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want, must := assert.New(t), require.New(t)

			origRun, origCfg, origOwners, origMethod := runAction, cfg, owners, method
			t.Cleanup(func() { runAction, cfg, owners, method = origRun, origCfg, origOwners, origMethod })
			cfg, owners, method = domain.Config{}, nil, string(repo.MergeMethodSquash)

			var gotCfg domain.Config
			runAction = func(_ context.Context, _ *slog.Logger, c domain.Config, _ ...string) (domain.Result, error) {
				gotCfg = c
				return domain.Result{Pulls: []domain.PullResult{}}, nil
			}

			appCmd := &cli.Command{
				Name:     "app",
				Writer:   &bytes.Buffer{},
				Commands: []*cli.Command{Command()},
				Metadata: map[string]any{app.LoggerMetadataKey: slog.New(
					slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelWarn}))},
			}

			must.NoError(appCmd.Run(context.Background(), tt.args))
			want.Equal(tt.want, gotCfg.Owners)
			want.NotContains(string(gotCfg.MergeMethod), " ")
		})
	}
}
