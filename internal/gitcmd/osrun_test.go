package gitcmd

import (
	"errors"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nicerobot/tools.admin/internal/constants"
)

func TestCommandAllowlist(t *testing.T) {
	tests := []struct {
		wantErr error
		name    string
		wantBin string
		args    []string
	}{
		{name: "git", args: []string{"git", "status"}, wantBin: "git"},
		{name: "gh", args: []string{"gh", "pr", "list"}, wantBin: "gh"},
		{name: "empty", args: nil, wantErr: constants.ErrCommand},
		{name: "disallowed binary", args: []string{"rm", "-rf"}, wantErr: constants.ErrCommand},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := command(tt.args)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantBin, cmd.Args[0])
		})
	}
}

func TestClassify(t *testing.T) {
	stdout := func() string { return "captured" }

	t.Run("clean exit returns stdout", func(t *testing.T) {
		res, err := classify(stdout, nil)
		require.NoError(t, err)
		assert.Equal(t, "captured", res.Stdout)
		assert.Equal(t, 0, res.ExitCode)
	})

	t.Run("non-zero exit returns exit code", func(t *testing.T) {
		runErr := exec.Command("sh", "-c", "exit 7").Run()
		res, err := classify(stdout, runErr)
		require.NoError(t, err)
		assert.Equal(t, 7, res.ExitCode)
		assert.Equal(t, "captured", res.Stdout)
	})

	t.Run("launch failure returns error", func(t *testing.T) {
		_, err := classify(stdout, errors.New("boom"))
		require.Error(t, err)
		assert.Equal(t, "boom", err.Error())
	})
}

func TestOSRun(t *testing.T) {
	t.Run("git runs", func(t *testing.T) {
		res, err := OSRun([]string{"git", "--version"})
		require.NoError(t, err)
		assert.Contains(t, res.Stdout, "git version")
		assert.Equal(t, 0, res.ExitCode)
	})

	t.Run("disallowed binary errors before exec", func(t *testing.T) {
		_, err := OSRun([]string{"rm", "-rf", "/"})
		require.ErrorIs(t, err, constants.ErrCommand)
	})
}
