package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestGitHubName(t *testing.T) {
	p, err := NewGitHubProvider("test-token", "")
	if err != nil {
		t.Fatalf("NewGitHubProvider: %v", err)
	}

	if got := p.Name(); got != "github" {
		t.Errorf("Name() = %q, want %q", got, "github")
	}
}

func TestGitHubGetAuthenticatedCloneURL(t *testing.T) {
	t.Run("default github.com", func(t *testing.T) {
		p, err := NewGitHubProvider("my-token", "")
		if err != nil {
			t.Fatalf("NewGitHubProvider: %v", err)
		}

		got := p.GetAuthenticatedCloneURL("acme-corp", "widget-api")
		want := "https://x-access-token:my-token@github.com/acme-corp/widget-api.git"
		if got != want {
			t.Errorf("GetAuthenticatedCloneURL() = %q, want %q", got, want)
		}
	})

	t.Run("custom enterprise base url", func(t *testing.T) {
		p, err := NewGitHubProvider("my-token", "https://github.example.com/api/v3/")
		if err != nil {
			t.Fatalf("NewGitHubProvider: %v", err)
		}

		got := p.GetAuthenticatedCloneURL("acme-corp", "widget-api")
		want := "https://x-access-token:my-token@github.example.com/acme-corp/widget-api.git"
		if got != want {
			t.Errorf("GetAuthenticatedCloneURL() = %q, want %q", got, want)
		}
	})
}

// newGitHubTestProvider creates a GitHubProvider whose client talks to the
// given httptest server instead of the real GitHub API.
func newGitHubTestProvider(t *testing.T, handler http.HandlerFunc) (*GitHubProvider, *httptest.Server) {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	p, err := NewGitHubProvider("test-token", "")
	if err != nil {
		t.Fatalf("NewGitHubProvider: %v", err)
	}

	// Point the underlying client at the test server. go-github requires
	// BaseURL to have a trailing slash.
	baseURL, err := url.Parse(server.URL + "/")
	if err != nil {
		t.Fatalf("parse test server url: %v", err)
	}
	p.client.BaseURL = baseURL

	return p, server
}

// writeJSON encodes v as the JSON response body. Shared by GitHub and GitLab
// httptest handlers.
func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("encode json: %v", err)
	}
}

func TestGitHubListNamespaces(t *testing.T) {
	p, _ := newGitHubTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/user":
			writeJSON(t, w, map[string]any{
				"login": "octocat",
				"name":  "The Octocat",
				"type":  "User",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/user/orgs":
			writeJSON(t, w, []map[string]any{
				{"login": "acme-corp"},
				{"login": "widgets-inc"},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	got, err := p.ListNamespaces(context.Background())
	if err != nil {
		t.Fatalf("ListNamespaces: %v", err)
	}

	want := []Namespace{
		{Slug: "octocat", Name: "The Octocat", Kind: NamespaceUser},
		{Slug: "acme-corp", Name: "acme-corp", Kind: NamespaceOrganization},
		{Slug: "widgets-inc", Name: "widgets-inc", Kind: NamespaceOrganization},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d namespaces, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("namespace[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}
