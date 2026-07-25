package github

import (
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nicerobot/tools.admin/internal/constants"
	"github.com/nicerobot/tools.admin/internal/repo"
)

func TestSearchDependabotPulls(t *testing.T) {
	var gotURL string
	c := newClient(t, func(r *http.Request) (*http.Response, error) {
		gotURL = r.URL.String()
		body := `{"items":[{"repository_url":"https://api.test/repos/acme/widget",` +
			`"number":7,"title":"bump grpc"}]}`
		return resp(http.StatusOK, body, nil), nil
	})

	items, err := c.SearchDependabotPulls("acme")
	require.NoError(t, err)

	want := assert.New(t)
	require.Len(t, items, 1)
	want.Equal(7, items[0].Number)
	want.Equal("bump grpc", items[0].Title)
	want.Contains(gotURL, "/search/issues")
	want.Contains(gotURL, "author%3Aapp%2Fdependabot")
	want.Contains(gotURL, "user%3Aacme")
	want.Contains(gotURL, "is%3Apr")
	want.Contains(gotURL, "is%3Aopen")
}

func TestSearchDependabotPullsError(t *testing.T) {
	c := newClient(t, func(*http.Request) (*http.Response, error) {
		return resp(http.StatusInternalServerError, "", nil), nil
	})
	_, err := c.SearchDependabotPulls("acme")
	assert.New(t).ErrorIs(err, constants.ErrHTTPStatus)
}

func TestGetPullRequest(t *testing.T) {
	var gotPath string
	c := newClient(t, func(r *http.Request) (*http.Response, error) {
		gotPath = r.URL.Path
		body := `{"number":7,"title":"bump","body":"Bumps foo from 1.0.0 to 1.1.0.",` +
			`"draft":false,"user":{"login":"dependabot[bot]"},"head":{"sha":"deadbeef","ref":"dependabot/x"}}`
		return resp(http.StatusOK, body, nil), nil
	})

	pull, err := c.GetPullRequest("acme", "widget", 7)
	require.NoError(t, err)

	want := assert.New(t)
	want.Equal("/repos/acme/widget/pulls/7", gotPath)
	want.Equal(repo.AuthorDependabot, pull.Author())
	want.Equal(repo.SHA("deadbeef"), pull.HeadSHA())
	want.False(pull.IsDraft)
}

func TestGetPullRequestError(t *testing.T) {
	c := newClient(t, func(*http.Request) (*http.Response, error) {
		return resp(http.StatusNotFound, "", nil), nil
	})
	_, err := c.GetPullRequest("acme", "widget", 7)
	assert.New(t).ErrorIs(err, constants.ErrHTTPStatus)
}

func TestListCheckRuns(t *testing.T) {
	var gotPath string
	c := newClient(t, func(r *http.Request) (*http.Response, error) {
		gotPath = r.URL.Path
		body := `{"check_runs":[{"name":"go","status":"completed","conclusion":"success"}]}`
		return resp(http.StatusOK, body, nil), nil
	})

	runs, err := c.ListCheckRuns("acme", "widget", "deadbeef")
	require.NoError(t, err)

	want := assert.New(t)
	want.Equal("/repos/acme/widget/commits/deadbeef/check-runs", gotPath)
	require.Len(t, runs, 1)
	want.True(runs[0].IsComplete())
	want.True(runs[0].IsPassing())
}

func TestListCheckRunsError(t *testing.T) {
	c := newClient(t, func(*http.Request) (*http.Response, error) {
		return resp(http.StatusForbidden, "", nil), nil
	})
	_, err := c.ListCheckRuns("acme", "widget", "sha")
	assert.New(t).ErrorIs(err, constants.ErrHTTPStatus)
}

func TestMergePullRequest(t *testing.T) {
	var gotMethod, gotPath, gotBody, gotContentType string
	c := newClient(t, func(r *http.Request) (*http.Response, error) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		return resp(http.StatusOK, `{"merged":true}`, nil), nil
	})

	require.NoError(t, c.MergePullRequest("acme", "widget", 7, repo.MergeMethodSquash))

	want := assert.New(t)
	want.Equal(http.MethodPut, gotMethod)
	want.Equal("/repos/acme/widget/pulls/7/merge", gotPath)
	want.JSONEq(`{"merge_method":"squash"}`, gotBody)
	want.Equal(acceptHeader, gotContentType)
}

