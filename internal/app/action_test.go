package app

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/nicerobot/tools.admin/internal/constants"
)

type (
	testConfig struct{}
	testResult struct {
		Value string `json:"value"`
	}
)

func TestDefault(t *testing.T) {
	t.Parallel()
	tests := []struct {
		wantErr      error
		run          Runner[testConfig, testResult]
		name         string
		wantContains string
	}{
		{
			name: "success renders json result",
			run: func(context.Context, *slog.Logger, testConfig, ...string) (testResult, error) {
				return testResult{Value: "ok"}, nil
			},
			wantContains: `"value": "ok"`,
		},
		{
			name: "runner error propagates",
			run: func(context.Context, *slog.Logger, testConfig, ...string) (testResult, error) {
				return testResult{}, constants.ErrNoTarget
			},
			wantErr: constants.ErrNoTarget,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want, must := assert.New(t), require.New(t)

			var out bytes.Buffer
			cfg := testConfig{}
			command := &cli.Command{
				Name:   "x",
				Writer: &out,
				Action: Default(&cfg, tt.run),
			}

			err := command.Run(context.Background(), []string{"x"})

			if tt.wantErr != nil {
				must.Error(err)
				want.ErrorIs(err, tt.wantErr)
				return
			}

			must.NoError(err)
			want.Contains(out.String(), tt.wantContains)
		})
	}
}
