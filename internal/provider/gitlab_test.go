package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGitLabName(t *testing.T) {
	p, err := NewGitLabProvider("my-token", "")
	if err != nil {
		t.Fatalf("NewGitLabProvider: %v", err)
	}

	if got := p.Name(); got != "gitlab" {
		t.Errorf("Name() = %q, want %q", got, "gitlab")
	}
}

func TestGitLabGetAuthenticatedCloneURL_DefaultHost(t *testing.T) {
	p, err := NewGitLabProvider("my-token", "")
	if err != nil {
		t.Fatalf("NewGitLabProvider: %v", err)
	}

	got := p.GetAuthenticatedCloneURL("my-group", "repo-one")
	want := "https://oauth2:my-token@gitlab.com/my-group/repo-one.git"

	if got != want {
		t.Errorf("GetAuthenticatedCloneURL() = %q, want %q", got, want)
	}
}

func TestGitLabGetAuthenticatedCloneURL_CustomHost(t *testing.T) {
	p, err := NewGitLabProvider("my-token", "https://gitlab.example.com")
	if err != nil {
		t.Fatalf("NewGitLabProvider: %v", err)
	}

	got := p.GetAuthenticatedCloneURL("my-group", "repo-one")
	want := "https://oauth2:my-token@gitlab.example.com/my-group/repo-one.git"

	if got != want {
		t.Errorf("GetAuthenticatedCloneURL() = %q, want %q", got, want)
	}
}

// newGitLabTestServer creates an httptest.Server backed by handler and
// returns a GitLabProvider configured to talk to it.
func newGitLabTestServer(t *testing.T, token string, handler http.HandlerFunc) (*GitLabProvider, *httptest.Server) {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	p, err := NewGitLabProvider(token, server.URL)
	if err != nil {
		t.Fatalf("NewGitLabProvider: %v", err)
	}

	return p, server
}

func TestGitLabListNamespaces(t *testing.T) {
	p, _ := newGitLabTestServer(t, "test-token", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/namespaces":
			writeJSON(t, w, []map[string]any{
				{
					"id":        1,
					"name":      "alice",
					"path":      "alice",
					"kind":      "user",
					"full_path": "alice",
				},
				{
					"id":        2,
					"name":      "My Group",
					"path":      "my-group",
					"kind":      "group",
					"full_path": "my-group",
				},
				{
					"id":        3,
					"name":      "Subgroup",
					"path":      "subgroup",
					"kind":      "group",
					"full_path": "my-group/subgroup",
				},
			})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	got, err := p.ListNamespaces(context.Background())
	if err != nil {
		t.Fatalf("ListNamespaces: %v", err)
	}

	want := []Namespace{
		{Slug: "alice", Name: "alice", Kind: NamespaceUser},
		{Slug: "my-group", Name: "My Group", Kind: NamespaceGroup},
		{Slug: "my-group/subgroup", Name: "Subgroup", Kind: NamespaceGroup},
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
