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

// TestGitLabListRepositories_Group verifies branch 1: listing repositories
// within a group namespace lists every project in that group.
func TestGitLabListRepositories_Group(t *testing.T) {
	const groupPath = "my-group"

	p, _ := newGitLabTestServer(t, "test-token", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/namespaces/"+groupPath:
			writeJSON(t, w, map[string]any{
				"id":        2,
				"name":      "My Group",
				"path":      groupPath,
				"kind":      "group",
				"full_path": groupPath,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/groups/"+groupPath+"/projects":
			writeJSON(t, w, []map[string]any{
				{
					"id":                  10,
					"name":                "repo-one",
					"path":                "repo-one",
					"path_with_namespace": groupPath + "/repo-one",
					"default_branch":      "main",
					"visibility":          "private",
					"namespace": map[string]any{
						"full_path": groupPath,
					},
				},
				{
					"id":                  11,
					"name":                "repo-two",
					"path":                "repo-two",
					"path_with_namespace": groupPath + "/repo-two",
					"default_branch":      "develop",
					"visibility":          "public",
					"namespace": map[string]any{
						"full_path": groupPath,
					},
				},
			})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	got, err := p.ListRepositories(context.Background(), groupPath)
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("got %d repos, want 2: %+v", len(got), got)
	}

	if got[0].Name != "repo-one" || got[0].Namespace != groupPath || got[0].Visibility != VisibilityPrivate {
		t.Errorf("repo[0] = %+v, unexpected", got[0])
	}
	if got[1].Name != "repo-two" || got[1].DefaultBranch != "develop" || got[1].Visibility != VisibilityPublic {
		t.Errorf("repo[1] = %+v, unexpected", got[1])
	}
}

// TestGitLabListRepositories_OwnNamespace verifies branch 2: when the
// namespace matches the authenticated user's own username, private projects
// must be included (via Owned=true listing).
func TestGitLabListRepositories_OwnNamespace(t *testing.T) {
	const username = "tokenowner"

	p, _ := newGitLabTestServer(t, "test-token", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/namespaces/"+username:
			writeJSON(t, w, map[string]any{
				"id":        5,
				"name":      username,
				"path":      username,
				"kind":      "user",
				"full_path": username,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/user":
			writeJSON(t, w, map[string]any{
				"id":       5,
				"username": username,
				"name":     "Token Owner",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects":
			q := r.URL.Query()
			if q.Get("owned") != "true" {
				t.Errorf("expected owned=true query param, got %q", q.Get("owned"))
			}
			writeJSON(t, w, []map[string]any{
				{
					"id":                  20,
					"name":                "public-repo",
					"path":                "public-repo",
					"path_with_namespace": username + "/public-repo",
					"default_branch":      "main",
					"visibility":          "public",
					"namespace": map[string]any{
						"full_path": username,
					},
					"statistics": map[string]any{
						"repository_size": 2048,
					},
				},
				{
					"id":                  21,
					"name":                "private-repo",
					"path":                "private-repo",
					"path_with_namespace": username + "/private-repo",
					"default_branch":      "main",
					"visibility":          "private",
					"namespace": map[string]any{
						"full_path": username,
					},
					"statistics": map[string]any{
						"repository_size": 4096,
					},
				},
			})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	got, err := p.ListRepositories(context.Background(), username)
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("got %d repos, want 2: %+v", len(got), got)
	}

	var foundPrivate bool
	for _, repo := range got {
		if repo.Name == "private-repo" {
			foundPrivate = true
			if repo.Visibility != VisibilityPrivate {
				t.Errorf("private-repo visibility = %q, want %q", repo.Visibility, VisibilityPrivate)
			}
			if repo.SizeKB != 4 {
				t.Errorf("private-repo SizeKB = %d, want 4", repo.SizeKB)
			}
		}
	}
	if !foundPrivate {
		t.Errorf("expected private-repo to be included in own-namespace listing, got %+v", got)
	}
}

// TestGitLabListRepositories_OtherUserNamespace verifies branch 3 (the bug
// fix): listing repositories for ANOTHER user's personal namespace (not the
// token owner) must return that user's PUBLIC projects, not an empty slice.
func TestGitLabListRepositories_OtherUserNamespace(t *testing.T) {
	const tokenOwner = "tokenowner"
	const otherUser = "someotheruser"

	p, _ := newGitLabTestServer(t, "test-token", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/namespaces/"+otherUser:
			writeJSON(t, w, map[string]any{
				"id":        7,
				"name":      otherUser,
				"path":      otherUser,
				"kind":      "user",
				"full_path": otherUser,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/user":
			writeJSON(t, w, map[string]any{
				"id":       5,
				"username": tokenOwner,
				"name":     "Token Owner",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/users/"+otherUser+"/projects":
			writeJSON(t, w, []map[string]any{
				{
					"id":                  30,
					"name":                "other-public-repo",
					"path":                "other-public-repo",
					"path_with_namespace": otherUser + "/other-public-repo",
					"default_branch":      "main",
					"visibility":          "public",
					"namespace": map[string]any{
						"full_path": otherUser,
					},
					"statistics": map[string]any{
						"repository_size": 1024,
					},
				},
			})
		// Guard: the buggy v0.1.0 behavior listed the TOKEN OWNER's own
		// projects (owned=true on /projects) and filtered client-side. The
		// fixed implementation must NOT call this endpoint for another
		// user's namespace.
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects":
			t.Errorf("unexpected call to /projects (owned listing) when resolving another user's namespace")
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	got, err := p.ListRepositories(context.Background(), otherUser)
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("got %d repos, want 1 (bug fix regression): %+v", len(got), got)
	}

	if got[0].Name != "other-public-repo" {
		t.Errorf("repo[0].Name = %q, want %q", got[0].Name, "other-public-repo")
	}
	if got[0].Namespace != otherUser {
		t.Errorf("repo[0].Namespace = %q, want %q", got[0].Namespace, otherUser)
	}
	if got[0].Visibility != VisibilityPublic {
		t.Errorf("repo[0].Visibility = %q, want %q", got[0].Visibility, VisibilityPublic)
	}
	if got[0].SizeKB != 1 {
		t.Errorf("repo[0].SizeKB = %d, want 1", got[0].SizeKB)
	}
}
