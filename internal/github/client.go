// Package github is the GitHub REST client tools.admin uses, together with the
// JSON-decoded model shapes it consumes. All I/O goes through an injected Doer so
// tests run with a fake transport — no network. It reproduces the original
// client's endpoint-selection strategy (org vs authenticated-user vs
// App-installation vs public user) and Link-header pagination exactly. It is a
// pure implementation package with no knowledge of the CLI.
package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/nicerobot/tools.admin/internal/constants"
	"github.com/nicerobot/tools.admin/internal/repo"
)

const (
	// DefaultBaseURL is the production GitHub REST API root.
	DefaultBaseURL = "https://api.github.com"
	tokenEnv       = "GH_TOKEN"
	apiURLEnv      = "GITHUB_API_URL"
	acceptHeader   = "application/vnd.github+json"
	apiVersion     = "2022-11-28"
	perPage        = "100"
	httpTimeout    = 30 * time.Second
)

// NewFromEnv builds the production client: a no-redirect HTTP transport (so
// RepoExists can observe a 301 instead of chasing it), the base URL from
// GITHUB_API_URL (falling back to DefaultBaseURL when unset), and the token from
// GH_TOKEN — all read through the injected env lookup.
func NewFromEnv(env EnvLookup) (Client, error) {
	return New(productionDoer(), BaseURL(env(apiURLEnv)), env)
}

