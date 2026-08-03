package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHandlerTypeFor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		format LogFormat
		want   handlerType
	}{
		{name: "json", format: "json", want: handlerTypeJSON},
		{name: "text", format: "text", want: handlerTypeText},
		{name: "empty defaults to text", format: "", want: handlerTypeText},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.New(t).Equal(tt.want, handlerTypeFor(tt.format))
		})
	}
}
