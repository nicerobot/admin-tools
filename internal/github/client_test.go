package github

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nicerobot/tools.admin/internal/constants"
	"github.com/nicerobot/tools.admin/internal/repo"
)

type fakeDoer struct {
	fn func(*http.Request) (*http.Response, error)
}

func (f fakeDoer) Do(r *http.Request) (*http.Response, error) { return f.fn(r) }

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read boom") }
func (errReader) Close() error             { return nil }

func resp(code int, body string, header http.Header) *http.Response {
	if header == nil {
		header = http.Header{}
	}
	return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body)), Header: header}
}

func tokenEnvFn(tok string) EnvLookup { return func(string) string { return tok } }

func newClient(t *testing.T, fn func(*http.Request) (*http.Response, error)) Client {
	t.Helper()
	c, err := New(fakeDoer{fn}, "https://api.test", tokenEnvFn("tok"))
	require.NoError(t, err)
	return c
}

func reposArray(names ...string) string {
	parts := make([]string, len(names))
	for i, n := range names {
		parts[i] = fmt.Sprintf(`{"name":%q}`, n)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func TestNewNoToken(t *testing.T) {
	_, err := New(fakeDoer{}, "", tokenEnvFn(""))
	require.ErrorIs(t, err, constants.ErrNoAuth)
}

func TestNewDefaultBaseURL(t *testing.T) {
	c, err := New(fakeDoer{}, "", tokenEnvFn("tok"))
	require.NoError(t, err)
	assert.Equal(t, DefaultBaseURL, c.baseURL)
}

func TestNewExplicitBaseURL(t *testing.T) {
	c, err := New(fakeDoer{}, "https://api.test", tokenEnvFn("tok"))
	require.NoError(t, err)
	assert.Equal(t, "https://api.test", c.baseURL)
}

func TestRequestHeaders(t *testing.T) {
	c := newClient(t, func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, "Bearer tok", r.Header.Get("Authorization"))
		assert.Equal(t, acceptHeader, r.Header.Get("Accept"))
		assert.Equal(t, apiVersion, r.Header.Get("X-GitHub-Api-Version"))
		return resp(200, `{"login":"x","type":"User"}`, nil), nil
	})
	_, err := c.GetAccountType("x")
	require.NoError(t, err)
}

func TestGetAccountType(t *testing.T) {
	c := newClient(t, func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, "/users/myorg", r.URL.Path)
		return resp(200, `{"login":"myorg","type":"Organization"}`, nil), nil
	})
	at, err := c.GetAccountType("myorg")
	require.NoError(t, err)
	assert.Equal(t, repo.AccountTypeOrganization, at)
}

func TestGetAccountTypeHTTPError(t *testing.T) {
	c := newClient(t, func(*http.Request) (*http.Response, error) {
		return resp(404, `{"message":"Not Found"}`, nil), nil
	})
	_, err := c.GetAccountType("nope")
	require.ErrorIs(t, err, constants.ErrHTTPStatus)
}

func TestGetAccountTypeDecodeError(t *testing.T) {
	c := newClient(t, func(*http.Request) (*http.Response, error) {
		return resp(200, `not json`, nil), nil
	})
	_, err := c.GetAccountType("x")
	require.ErrorIs(t, err, constants.ErrDecodeResponse)
}

func TestGetAccountTypeBodyReadError(t *testing.T) {
	c := newClient(t, func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: errReader{}, Header: http.Header{}}, nil
	})
	_, err := c.GetAccountType("x")
	require.ErrorIs(t, err, constants.ErrHTTPStatus)
}

func TestRepoExists(t *testing.T) {
	cases := []struct {
		code int
		want bool
		err  bool
	}{
		{200, true, false},
		{404, false, false},
		{301, false, false},
		{500, false, true},
	}
	for _, tc := range cases {
		c := newClient(t, func(r *http.Request) (*http.Response, error) {
			assert.Equal(t, "/repos/o/name", r.URL.Path)
			return resp(tc.code, `{}`, nil), nil
		})
		got, err := c.RepoExists("o", "name")
		if tc.err {
			require.ErrorIs(t, err, constants.ErrHTTPStatus)
			continue
		}
		require.NoError(t, err)
		assert.Equal(t, tc.want, got)
	}
}

func TestRepoExistsDoError(t *testing.T) {
	c := newClient(t, func(*http.Request) (*http.Response, error) {
		return nil, errors.New("transport down")
	})
	_, err := c.RepoExists("o", "n")
	require.ErrorIs(t, err, constants.ErrHTTPStatus)
}

func userJSON(login, kind string) string {
	return fmt.Sprintf(`{"login":%q,"type":%q}`, login, kind)
}

