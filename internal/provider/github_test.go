package provider

import (
	"context"
	"encoding/json"
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
