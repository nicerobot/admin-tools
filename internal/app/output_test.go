package app

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nicerobot/tools.admin/internal/constants"
)

// payload is a minimal renderable result, so the encoders are exercised on a
// shape whose rendering is unambiguous in both formats.
type payload struct {
	Value string `json:"value"`
}

func TestOutput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		wantErr      error
		name         string
		format       format
		wantContains string
	}{
		{name: "json", format: formatJSON, wantContains: `"value": "x"`},
		{name: "yaml", format: formatYAML, wantContains: "value: x"},
		{name: "empty defaults to json", format: format(""), wantContains: `"value": "x"`},
		{name: "invalid format", format: format("toml"), wantErr: constants.ErrInvalidValue},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want, must := assert.New(t), require.New(t)

			var buf bytes.Buffer
			err := output(&buf, tt.format, payload{Value: "x"})

			if tt.wantErr != nil {
				must.Error(err)
				want.ErrorIs(err, tt.wantErr)
				return
			}

			must.NoError(err)
			want.Contains(buf.String(), tt.wantContains)
		})
	}
}
