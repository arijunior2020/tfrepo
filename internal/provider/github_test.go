package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/go-github/v74/github"
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

// decodeGitHubRepo converts a map of raw GitHub API repository fields into a
// *github.Repository via JSON, for unit-testing toRepositorySummary directly
// without an HTTP round trip.
func decodeGitHubRepo(t *testing.T, fields map[string]any) *github.Repository {
	t.Helper()

	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("marshal repo fields: %v", err)
	}

	var repo github.Repository
	if err := json.Unmarshal(data, &repo); err != nil {
		t.Fatalf("unmarshal repo fields: %v", err)
	}
	return &repo
}

func TestGitHubListRepositories_Organization(t *testing.T) {
	p, _ := newGitHubTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/users/acme-corp":
			writeJSON(t, w, map[string]any{
				"login": "acme-corp",
				"type":  "Organization",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/orgs/acme-corp/repos":
			writeJSON(t, w, []map[string]any{
				{
					"name":           "widget-api",
					"owner":          map[string]any{"login": "acme-corp"},
					"default_branch": "main",
					"private":        false,
					"size":           1234,
				},
				{
					"name":           "widget-internal",
					"owner":          map[string]any{"login": "acme-corp"},
					"default_branch": "develop",
					"visibility":     "internal",
					"private":        true,
					"size":           5678,
				},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	got, err := p.ListRepositories(context.Background(), "acme-corp")
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}

	want := []RepositorySummary{
		{Name: "widget-api", Namespace: "acme-corp", DefaultBranch: "main", Visibility: VisibilityPublic, SizeKB: 1234},
		{Name: "widget-internal", Namespace: "acme-corp", DefaultBranch: "develop", Visibility: VisibilityInternal, SizeKB: 5678},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d repos, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("repo[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestGitHubListRepositories_OwnNamespace(t *testing.T) {
	p, _ := newGitHubTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/users/octocat":
			writeJSON(t, w, map[string]any{
				"login": "octocat",
				"type":  "User",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/user":
			writeJSON(t, w, map[string]any{
				"login": "octocat",
				"name":  "The Octocat",
				"type":  "User",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/user/repos":
			if got := r.URL.Query().Get("affiliation"); got != "owner" {
				t.Errorf("expected affiliation=owner query param, got %q", got)
			}
			writeJSON(t, w, []map[string]any{
				{
					"name":           "public-repo",
					"owner":          map[string]any{"login": "octocat"},
					"default_branch": "main",
					"private":        false,
					"size":           10,
				},
				{
					"name":           "secret-repo",
					"owner":          map[string]any{"login": "octocat"},
					"default_branch": "main",
					"private":        true,
					"size":           20,
				},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	got, err := p.ListRepositories(context.Background(), "octocat")
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}

	want := []RepositorySummary{
		{Name: "public-repo", Namespace: "octocat", DefaultBranch: "main", Visibility: VisibilityPublic, SizeKB: 10},
		{Name: "secret-repo", Namespace: "octocat", DefaultBranch: "main", Visibility: VisibilityPrivate, SizeKB: 20},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d repos, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("repo[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestGitHubListRepositories_OtherUserNamespace(t *testing.T) {
	p, _ := newGitHubTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/users/someone-else":
			writeJSON(t, w, map[string]any{
				"login": "someone-else",
				"type":  "User",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/user":
			writeJSON(t, w, map[string]any{
				"login": "octocat",
				"name":  "The Octocat",
				"type":  "User",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/users/someone-else/repos":
			writeJSON(t, w, []map[string]any{
				{
					"name":           "their-public-repo",
					"owner":          map[string]any{"login": "someone-else"},
					"default_branch": "main",
					"private":        false,
					"size":           42,
				},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	got, err := p.ListRepositories(context.Background(), "someone-else")
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}

	want := []RepositorySummary{
		{Name: "their-public-repo", Namespace: "someone-else", DefaultBranch: "main", Visibility: VisibilityPublic, SizeKB: 42},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d repos, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("repo[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestGitHubToRepositorySummary_VisibilityMapping(t *testing.T) {
	p, err := NewGitHubProvider("token", "")
	if err != nil {
		t.Fatalf("NewGitHubProvider: %v", err)
	}

	t.Run("internal visibility wins regardless of private flag", func(t *testing.T) {
		repo := decodeGitHubRepo(t, map[string]any{
			"name":       "internal-repo",
			"visibility": "internal",
			"private":    false,
			"size":       100,
		})

		got := p.toRepositorySummary(repo, "acme-corp")
		if got.Visibility != VisibilityInternal {
			t.Errorf("Visibility = %q, want %q", got.Visibility, VisibilityInternal)
		}
	})

	t.Run("falls back to private when visibility absent and private true", func(t *testing.T) {
		repo := decodeGitHubRepo(t, map[string]any{
			"name":    "secret-repo",
			"private": true,
			"size":    50,
		})

		got := p.toRepositorySummary(repo, "octocat")
		if got.Visibility != VisibilityPrivate {
			t.Errorf("Visibility = %q, want %q", got.Visibility, VisibilityPrivate)
		}
	})

	t.Run("falls back to public when visibility absent and private false", func(t *testing.T) {
		repo := decodeGitHubRepo(t, map[string]any{
			"name":    "open-repo",
			"private": false,
			"size":    50,
		})

		got := p.toRepositorySummary(repo, "octocat")
		if got.Visibility != VisibilityPublic {
			t.Errorf("Visibility = %q, want %q", got.Visibility, VisibilityPublic)
		}
	})

	t.Run("missing size defaults to zero", func(t *testing.T) {
		repo := decodeGitHubRepo(t, map[string]any{
			"name":    "no-size-repo",
			"private": false,
		})

		got := p.toRepositorySummary(repo, "octocat")
		if got.SizeKB != 0 {
			t.Errorf("SizeKB = %d, want 0", got.SizeKB)
		}
	})

	t.Run("uses owner login over namespace argument when present", func(t *testing.T) {
		repo := decodeGitHubRepo(t, map[string]any{
			"name":    "forked-repo",
			"owner":   map[string]any{"login": "real-owner"},
			"private": false,
		})

		got := p.toRepositorySummary(repo, "namespace-arg")
		if got.Namespace != "real-owner" {
			t.Errorf("Namespace = %q, want %q", got.Namespace, "real-owner")
		}
	})

	t.Run("falls back to namespace argument when owner absent", func(t *testing.T) {
		repo := decodeGitHubRepo(t, map[string]any{
			"name":    "no-owner-repo",
			"private": false,
		})

		got := p.toRepositorySummary(repo, "namespace-arg")
		if got.Namespace != "namespace-arg" {
			t.Errorf("Namespace = %q, want %q", got.Namespace, "namespace-arg")
		}
	})
}

func TestGitHubGetRepositoryDetails(t *testing.T) {
	p, _ := newGitHubTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme-corp/widget-api":
			writeJSON(t, w, map[string]any{
				"name":           "widget-api",
				"owner":          map[string]any{"login": "acme-corp"},
				"default_branch": "main",
				"private":        false,
				"size":           1234,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme-corp/widget-api/branches":
			writeJSON(t, w, []map[string]any{
				{"name": "main", "commit": map[string]any{"sha": "aaa111"}},
				{"name": "develop", "commit": map[string]any{"sha": "bbb222"}},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme-corp/widget-api/tags":
			writeJSON(t, w, []map[string]any{
				{"name": "v1.0.0", "commit": map[string]any{"sha": "ccc333"}},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	got, err := p.GetRepositoryDetails(context.Background(), "acme-corp", "widget-api")
	if err != nil {
		t.Fatalf("GetRepositoryDetails: %v", err)
	}

	if got.Name != "widget-api" || got.Namespace != "acme-corp" || got.DefaultBranch != "main" {
		t.Errorf("unexpected summary: %+v", got.RepositorySummary)
	}
	if got.Visibility != VisibilityPublic {
		t.Errorf("Visibility = %q, want %q", got.Visibility, VisibilityPublic)
	}
	if got.SizeKB != 1234 {
		t.Errorf("SizeKB = %d, want 1234", got.SizeKB)
	}

	wantBranches := []string{"main", "develop"}
	if len(got.Branches) != len(wantBranches) {
		t.Fatalf("Branches = %v, want %v", got.Branches, wantBranches)
	}
	for i := range wantBranches {
		if got.Branches[i] != wantBranches[i] {
			t.Errorf("Branches[%d] = %q, want %q", i, got.Branches[i], wantBranches[i])
		}
	}

	wantTags := []string{"v1.0.0"}
	if len(got.Tags) != len(wantTags) {
		t.Fatalf("Tags = %v, want %v", got.Tags, wantTags)
	}
	for i := range wantTags {
		if got.Tags[i] != wantTags[i] {
			t.Errorf("Tags[%d] = %q, want %q", i, got.Tags[i], wantTags[i])
		}
	}
}

func TestGitHubGetRepositoryState(t *testing.T) {
	p, _ := newGitHubTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme-corp/widget-api/branches":
			writeJSON(t, w, []map[string]any{
				{"name": "main", "commit": map[string]any{"sha": "aaa111"}},
				{"name": "develop", "commit": map[string]any{"sha": "bbb222"}},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme-corp/widget-api/tags":
			writeJSON(t, w, []map[string]any{
				{"name": "v1.0.0", "commit": map[string]any{"sha": "ccc333"}},
				{"name": "v1.1.0", "commit": map[string]any{"sha": "ddd444"}},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	got, err := p.GetRepositoryState(context.Background(), "acme-corp", "widget-api")
	if err != nil {
		t.Fatalf("GetRepositoryState: %v", err)
	}

	wantBranches := map[string]string{"main": "aaa111", "develop": "bbb222"}
	wantTags := map[string]string{"v1.0.0": "ccc333", "v1.1.0": "ddd444"}

	if len(got.Branches) != len(wantBranches) {
		t.Fatalf("Branches = %v, want %v", got.Branches, wantBranches)
	}
	for k, v := range wantBranches {
		if got.Branches[k] != v {
			t.Errorf("Branches[%q] = %q, want %q", k, got.Branches[k], v)
		}
	}

	if len(got.Tags) != len(wantTags) {
		t.Fatalf("Tags = %v, want %v", got.Tags, wantTags)
	}
	for k, v := range wantTags {
		if got.Tags[k] != v {
			t.Errorf("Tags[%q] = %q, want %q", k, got.Tags[k], v)
		}
	}
}

func TestGitHubCreateRepository_Organization(t *testing.T) {
	p, _ := newGitHubTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/users/acme-corp":
			writeJSON(t, w, map[string]any{
				"login": "acme-corp",
				"type":  "Organization",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/orgs/acme-corp/repos":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			if body["name"] != "new-repo" {
				t.Errorf("request name = %v, want %q", body["name"], "new-repo")
			}
			if body["description"] != "a new repo" {
				t.Errorf("request description = %v, want %q", body["description"], "a new repo")
			}
			if body["visibility"] != "private" {
				t.Errorf("request visibility = %v, want %q", body["visibility"], "private")
			}
			writeJSON(t, w, map[string]any{
				"name":           "new-repo",
				"owner":          map[string]any{"login": "acme-corp"},
				"default_branch": "main",
				"visibility":     "private",
				"private":        true,
				"size":           0,
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	got, err := p.CreateRepository(context.Background(), "acme-corp", CreateRepositoryInput{
		Name:        "new-repo",
		Visibility:  VisibilityPrivate,
		Description: "a new repo",
	})
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}

	want := RepositorySummary{
		Name:          "new-repo",
		Namespace:     "acme-corp",
		DefaultBranch: "main",
		Visibility:    VisibilityPrivate,
		SizeKB:        0,
	}
	if got != want {
		t.Errorf("CreateRepository = %+v, want %+v", got, want)
	}
}

func TestGitHubCreateRepository_OwnNamespace(t *testing.T) {
	p, _ := newGitHubTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/users/octocat":
			writeJSON(t, w, map[string]any{
				"login": "octocat",
				"type":  "User",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/user/repos":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			if body["name"] != "personal-repo" {
				t.Errorf("request name = %v, want %q", body["name"], "personal-repo")
			}
			if body["private"] != true {
				t.Errorf("request private = %v, want true", body["private"])
			}
			if _, ok := body["visibility"]; ok {
				t.Errorf("did not expect visibility field in personal repo create request, got %v", body["visibility"])
			}
			writeJSON(t, w, map[string]any{
				"name":           "personal-repo",
				"owner":          map[string]any{"login": "octocat"},
				"default_branch": "main",
				"private":        true,
				"size":           0,
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	got, err := p.CreateRepository(context.Background(), "octocat", CreateRepositoryInput{
		Name:        "personal-repo",
		Visibility:  VisibilityPrivate,
		Description: "",
	})
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}

	want := RepositorySummary{
		Name:          "personal-repo",
		Namespace:     "octocat",
		DefaultBranch: "main",
		Visibility:    VisibilityPrivate,
		SizeKB:        0,
	}
	if got != want {
		t.Errorf("CreateRepository = %+v, want %+v", got, want)
	}
}

func TestGitHubProviderListLabels(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/my-org/repo-a/labels", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"name":"bug","color":"d73a4a","description":"Something is wrong"},{"name":"enhancement","color":"a2eeef","description":""}]`)
	})
	p, _ := newGitHubTestProvider(t, mux.ServeHTTP)

	labels, err := p.ListLabels(context.Background(), "my-org", "repo-a")
	if err != nil {
		t.Fatalf("ListLabels: %v", err)
	}
	if len(labels) != 2 {
		t.Fatalf("len(labels) = %d, want 2", len(labels))
	}
	if labels[0].Name != "bug" || labels[0].Color != "d73a4a" || labels[0].Description != "Something is wrong" {
		t.Errorf("labels[0] = %+v", labels[0])
	}
	if labels[1].Name != "enhancement" || labels[1].Color != "a2eeef" {
		t.Errorf("labels[1] = %+v", labels[1])
	}
}

func TestGitHubProviderCreateLabel(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/my-org/repo-a/labels", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"name":"bug","color":"d73a4a","description":"Something is wrong"}`)
	})
	p, _ := newGitHubTestProvider(t, mux.ServeHTTP)

	got, err := p.CreateLabel(context.Background(), "my-org", "repo-a", Label{Name: "bug", Color: "d73a4a", Description: "Something is wrong"})
	if err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	if got.Name != "bug" || got.Color != "d73a4a" {
		t.Errorf("got %+v", got)
	}
}

func TestGitHubProviderListMilestones(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/my-org/repo-a/milestones", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != "all" {
			http.Error(w, "expected ?state=all", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"number":3,"title":"v1.0","description":"First stable release","state":"open","due_on":null}]`)
	})
	p, _ := newGitHubTestProvider(t, mux.ServeHTTP)

	milestones, err := p.ListMilestones(context.Background(), "my-org", "repo-a")
	if err != nil {
		t.Fatalf("ListMilestones: %v", err)
	}
	if len(milestones) != 1 {
		t.Fatalf("len = %d, want 1", len(milestones))
	}
	ms := milestones[0]
	if ms.ExternalID != 3 || ms.Title != "v1.0" || ms.Description != "First stable release" {
		t.Errorf("milestones[0] = %+v", ms)
	}
	if ms.State != MilestoneStateOpen {
		t.Errorf("State = %q, want open", ms.State)
	}
	if ms.DueDate != nil {
		t.Errorf("DueDate should be nil, got %v", ms.DueDate)
	}
}

func TestGitHubProviderCreateMilestone(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/my-org/repo-a/milestones", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"number":7,"title":"v1.0","description":"First stable release","state":"open","due_on":null}`)
	})
	p, _ := newGitHubTestProvider(t, mux.ServeHTTP)

	got, err := p.CreateMilestone(context.Background(), "my-org", "repo-a", Milestone{
		Title:       "v1.0",
		Description: "First stable release",
		State:       MilestoneStateOpen,
	})
	if err != nil {
		t.Fatalf("CreateMilestone: %v", err)
	}
	if got.ExternalID != 7 || got.Title != "v1.0" {
		t.Errorf("got %+v", got)
	}
}

func TestGitHubProviderListIssues(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/myorg/myrepo/issues", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != "all" {
			http.Error(w, "expected ?state=all", http.StatusBadRequest)
			return
		}
		writeJSON(t, w, []*github.Issue{
			{
				Number: github.Ptr(1),
				Title:  github.Ptr("Real issue"),
				Body:   github.Ptr("body text"),
				State:  github.Ptr("open"),
				Labels: []*github.Label{
					{Name: github.Ptr("bug")},
				},
			},
			{
				Number: github.Ptr(2),
				Title:  github.Ptr("A pull request"),
				Body:   github.Ptr("pr body"),
				State:  github.Ptr("open"),
				PullRequestLinks: &github.PullRequestLinks{
					URL: github.Ptr("https://api.github.com/repos/myorg/myrepo/pulls/2"),
				},
			},
		})
	})
	p, _ := newGitHubTestProvider(t, mux.ServeHTTP)

	issues, err := p.ListIssues(context.Background(), "myorg", "myrepo")
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("got %d issues, want 1 (PR should be filtered)", len(issues))
	}
	got := issues[0]
	if got.ExternalID != 1 {
		t.Errorf("ExternalID = %d, want 1", got.ExternalID)
	}
	if got.State != "open" {
		t.Errorf("State = %q, want %q", got.State, "open")
	}
	if len(got.Labels) != 1 || got.Labels[0] != "bug" {
		t.Errorf("Labels = %v, want [bug]", got.Labels)
	}
}

func TestGitHubProviderCreateIssue(t *testing.T) {
	editCalled := false

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/myorg/myrepo/issues", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		writeJSON(t, w, &github.Issue{
			Number: github.Ptr(10),
			Title:  github.Ptr("My issue"),
			Body:   github.Ptr(""),
			State:  github.Ptr("open"),
		})
	})
	mux.HandleFunc("/repos/myorg/myrepo/issues/10", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		editCalled = true
		w.Header().Set("Content-Type", "application/json")
		writeJSON(t, w, &github.Issue{
			Number: github.Ptr(10),
			Title:  github.Ptr("My issue"),
			Body:   github.Ptr(""),
			State:  github.Ptr("closed"),
		})
	})
	p, _ := newGitHubTestProvider(t, mux.ServeHTTP)

	got, err := p.CreateIssue(context.Background(), "myorg", "myrepo", Issue{
		Title: "My issue",
		Body:  "",
		State: "closed",
	})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if !editCalled {
		t.Error("expected Edit to be called to close the issue, but it was not")
	}
	if got.State != "closed" {
		t.Errorf("State = %q, want %q", got.State, "closed")
	}
	if got.ExternalID != 10 {
		t.Errorf("ExternalID = %d, want 10", got.ExternalID)
	}
}

func TestGitHubProviderListPullRequests(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/myorg/myrepo/pulls", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != "open" {
			t.Errorf("state = %q, want %q", r.URL.Query().Get("state"), "open")
		}
		writeJSON(t, w, []*github.PullRequest{
			{
				Number: github.Ptr(3),
				Title:  github.Ptr("Add feature"),
				Body:   github.Ptr("body text"),
				State:  github.Ptr("open"),
				Head:   &github.PullRequestBranch{Ref: github.Ptr("feature/add")},
				Base:   &github.PullRequestBranch{Ref: github.Ptr("main")},
			},
		})
	})

	p, _ := newGitHubTestProvider(t, mux.ServeHTTP)
	prs, err := p.ListPullRequests(t.Context(), "myorg", "myrepo")
	if err != nil {
		t.Fatalf("ListPullRequests: %v", err)
	}
	if len(prs) != 1 {
		t.Fatalf("len(prs) = %d, want 1", len(prs))
	}
	got := prs[0]
	if got.ExternalID != 3 {
		t.Errorf("ExternalID = %d, want 3", got.ExternalID)
	}
	if got.Title != "Add feature" {
		t.Errorf("Title = %q, want %q", got.Title, "Add feature")
	}
	if got.SourceBranch != "feature/add" {
		t.Errorf("SourceBranch = %q, want feature/add", got.SourceBranch)
	}
	if got.TargetBranch != "main" {
		t.Errorf("TargetBranch = %q, want main", got.TargetBranch)
	}
	if got.State != "open" {
		t.Errorf("State = %q, want open", got.State)
	}
}

func TestGitHubProviderCreatePullRequest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/myorg/myrepo/pulls", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, &github.PullRequest{
			Number: github.Ptr(10),
			Title:  github.Ptr("Created PR"),
			Body:   github.Ptr("body"),
			State:  github.Ptr("open"),
			Head:   &github.PullRequestBranch{Ref: github.Ptr("feature/new")},
			Base:   &github.PullRequestBranch{Ref: github.Ptr("main")},
		})
	})

	p, _ := newGitHubTestProvider(t, mux.ServeHTTP)
	result, err := p.CreatePullRequest(t.Context(), "myorg", "myrepo", PullRequest{
		Title:        "Created PR",
		Body:         "body",
		State:        "open",
		SourceBranch: "feature/new",
		TargetBranch: "main",
	})
	if err != nil {
		t.Fatalf("CreatePullRequest: %v", err)
	}
	if result.ExternalID != 10 {
		t.Errorf("ExternalID = %d, want 10", result.ExternalID)
	}
	if result.SourceBranch != "feature/new" {
		t.Errorf("SourceBranch = %q, want feature/new", result.SourceBranch)
	}
	if result.State != "open" {
		t.Errorf("State = %q, want open", result.State)
	}
}

func TestGitHubProviderCreateIssueOpen(t *testing.T) {
	editCalled := false

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/myorg/myrepo/issues", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		writeJSON(t, w, &github.Issue{
			Number: github.Ptr(11),
			Title:  github.Ptr("Open issue"),
			Body:   github.Ptr(""),
			State:  github.Ptr("open"),
		})
	})
	mux.HandleFunc("/repos/myorg/myrepo/issues/11", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		editCalled = true
		w.Header().Set("Content-Type", "application/json")
		writeJSON(t, w, &github.Issue{
			Number: github.Ptr(11),
			Title:  github.Ptr("Open issue"),
			Body:   github.Ptr(""),
			State:  github.Ptr("open"),
		})
	})
	p, _ := newGitHubTestProvider(t, mux.ServeHTTP)

	got, err := p.CreateIssue(context.Background(), "myorg", "myrepo", Issue{
		Title: "Open issue",
		Body:  "",
		State: "open",
	})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if editCalled {
		t.Error("expected Edit to not be called for open issues, but it was")
	}
	if got.State != "open" {
		t.Errorf("State = %q, want %q", got.State, "open")
	}
	if got.ExternalID != 11 {
		t.Errorf("ExternalID = %d, want 11", got.ExternalID)
	}
}
