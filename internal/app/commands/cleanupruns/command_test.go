package cleanupruns

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/nicerobot/tools.admin/internal/app"
	domain "github.com/nicerobot/tools.admin/internal/domain/cleanupruns"
)

func TestCleanupRunsCommand(t *testing.T) {
	tests := []struct {
		name         string
		wantContains string
		args         []string
		wantCfg      domain.Config
	}{
		{
			name:         "defaults",
			args:         []string{"app", "cleanup-runs"},
			wantCfg:      domain.Config{Days: 30, Keep: 5},
			wantContains: `"repos_scanned": 0`,
		},
		{
			name: "binds every flag",
			args: []string{
				"app", "cleanup-runs",
				"--owner", "nicerobot", "--repo", "widget",
				"--days", "7", "--keep", "2", "--dry-run",
			},
			wantCfg:      domain.Config{Owner: "nicerobot", Repo: "widget", Days: 7, Keep: 2, IsDryRun: true},
			wantContains: `"dry_run": true`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want, must := assert.New(t), require.New(t)

			origRun, origCfg := runAction, cfg
			t.Cleanup(func() { runAction, cfg = origRun, origCfg })

			var gotCfg domain.Config
			runAction = func(_ context.Context, _ *slog.Logger, c domain.Config, _ ...string) (domain.Result, error) {
				gotCfg = c
				return domain.Result{IsDryRun: bool(c.IsDryRun)}, nil
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

// TestCleanupRunsEnvBindings proves every flag binds its RADM_* environment
// variable: with no flags on the command line, the env values land in the
// domain Config.
func TestCleanupRunsEnvBindings(t *testing.T) {
	want, must := assert.New(t), require.New(t)

	t.Setenv("RADM_OWNER", "envorg")
	t.Setenv("RADM_REPO", "envrepo")
	t.Setenv("RADM_DAYS", "11")
	t.Setenv("RADM_KEEP", "3")
	t.Setenv("RADM_DRY_RUN", "true")

	origRun, origCfg := runAction, cfg
	t.Cleanup(func() { runAction, cfg = origRun, origCfg })

	var gotCfg domain.Config
	runAction = func(_ context.Context, _ *slog.Logger, c domain.Config, _ ...string) (domain.Result, error) {
		gotCfg = c
		return domain.Result{}, nil
	}

	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelWarn}))
	appCmd := &cli.Command{
		Name:     "app",
		Writer:   &bytes.Buffer{},
		Commands: []*cli.Command{Command()},
		Metadata: map[string]any{app.LoggerMetadataKey: logger},
	}

	must.NoError(appCmd.Run(context.Background(), []string{"app", "cleanup-runs"}))
	want.Equal(domain.Config{Owner: "envorg", Repo: "envrepo", Days: 11, Keep: 3, IsDryRun: true}, gotCfg)
}
