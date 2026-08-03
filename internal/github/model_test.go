package github_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/nicerobot/tools.admin/internal/github"
	"github.com/nicerobot/tools.admin/internal/repo"
)

func TestUserAccountType(t *testing.T) {
	u := github.User{Login: "myorg", Type: "Organization"}
	assert.Equal(t, repo.AccountTypeOrganization, u.AccountType())
}

func TestRepositoryVisibilityPublic(t *testing.T) {
	r := github.Repository{IsPrivate: false}
	assert.Equal(t, repo.VisibilityPublic, r.Visibility())
}

func TestRepositoryVisibilityPrivate(t *testing.T) {
	r := github.Repository{IsPrivate: true}
	assert.Equal(t, repo.VisibilityPrivate, r.Visibility())
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
			assert.New(t).Equal(tt.want, github.SearchItem{RepositoryURL: tt.url}.RepoName())
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
			run := github.CheckRun{Name: "go", Status: tt.status, Conclusion: tt.conclusion}
			want := assert.New(t)
			want.Equal(tt.wantComplete, run.IsComplete())
			want.Equal(tt.wantPassing, run.IsPassing())
		})
	}
}
