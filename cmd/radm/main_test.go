package main

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/nicerobot/tools.admin/internal/app"
	"github.com/nicerobot/tools.admin/internal/constants"
)

func TestRun_Version(t *testing.T) {
	want, must := assert.New(t), require.New(t)

	var stdout bytes.Buffer
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	appCmd := createApp(func(*cli.Command) *slog.Logger { return logger })
	appCmd.Writer = &stdout

	must.NoError(appCmd.Run(context.Background(), []string{"radm", "--version"}))
	want.Contains(stdout.String(), version)
}

func TestCreateApp(t *testing.T) {
	want, must := assert.New(t), require.New(t)

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	appCmd := createApp(func(*cli.Command) *slog.Logger { return logger })

	want.Equal(name, appCmd.Name)
	want.Equal(version, appCmd.Version)
	must.NotEmpty(appCmd.Commands)

	for _, expected := range []string{"snapshot", "create-pr", "cleanup-runs"} {
		found := false
		for _, cmd := range appCmd.Commands {
			if cmd.Name == expected {
				found = true
				break
			}
		}
		want.True(found, "expected command %q not found", expected)
	}
}

func TestRun_ExitCodes(t *testing.T) {
	original := appCreator
	t.Cleanup(func() { appCreator = original })

	want := assert.New(t)

	appCreator = func(app.GetLoggerFunc) *cli.Command {
		return &cli.Command{Name: "x", Writer: &bytes.Buffer{}}
	}
	want.Equal(0, run([]string{"x"}), "successful run exits 0")

	appCreator = func(app.GetLoggerFunc) *cli.Command {
		return &cli.Command{
			Name:   "x",
			Writer: &bytes.Buffer{},
			Action: func(context.Context, *cli.Command) error { return constants.ErrNoTarget },
		}
	}
	want.Equal(1, run([]string{"x"}), "failed run exits 1")
}

func TestMainEntry(t *testing.T) {
	originalCreator, originalExit, originalArgs := appCreator, osExit, os.Args
	t.Cleanup(func() { appCreator, osExit, os.Args = originalCreator, originalExit, originalArgs })

	var code int
	osExit = func(c int) { code = c }
	appCreator = func(app.GetLoggerFunc) *cli.Command {
		return &cli.Command{Name: "x", Writer: &bytes.Buffer{}}
	}
	os.Args = []string{"x"}

	main()
	assert.New(t).Equal(0, code)
}

func TestProductionLogger(t *testing.T) {
	assert.New(t).NotNil(productionLogger(nil))
}

// TestBeforeHookSetsLogger drives a real subcommand so the root Before hook runs,
// proving it stores the resolved logger in command metadata. The snapshot command
// errors (no settings.yml in the temp dir), which is irrelevant: the Before hook
// fires before the action.
func TestBeforeHookSetsLogger(t *testing.T) {
	t.Setenv("GH_TOKEN", "tok")
	want := assert.New(t)

	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelError}))
	appCmd := createApp(func(*cli.Command) *slog.Logger { return logger })
	appCmd.Writer = &bytes.Buffer{}

	_ = appCmd.Run(context.Background(), []string{"radm", "snapshot", "--settings-path", t.TempDir(), "x"})

	want.Same(logger, appCmd.Metadata[app.LoggerMetadataKey])
}
