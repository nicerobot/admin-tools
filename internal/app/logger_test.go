package app

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/urfave/cli/v3"
)

func TestParseLevel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		level LogLevel
		want  slog.Level
	}{
		{name: "debug", level: "debug", want: slog.LevelDebug},
		{name: "error", level: "error", want: slog.LevelError},
		{name: "empty defaults to info", level: "", want: slog.LevelInfo},
		{name: "unknown defaults to info", level: "bogus", want: slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.New(t).Equal(tt.want, parseLevel(tt.level))
		})
	}
}

func TestNewLogger(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		cfg          LoggerConfig
		wantContains string
	}{
		{name: "text format", cfg: LoggerConfig{LogFormat: "text", LogLevel: "info"}, wantContains: "msg=hello"},
		{name: "json format", cfg: LoggerConfig{LogFormat: "json", LogLevel: "info"}, wantContains: `"msg":"hello"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := assert.New(t)

			var buf bytes.Buffer
			logger := NewLogger(&buf, tt.cfg)

			logger.Info("hello")
			want.Contains(buf.String(), tt.wantContains)
		})
	}
}

func TestGetLogger(t *testing.T) {
	t.Parallel()

	t.Run("nil command returns default", func(t *testing.T) {
		assert.New(t).NotNil(GetLogger(nil))
	})

	t.Run("command without logger returns default", func(t *testing.T) {
		command := &cli.Command{Name: "x"}
		assert.New(t).NotNil(GetLogger(command))
	})

	t.Run("command with logger returns it", func(t *testing.T) {
		want := assert.New(t)

		logger := slog.New(slog.NewTextHandler(&strings.Builder{}, nil))
		command := &cli.Command{
			Name:     "x",
			Metadata: map[string]any{LoggerMetadataKey: logger},
		}

		want.Same(logger, GetLogger(command))
	})
}
