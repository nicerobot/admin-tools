package snapshot

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/nicerobot/tools.admin/internal/app"
	"github.com/nicerobot/tools.admin/internal/constants"
	domain "github.com/nicerobot/tools.admin/internal/domain/snapshot"
)

func TestSnapshotCommand(t *testing.T) {
	tests := []struct {
		wantCfg      domain.Config
		name         string
		wantContains string
		args         []string
		wantArgs     []string
	}{
		{
			name:         "binds settings-path and passes owner positionally",
			args:         []string{"app", "snapshot", "--settings-path", ".gh", "myorg"},
			wantCfg:      domain.Config{SettingsPath: ".gh"},
			wantArgs:     []string{"myorg"},
			wantContains: `"owner": "myorg"`,
		},
		{
			name:         "settings-path defaults",
			args:         []string{"app", "snapshot", "nicerobot"},
			wantCfg:      domain.Config{SettingsPath: ".github"},
			wantArgs:     []string{"nicerobot"},
			wantContains: `"owner": "nicerobot"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want, must := assert.New(t), require.New(t)

			origRun, origCfg := runAction, cfg
			t.Cleanup(func() { runAction, cfg = origRun, origCfg })

			var gotCfg domain.Config
			var gotArgs []string
			runAction = func(_ context.Context, _ *slog.Logger, c domain.Config, args ...string) (domain.Result, error) {
				gotCfg, gotArgs = c, args
				return domain.Result{Owner: args[0]}, nil
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
			want.Equal(tt.wantArgs, gotArgs)
			want.Contains(stdout.String(), tt.wantContains)
		})
	}
}

// TestSnapshotEnvBindings proves the settings-path flag binds its RADM_*
// environment variable: with no flag on the command line, the env value lands
// in the domain Config.
func TestSnapshotEnvBindings(t *testing.T) {
	want, must := assert.New(t), require.New(t)

	t.Setenv("RADM_SETTINGS_PATH", ".envgh")

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

	must.NoError(appCmd.Run(context.Background(), []string{"app", "snapshot", "envorg"}))
	want.Equal(domain.Config{SettingsPath: ".envgh"}, gotCfg)
}

// TestSnapshotCommandMissingOwner drives the real domain Run: with no positional
// argument the command fails with ErrMissingArgument before touching any
// collaborator, so no token or filesystem is needed.
func TestSnapshotCommandMissingOwner(t *testing.T) {
	must := require.New(t)

	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelError}))
	appCmd := &cli.Command{
		Name:     "app",
		Writer:   &bytes.Buffer{},
		Commands: []*cli.Command{Command()},
		Metadata: map[string]any{app.LoggerMetadataKey: logger},
	}

	err := appCmd.Run(context.Background(), []string{"app", "snapshot"})
	must.ErrorIs(err, constants.ErrMissingArgument)
}