func TestMergePullRequestRefused(t *testing.T) {
	c := newClient(t, func(*http.Request) (*http.Response, error) {
		return resp(http.StatusMethodNotAllowed, `{"message":"not mergeable"}`, nil), nil
	})
	err := c.MergePullRequest("acme", "widget", 7, repo.MergeMethodSquash)
	assert.New(t).ErrorIs(err, constants.ErrHTTPStatus)
}

func TestMergePullRequestTransportFailure(t *testing.T) {
	c := newClient(t, func(*http.Request) (*http.Response, error) {
		return nil, errors.New("dial failed")
	})
	err := c.MergePullRequest("acme", "widget", 7, repo.MergeMethodSquash)
	assert.New(t).ErrorIs(err, constants.ErrHTTPStatus)
}

func TestMergePullRequestUnreadableBody(t *testing.T) {
	c := newClient(t, func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: errReader{}, Header: http.Header{}}, nil
	})
	err := c.MergePullRequest("acme", "widget", 7, repo.MergeMethodSquash)
	assert.New(t).ErrorIs(err, constants.ErrHTTPStatus)
}

func TestSendRejectsUnbuildableRequest(t *testing.T) {
	c, err := New(fakeDoer{}, "://bad-url", tokenEnvFn("tok"))
	require.NoError(t, err)
	err = c.MergePullRequest("acme", "widget", 7, repo.MergeMethodSquash)
	assert.New(t).ErrorIs(err, constants.ErrHTTPStatus)
}

func TestSendRejectsUnencodablePayload(t *testing.T) {
	c := newClient(t, func(*http.Request) (*http.Response, error) {
		return resp(http.StatusOK, "{}", nil), nil
	})
	_, err := c.send(http.MethodPut, "https://api.test/x", make(chan int))
	assert.New(t).ErrorIs(err, constants.ErrDecodeResponse)
}

func TestRateRemaining(t *testing.T) {
	var gotPath string
	c := newClient(t, func(r *http.Request) (*http.Response, error) {
		gotPath = r.URL.Path
		return resp(http.StatusOK, `{"resources":{"core":{"remaining":4321,"limit":5000}}}`, nil), nil
	})

	remaining, err := c.RateRemaining()
	require.NoError(t, err)

	want := assert.New(t)
	want.Equal("/rate_limit", gotPath)
	want.Equal(repo.RateFloor(4321), remaining)
}

func TestRateRemainingError(t *testing.T) {
	c := newClient(t, func(*http.Request) (*http.Response, error) {
		return resp(http.StatusServiceUnavailable, "", nil), nil
	})
	_, err := c.RateRemaining()
	assert.New(t).ErrorIs(err, constants.ErrHTTPStatus)
}

func TestSearchItemRepoName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		url  string
		want repo.Name
	}{
		{name: "normal url", url: "https://api.github.com/repos/acme/widget", want: "widget"},
		{name: "dotted repo name", url: "https://api.github.com/repos/acme/docs.widget", want: "docs.widget"},
		{name: "empty url", url: "", want: ""},
		{name: "no separator", url: "widget", want: ""},
		{name: "trailing slash", url: "https://api.github.com/repos/acme/", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.New(t).Equal(tt.want, SearchItem{RepositoryURL: tt.url}.RepoName())
		})
	}
}

func TestCheckRunVerdicts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		status       string
		conclusion   string
		wantComplete bool
		wantPassing  bool
	}{
		{name: "success", status: "completed", conclusion: "success", wantComplete: true, wantPassing: true},
		{name: "neutral", status: "completed", conclusion: "neutral", wantComplete: true, wantPassing: true},
		{name: "skipped", status: "completed", conclusion: "skipped", wantComplete: true, wantPassing: true},
		{name: "failure", status: "completed", conclusion: "failure", wantComplete: true},
		{name: "cancelled", status: "completed", conclusion: "cancelled", wantComplete: true},
		{name: "timed out", status: "completed", conclusion: "timed_out", wantComplete: true},
		{name: "in progress", status: "in_progress"},
		{name: "queued", status: "queued"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			run := CheckRun{Name: "go", Status: tt.status, Conclusion: tt.conclusion}
			want := assert.New(t)
			want.Equal(tt.wantComplete, run.IsComplete())
			want.Equal(tt.wantPassing, run.IsPassing())
		})
	}
}