func TestListReposOrg(t *testing.T) {
	var reposQuery string
	c := newClient(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/users/myorg":
			return resp(200, userJSON("myorg", "Organization"), nil), nil
		case "/orgs/myorg/repos":
			reposQuery = r.URL.RawQuery
			return resp(200, reposArray("a", "b"), nil), nil
		}
		return resp(404, `{}`, nil), nil
	})
	got, err := c.ListRepos("myorg")
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Contains(t, reposQuery, "type=all")
	assert.Contains(t, reposQuery, "per_page=100")
}

func TestListReposAuthenticatedUser(t *testing.T) {
	var query string
	c := newClient(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/users/nicerobot":
			return resp(200, userJSON("nicerobot", "User"), nil), nil
		case "/user":
			return resp(200, `{"login":"nicerobot"}`, nil), nil
		case "/user/repos":
			query = r.URL.RawQuery
			return resp(200, reposArray("r1"), nil), nil
		}
		return resp(404, `{}`, nil), nil
	})
	got, err := c.ListRepos("nicerobot")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Contains(t, query, "affiliation=owner")
}

func TestListReposOtherUser(t *testing.T) {
	var query string
	c := newClient(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/users/other":
			return resp(200, userJSON("other", "User"), nil), nil
		case "/user":
			return resp(200, `{"login":"me"}`, nil), nil
		case "/installation/repositories":
			return resp(403, `{"message":"Forbidden"}`, nil), nil
		case "/users/other/repos":
			query = r.URL.RawQuery
			return resp(200, reposArray("o1"), nil), nil
		}
		return resp(404, `{}`, nil), nil
	})
	got, err := c.ListRepos("other")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Contains(t, query, "type=owner")
}

func TestListReposAppToken(t *testing.T) {
	body := `{"total_count":4,"repositories":[` +
		`{"name":"repo1","owner":{"login":"nicerobot"}},` +
		`{"name":"repo2","owner":{"login":"nicerobot"}},` +
		`{"name":"other","owner":{"login":"someoneelse"}},` +
		`"junk"]}`
	c := newClient(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/users/nicerobot":
			return resp(200, userJSON("nicerobot", "User"), nil), nil
		case "/user":
			return resp(403, `{"message":"Forbidden"}`, nil), nil
		case "/installation/repositories":
			return resp(200, body, nil), nil
		}
		return resp(404, `{}`, nil), nil
	})
	got, err := c.ListRepos("nicerobot")
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "repo1", got[0].Name)
	assert.Equal(t, "repo2", got[1].Name)
}

func TestListReposPagination(t *testing.T) {
	page1 := make([]string, 100)
	for i := range page1 {
		page1[i] = fmt.Sprintf("repo%d", i)
	}
	c := newClient(t, func(r *http.Request) (*http.Response, error) {
		switch {
		case r.URL.Path == "/users/nicerobot":
			return resp(200, userJSON("nicerobot", "User"), nil), nil
		case r.URL.Path == "/user":
			return resp(200, `{"login":"nicerobot"}`, nil), nil
		case r.URL.Path == "/user/repos" && r.URL.Query().Get("page") == "2":
			return resp(200, reposArray("repo100", "repo101"), nil), nil
		case r.URL.Path == "/user/repos":
			h := http.Header{}
			h.Set("Link", `<https://api.test/user/repos?page=2>; rel="next"`)
			return resp(200, reposArray(page1...), h), nil
		}
		return resp(404, `{}`, nil), nil
	})
	got, err := c.ListRepos("nicerobot")
	require.NoError(t, err)
	assert.Len(t, got, 102)
}

func TestListReposAccountTypeError(t *testing.T) {
	c := newClient(t, func(*http.Request) (*http.Response, error) {
		return resp(404, `{}`, nil), nil
	})
	_, err := c.ListRepos("x")
	require.ErrorIs(t, err, constants.ErrHTTPStatus)
}

func TestListReposListError(t *testing.T) {
	c := newClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/users/myorg" {
			return resp(200, userJSON("myorg", "Organization"), nil), nil
		}
		return resp(500, `{}`, nil), nil
	})
	_, err := c.ListRepos("myorg")
	require.ErrorIs(t, err, constants.ErrHTTPStatus)
}

func TestListReposDecodeError(t *testing.T) {
	c := newClient(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/users/other":
			return resp(200, userJSON("other", "User"), nil), nil
		case "/user":
			return resp(200, `{"login":"me"}`, nil), nil
		case "/installation/repositories":
			return resp(403, `{}`, nil), nil
		}
		return resp(200, `[{"name":123}]`, nil), nil
	})
	_, err := c.ListRepos("other")
	require.ErrorIs(t, err, constants.ErrDecodeResponse)
}