// productionDoer returns an *http.Client that does NOT follow redirects, so a
// 301 (renamed/transferred repo) is observable rather than chased.
func productionDoer() *http.Client {
	return &http.Client{
		Timeout: httpTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

var linkNextRe = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

// Doer issues an HTTP request; satisfied by *http.Client in production.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// EnvLookup reads an environment variable; injected for testability.
type EnvLookup func(key string) string

// BaseURL is a GitHub REST API root URL.
type BaseURL string

// authToken is the bearer token authenticating every API request.
type authToken string

// collectionKey is the object field holding a page's item array; empty when the
// page body is the array itself.
type collectionKey string

// queryParam is one half of a query-string key/value pair.
type queryParam string

// linkHeader is an HTTP Link header value carrying pagination relations.
type linkHeader string

// statusCode is an HTTP response status code.
type statusCode int

// requestURL is the path or URL a request targeted, reported in errors.
type requestURL string

// Client is the GitHub API client. It is an immutable value safe to copy.
type Client struct {
	doer    Doer
	baseURL string
	token   authToken
}

// New builds a Client, reading the token from GH_TOKEN via env. A missing token
// is fatal (ErrNoAuth), matching the original constructor. An empty baseURL
// resolves to DefaultBaseURL.
func New(doer Doer, baseURL BaseURL, env EnvLookup) (Client, error) {
	token := authToken(env(tokenEnv))
	if token == "" {
		return Client{}, constants.ErrNoAuth.With(nil)
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return Client{doer: doer, baseURL: string(baseURL), token: token}, nil
}

// GetAccountType returns the owner's account type (Organization, User, ...).
func (c Client) GetAccountType(owner repo.Owner) (repo.AccountType, error) {
	var u User
	if err := c.getJSON("/users/"+string(owner), &u); err != nil {
		return "", err
	}
	return u.AccountType(), nil
}

// RepoExists reports whether a repo exists at exactly owner/name. 404 and 301
// (renamed/transferred) count as gone; any other non-2xx is an error so the
// caller can abort safely.
func (c Client) RepoExists(owner repo.Owner, name repo.Name) (bool, error) {
	path := fmt.Sprintf("/repos/%s/%s", owner, name)
	resp, err := c.do(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return false, err
	}
	if resp.status == http.StatusNotFound || resp.status == http.StatusMovedPermanently {
		return false, nil
	}
	if !is2xx(resp.status) {
		return false, statusErr(resp.status, requestURL(path))
	}
	return true, nil
}

// ListRepos lists every repository owner owns, choosing the endpoint by account
// type and token kind.
func (c Client) ListRepos(owner repo.Owner) ([]Repository, error) {
	at, err := c.GetAccountType(owner)
	if err != nil {
		return nil, err
	}
	raw, err := c.listReposRaw(owner, at)
	if err != nil {
		return nil, err
	}
	return decode[Repository](raw)
}

func (c Client) listReposRaw(owner repo.Owner, at repo.AccountType) ([]json.RawMessage, error) {
	if at == repo.AccountTypeOrganization {
		return c.paginate("/orgs/"+string(owner)+"/repos", params("type", "all"), "")
	}
	return c.listUserRepos(owner)
}

func (c Client) listUserRepos(owner repo.Owner) ([]json.RawMessage, error) {
	login, err := c.authenticatedLogin()
	if err != nil {
		return nil, err
	}
	if login != "" && login == string(owner) {
		return c.paginate("/user/repos", params("affiliation", "owner"), "")
	}
	install, ok, err := c.listInstallationRepos(owner)
	if err != nil {
		return nil, err
	}
	if ok {
		return install, nil
	}
	return c.paginate("/users/"+string(owner)+"/repos", params("type", "owner"), "")
}

func (c Client) listInstallationRepos(owner repo.Owner) ([]json.RawMessage, bool, error) {
	raw, err := c.paginate("/installation/repositories", params(), "repositories")
	if err != nil {
		if errors.Is(err, constants.ErrHTTPStatus) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return filterByOwner(raw, owner), true, nil
}

func (c Client) authenticatedLogin() (string, error) {
	resp, err := c.do(http.MethodGet, c.baseURL+"/user", nil)
	if err != nil {
		return "", err
	}
	if !is2xx(resp.status) {
		return "", nil
	}
	var u loginProbe
	if err := json.Unmarshal(resp.body, &u); err != nil {
		return "", decodeErr(err)
	}
	return u.Login, nil
}

// loginProbe decodes just the login field of a GitHub user object.
type loginProbe struct {
	Login string `json:"login"`
}

// ListWorkflowRuns lists completed runs, optionally filtered to those created
// before the given date (the GitHub created=<YYYY-MM-DD syntax).
func (c Client) ListWorkflowRuns(
	owner repo.Owner,
	name repo.Name,
	before repo.CreatedBefore,
) ([]WorkflowRun, error) {
	path := fmt.Sprintf("/repos/%s/%s/actions/runs", owner, name)
	raw, err := c.paginate(path, runParams(before), "workflow_runs")
	if err != nil {
		return nil, err
	}
	return decode[WorkflowRun](raw)
}

// DeleteWorkflowRun deletes a single workflow run by id.
func (c Client) DeleteWorkflowRun(owner repo.Owner, name repo.Name, id repo.RunID) error {
	path := fmt.Sprintf("/repos/%s/%s/actions/runs/%d", owner, name, id)
	resp, err := c.do(http.MethodDelete, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	if !is2xx(resp.status) {
		return statusErr(resp.status, requestURL(path))
	}
	return nil
}

// paginate walks Link rel="next" pages, collecting raw items from each. When
// itemsKey is non-empty the page body is an object whose itemsKey field is the
// array; otherwise the body is the array itself.
func (c Client) paginate(path string, query url.Values, itemsKey collectionKey) ([]json.RawMessage, error) {
	var out []json.RawMessage
	next := c.baseURL + path
	for next != "" {
		items, link, err := c.page(next, query, itemsKey)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
		next = link
		query = nil
	}
	return out, nil
}

func (c Client) page(rawurl string, query url.Values, itemsKey collectionKey) ([]json.RawMessage, string, error) {
	resp, err := c.do(http.MethodGet, rawurl, query)
	if err != nil {
		return nil, "", err
	}
	if !is2xx(resp.status) {
		return nil, "", statusErr(resp.status, requestURL(rawurl))
	}
	items, err := extractItems(resp.body, itemsKey)
	if err != nil {
		return nil, "", err
	}
	return items, nextLink(linkHeader(resp.header.Get("Link"))), nil
}

func (c Client) getJSON(path string, v any) error {
	resp, err := c.do(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	if !is2xx(resp.status) {
		return statusErr(resp.status, requestURL(path))
	}
	if err := json.Unmarshal(resp.body, v); err != nil {
		return decodeErr(err)
	}
	return nil
}

// response is the part of an HTTP reply the client acts on: the status code, the
// headers (for Link pagination), and the fully-read body. do owns and closes the
// underlying response body, so no *http.Response escapes — callers never hold an
// unclosed body.
type response struct {
	header http.Header
	body   []byte
	status statusCode
}

func (c Client) do(method, rawurl string, query url.Values) (response, error) {
	full := rawurl
	if query != nil {
		full = rawurl + "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(context.Background(), method, full, nil)
	if err != nil {
		return response{}, constants.ErrHTTPStatus.With(err, "url", full)
	}
	setHeaders(req, c.token)
	resp, err := c.doer.Do(req)
	if err != nil {
		return response{}, constants.ErrHTTPStatus.With(err, "url", full)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return response{}, constants.ErrHTTPStatus.With(err, "url", full)
	}
	return response{status: statusCode(resp.StatusCode), header: resp.Header, body: body}, nil
}

func setHeaders(req *http.Request, token authToken) {
	req.Header.Set("Authorization", "Bearer "+string(token))
	req.Header.Set("Accept", acceptHeader)
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
}

func extractItems(body []byte, itemsKey collectionKey) ([]json.RawMessage, error) {
	if itemsKey == "" {
		return unmarshalArray(body)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil, decodeErr(err)
	}
	return unmarshalArray(obj[string(itemsKey)])
}

func unmarshalArray(body []byte) ([]json.RawMessage, error) {
	var arr []json.RawMessage
	if err := json.Unmarshal(body, &arr); err != nil {
		return nil, decodeErr(err)
	}
	return arr, nil
}

// ownerProbe decodes just the owner login of a GitHub repository object.
type ownerProbe struct {
	Owner loginProbe `json:"owner"`
}

func filterByOwner(raw []json.RawMessage, owner repo.Owner) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(raw))
	for _, r := range raw {
		var probe ownerProbe
		if json.Unmarshal(r, &probe) == nil && probe.Owner.Login == string(owner) {
			out = append(out, r)
		}
	}
	return out
}

func decode[T any](raw []json.RawMessage) ([]T, error) {
	out := make([]T, 0, len(raw))
	for _, r := range raw {
		var v T
		if err := json.Unmarshal(r, &v); err != nil {
			return nil, decodeErr(err)
		}
		out = append(out, v)
	}
	return out, nil
}

func runParams(before repo.CreatedBefore) url.Values {
	v := params("status", "completed")
	if before != "" {
		v.Set("created", "<"+string(before))
	}
	return v
}

func params(pairs ...queryParam) url.Values {
	v := url.Values{}
	v.Set("per_page", perPage)
	for i := 0; i+1 < len(pairs); i += 2 {
		v.Set(string(pairs[i]), string(pairs[i+1]))
	}
	return v
}

func nextLink(header linkHeader) string {
	m := linkNextRe.FindStringSubmatch(string(header))
	if m == nil {
		return ""
	}
	return m[1]
}

func is2xx(code statusCode) bool { return code >= 200 && code < 300 }

func statusErr(code statusCode, where requestURL) error {
	return constants.ErrHTTPStatus.With(nil, "status", code, "url", where)
}

func decodeErr(err error) error { return constants.ErrDecodeResponse.With(err) }
