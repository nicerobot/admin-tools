package github

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nicerobot/tools.admin/internal/constants"
)

// TestReadBodyRefusesABodyOverTheCeiling names the guard's claim.
//
// io.ReadAll grows a buffer until the reader stops, and nothing on the far end
// of a network socket is obliged to. Without the ceiling a hostile endpoint, a
// proxy injecting an error page, or a malfunctioning server takes the process
// down through memory rather than through any status this client checks — a
// remote-triggered, total failure.
// The body here is ENDLESS, and that is the point. A finite oversized reader
// cannot prove this guard: it terminates on its own, so io.ReadAll returns the
// same bytes whether or not the read was ever bounded, and the length check
// alone still reports the error. That test passed with io.LimitReader deleted —
// it pinned the symptom and not the protection. An endless body can only be
// survived by bounding the READ, which is the property that keeps a remote peer
// from exhausting memory.
func TestReadBodyRefusesABodyOverTheCeiling(t *testing.T) {
	t.Parallel()

	endless := &endlessReader{stopAfter: maxResponseBytes * 2}

	_, err := readBody(endless)

	require.ErrorIs(t, err, constants.ErrResponseTooLarge)
	assert.NotErrorIs(t, err, errReadTooFar, "the read must stop at the ceiling, not merely be measured after it")
	assert.LessOrEqual(t, endless.served, maxResponseBytes+1,
		"no more than the ceiling plus its probe byte may ever be pulled into memory")
}

// errReadTooFar marks a read that ran past any bound the caller should have
// imposed. A reader that keeps yielding forever would hang the test instead of
// failing it, so this converts "unbounded" into a visible verdict.
var errReadTooFar = errors.New("read continued past the ceiling")

// endlessReader yields bytes without end until stopAfter is exceeded, then
// fails, standing in for a peer that never closes the connection.
type endlessReader struct {
	served    int
	stopAfter int
}

func (e *endlessReader) Read(p []byte) (int, error) {
	if e.served > e.stopAfter {
		return 0, errReadTooFar
	}
	e.served += len(p)
	return len(p), nil
}

// TestReadBodyAcceptsABodyExactlyAtTheCeiling pins the boundary in the safe
// direction: the limit is read with one spare byte precisely so a body ending
// AT the ceiling is returned whole rather than silently truncated.
func TestReadBodyAcceptsABodyExactlyAtTheCeiling(t *testing.T) {
	t.Parallel()

	exact := strings.NewReader(strings.Repeat("a", maxResponseBytes))

	data, err := readBody(exact)

	require.NoError(t, err)
	assert.Len(t, data, maxResponseBytes, "a body at the limit is complete, not truncated")
}

// TestReadBodyReturnsAnOrdinaryBodyAndSurfacesReadFailures covers the two
// remaining paths: the common case, and a transport error that is not a size
// problem and must not be reported as one.
func TestReadBodyReturnsAnOrdinaryBodyAndSurfacesReadFailures(t *testing.T) {
	t.Parallel()

	data, err := readBody(strings.NewReader("ok"))
	require.NoError(t, err)
	assert.Equal(t, []byte("ok"), data)

	broken := errors.New("connection reset")
	_, err = readBody(failingReader{err: broken})
	require.ErrorIs(t, err, broken)
	assert.NotErrorIs(t, err, constants.ErrResponseTooLarge, "a read failure is not a size failure")
}

// failingReader fails every read, standing in for a dropped connection.
type failingReader struct{ err error }

func (f failingReader) Read([]byte) (int, error) { return 0, f.err }

var _ io.Reader = failingReader{}