func TestListReposAuthLoginDecodeError(t *testing.T) {
	c := newClient(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/users/x":
			return resp(200, userJSON("x", "User"), nil), nil
		case "/user":
			return resp(200, `{invalid`, nil), nil
		}
		return resp(404, `{}`, nil), nil
	})
	_, err := c.ListRepos("x")
	require.ErrorIs(t, err, constants.ErrDecodeResponse)
}

func TestListReposInstallationNonHTTPError(t *testing.T) {
	c := newClient(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/users/x":
			return resp(200, userJSON("x", "User"), nil), nil
		case "/user":
			return resp(403, `{}`, nil), nil
		case "/installation/repositories":
			return resp(200, `{"repositories":5}`, nil), nil // items not an array → decode error
		}
		return resp(404, `{}`, nil), nil
	})
	_, err := c.ListRepos("x")
	require.ErrorIs(t, err, constants.ErrDecodeResponse)
}

func TestListWorkflowRunsWithBefore(t *testing.T) {
	var query string
	c := newClient(t, func(r *http.Request) (*http.Response, error) {
		query = r.URL.RawQuery
		return resp(
			200,
			`{"total_count":2,"workflow_runs":[{"id":1,"workflow_id":1},{"id":2,"workflow_id":1}]}`,
			nil,
		), nil
	})
	runs, err := c.ListWorkflowRuns("nicerobot", "repo1", "2025-12-01")
	require.NoError(t, err)
	require.Len(t, runs, 2)
	assert.Contains(t, query, "created=%3C2025-12-01") // <2025-12-01 url-encoded
	assert.Contains(t, query, "status=completed")
}

func TestListWorkflowRunsNoBefore(t *testing.T) {
	var query string
	c := newClient(t, func(r *http.Request) (*http.Response, error) {
		query = r.URL.RawQuery
		return resp(200, `{"workflow_runs":[]}`, nil), nil
	})
	_, err := c.ListWorkflowRuns("nicerobot", "repo1", "")
	require.NoError(t, err)
	assert.NotContains(t, query, "created=")
}

func TestListWorkflowRunsPagination(t *testing.T) {
	c := newClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("page") == "2" {
			return resp(200, `{"workflow_runs":[{"id":2,"workflow_id":1}]}`, nil), nil
		}
		h := http.Header{}
		h.Set("Link", `<https://api.test/repos/o/r/actions/runs?page=2>; rel="next"`)
		return resp(200, `{"workflow_runs":[{"id":1,"workflow_id":1}]}`, h), nil
	})
	runs, err := c.ListWorkflowRuns("o", "r", "")
	require.NoError(t, err)
	assert.Len(t, runs, 2)
}

func TestListWorkflowRunsDoError(t *testing.T) {
	c := newClient(t, func(*http.Request) (*http.Response, error) {
		return nil, errors.New("down")
	})
	_, err := c.ListWorkflowRuns("o", "r", "")
	require.ErrorIs(t, err, constants.ErrHTTPStatus)
}

func TestListReposAuthLoginDoError(t *testing.T) {
	c := newClient(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/users/x":
			return resp(200, userJSON("x", "User"), nil), nil
		case "/user":
			return nil, errors.New("down")
		}
		return resp(404, `{}`, nil), nil
	})
	_, err := c.ListRepos("x")
	require.ErrorIs(t, err, constants.ErrHTTPStatus)
}

func TestListWorkflowRunsDecodeError(t *testing.T) {
	c := newClient(t, func(*http.Request) (*http.Response, error) {
		return resp(200, `{"workflow_runs":[{"id":"notint"}]}`, nil), nil
	})
	_, err := c.ListWorkflowRuns("o", "r", "")
	require.ErrorIs(t, err, constants.ErrDecodeResponse)
}

func TestDeleteWorkflowRun(t *testing.T) {
	c := newClient(t, func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/repos/o/r/actions/runs/42", r.URL.Path)
		return resp(204, "", nil), nil
	})
	require.NoError(t, c.DeleteWorkflowRun("o", "r", 42))
}

func TestDeleteWorkflowRunError(t *testing.T) {
	c := newClient(t, func(*http.Request) (*http.Response, error) {
		return resp(500, `{}`, nil), nil
	})
	require.ErrorIs(t, c.DeleteWorkflowRun("o", "r", 42), constants.ErrHTTPStatus)
}

func TestDeleteWorkflowRunDoError(t *testing.T) {
	c := newClient(t, func(*http.Request) (*http.Response, error) {
		return nil, errors.New("down")
	})
	require.ErrorIs(t, c.DeleteWorkflowRun("o", "r", 42), constants.ErrHTTPStatus)
}

func TestDoRequestBuildError(t *testing.T) {
	c := newClient(t, func(*http.Request) (*http.Response, error) {
		return resp(200, "", nil), nil
	})
	_, err := c.do(http.MethodGet, "://bad-url", nil)
	require.ErrorIs(t, err, constants.ErrHTTPStatus)
}

