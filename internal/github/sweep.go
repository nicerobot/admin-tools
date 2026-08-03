// This file holds the Dependabot-sweep half of the client: the endpoints the
// sweeper drives (pull-request search, pull-request fetch, check runs, merge,
// and the rate budget) together with the JSON-bodied request path only the
// merge needs. It is separate from client.go because it is a separate concern —
// the generic REST plumbing there serves every command, this serves one — and
// keeping it apart is what lets sweep_test.go be the unit tests of a source
// file that names it.

package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/nicerobot/tools.admin/internal/constants"
	"github.com/nicerobot/tools.admin/internal/repo"
)

// SearchDependabotPulls finds every open Dependabot pull request across an
// owner in one paginated query. This is the sweeper's discovery step: it costs
// a request per owner rather than a request per repository, so a fleet sweep
// spends tens of requests instead of hundreds.
func (c Client) SearchDependabotPulls(owner repo.Owner) ([]SearchItem, error) {
	query := url.Values{}
	query.Set("q", "is:pr is:open author:app/"+dependabotApp+" user:"+string(owner))
	query.Set("per_page", perPage)
	raw, err := c.paginate("/search/issues", query, "items")
	if err != nil {
		return nil, err
	}
	return decode[SearchItem](raw)
}

// GetPullRequest fetches one pull request in full, which the search result does
// not carry: the head commit to gate checks on and the body Dependabot writes
// version transitions into.
func (c Client) GetPullRequest(
	owner repo.Owner,
	name repo.Name,
	number repo.PullNumber,
) (PullRequest, error) {
	var pull PullRequest
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, name, number)
	if err := c.getJSON(path, &pull); err != nil {
		return PullRequest{}, err
	}
	return pull, nil
}

// dependabotApp is the app slug search matches Dependabot pull requests by.
const dependabotApp = "dependabot"

// ListCheckRuns lists the check runs reported against a commit.
func (c Client) ListCheckRuns(owner repo.Owner, name repo.Name, sha repo.SHA) ([]CheckRun, error) {
	path := fmt.Sprintf("/repos/%s/%s/commits/%s/check-runs", owner, name, sha)
	raw, err := c.paginate(path, params(), "check_runs")
	if err != nil {
		return nil, err
	}
	return decode[CheckRun](raw)
}

// MergePullRequest merges a pull request with the given method. A non-2xx reply
// is returned as ErrHTTPStatus carrying the status, so the caller can classify a
// refusal (405 not mergeable, 409 head moved) as a skip rather than a failure.
func (c Client) MergePullRequest(
	owner repo.Owner,
	name repo.Name,
	number repo.PullNumber,
	method repo.MergeMethod,
) error {
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/merge", owner, name, number)
	body := mergeBody{MergeMethod: string(method)}
	resp, err := c.send(http.MethodPut, c.baseURL+path, body)
	if err != nil {
		return err
	}
	if !is2xx(resp.status) {
		return statusErr(resp.status, requestURL(path))
	}
	return nil
}

// mergeBody is the merge endpoint's request payload.
type mergeBody struct {
	MergeMethod string `json:"merge_method"`
}

// RateRemaining reports the core API requests still available. The rate_limit
// endpoint does not itself count against the budget, so polling it is free.
func (c Client) RateRemaining() (repo.RateFloor, error) {
	var env rateLimitEnvelope
	if err := c.getJSON("/rate_limit", &env); err != nil {
		return 0, err
	}
	return repo.RateFloor(env.Resources.Core.Remaining), nil
}

// send issues a request carrying a JSON body.
func (c Client) send(method, rawurl string, payload any) (response, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return response{}, decodeErr(err)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, rawurl, bytes.NewReader(encoded))
	if err != nil {
		return response{}, constants.ErrHTTPStatus.With(err, "url", rawurl)
	}
	setHeaders(req, c.token)
	req.Header.Set("Content-Type", acceptHeader)
	resp, err := c.doer.Do(req)
	if err != nil {
		return response{}, constants.ErrHTTPStatus.With(err, "url", rawurl)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return response{}, constants.ErrHTTPStatus.With(err, "url", rawurl)
	}
	return response{status: statusCode(resp.StatusCode), header: resp.Header, body: body}, nil
}
