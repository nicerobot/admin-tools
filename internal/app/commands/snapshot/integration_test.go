//go:build integration

package snapshot

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/nicerobot/tools.admin/internal/app"
)

// TestIntegration_SnapshotWritesOverrides assembles the real cli.Command and runs
// `snapshot` end-to-end against an httptest GitHub API, writing real override
// files into a temp settings directory — exercising the full app → domain →
// github/settings/overrides wiring with no real network.
func TestIntegration_SnapshotWritesOverrides(t *testing.T) {
	want, must := assert.New(t), require.New(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/users/acme":
			_, _ = w.Write([]byte(`{"login":"acme","type":"Organization"}`))
		case "/orgs/acme/repos":
			_, _ = w.Write([]byte(`[{"name":"widget","private":true,"default_branch":"main","has_issues":true}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	settingsDir := t.TempDir()
	must.NoError(
		os.WriteFile(
			filepath.Join(settingsDir, "settings.yml"),
			[]byte("repository:\n  default_branch: main\n"),
			0o644,
		),
	)

	t.Setenv("GH_TOKEN", "tok")
	t.Setenv("GITHUB_API_URL", server.URL)

	var stdout bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelError}))
	appCmd := &cli.Command{
		Name:     "app",
		Writer:   &stdout,
		Commands: []*cli.Command{Command()},
		Metadata: map[string]any{app.LoggerMetadataKey: logger},
	}

	err := appCmd.Run(
		context.Background(),
		[]string{"app", "snapshot", "--settings-path", settingsDir, "acme"},
	)
	must.NoError(err)

	// The rendered JSON result names the owner, source and written file.
	out := stdout.String()
	want.Contains(out, `"owner": "acme"`)
	want.Contains(out, `"comment_source": "org"`)
	want.Contains(out, filepath.Join(settingsDir, "repos", "widget.yml"))

	// The real override file is on disk with the expected settings-override shape.
	data, err := os.ReadFile(filepath.Join(settingsDir, "repos", "widget.yml"))
	must.NoError(err)
	body := string(data)
	want.Contains(body, "# acme/widget — overrides from org defaults")
	want.Contains(body, "repository:")
	want.Contains(body, "has_issues: true")
}