func TestExtractItemsArrayInvalid(t *testing.T) {
	_, err := extractItems([]byte("not json"), "")
	require.ErrorIs(t, err, constants.ErrDecodeResponse)
}

func TestExtractItemsObjectInvalidOuter(t *testing.T) {
	_, err := extractItems([]byte("not json"), "workflow_runs")
	require.ErrorIs(t, err, constants.ErrDecodeResponse)
}

func TestExtractItemsObjectValid(t *testing.T) {
	items, err := extractItems([]byte(`{"k":[1,2,3]}`), "k")
	require.NoError(t, err)
	assert.Len(t, items, 3)
}

func TestNextLink(t *testing.T) {
	assert.Equal(t, "https://x/y?page=2", nextLink(`<https://x/y?page=2>; rel="next"`))
	assert.Empty(t, nextLink(`<https://x/y?page=2>; rel="prev"`))
	assert.Empty(t, nextLink(""))
}

func TestNewFromEnv(t *testing.T) {
	t.Run("builds client from env", func(t *testing.T) {
		env := func(key string) string {
			switch key {
			case tokenEnv:
				return "tok"
			case apiURLEnv:
				return "https://ghe.example/api/v3"
			}
			return ""
		}
		c, err := NewFromEnv(env)
		require.NoError(t, err)
		assert.Equal(t, "https://ghe.example/api/v3", c.baseURL)
	})

	t.Run("empty api url falls back to default", func(t *testing.T) {
		env := func(key string) string {
			if key == tokenEnv {
				return "tok"
			}
			return ""
		}
		c, err := NewFromEnv(env)
		require.NoError(t, err)
		assert.Equal(t, DefaultBaseURL, c.baseURL)
	})

	t.Run("missing token errors", func(t *testing.T) {
		_, err := NewFromEnv(tokenEnvFn(""))
		require.ErrorIs(t, err, constants.ErrNoAuth)
	})
}

// closeRecorder is a response body that records having been closed.
type closeRecorder struct {
	io.Reader
	closed bool
}

func (c *closeRecorder) Close() error {
	c.closed = true
	return nil
}

// TestResponseBodyIsClosedBeforeReturning pins what response documents: do and
// send own the *http.Response, so a caller receives bytes rather than an open
// body it would have to remember to close. A leaked body is invisible to every
// other test — the fake transport never exhausts a descriptor — so the Close is
// asserted directly, on both the plain request path and the JSON-bodied one.
func TestResponseBodyIsClosedBeforeReturning(t *testing.T) {
	t.Run("get path", func(t *testing.T) {
		body := &closeRecorder{Reader: strings.NewReader(`{"login":"acme","type":"Organization"}`)}
		c := newClient(t, func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: body, Header: http.Header{}}, nil
		})

		_, err := c.GetAccountType("acme")

		require.NoError(t, err)
		assert.True(t, body.closed)
	})

	t.Run("send path", func(t *testing.T) {
		body := &closeRecorder{Reader: strings.NewReader(`{"merged":true}`)}
		c := newClient(t, func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: body, Header: http.Header{}}, nil
		})

		require.NoError(t, c.MergePullRequest("acme", "widget", 7, repo.MergeMethodSquash))
		assert.True(t, body.closed)
	})
}

// TestClientIsAnImmutableValueSafeToCopy pins the property Client documents. A
// copy is taken and both values are driven through the same call: they issue
// byte-for-byte identical requests and return the same answer, so a copy is
// interchangeable with the value it came from. A pointer receiver or mutable
// state (a cursor, a cached token) would surface here as divergence between the
// two invocations.
func TestClientIsAnImmutableValueSafeToCopy(t *testing.T) {
	var seen []string
	original := newClient(t, func(r *http.Request) (*http.Response, error) {
		seen = append(seen, r.Method+" "+r.URL.String()+" "+r.Header.Get("Authorization"))
		return resp(http.StatusOK, `{"login":"acme","type":"Organization"}`, nil), nil
	})
	copied := original

	fromCopy, err := copied.GetAccountType("acme")
	require.NoError(t, err)
	fromOriginal, err := original.GetAccountType("acme")
	require.NoError(t, err)

	assert.Equal(t, fromCopy, fromOriginal)
	require.Len(t, seen, 2)
	assert.Equal(t, seen[0], seen[1])
}

func TestProductionDoer(t *testing.T) {
	client := productionDoer()
	require.NotNil(t, client.CheckRedirect)
	assert.Equal(t, httpTimeout, client.Timeout)
	// The no-redirect policy returns ErrUseLastResponse so a 301 is observable.
	assert.ErrorIs(t, client.CheckRedirect(nil, nil), http.ErrUseLastResponse)
}
