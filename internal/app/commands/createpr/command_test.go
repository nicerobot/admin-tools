package createpr

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/nicerobot/tools.admin/internal/app"
	domain "github.com/nicerobot/tools.admin/internal/domain/createpr"
)

func TestCreatePRCommand(t *testing.T) {
	tests := []struct {
		wantCfg      domain.Config
		name         string
		wantContains string
		args         []string
	}{
		{
			name:         "defaults",
			args:         []string{"app", "create-pr"},
			wantCfg:      domain.Config{SettingsPath: ".github", Branch: "settings-sync/snapshot", Base: "main"},
			wantContains: `"branch": "settings-sync/snapshot"`,
		},
		{
			name: "overrides branch and base",
			args: []string{
				"app",
				"create-pr",
				"--settings-path",
				".gh",
				"--branch",
				"x/y",
				"--base",
				"develop",
			},
			wantCfg:      domain.Config{SettingsPath: ".gh", Branch: "x/y", Base: "develop"},
			wantContains: `"branch": "x/y"`,
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
				return domain.Result{Branch: string(c.Branch)}, nil
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

// TestCreatePREnvBindings proves every flag binds its RADM_* environment
// variable: with no flags on the command line, the env values land in the
// domain Config.
func TestCreatePREnvBindings(t *testing.T) {
	want, must := assert.New(t), require.New(t)

	t.Setenv("RADM_SETTINGS_PATH", ".envgh")
	t.Setenv("RADM_BRANCH", "env/branch")
	t.Setenv("RADM_BASE", "envbase")

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

	must.NoError(appCmd.Run(context.Background(), []string{"app", "create-pr"}))
	want.Equal(domain.Config{SettingsPath: ".envgh", Branch: "env/branch", Base: "envbase"}, gotCfg)
}
