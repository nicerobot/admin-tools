package github

import (
	"io"

	"github.com/nicerobot/tools.admin/internal/constants"
)

// maxResponseBytes bounds a single GitHub API response held in memory.
//
// 32 MiB is far above any real response — the largest paginated page this
// client asks for is orders of magnitude smaller — so the limit is invisible in
// normal operation and only engages on a response that has stopped being
// plausible.
const maxResponseBytes = 32 << 20

// readBody drains a response body into memory with a ceiling.
//
// io.ReadAll grows a buffer until the reader says stop, and nothing on the far
// end of a network socket is obliged to. A hostile endpoint, a proxy injecting
// an error page, or simply a malfunctioning server can therefore take the
// process down through memory rather than through any status code this client
// checks. The failure is remote-triggered and total, which is what separates it
// from an ordinary read error.
//
// The limit is read with one extra byte so hitting it is distinguishable from a
// body that merely ends there: at the ceiling exactly, the read is truncated and
// silently wrong, which is the outcome worth refusing.
func readBody(body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxResponseBytes {
		return nil, constants.ErrResponseTooLarge.With(nil, "limit", maxResponseBytes)
	}
	return data, nil
}
