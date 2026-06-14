# Internal Provider Layer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `internal/provider` — provider-neutral repository types, the `RepositoryProvider` interface, and `GitHubProvider`/`GitLabProvider` implementations backed by `google/go-github` and `gitlab.com/gitlab-org/api/client-go`, including a fix for a namespace-resolution bug present in the Node v0.1.0 `GitLabProvider`.

**Architecture:** `types.go` defines provider-neutral data types (`Namespace`, `RepositorySummary`, `RepositoryDetails`, `RepositoryState`, `CreateRepositoryInput`) and the `RepositoryProvider` interface that `internal/core` (Plan 3) and `internal/cli` (Plan 4) will depend on. `github.go` and `gitlab.go` each implement that interface against their respective REST APIs via official Go client libraries, normalizing each platform's data into the shared types. Tests use `httptest.NewServer` to simulate each platform's REST API — no mocked client libraries — per the design spec.

**Tech Stack:** Go (module `go` directive bumped from 1.23.6 to 1.24.0 in Task 7 — required by the GitLab client), `github.com/google/go-github/v74` v74.0.0, `gitlab.com/gitlab-org/api/client-go` v1.46.0, stdlib `net/http/httptest` for tests.

---

## Notes for the Implementer

These apply across all tasks — read once before starting:

