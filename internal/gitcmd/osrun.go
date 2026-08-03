package gitcmd

import (
	"errors"
	"os/exec"
	"strings"

	"github.com/nicerobot/tools.admin/internal/constants"
)

// binaries are the only executables gitcmd ever launches. Dispatching on a fixed
// set keeps each exec.Command call a constant-command call (no variable command
// path) and rejects anything else.
var binaries = map[string]struct{}{gitBin: {}, ghBin: {}}

// OSRun executes git/gh, capturing stdout. A clean exit and a non-zero exit both
// return a Result (the latter carrying the exit code); only a failure to execute
// the binary at all returns an error. It is the production RunFunc.
func OSRun(args []string) (Result, error) {
	cmd, err := command(args)
	if err != nil {
		return Result{}, err
	}
	var out strings.Builder
	cmd.Stdout = &out
	return classify(out.String, cmd.Run())
}

// command builds the *exec.Cmd for an allowlisted binary, keeping the command
// name a constant literal so it is never a variable-command subprocess.
func command(args []string) (*exec.Cmd, error) {
	if len(args) == 0 {
		return nil, constants.ErrCommand.With(nil, "args", "empty")
	}
	if _, ok := binaries[args[0]]; !ok {
		return nil, constants.ErrCommand.With(nil, "binary", args[0])
	}
	if args[0] == ghBin {
		return exec.Command(ghBin, args[1:]...), nil
	}
	return exec.Command(gitBin, args[1:]...), nil
}

// classify turns the os/exec outcome into a Result: a clean run yields the
// captured stdout; a non-zero exit yields the exit code; a failure to launch the
// binary at all surfaces as an error.
func classify(stdout func() string, err error) (Result, error) {
	if err == nil {
		return Result{Stdout: stdout()}, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return Result{Stdout: stdout(), ExitCode: exit.ExitCode()}, nil
	}
	return Result{}, err
}
