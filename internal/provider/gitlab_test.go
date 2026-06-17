package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestGitLabGetRepositoryDetails(t *testing.T) {
	const namespace = "my-group"
	const repo = "repo-one"
	projectID := namespace + "/" + repo

	p, _ := newGitLabTestServer(t, "test-token", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/"+projectID:
			q := r.URL.Query()
			if q.Get("statistics") != "true" {
				t.Errorf("expected statistics=true query param, got %q", q.Get("statistics"))
			}
			writeJSON(t, w, map[string]any{
				"id":                  10,
				"name":                repo,
				"path":                repo,
				"path_with_namespace": projectID,
				"default_branch":      "main",
				"visibility":          "private",
				"namespace": map[string]any{
					"full_path": namespace,
				},
				"statistics": map[string]any{
					"repository_size": 1536, // -> 1.5 KB, rounds to 2
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/"+projectID+"/repository/branches":
			writeJSON(t, w, []map[string]any{
				{
					"name": "main",
					"commit": map[string]any{
						"id": "abc123",
					},
				},
				{
					"name": "develop",
					"commit": map[string]any{
						"id": "def456",
					},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/"+projectID+"/repository/tags":
			writeJSON(t, w, []map[string]any{
				{
					"name": "v1.0.0",
					"commit": map[string]any{
						"id": "tag111",
					},
				},
			})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	got, err := p.GetRepositoryDetails(context.Background(), namespace, repo)
	if err != nil {
		t.Fatalf("GetRepositoryDetails: %v", err)
	}

	if got.Name != repo {
		t.Errorf("Name = %q, want %q", got.Name, repo)
	}
	if got.Namespace != namespace {
		t.Errorf("Namespace = %q, want %q", got.Namespace, namespace)
	}
	if got.DefaultBranch != "main" {
		t.Errorf("DefaultBranch = %q, want %q", got.DefaultBranch, "main")
	}
	if got.Visibility != VisibilityPrivate {
		t.Errorf("Visibility = %q, want %q", got.Visibility, VisibilityPrivate)
	}
	if got.SizeKB != 2 {
		t.Errorf("SizeKB = %d, want 2 (round(1536/1024))", got.SizeKB)
	}
	if !equalStringSlices(got.Branches, []string{"main", "develop"}) {
		t.Errorf("Branches = %v, want [main develop]", got.Branches)
	}
	if !equalStringSlices(got.Tags, []string{"v1.0.0"}) {
		t.Errorf("Tags = %v, want [v1.0.0]", got.Tags)
	}
}

func TestGitLabGetRepositoryState(t *testing.T) {
	const namespace = "my-group"
	const repo = "repo-one"
	projectID := namespace + "/" + repo

	p, _ := newGitLabTestServer(t, "test-token", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/"+projectID+"/repository/branches":
			writeJSON(t, w, []map[string]any{
				{
					"name": "main",
					"commit": map[string]any{
						"id": "sha-main",
					},
				},
				{
					"name": "feature/x",
					"commit": map[string]any{
						"id": "sha-feature",
					},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/"+projectID+"/repository/tags":
			writeJSON(t, w, []map[string]any{
				{
					"name": "v1.0.0",
					"commit": map[string]any{
						"id": "sha-tag-1",
					},
				},
				{
					"name": "v2.0.0",
					"commit": map[string]any{
						"id": "sha-tag-2",
					},
				},
			})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	got, err := p.GetRepositoryState(context.Background(), namespace, repo)
	if err != nil {
		t.Fatalf("GetRepositoryState: %v", err)
	}

	wantBranches := map[string]string{
		"main":      "sha-main",
		"feature/x": "sha-feature",
	}
	wantTags := map[string]string{
		"v1.0.0": "sha-tag-1",
		"v2.0.0": "sha-tag-2",
	}

	if len(got.Branches) != len(wantBranches) {
		t.Fatalf("Branches = %v, want %v", got.Branches, wantBranches)
	}
	for name, sha := range wantBranches {
		if got.Branches[name] != sha {
			t.Errorf("Branches[%q] = %q, want %q", name, got.Branches[name], sha)
		}
	}

	if len(got.Tags) != len(wantTags) {
		t.Fatalf("Tags = %v, want %v", got.Tags, wantTags)
	}
	for name, sha := range wantTags {
		if got.Tags[name] != sha {
			t.Errorf("Tags[%q] = %q, want %q", name, got.Tags[name], sha)
		}
	}
}

func newGitLabTestProvider(t *testing.T, serverURL string) *GitLabProvider {
	t.Helper()
	p, err := NewGitLabProvider("test-token", serverURL)
	if err != nil {
		t.Fatalf("NewGitLabProvider: %v", err)
	}
	return p
}

func decodeJSONBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()

	var body map[string]any
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	return body
}

func TestGitLabCreateRepository(t *testing.T) {
	const namespace = "my-group"

	p, _ := newGitLabTestServer(t, "test-token", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/namespaces/"+namespace:
			writeJSON(t, w, map[string]any{
				"id":        2,
				"name":      "My Group",
				"path":      namespace,
				"kind":      "group",
				"full_path": namespace,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects":
			body := decodeJSONBody(t, r)

			if body["name"] != "new-repo" {
				t.Errorf("request name = %v, want %q", body["name"], "new-repo")
			}
			if body["path"] != "new-repo" {
				t.Errorf("request path = %v, want %q", body["path"], "new-repo")
			}
			if fmt.Sprintf("%v", body["namespace_id"]) != "2" {
				t.Errorf("request namespace_id = %v, want 2", body["namespace_id"])
			}
			if body["visibility"] != "private" {
				t.Errorf("request visibility = %v, want %q", body["visibility"], "private")
			}
			if body["description"] != "A new repo" {
				t.Errorf("request description = %v, want %q", body["description"], "A new repo")
			}

			writeJSON(t, w, map[string]any{
				"id":                  40,
				"name":                "new-repo",
				"path":                "new-repo",
				"path_with_namespace": namespace + "/new-repo",
				"default_branch":      "main",
				"visibility":          "private",
				"namespace": map[string]any{
					"full_path": namespace,
				},
				"statistics": map[string]any{
					"repository_size": 0,
				},
			})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	got, err := p.CreateRepository(context.Background(), namespace, CreateRepositoryInput{
		Name:        "new-repo",
		Visibility:  VisibilityPrivate,
		Description: "A new repo",
	})
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}

	if got.Name != "new-repo" {
		t.Errorf("Name = %q, want %q", got.Name, "new-repo")
	}
	if got.Namespace != namespace {
		t.Errorf("Namespace = %q, want %q", got.Namespace, namespace)
	}
	if got.Visibility != VisibilityPrivate {
		t.Errorf("Visibility = %q, want %q", got.Visibility, VisibilityPrivate)
	}
	if got.SizeKB != 0 {
		t.Errorf("SizeKB = %d, want 0", got.SizeKB)
	}
}

func TestGitLabProviderListLabels(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/my-group%2Frepo-a/labels", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"id":1,"name":"bug","color":"#d73a4a","description":"Something is wrong"},{"id":2,"name":"enhancement","color":"#a2eeef","description":""}]`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	p := newGitLabTestProvider(t, server.URL)

	labels, err := p.ListLabels(context.Background(), "my-group", "repo-a")
	if err != nil {
		t.Fatalf("ListLabels: %v", err)
	}
	if len(labels) != 2 {
		t.Fatalf("len = %d, want 2", len(labels))
	}
	if labels[0].Name != "bug" || labels[0].Color != "d73a4a" {
		t.Errorf("labels[0] = %+v, want Color without #", labels[0])
	}
	if labels[1].Name != "enhancement" || labels[1].Color != "a2eeef" {
		t.Errorf("labels[1] = %+v", labels[1])
	}
}

func TestGitLabProviderCreateLabel(t *testing.T) {
	var receivedBody []byte
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/my-group%2Frepo-a/labels", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var err error
		receivedBody, err = io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"id":1,"name":"bug","color":"#d73a4a","description":"Something is wrong"}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	p := newGitLabTestProvider(t, server.URL)

	got, err := p.CreateLabel(context.Background(), "my-group", "repo-a", Label{Name: "bug", Color: "d73a4a", Description: "Something is wrong"})
	if err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	if got.Name != "bug" || got.Color != "d73a4a" {
		t.Errorf("got %+v", got)
	}
	if !strings.Contains(string(receivedBody), "#d73a4a") {
		t.Errorf("request body %q should contain #d73a4a", receivedBody)
	}
}

func TestGitLabProviderListMilestones(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/my-group%2Frepo-a/milestones", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"id":10,"title":"v1.0","description":"First stable release","state":"active","due_date":null}]`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	p := newGitLabTestProvider(t, server.URL)

	milestones, err := p.ListMilestones(context.Background(), "my-group", "repo-a")
	if err != nil {
		t.Fatalf("ListMilestones: %v", err)
	}
	if len(milestones) != 1 {
		t.Fatalf("len = %d, want 1", len(milestones))
	}
	ms := milestones[0]
	if ms.ExternalID != 10 || ms.Title != "v1.0" || ms.State != MilestoneStateOpen {
		t.Errorf("milestones[0] = %+v", ms)
	}
	if ms.DueDate != nil {
		t.Errorf("DueDate should be nil, got %v", ms.DueDate)
	}
}

func TestGitLabProviderCreateMilestone(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/my-group%2Frepo-a/milestones", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"id":10,"title":"v1.0","description":"First stable release","state":"active","due_date":null}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	p := newGitLabTestProvider(t, server.URL)

	got, err := p.CreateMilestone(context.Background(), "my-group", "repo-a", Milestone{
		Title:       "v1.0",
		Description: "First stable release",
		State:       MilestoneStateOpen,
	})
	if err != nil {
		t.Fatalf("CreateMilestone: %v", err)
	}
	if got.ExternalID != 10 || got.Title != "v1.0" {
		t.Errorf("got %+v", got)
	}
}