1. **Shared package, shared test namespace.** `github.go`/`github_test.go` and `gitlab.go`/`gitlab_test.go` all live in `package provider`. Top-level test function and helper names must be unique across *both* files. This plan uses `TestGitHub*`/`TestGitLab*` prefixes for every test function, and `newGitHubTestProvider`/`newGitLabTestServer` for the two server-setup helpers. The `writeJSON` helper is shared — it is defined **once**, in Task 3, and reused (not redefined) from Task 8 onward.
2. **Append-only file edits.** Every task that modifies `github.go`, `github_test.go`, `gitlab.go`, or `gitlab_test.go` appends new top-level declarations to the end of the file (Go doesn't care about declaration order). When a task changes the *import block*, the step shows the full new import block to replace the existing one at the top of the file.
3. **"Run test to verify it fails" steps** will almost always fail with a **compile error** (e.g. `p.ListNamespaces undefined (type *GitHubProvider has no field or method ListNamespaces)`), not a runtime assertion failure. That's expected — it's the TDD red step proving the method didn't exist yet.
4. **Go 1.24 toolchain bump (Task 7).** `gitlab.com/gitlab-org/api/client-go` requires `go >= 1.24.0`. Running `go get` for it will automatically bump the `go` directive in `go.mod` to `1.24.0` and may trigger Go's automatic toolchain download (`GOTOOLCHAIN=auto`, the default) if the locally installed toolchain is older. This is expected and was verified during research — no manual intervention needed beyond running the commands.
5. **Interface satisfaction checks** (`var _ RepositoryProvider = (*GitHubProvider)(nil)` and the GitLab equivalent) are added in the *last* task for each provider (Task 6 and Task 11), once every interface method exists.

---

## File Structure

```
internal/provider/
├── types.go        — provider-neutral types + RepositoryProvider interface (Task 1)
├── types_test.go   — JSON encoding tests for the shared types (Task 1)
├── github.go       — GitHubProvider (Tasks 2-6)
├── github_test.go  — GitHubProvider tests, httptest-based (Tasks 2-6)
├── gitlab.go       — GitLabProvider, including the namespace-resolution fix (Tasks 7-11)
└── gitlab_test.go  — GitLabProvider tests, httptest-based (Tasks 7-11)
```

`go.mod` gains `github.com/google/go-github/v74` (Task 2) and `gitlab.com/gitlab-org/api/client-go` (Task 7, which also bumps the `go` directive to 1.24.0). `.github/workflows/ci.yml` bumps `go-version` to `'1.24'` (Task 7).

---

### Task 1: Provider-neutral types and `RepositoryProvider` interface

**Files:**
- Create: `internal/provider/types.go`
- Test: `internal/provider/types_test.go`

- [ ] **Step 1: Write the failing test**

```go
package provider

import (
	"encoding/json"
	"testing"
)

func TestRepositorySummaryJSON(t *testing.T) {
	summary := RepositorySummary{
		Name:          "widget-api",
		Namespace:     "acme-corp",
		DefaultBranch: "main",
		Visibility:    VisibilityPrivate,
		SizeKB:        1234,
	}

	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	want := `{"name":"widget-api","namespace":"acme-corp","defaultBranch":"main","visibility":"private","sizeKb":1234}`
	if string(data) != want {
		t.Errorf("Marshal() = %s, want %s", data, want)
	}
}

func TestRepositoryDetailsJSON(t *testing.T) {
	details := RepositoryDetails{
		RepositorySummary: RepositorySummary{
			Name:          "widget-api",
			Namespace:     "acme-corp",
			DefaultBranch: "main",
			Visibility:    VisibilityPublic,
			SizeKB:        10,
		},
		Branches: []string{"main", "develop"},
		Tags:     []string{"v1.0.0"},
	}

	data, err := json.Marshal(details)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	want := `{"name":"widget-api","namespace":"acme-corp","defaultBranch":"main","visibility":"public","sizeKb":10,"branches":["main","develop"],"tags":["v1.0.0"]}`
	if string(data) != want {
		t.Errorf("Marshal() = %s, want %s", data, want)
	}
}
```

This is the first file in `internal/provider/`, so creating it also creates the directory and the `provider` package.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/provider/... -v`

Expected: FAIL with compile errors like:
```
./types_test.go:9:13: undefined: RepositorySummary
./types_test.go:14:30: undefined: VisibilityPrivate
./types_test.go:30:13: undefined: RepositoryDetails
...
```

- [ ] **Step 3: Write the implementation**

```go
package provider

import "context"

// NamespaceKind identifies what kind of account a Namespace represents.
type NamespaceKind string

const (
	NamespaceOrganization NamespaceKind = "organization"
	NamespaceUser         NamespaceKind = "user"
	NamespaceGroup        NamespaceKind = "group"
)

// Visibility is the access level of a repository.
type Visibility string

const (
	VisibilityPublic   Visibility = "public"
	VisibilityPrivate  Visibility = "private"
	VisibilityInternal Visibility = "internal"
)

// Namespace is an account or group that can own repositories.
type Namespace struct {
	Slug string        `json:"slug"` // org login (GitHub) or group path (GitLab)
	Name string        `json:"name"`
	Kind NamespaceKind `json:"kind"`
}

// RepositorySummary is the provider-neutral view of a repository used when
// listing repositories.
type RepositorySummary struct {
	Name          string     `json:"name"`
	Namespace     string     `json:"namespace"`
	DefaultBranch string     `json:"defaultBranch"`
	Visibility    Visibility `json:"visibility"`
	SizeKB        int64      `json:"sizeKb"`
}

// RepositoryDetails extends RepositorySummary with the full list of branch
// and tag names.
type RepositoryDetails struct {
	RepositorySummary
	Branches []string `json:"branches"`
	Tags     []string `json:"tags"`
}

// RepositoryState maps branch and tag names to their current commit SHAs.
type RepositoryState struct {
	Branches map[string]string `json:"branches"` // branch -> commit SHA
	Tags     map[string]string `json:"tags"`     // tag -> commit SHA
}

// CreateRepositoryInput describes a repository to create on the destination
// provider.
type CreateRepositoryInput struct {
	Name        string
	Visibility  Visibility
	Description string
}

// RepositoryProvider is the interface implemented by GitHubProvider and
// GitLabProvider.
type RepositoryProvider interface {
	Name() string // "github" | "gitlab"

	// Scan
	ListNamespaces(ctx context.Context) ([]Namespace, error)
	ListRepositories(ctx context.Context, namespace string) ([]RepositorySummary, error)
	GetRepositoryDetails(ctx context.Context, namespace, repo string) (RepositoryDetails, error)

	// Migrate (destination side)
	CreateRepository(ctx context.Context, namespace string, input CreateRepositoryInput) (RepositorySummary, error)

	// Git transport — token-embedded URL, used only in memory, never logged
	GetAuthenticatedCloneURL(namespace, repo string) string

	// Validate
	GetRepositoryState(ctx context.Context, namespace, repo string) (RepositoryState, error)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/provider/... -v && go vet ./...`

Expected: `TestRepositorySummaryJSON` and `TestRepositoryDetailsJSON` PASS, `go vet` clean.

- [ ] **Step 5: Commit**

```bash
git add internal/provider/types.go internal/provider/types_test.go
git commit -m "feat(provider): add provider-neutral types and RepositoryProvider interface"
```

---

### Task 2: GitHubProvider skeleton (constructor, Name, GetAuthenticatedCloneURL)

**Files:**
- Modify: `go.mod` (add `github.com/google/go-github/v74`)
- Create: `internal/provider/github.go`
- Test: `internal/provider/github_test.go`

- [ ] **Step 1: Add the go-github dependency**

```bash
go get github.com/google/go-github/v74@v74.0.0
go mod tidy
```

Expected: `go.mod` gains `require github.com/google/go-github/v74 v74.0.0` and an indirect `require github.com/google/go-querystring ...`. The `go` directive stays `1.23.6` (go-github v74 requires `go >= 1.23.0`).

- [ ] **Step 2: Write the failing test**

```go
package provider

import "testing"

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
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/provider/... -v -run TestGitHub`

Expected: FAIL — `undefined: NewGitHubProvider`.

- [ ] **Step 4: Write the implementation**

```go
package provider

import (
	"fmt"
	"net/url"

	"github.com/google/go-github/v74/github"
)

const (
	defaultGitHubBaseURL = "https://api.github.com/"
	defaultGitHubHost    = "github.com"
)

// GitHubProvider implements RepositoryProvider for GitHub (github.com and
// GitHub Enterprise Server).
type GitHubProvider struct {
	client  *github.Client
	token   string
	baseURL string
}

// NewGitHubProvider constructs a GitHubProvider authenticated with token.
//
// If baseURL is empty, the provider talks to the public GitHub API
// (https://api.github.com/). If baseURL is non-empty, it is used verbatim as
// the API base URL (e.g. for GitHub Enterprise Server, something like
// "https://github.example.com/api/v3/").
func NewGitHubProvider(token, baseURL string) (*GitHubProvider, error) {
	client := github.NewClient(nil).WithAuthToken(token)

	if baseURL != "" {
		parsed, err := url.Parse(baseURL)
		if err != nil {
			return nil, fmt.Errorf("parse github base url %q: %w", baseURL, err)
		}
		if parsed.Path == "" || parsed.Path[len(parsed.Path)-1] != '/' {
			parsed.Path += "/"
		}
		client.BaseURL = parsed
	}

	effectiveBaseURL := baseURL
	if effectiveBaseURL == "" {
		effectiveBaseURL = defaultGitHubBaseURL
	}

	return &GitHubProvider{
		client:  client,
		token:   token,
		baseURL: effectiveBaseURL,
	}, nil
}

// Name returns the provider identifier.
func (p *GitHubProvider) Name() string {
	return "github"
}

// GetAuthenticatedCloneURL returns an HTTPS clone URL with the provider
// token embedded for non-interactive authentication.
func (p *GitHubProvider) GetAuthenticatedCloneURL(namespace, repo string) string {
	return fmt.Sprintf("https://x-access-token:%s@%s/%s/%s.git", p.token, p.gitHost(), namespace, repo)
}

// gitHost returns the git hostname used to build authenticated clone URLs:
// github.com for the default API base URL, or the host parsed from a custom
// (GitHub Enterprise) base URL.
func (p *GitHubProvider) gitHost() string {
	if p.baseURL == defaultGitHubBaseURL {
		return defaultGitHubHost
	}
	if parsed, err := url.Parse(p.baseURL); err == nil && parsed.Host != "" {
		return parsed.Host
	}
	return defaultGitHubHost
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/provider/... -v -run TestGitHub && go vet ./...`

Expected: `TestGitHubName` and `TestGitHubGetAuthenticatedCloneURL` (both subtests) PASS.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/provider/github.go internal/provider/github_test.go
git commit -m "feat(provider): add GitHubProvider skeleton with clone URL support"
```

---

### Task 3: `GitHubProvider.ListNamespaces`

**Files:**
- Modify: `internal/provider/github.go`
- Modify: `internal/provider/github_test.go`

- [ ] **Step 1: Write the failing test**

Replace the import block at the top of `internal/provider/github_test.go` with:

```go
import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)
```

Then append the following to the end of `internal/provider/github_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/provider/... -v -run TestGitHubListNamespaces`

Expected: FAIL — `p.ListNamespaces undefined (type *GitHubProvider has no field or method ListNamespaces)`.

- [ ] **Step 3: Write the implementation**

Replace the import block at the top of `internal/provider/github.go` with:

```go
import (
	"context"
	"fmt"
	"net/url"

	"github.com/google/go-github/v74/github"
)
```

Then append the following to the end of `internal/provider/github.go`:

```go
// ListNamespaces returns the authenticated user's namespace plus every
// organization the authenticated user belongs to.
func (p *GitHubProvider) ListNamespaces(ctx context.Context) ([]Namespace, error) {
	user, _, err := p.client.Users.Get(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("get authenticated user: %w", err)
	}

	namespaces := make([]Namespace, 0, 1)

	name := user.GetName()
	if name == "" {
		name = user.GetLogin()
	}
	namespaces = append(namespaces, Namespace{
		Slug: user.GetLogin(),
		Name: name,
		Kind: NamespaceUser,
	})

	opts := &github.ListOptions{PerPage: 100}
	for {
		orgs, resp, err := p.client.Organizations.List(ctx, "", opts)
		if err != nil {
			return nil, fmt.Errorf("list organizations for authenticated user: %w", err)
		}
		for _, org := range orgs {
			namespaces = append(namespaces, Namespace{
				Slug: org.GetLogin(),
				Name: org.GetLogin(),
				Kind: NamespaceOrganization,
			})
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return namespaces, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/provider/... -v -run TestGitHub && go vet ./...`

Expected: all `TestGitHub*` tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/provider/github.go internal/provider/github_test.go
git commit -m "feat(provider): implement GitHubProvider.ListNamespaces"
```

---

### Task 4: `GitHubProvider.ListRepositories` (3-branch namespace resolution)

**Files:**
- Modify: `internal/provider/github.go`
- Modify: `internal/provider/github_test.go`

This is the GitHub side of the "organization / own user / other user" resolution that the GitLab provider (Task 9) mirrors.

- [ ] **Step 1: Write the failing tests**

Replace the import block at the top of `internal/provider/github_test.go` with:

```go
import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/go-github/v74/github"
)
```

Then append the following to the end of `internal/provider/github_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/provider/... -v -run TestGitHubListRepositories`

Expected: FAIL — `p.ListRepositories undefined (type *GitHubProvider has no field or method ListRepositories)`.

- [ ] **Step 3: Write the implementation**

Append the following to the end of `internal/provider/github.go` (import block unchanged from Task 3):

```go
// ListRepositories lists the repositories visible to the authenticated user
// within the given namespace.
func (p *GitHubProvider) ListRepositories(ctx context.Context, namespace string) ([]RepositorySummary, error) {
	repos, err := p.listReposForNamespace(ctx, namespace)
	if err != nil {
		return nil, err
	}

	summaries := make([]RepositorySummary, 0, len(repos))
	for _, repo := range repos {
		summaries = append(summaries, p.toRepositorySummary(repo, namespace))
	}
	return summaries, nil
}

// listReposForNamespace resolves which listing to use based on the
// namespace's account type: organization namespaces use the org repository
// listing, the authenticated user's own namespace uses the "owner
// affiliation" listing (which includes private repositories), and any other
// namespace falls back to the public-repositories-only listing for that
// user.
func (p *GitHubProvider) listReposForNamespace(ctx context.Context, namespace string) ([]*github.Repository, error) {
	account, _, err := p.client.Users.Get(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("get account %s: %w", namespace, err)
	}

	if account.GetType() == "Organization" {
		var repos []*github.Repository
		opts := &github.RepositoryListByOrgOptions{
			ListOptions: github.ListOptions{PerPage: 100},
		}
		for {
			page, resp, err := p.client.Repositories.ListByOrg(ctx, namespace, opts)
			if err != nil {
				return nil, fmt.Errorf("list repositories for org %s: %w", namespace, err)
			}
			repos = append(repos, page...)
			if resp.NextPage == 0 {
				break
			}
			opts.Page = resp.NextPage
		}
		return repos, nil
	}

	// GET /users/{username}/repos only returns PUBLIC repos, even for the
	// token owner. To include private repos for the token owner's own
	// namespace, use GET /user/repos.
	authenticatedUser, _, err := p.client.Users.Get(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("get authenticated user: %w", err)
	}

	if authenticatedUser.GetLogin() == namespace {
		var repos []*github.Repository
		opts := &github.RepositoryListByAuthenticatedUserOptions{
			Affiliation: "owner",
			ListOptions: github.ListOptions{PerPage: 100},
		}
		for {
			page, resp, err := p.client.Repositories.ListByAuthenticatedUser(ctx, opts)
			if err != nil {
				return nil, fmt.Errorf("list repositories for authenticated user: %w", err)
			}
			repos = append(repos, page...)
			if resp.NextPage == 0 {
				break
			}
			opts.Page = resp.NextPage
		}
		return repos, nil
	}

	var repos []*github.Repository
	opts := &github.RepositoryListByUserOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}
	for {
		page, resp, err := p.client.Repositories.ListByUser(ctx, namespace, opts)
		if err != nil {
			return nil, fmt.Errorf("list repositories for user %s: %w", namespace, err)
		}
		repos = append(repos, page...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return repos, nil
}

// toRepositorySummary maps a go-github Repository onto the provider-neutral
// RepositorySummary.
func (p *GitHubProvider) toRepositorySummary(repo *github.Repository, namespace string) RepositorySummary {
	ns := namespace
	if owner := repo.GetOwner(); owner != nil && owner.GetLogin() != "" {
		ns = owner.GetLogin()
	}

	var visibility Visibility
	switch {
	case repo.GetVisibility() == "internal":
		visibility = VisibilityInternal
	case repo.GetPrivate():
		visibility = VisibilityPrivate
	default:
		visibility = VisibilityPublic
	}

	return RepositorySummary{
		Name:          repo.GetName(),
		Namespace:     ns,
		DefaultBranch: repo.GetDefaultBranch(),
		Visibility:    visibility,
		SizeKB:        int64(repo.GetSize()),
	}
}
```

> **Note:** `Repository.Visibility` is documented by go-github as only being set on Create/Edit requests, but it unmarshals correctly from `GET`/`List` JSON responses too — that comment describes go-github's *request*-building behavior, not JSON decoding. The `internal`/`private`/`public` mapping above works for both.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/provider/... -v -run TestGitHub && go vet ./...`

Expected: all `TestGitHub*` tests PASS, including the three `ListRepositories` branches and the visibility-mapping subtests.

- [ ] **Step 5: Commit**

```bash
git add internal/provider/github.go internal/provider/github_test.go
git commit -m "feat(provider): implement GitHubProvider.ListRepositories"
```

---

### Task 5: `GitHubProvider.GetRepositoryDetails` and `GetRepositoryState`

**Files:**
- Modify: `internal/provider/github.go`
- Modify: `internal/provider/github_test.go`

- [ ] **Step 1: Write the failing tests**

Append the following to the end of `internal/provider/github_test.go` (import block unchanged from Task 4):

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/provider/... -v -run TestGitHubGetRepository`

Expected: FAIL — `p.GetRepositoryDetails undefined (type *GitHubProvider has no field or method GetRepositoryDetails)`.

- [ ] **Step 3: Write the implementation**

Append the following to the end of `internal/provider/github.go` (import block unchanged):

```go
// GetRepositoryDetails returns repository metadata together with the full
// list of branch and tag names.
func (p *GitHubProvider) GetRepositoryDetails(ctx context.Context, namespace, repo string) (RepositoryDetails, error) {
	data, _, err := p.client.Repositories.Get(ctx, namespace, repo)
	if err != nil {
		return RepositoryDetails{}, fmt.Errorf("get repository %s/%s: %w", namespace, repo, err)
	}

	branches, err := p.listAllBranches(ctx, namespace, repo)
	if err != nil {
		return RepositoryDetails{}, err
	}

	tags, err := p.listAllTags(ctx, namespace, repo)
	if err != nil {
		return RepositoryDetails{}, err
	}

	branchNames := make([]string, 0, len(branches))
	for _, branch := range branches {
		branchNames = append(branchNames, branch.GetName())
	}

	tagNames := make([]string, 0, len(tags))
	for _, tag := range tags {
		tagNames = append(tagNames, tag.GetName())
	}

	return RepositoryDetails{
		RepositorySummary: p.toRepositorySummary(data, namespace),
		Branches:          branchNames,
		Tags:              tagNames,
	}, nil
}

// GetRepositoryState returns every branch and tag mapped to its current
// commit SHA.
func (p *GitHubProvider) GetRepositoryState(ctx context.Context, namespace, repo string) (RepositoryState, error) {
	branches, err := p.listAllBranches(ctx, namespace, repo)
	if err != nil {
		return RepositoryState{}, err
	}

	tags, err := p.listAllTags(ctx, namespace, repo)
	if err != nil {
		return RepositoryState{}, err
	}

	branchMap := make(map[string]string, len(branches))
	for _, branch := range branches {
		branchMap[branch.GetName()] = branch.GetCommit().GetSHA()
	}

	tagMap := make(map[string]string, len(tags))
	for _, tag := range tags {
		tagMap[tag.GetName()] = tag.GetCommit().GetSHA()
	}

	return RepositoryState{
		Branches: branchMap,
		Tags:     tagMap,
	}, nil
}

// listAllBranches collects every branch across all pages.
func (p *GitHubProvider) listAllBranches(ctx context.Context, namespace, repo string) ([]*github.Branch, error) {
	var branches []*github.Branch
	opts := &github.BranchListOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}
	for {
		page, resp, err := p.client.Repositories.ListBranches(ctx, namespace, repo, opts)
		if err != nil {
			return nil, fmt.Errorf("list branches for %s/%s: %w", namespace, repo, err)
		}
		branches = append(branches, page...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return branches, nil
}

// listAllTags collects every tag across all pages.
func (p *GitHubProvider) listAllTags(ctx context.Context, namespace, repo string) ([]*github.RepositoryTag, error) {
	var tags []*github.RepositoryTag
	opts := &github.ListOptions{PerPage: 100}
	for {
		page, resp, err := p.client.Repositories.ListTags(ctx, namespace, repo, opts)
		if err != nil {
			return nil, fmt.Errorf("list tags for %s/%s: %w", namespace, repo, err)
		}
		tags = append(tags, page...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return tags, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/provider/... -v -run TestGitHub && go vet ./...`

Expected: all `TestGitHub*` tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/provider/github.go internal/provider/github_test.go
git commit -m "feat(provider): implement GitHubProvider repository details and state"
```

---

### Task 6: `GitHubProvider.CreateRepository`

**Files:**
- Modify: `internal/provider/github.go`
- Modify: `internal/provider/github_test.go`

- [ ] **Step 1: Write the failing tests**

Append the following to the end of `internal/provider/github_test.go` (import block unchanged):

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/provider/... -v -run TestGitHubCreateRepository`

Expected: FAIL — `p.CreateRepository undefined (type *GitHubProvider has no field or method CreateRepository)`.

- [ ] **Step 3: Write the implementation**

Append the following to the end of `internal/provider/github.go` (import block unchanged):

```go
// CreateRepository creates a new repository within namespace. If namespace
// resolves to a GitHub organization, the repository is created in that
// organization; otherwise it is created for the authenticated user.
func (p *GitHubProvider) CreateRepository(ctx context.Context, namespace string, input CreateRepositoryInput) (RepositorySummary, error) {
	account, _, err := p.client.Users.Get(ctx, namespace)
	if err != nil {
		return RepositorySummary{}, fmt.Errorf("get account %s: %w", namespace, err)
	}

	repo := &github.Repository{
		Name:        github.Ptr(input.Name),
		Description: github.Ptr(input.Description),
	}

	org := ""
	if account.GetType() == "Organization" {
		org = namespace
		repo.Visibility = github.Ptr(string(input.Visibility))
	} else {
		repo.Private = github.Ptr(input.Visibility != VisibilityPublic)
	}

	data, _, err := p.client.Repositories.Create(ctx, org, repo)
	if err != nil {
		return RepositorySummary{}, fmt.Errorf("create repository %s/%s: %w", namespace, input.Name, err)
	}

	return p.toRepositorySummary(data, namespace), nil
}

// Compile-time check that GitHubProvider implements RepositoryProvider.
var _ RepositoryProvider = (*GitHubProvider)(nil)
```

> **Note:** v74 has a single unified `RepositoriesService.Create(ctx, org string, repo *Repository)` — if `org == ""` it POSTs to `/user/repos` (authenticated user); if `org != ""` it POSTs to `/orgs/{org}/repos`. This one call replaces the TS version's separate `createInOrg`/`createForAuthenticatedUser` — the org/personal branching is just about which `org` string and which `Repository` fields (`Visibility` vs `Private`) to set. Personal GitHub accounts can't be "internal", hence `Private` rather than `Visibility` in that branch.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/provider/... -v -run TestGitHub && go vet ./...`

Expected: all `TestGitHub*` tests PASS. The `var _ RepositoryProvider = (*GitHubProvider)(nil)` line confirms `*GitHubProvider` now satisfies the full interface.

- [ ] **Step 5: Commit**

```bash
git add internal/provider/github.go internal/provider/github_test.go
git commit -m "feat(provider): implement GitHubProvider.CreateRepository"
```

---

### Task 7: GitLabProvider skeleton (constructor, Name, GetAuthenticatedCloneURL) + go.mod/CI bump

**Files:**
- Modify: `go.mod` (add `gitlab.com/gitlab-org/api/client-go`, bumps `go` directive to 1.24.0)
- Modify: `.github/workflows/ci.yml` (bump `go-version` to `'1.24'`)
- Create: `internal/provider/gitlab.go`
- Test: `internal/provider/gitlab_test.go`

- [ ] **Step 1: Add the GitLab client dependency**

```bash
go get gitlab.com/gitlab-org/api/client-go@v1.46.0
go mod tidy
```

Expected: `go.mod` gains `require gitlab.com/gitlab-org/api/client-go v1.46.0` plus indirect requires (`github.com/google/go-querystring`, `github.com/hashicorp/go-cleanhttp`, `github.com/hashicorp/go-retryablehttp`, `golang.org/x/oauth2`, `golang.org/x/time`). The `go` directive is automatically bumped from `1.23.6` to `1.24.0` (the minimum required by this dependency) — Go's toolchain manager downloads a newer toolchain automatically if needed.

Verify with:

```bash
head -5 go.mod
```

Expected: `go 1.24.0` on the third line.

- [ ] **Step 2: Update CI to match the new Go version requirement**

In `.github/workflows/ci.yml`, change:

```yaml
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
```

to:

```yaml
      - uses: actions/setup-go@v5
        with:
          go-version: '1.24'
```

- [ ] **Step 3: Write the failing test**

```go
package provider

import "testing"

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
```

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./internal/provider/... -v -run TestGitLab`

Expected: FAIL — `undefined: NewGitLabProvider`.

- [ ] **Step 5: Write the implementation**

```go
package provider

import (
	"fmt"
	"net/url"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

const gitlabDefaultBaseURL = "https://gitlab.com"

// listPerPage is the page size used for all paginated GitLab API requests.
const listPerPage = 100

// GitLabProvider implements RepositoryProvider against the GitLab REST API
// using gitlab.com/gitlab-org/api/client-go.
type GitLabProvider struct {
	client  *gitlab.Client
	token   string
	baseURL string
}

// NewGitLabProvider creates a GitLabProvider authenticated with token.
// If baseURL is empty, it defaults to https://gitlab.com.
func NewGitLabProvider(token, baseURL string) (*GitLabProvider, error) {
	if baseURL == "" {
		baseURL = gitlabDefaultBaseURL
	}

	client, err := gitlab.NewClient(token, gitlab.WithBaseURL(baseURL))
	if err != nil {
		return nil, fmt.Errorf("create gitlab client: %w", err)
	}

	return &GitLabProvider{
		client:  client,
		token:   token,
		baseURL: baseURL,
	}, nil
}

// Name returns the provider's identifier.
func (p *GitLabProvider) Name() string {
	return "gitlab"
}

// GetAuthenticatedCloneURL returns an HTTPS clone URL that embeds the
// provider token for authentication.
func (p *GitLabProvider) GetAuthenticatedCloneURL(namespace, repo string) string {
	host := "gitlab.com"
	if u, err := url.Parse(p.baseURL); err == nil && u.Host != "" {
		host = u.Host
	}

	return fmt.Sprintf("https://oauth2:%s@%s/%s/%s.git", p.token, host, namespace, repo)
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/provider/... -v -run TestGitLab && go vet ./...`

Expected: `TestGitLabName`, `TestGitLabGetAuthenticatedCloneURL_DefaultHost`, `TestGitLabGetAuthenticatedCloneURL_CustomHost` PASS.

- [ ] **Step 7: Run the full test suite to confirm the Go version bump didn't break anything**

Run: `go build ./... && go test ./... -race`

Expected: all packages (`internal/security`, `internal/provider`) build and pass under the new `go 1.24.0` directive.

- [ ] **Step 8: Commit**

```bash
git add go.mod go.sum .github/workflows/ci.yml internal/provider/gitlab.go internal/provider/gitlab_test.go
git commit -m "feat(provider): add GitLabProvider skeleton with clone URL support"
```

---

### Task 8: `GitLabProvider.ListNamespaces`

**Files:**
- Modify: `internal/provider/gitlab.go`
- Modify: `internal/provider/gitlab_test.go`

- [ ] **Step 1: Write the failing test**

Replace the import block at the top of `internal/provider/gitlab_test.go` with:

```go
import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)
```

Then append the following to the end of `internal/provider/gitlab_test.go`:

```go
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
```

`writeJSON` is the helper defined in `github_test.go` (Task 3) — it is **not** redefined here, since both files share `package provider`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/provider/... -v -run TestGitLabListNamespaces`

Expected: FAIL — `p.ListNamespaces undefined (type *GitLabProvider has no field or method ListNamespaces)`.

- [ ] **Step 3: Write the implementation**

Replace the import block at the top of `internal/provider/gitlab.go` with:

```go
import (
	"context"
	"fmt"
	"net/url"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)
```

Then append the following to the end of `internal/provider/gitlab.go`:

```go
// ListNamespaces returns every namespace (user or group, including
// subgroups) visible to the authenticated token.
func (p *GitLabProvider) ListNamespaces(ctx context.Context) ([]Namespace, error) {
	var result []Namespace

	opts := &gitlab.ListNamespacesOptions{
		ListOptions: gitlab.ListOptions{PerPage: listPerPage},
	}

	for {
		namespaces, resp, err := p.client.Namespaces.ListNamespaces(opts, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list namespaces: %w", err)
		}

		for _, ns := range namespaces {
			result = append(result, Namespace{
				Slug: ns.FullPath,
				Name: ns.Name,
				Kind: namespaceKindFromString(ns.Kind),
			})
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return result, nil
}

// namespaceKindFromString maps GitLab's namespace "kind" string to the
// shared NamespaceKind type.
func namespaceKindFromString(kind string) NamespaceKind {
	switch kind {
	case "group":
		return NamespaceGroup
	case "user":
		return NamespaceUser
	default:
		return NamespaceKind(kind)
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/provider/... -v -run TestGitLab && go vet ./...`

Expected: all `TestGitLab*` tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/provider/gitlab.go internal/provider/gitlab_test.go
git commit -m "feat(provider): implement GitLabProvider.ListNamespaces"
```

---

### Task 9: `GitLabProvider.ListRepositories` — the namespace-resolution fix

**Files:**
- Modify: `internal/provider/gitlab.go`
- Modify: `internal/provider/gitlab_test.go`

This is the core deliverable of Plan 2. The Node v0.1.0 `GitLabProvider.listProjectsForNamespace` resolved "another user's personal namespace" by listing the token owner's own projects (`owned: true`) and filtering by `namespace.full_path === namespace` — which silently returns `[]` for any namespace that isn't the token owner's own. This task fixes that with a 3-branch resolution that mirrors `GitHubProvider.ListRepositories` (Task 4):

1. `Kind == "group"` → list every project in that group/subgroup the token can see (`Groups.ListGroupProjects`).
2. `Kind == "user"`, namespace == token owner's own username → list the token owner's own projects, including private (`Projects.ListProjects` with `Owned: true`).
3. `Kind == "user"`, namespace != token owner → list that user's publicly visible projects (`Projects.ListUserProjects`, which accepts a username string directly — **no separate username→ID lookup needed**, simplifying the design spec's original sketch).

- [ ] **Step 1: Write the failing tests**

Append the following to the end of `internal/provider/gitlab_test.go` (import block unchanged from Task 8):

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/provider/... -v -run TestGitLabListRepositories`

Expected: FAIL — `p.ListRepositories undefined (type *GitLabProvider has no field or method ListRepositories)`.

- [ ] **Step 3: Write the implementation**

Replace the import block at the top of `internal/provider/gitlab.go` with:

```go
import (
	"context"
	"fmt"
	"math"
	"net/url"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)
```

Then append the following to the end of `internal/provider/gitlab.go`:

```go
// ListRepositories lists the repositories visible to the token within the
// given namespace.
//
// This fixes a bug present in the v0.1.0 Node implementation: that version
// resolved "another user's personal namespace" by listing the token owner's
// own projects and filtering by namespace, which silently returned an empty
// slice for any namespace that wasn't the token owner's own. This
// implementation instead branches on the namespace kind:
//
//  1. "group"       -> list all projects within that group/subgroup that the
//     token can see (Groups.ListGroupProjects).
//  2. "user", self  -> list the authenticated user's own projects, including
//     private ones (Projects.ListProjects with Owned=true).
//  3. "user", other -> list that user's publicly visible projects
//     (Projects.ListUserProjects).
func (p *GitLabProvider) ListRepositories(ctx context.Context, namespace string) ([]RepositorySummary, error) {
	projects, err := p.listProjectsForNamespace(ctx, namespace)
	if err != nil {
		return nil, err
	}

	summaries := make([]RepositorySummary, 0, len(projects))
	for _, project := range projects {
		summaries = append(summaries, toRepositorySummary(project))
	}
	return summaries, nil
}

// listProjectsForNamespace implements the 3-branch namespace resolution
// described in ListRepositories.
func (p *GitLabProvider) listProjectsForNamespace(ctx context.Context, namespace string) ([]*gitlab.Project, error) {
	ns, _, err := p.client.Namespaces.GetNamespace(namespace, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("get namespace %q: %w", namespace, err)
	}

	switch ns.Kind {
	case "group":
		return p.listAllGroupProjects(ctx, namespace)
	case "user":
		currentUser, _, err := p.client.Users.CurrentUser(gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("get current user: %w", err)
		}

		if currentUser.Username == namespace {
			// Branch 2: the token owner's own personal namespace. List
			// their own projects, including private ones.
			return p.listOwnProjects(ctx)
		}

		// Branch 3: another user's personal namespace. List that user's
		// publicly visible projects.
		return p.listUserProjects(ctx, namespace)
	default:
		return nil, fmt.Errorf("namespace %q has unsupported kind %q", namespace, ns.Kind)
	}
}

// listAllGroupProjects lists every project within the given group/subgroup
// namespace that the authenticated token can see.
func (p *GitLabProvider) listAllGroupProjects(ctx context.Context, namespace string) ([]*gitlab.Project, error) {
	var result []*gitlab.Project

	opts := &gitlab.ListGroupProjectsOptions{
		ListOptions: gitlab.ListOptions{PerPage: listPerPage},
	}

	for {
		projects, resp, err := p.client.Groups.ListGroupProjects(namespace, opts, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list group projects for %q: %w", namespace, err)
		}

		result = append(result, projects...)

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return result, nil
}

// listOwnProjects lists all projects owned by the authenticated user,
// including private ones.
func (p *GitLabProvider) listOwnProjects(ctx context.Context) ([]*gitlab.Project, error) {
	var result []*gitlab.Project

	opts := &gitlab.ListProjectsOptions{
		ListOptions: gitlab.ListOptions{PerPage: listPerPage},
		Owned:       gitlab.Ptr(true),
		Statistics:  gitlab.Ptr(true),
	}

	for {
		projects, resp, err := p.client.Projects.ListProjects(opts, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list own projects: %w", err)
		}

		result = append(result, projects...)

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return result, nil
}

// listUserProjects lists the publicly visible projects belonging to the
// given username's personal namespace.
func (p *GitLabProvider) listUserProjects(ctx context.Context, username string) ([]*gitlab.Project, error) {
	var result []*gitlab.Project

	opts := &gitlab.ListProjectsOptions{
		ListOptions: gitlab.ListOptions{PerPage: listPerPage},
		Statistics:  gitlab.Ptr(true),
	}

	for {
		projects, resp, err := p.client.Projects.ListUserProjects(username, opts, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list projects for user %q: %w", username, err)
		}

		result = append(result, projects...)

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return result, nil
}

// toRepositorySummary maps a GitLab project to a RepositorySummary.
func toRepositorySummary(project *gitlab.Project) RepositorySummary {
	var sizeKB int64
	if project.Statistics != nil {
		sizeKB = int64(math.Round(float64(project.Statistics.RepositorySize) / 1024))
	}

	namespace := ""
	if project.Namespace != nil {
		namespace = project.Namespace.FullPath
	}

	return RepositorySummary{
		Name:          project.Path,
		Namespace:     namespace,
		DefaultBranch: project.DefaultBranch,
		Visibility:    Visibility(project.Visibility),
		SizeKB:        sizeKB,
	}
}
```

> **Note on `ListUserProjects`:** client-go's `Projects.ListUserProjects(uid any, ...)` accepts a username string directly and builds `GET /users/<username>/projects` — there's no need to resolve the username to a numeric user ID first, simplifying the design spec's original sketch.
>
> **Note on `ListGroupProjectsOptions`:** unlike `ListProjectsOptions`, it has no `Statistics` field — `/groups/:id/projects` doesn't support a `statistics` query param upstream, so group-listed projects won't have `Statistics` populated (handled by the `nil` check in `toRepositorySummary`).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/provider/... -v -run TestGitLab && go vet ./...`

Expected: all `TestGitLab*` tests PASS, including all three `ListRepositories` branches. `TestGitLabListRepositories_OtherUserNamespace` must return exactly 1 repo (not 0) — this is the regression test for the fix.

- [ ] **Step 5: Commit**

```bash
git add internal/provider/gitlab.go internal/provider/gitlab_test.go
git commit -m "fix(provider): implement GitLabProvider.ListRepositories with namespace resolution fix"
```

---

### Task 10: `GitLabProvider.GetRepositoryDetails` and `GetRepositoryState`

**Files:**
- Modify: `internal/provider/gitlab.go`
- Modify: `internal/provider/gitlab_test.go`

- [ ] **Step 1: Write the failing tests**

Append the following to the end of `internal/provider/gitlab_test.go` (import block unchanged from Task 9):

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/provider/... -v -run TestGitLabGetRepository`

Expected: FAIL — `p.GetRepositoryDetails undefined (type *GitLabProvider has no field or method GetRepositoryDetails)`.

- [ ] **Step 3: Write the implementation**

Append the following to the end of `internal/provider/gitlab.go` (import block unchanged):

```go
// GetRepositoryDetails returns the repository summary plus its branch and
// tag names.
func (p *GitLabProvider) GetRepositoryDetails(ctx context.Context, namespace, repo string) (RepositoryDetails, error) {
	projectID := namespace + "/" + repo

	project, _, err := p.client.Projects.GetProject(projectID, &gitlab.GetProjectOptions{
		Statistics: gitlab.Ptr(true),
	}, gitlab.WithContext(ctx))
	if err != nil {
		return RepositoryDetails{}, fmt.Errorf("get project %q: %w", projectID, err)
	}

	branchNames, err := p.listAllBranchNames(ctx, projectID)
	if err != nil {
		return RepositoryDetails{}, err
	}

	tagNames, err := p.listAllTagNames(ctx, projectID)
	if err != nil {
		return RepositoryDetails{}, err
	}

	return RepositoryDetails{
		RepositorySummary: toRepositorySummary(project),
		Branches:          branchNames,
		Tags:              tagNames,
	}, nil
}

// GetRepositoryState returns a map of branch and tag names to their current
// commit SHAs.
func (p *GitLabProvider) GetRepositoryState(ctx context.Context, namespace, repo string) (RepositoryState, error) {
	projectID := namespace + "/" + repo

	branches, err := p.listAllBranches(ctx, projectID)
	if err != nil {
		return RepositoryState{}, err
	}

	tags, err := p.listAllTags(ctx, projectID)
	if err != nil {
		return RepositoryState{}, err
	}

	branchSHAs := make(map[string]string, len(branches))
	for _, branch := range branches {
		if branch.Commit != nil {
			branchSHAs[branch.Name] = branch.Commit.ID
		}
	}

	tagSHAs := make(map[string]string, len(tags))
	for _, tag := range tags {
		if tag.Commit != nil {
			tagSHAs[tag.Name] = tag.Commit.ID
		}
	}

	return RepositoryState{
		Branches: branchSHAs,
		Tags:     tagSHAs,
	}, nil
}

// listAllBranches returns every branch for the given project, paginated.
func (p *GitLabProvider) listAllBranches(ctx context.Context, projectID string) ([]*gitlab.Branch, error) {
	var result []*gitlab.Branch

	opts := &gitlab.ListBranchesOptions{
		ListOptions: gitlab.ListOptions{PerPage: listPerPage},
	}

	for {
		branches, resp, err := p.client.Branches.ListBranches(projectID, opts, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list branches for %q: %w", projectID, err)
		}

		result = append(result, branches...)

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return result, nil
}

// listAllTags returns every tag for the given project, paginated.
func (p *GitLabProvider) listAllTags(ctx context.Context, projectID string) ([]*gitlab.Tag, error) {
	var result []*gitlab.Tag

	opts := &gitlab.ListTagsOptions{
		ListOptions: gitlab.ListOptions{PerPage: listPerPage},
	}

	for {
		tags, resp, err := p.client.Tags.ListTags(projectID, opts, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list tags for %q: %w", projectID, err)
		}

		result = append(result, tags...)

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return result, nil
}

// listAllBranchNames returns only the names of every branch for the given
// project.
func (p *GitLabProvider) listAllBranchNames(ctx context.Context, projectID string) ([]string, error) {
	branches, err := p.listAllBranches(ctx, projectID)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(branches))
	for _, branch := range branches {
		names = append(names, branch.Name)
	}
	return names, nil
}

// listAllTagNames returns only the names of every tag for the given project.
func (p *GitLabProvider) listAllTagNames(ctx context.Context, projectID string) ([]string, error) {
	tags, err := p.listAllTags(ctx, projectID)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(tags))
	for _, tag := range tags {
		names = append(names, tag.Name)
	}
	return names, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/provider/... -v -run TestGitLab && go vet ./...`

Expected: all `TestGitLab*` tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/provider/gitlab.go internal/provider/gitlab_test.go
git commit -m "feat(provider): implement GitLabProvider repository details and state"
```

---

### Task 11: `GitLabProvider.CreateRepository`

**Files:**
- Modify: `internal/provider/gitlab.go`
- Modify: `internal/provider/gitlab_test.go`

- [ ] **Step 1: Write the failing test**

Replace the import block at the top of `internal/provider/gitlab_test.go` with:

```go
import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)
```

Then append the following to the end of `internal/provider/gitlab_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/provider/... -v -run TestGitLabCreateRepository`

Expected: FAIL — `p.CreateRepository undefined (type *GitLabProvider has no field or method CreateRepository)`.

- [ ] **Step 3: Write the implementation**

Append the following to the end of `internal/provider/gitlab.go` (import block unchanged):

```go
// CreateRepository creates a new project within the given namespace.
func (p *GitLabProvider) CreateRepository(ctx context.Context, namespace string, input CreateRepositoryInput) (RepositorySummary, error) {
	ns, _, err := p.client.Namespaces.GetNamespace(namespace, gitlab.WithContext(ctx))
	if err != nil {
		return RepositorySummary{}, fmt.Errorf("get namespace %q: %w", namespace, err)
	}

	project, _, err := p.client.Projects.CreateProject(&gitlab.CreateProjectOptions{
		Name:        gitlab.Ptr(input.Name),
		Path:        gitlab.Ptr(input.Name),
		NamespaceID: gitlab.Ptr(ns.ID),
		Visibility:  gitlab.Ptr(gitlab.VisibilityValue(input.Visibility)),
		Description: gitlab.Ptr(input.Description),
	}, gitlab.WithContext(ctx))
	if err != nil {
		return RepositorySummary{}, fmt.Errorf("create project %q in namespace %q: %w", input.Name, namespace, err)
	}

	return toRepositorySummary(project), nil
}

// Compile-time check that GitLabProvider implements RepositoryProvider.
var _ RepositoryProvider = (*GitLabProvider)(nil)
```

> **Note:** `input.Visibility` (type `Visibility`, underlying `string`) converts directly to `gitlab.VisibilityValue` (also underlying `string`) via `gitlab.VisibilityValue(input.Visibility)` — Go allows direct conversion between named types sharing an underlying type.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/provider/... -v && go vet ./...`

Expected: every test in `internal/provider` PASSES. The `var _ RepositoryProvider = (*GitLabProvider)(nil)` line confirms `*GitLabProvider` now satisfies the full interface.

- [ ] **Step 5: Run the full repo test suite**

Run: `go build ./... && go test ./... -race`

Expected: `internal/security` and `internal/provider` packages all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/provider/gitlab.go internal/provider/gitlab_test.go
git commit -m "feat(provider): implement GitLabProvider.CreateRepository"
```

---

## After All Tasks

Once all 11 tasks are complete and committed, use **superpowers:finishing-a-development-branch** to verify the full suite (`go build ./... && go vet ./... && go test ./... -race`) and present completion options (this repo works directly on `main`, so "merge" isn't applicable — push to `origin/main` once the user confirms).
