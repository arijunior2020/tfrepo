# tfrepo migrate-labels: Labels + Milestones Migration (Plan 8)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `tfrepo migrate-labels` — mirrors labels and milestones from every source repository to its target counterpart (per `migration-plan.json`), writing `labels-report.json` with a per-repository `milestoneIdMap` (source ExternalID → target ExternalID) required by Plan 9 (issue migration).

**Architecture:** 4 new methods added to `RepositoryProvider` interface (`ListLabels`, `CreateLabel`, `ListMilestones`, `CreateMilestone`) with concrete implementations in both GitHub and GitLab providers. Core function `MigrateLabelsAndMilestones` in `internal/core/labels.go` applies skip-if-exists (idempotent), collects per-repo errors without aborting, and builds `MilestoneIDMap`. CLI layer follows the `migrate.go` pattern exactly. `Label.Color` is normalized to hex-without-`#`; providers add/strip `#` internally.

**Tech Stack:** Go 1.24 · `github.com/google/go-github/v74` · `gitlab.com/gitlab-org/api/client-go v1.46.0` · Cobra v1.10.2 · `strconv` (milestone ID map keys) · `encoding/json` (artifacts)

---

## File Structure

**Create:**
- `internal/provider/labels.go` — `Label`, `Milestone`, `MilestoneState` types
- `internal/provider/labels_test.go` — JSON round-trip tests for new types
- `internal/core/labels.go` — `MigrateLabelsAndMilestones`, `LabelsMigrateProviders`, helpers
- `internal/core/labels_test.go` — core logic tests with controllable fake providers
- `internal/cli/migrate_labels.go` — `newMigrateLabelsCommand`, `runMigrateLabels`, `labelsReportPath`
- `internal/cli/migrate_labels_test.go` — CLI tests

**Modify:**
- `internal/provider/types.go` — add 4 new methods to `RepositoryProvider` interface
- `internal/provider/github.go` — implement the 4 new methods
- `internal/provider/github_test.go` — add tests for the 4 new GitHub methods
- `internal/provider/gitlab.go` — implement the 4 new methods
- `internal/provider/gitlab_test.go` — add tests for the 4 new GitLab methods
- `internal/core/migrate_test.go` — add 4 stub methods to `fakeMigrateProvider`
- `internal/core/artifacts.go` — add `LabelsMigrateResult`, `LabelsReport` types
- `internal/cli/destroy.go` — add `labelsReportPath` to `migrationArtifactPaths`
- `internal/cli/destroy_test.go` — update hardcoded `"Removidos 4 arquivos."` → `"Removidos 5 arquivos."`
- `internal/cli/root.go` — `root.AddCommand(newMigrateLabelsCommand(exitCode))`
- `README.md` — document `tfrepo migrate-labels`, update pipeline and Artefatos table

---

## Critical Context

### compile-time check in `migrate_test.go`

`internal/core/migrate_test.go` line 69 has:
```go
var _ provider.RepositoryProvider = (*fakeMigrateProvider)(nil)
```
Adding new methods to `RepositoryProvider` in Task 1 **will break compilation** of the `core` package tests until stubs are added to `fakeMigrateProvider`. Task 1 adds both the interface change and the stubs **in a single commit**. Never split them.

### Color normalization

- `Label.Color` stores hex **without** `#` (e.g. `"e11d48"`).
- GitHub API returns and accepts colors WITHOUT `#` — no transformation needed.
- GitLab API returns and accepts colors WITH `#` — strip `#` on read, prepend `#` on write.

### ExternalID semantics

- **GitHub** milestone `ExternalID` = milestone **number** (`m.GetNumber()` → `int`, cast to `int64`).
- **GitLab** milestone `ExternalID` = milestone **ID** (`m.ID` → `int`, cast to `int64`).

### MilestoneIDMap

- Type: `map[string]int64` (string keys for JSON spec compliance; JSON objects require string keys).
- Key: `strconv.FormatInt(sourceMilestone.ExternalID, 10)`.
- Value: target `ExternalID` assigned after `CreateMilestone` (or already-existing target ExternalID for skipped milestones — they must ALSO appear in the map).

### GitLab milestone state

GitLab's `CreateMilestoneOptions` has no `State` field — milestones are always created as "active". To close one, call `UpdateMilestone` afterwards with `StateEvent: gitlab.Ptr("close")`. Verify the exact field name via:
```bash
grep -r "StateEvent" $(go env GOPATH)/pkg/mod/gitlab.com/gitlab-org/api/client-go@v1.46.0/milestones.go
```

### destroy_test.go count

`TestRunDestroyRemovesMigrationArtifacts` (line ~39) currently checks:
```go
if !strings.Contains(stdout.String(), "Removidos 4 arquivos.") {
```
After Task 5 adds `labelsReportPath` to `migrationArtifactPaths`, the count becomes 5. Update the test in the same commit.

### GitHub pagination pattern (from existing `github.go`)

```go
opts := &github.ListOptions{PerPage: 100}
for {
    page, resp, err := p.client.SomeAPI.ListSomething(ctx, owner, repo, opts)
    if err != nil { return nil, fmt.Errorf("...") }
    items = append(items, page...)
    if resp.NextPage == 0 { break }
    opts.Page = resp.NextPage
}
```

### GitLab pagination pattern (from existing `gitlab.go`)

```go
// listPerPage = 100 is already a package-level const in gitlab.go
opts := &gitlab.ListXxxOptions{ListOptions: gitlab.ListOptions{PerPage: listPerPage}}
for {
    items, resp, err := p.client.Xxx.ListXxx(pid, opts, gitlab.WithContext(ctx))
    if err != nil { return nil, fmt.Errorf("...") }
    result = append(result, items...)
    if resp.NextPage == 0 { break }
    opts.Page = resp.NextPage
}
```

---

## Task 1: Types, Interface Extension, Mock Stubs

**Files:**
- Create: `internal/provider/labels.go`
- Create: `internal/provider/labels_test.go`
- Modify: `internal/provider/types.go`
- Modify: `internal/core/migrate_test.go`

- [ ] **Step 1: Write the failing tests for the new types**

Create `internal/provider/labels_test.go`:

```go
package provider

import (
	"encoding/json"
	"testing"
	"time"
)

func TestLabelJSONRoundTrip(t *testing.T) {
	original := Label{
		Name:        "bug",
		Description: "Something is wrong",
		Color:       "d73a4a",
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got Label
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got != original {
		t.Errorf("got %+v, want %+v", got, original)
	}
}

func TestMilestoneJSONRoundTrip(t *testing.T) {
	dueDate := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	original := Milestone{
		ExternalID:  42,
		Title:       "v1.0",
		Description: "First stable release",
		State:       MilestoneStateOpen,
		DueDate:     &dueDate,
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got Milestone
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.ExternalID != original.ExternalID ||
		got.Title != original.Title ||
		got.Description != original.Description ||
		got.State != original.State ||
		got.DueDate == nil || !got.DueDate.Equal(*original.DueDate) {
		t.Errorf("got %+v, want %+v", got, original)
	}
}

func TestMilestoneNilDueDateRoundTrip(t *testing.T) {
	original := Milestone{
		ExternalID: 1,
		Title:      "Sprint 1",
		State:      MilestoneStateClosed,
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got Milestone
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.DueDate != nil {
		t.Errorf("DueDate = %v, want nil", got.DueDate)
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail (types undefined)**

```bash
go test ./internal/provider/... -run "TestLabel|TestMilestone" -v
```
Expected: compile error — `Label`, `Milestone`, `MilestoneState` are not defined.

- [ ] **Step 3: Create `internal/provider/labels.go`**

```go
package provider

import "time"

// MilestoneState indicates whether a milestone is open or closed.
type MilestoneState string

const (
	MilestoneStateOpen   MilestoneState = "open"
	MilestoneStateClosed MilestoneState = "closed"
)

// Label is a provider-neutral repository label.
// Color is a hex string WITHOUT the '#' prefix (e.g. "e11d48").
// Providers that require '#' (GitLab) add/strip it internally.
type Label struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
}

// Milestone is a provider-neutral repository milestone.
// ExternalID is the provider-assigned integer used to reference this
// milestone in issues: the milestone number on GitHub, the milestone ID
// on GitLab.
type Milestone struct {
	ExternalID  int64          `json:"externalId"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	State       MilestoneState `json:"state"`
	DueDate     *time.Time     `json:"dueDate,omitempty"`
}
```

- [ ] **Step 4: Add 4 methods to `RepositoryProvider` in `internal/provider/types.go`**

The current interface ends with:
```go
	// Validate
	GetRepositoryState(ctx context.Context, namespace, repo string) (RepositoryState, error)
}
```

Replace that closing brace block with:
```go
	// Validate
	GetRepositoryState(ctx context.Context, namespace, repo string) (RepositoryState, error)

	// Labels and milestones (tfrepo migrate-labels)
	ListLabels(ctx context.Context, namespace, repo string) ([]Label, error)
	CreateLabel(ctx context.Context, namespace, repo string, label Label) (Label, error)
	ListMilestones(ctx context.Context, namespace, repo string) ([]Milestone, error)
	CreateMilestone(ctx context.Context, namespace, repo string, m Milestone) (Milestone, error)
}
```

`types.go` imports only `"context"` — no new import is needed (types are defined in `labels.go` in the same package).

- [ ] **Step 5: Add 4 stub methods to `fakeMigrateProvider` in `internal/core/migrate_test.go`**

After the existing `GetRepositoryState` stub (around line 66) and before the compile-time check `var _ ...` (line 69), add:

```go
func (f *fakeMigrateProvider) ListLabels(ctx context.Context, namespace, repo string) ([]provider.Label, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeMigrateProvider) CreateLabel(ctx context.Context, namespace, repo string, label provider.Label) (provider.Label, error) {
	return provider.Label{}, errors.New("not implemented")
}

func (f *fakeMigrateProvider) ListMilestones(ctx context.Context, namespace, repo string) ([]provider.Milestone, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeMigrateProvider) CreateMilestone(ctx context.Context, namespace, repo string, m provider.Milestone) (provider.Milestone, error) {
	return provider.Milestone{}, errors.New("not implemented")
}
```

No new imports needed — `errors` and `provider` are already imported in that file.

- [ ] **Step 6: Run the new type tests to verify they pass**

```bash
go test ./internal/provider/... -run "TestLabel|TestMilestone" -v
```
Expected: 3 tests PASS.

- [ ] **Step 7: Run all tests to verify everything still compiles**

```bash
go test ./...
```
Expected: all existing tests PASS (the `core` package tests now compile because `fakeMigrateProvider` satisfies the extended interface).

- [ ] **Step 8: Commit**

```bash
git add internal/provider/labels.go internal/provider/labels_test.go \
        internal/provider/types.go internal/core/migrate_test.go
git commit -m "feat(provider): add Label/Milestone types and extend RepositoryProvider interface"
```

---

## Task 2: GitHub Provider Implementation

**Files:**
- Modify: `internal/provider/github.go`
- Modify: `internal/provider/github_test.go`

**GitHub API specifics:**
- `p.client.Issues.ListLabelsByRepo(ctx, owner, repo, *github.ListOptions)` → `([]*github.Label, *Response, error)`. `(*github.Label).GetColor()` returns hex WITHOUT `#`.
- `p.client.Issues.CreateLabel(ctx, owner, repo, *github.Label)` → `(*github.Label, *Response, error)`. `Color` field accepts hex WITHOUT `#`.
- `p.client.Issues.ListMilestones(ctx, owner, repo, *github.ListMilestonesOptions)` → `([]*github.Milestone, *Response, error)`. Use `State: "all"` to retrieve both open and closed. `GetNumber()` returns the milestone number (ExternalID). `GetBody()` is the description field.
- `p.client.Issues.CreateMilestone(ctx, owner, repo, *github.Milestone)` → `(*github.Milestone, *Response, error)`.
- `github.Timestamp` embeds `time.Time`; check `duo.IsZero()` (via embedded method) to detect absent due dates.
- For pagination of milestones, `opts.ListOptions.Page = resp.NextPage` (nested struct).

- [ ] **Step 1: Write failing tests for the 4 new GitHub methods**

Study the existing httptest setup in `internal/provider/github_test.go` to find how `newGitHubTestProvider` (or equivalent helper) is defined, then add the following tests. If a helper doesn't exist, create one:

```go
// newGitHubTestProvider creates a *GitHubProvider pointing at the given test server URL.
func newGitHubTestProvider(t *testing.T, serverURL string) *GitHubProvider {
    t.Helper()
    client := github.NewClient(nil)
    u, err := url.Parse(serverURL + "/")
    if err != nil {
        t.Fatalf("parse server URL: %v", err)
    }
    client.BaseURL = u
    client.UploadURL = u
    return &GitHubProvider{client: client}
}
```

Then add:

```go
func TestGitHubProviderListLabels(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/my-org/repo-a/labels", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"name":"bug","color":"d73a4a","description":"Something is wrong"},{"name":"enhancement","color":"a2eeef","description":""}]`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	p := newGitHubTestProvider(t, server.URL)

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
	server := httptest.NewServer(mux)
	defer server.Close()

	p := newGitHubTestProvider(t, server.URL)

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
		fmt.Fprint(w, `[{"number":3,"title":"v1.0","body":"First stable release","state":"open","due_on":null}]`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	p := newGitHubTestProvider(t, server.URL)

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
		fmt.Fprint(w, `{"number":7,"title":"v1.0","body":"First stable release","state":"open","due_on":null}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	p := newGitHubTestProvider(t, server.URL)

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
```

- [ ] **Step 2: Run tests to confirm they fail (methods not defined)**

```bash
go test ./internal/provider/... -run "TestGitHubProviderListLabels|TestGitHubProviderCreateLabel|TestGitHubProviderListMilestones|TestGitHubProviderCreateMilestone" -v
```
Expected: compile error — methods not defined on `*GitHubProvider`.

- [ ] **Step 3: Implement the 4 methods in `internal/provider/github.go`**

Add these functions BEFORE the compile-time check `var _ RepositoryProvider = (*GitHubProvider)(nil)`:

```go
// ListLabels returns all labels defined on the repository.
func (p *GitHubProvider) ListLabels(ctx context.Context, namespace, repo string) ([]Label, error) {
	var labels []Label
	opts := &github.ListOptions{PerPage: 100}
	for {
		page, resp, err := p.client.Issues.ListLabelsByRepo(ctx, namespace, repo, opts)
		if err != nil {
			return nil, fmt.Errorf("list labels for %s/%s: %w", namespace, repo, err)
		}
		for _, l := range page {
			labels = append(labels, Label{
				Name:        l.GetName(),
				Description: l.GetDescription(),
				Color:       l.GetColor(),
			})
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return labels, nil
}

// CreateLabel creates a new label on the repository.
func (p *GitHubProvider) CreateLabel(ctx context.Context, namespace, repo string, label Label) (Label, error) {
	input := &github.Label{
		Name:        github.Ptr(label.Name),
		Description: github.Ptr(label.Description),
		Color:       github.Ptr(label.Color),
	}
	created, _, err := p.client.Issues.CreateLabel(ctx, namespace, repo, input)
	if err != nil {
		return Label{}, fmt.Errorf("create label %q in %s/%s: %w", label.Name, namespace, repo, err)
	}
	return Label{
		Name:        created.GetName(),
		Description: created.GetDescription(),
		Color:       created.GetColor(),
	}, nil
}

// ListMilestones returns all milestones (open and closed) for the repository.
func (p *GitHubProvider) ListMilestones(ctx context.Context, namespace, repo string) ([]Milestone, error) {
	var milestones []Milestone
	opts := &github.ListMilestonesOptions{
		State:       "all",
		ListOptions: github.ListOptions{PerPage: 100},
	}
	for {
		page, resp, err := p.client.Issues.ListMilestones(ctx, namespace, repo, opts)
		if err != nil {
			return nil, fmt.Errorf("list milestones for %s/%s: %w", namespace, repo, err)
		}
		for _, m := range page {
			ms := Milestone{
				ExternalID:  int64(m.GetNumber()),
				Title:       m.GetTitle(),
				Description: m.GetBody(),
				State:       MilestoneStateOpen,
			}
			if m.GetState() == "closed" {
				ms.State = MilestoneStateClosed
			}
			if duo := m.GetDueOn(); !duo.IsZero() {
				t := duo.Time
				ms.DueDate = &t
			}
			milestones = append(milestones, ms)
		}
		if resp.NextPage == 0 {
			break
		}
		opts.ListOptions.Page = resp.NextPage
	}
	return milestones, nil
}

// CreateMilestone creates a new milestone on the repository.
// ExternalID in the returned Milestone reflects the newly assigned milestone number.
func (p *GitHubProvider) CreateMilestone(ctx context.Context, namespace, repo string, m Milestone) (Milestone, error) {
	state := string(m.State)
	input := &github.Milestone{
		Title:       github.Ptr(m.Title),
		Description: github.Ptr(m.Description),
		State:       github.Ptr(state),
	}
	if m.DueDate != nil {
		input.DueOn = &github.Timestamp{Time: *m.DueDate}
	}
	created, _, err := p.client.Issues.CreateMilestone(ctx, namespace, repo, input)
	if err != nil {
		return Milestone{}, fmt.Errorf("create milestone %q in %s/%s: %w", m.Title, namespace, repo, err)
	}
	result := Milestone{
		ExternalID:  int64(created.GetNumber()),
		Title:       created.GetTitle(),
		Description: created.GetBody(),
		State:       MilestoneStateOpen,
	}
	if created.GetState() == "closed" {
		result.State = MilestoneStateClosed
	}
	if duo := created.GetDueOn(); !duo.IsZero() {
		t := duo.Time
		result.DueDate = &t
	}
	return result, nil
}
```

- [ ] **Step 4: Run the new tests to verify they pass**

```bash
go test ./internal/provider/... -run "TestGitHubProviderListLabels|TestGitHubProviderCreateLabel|TestGitHubProviderListMilestones|TestGitHubProviderCreateMilestone" -v
```
Expected: all 4 tests PASS.

- [ ] **Step 5: Run all tests**

```bash
go test ./...
```
Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/provider/github.go internal/provider/github_test.go
git commit -m "feat(provider/github): implement ListLabels, CreateLabel, ListMilestones, CreateMilestone"
```

---

## Task 3: GitLab Provider Implementation

**Files:**
- Modify: `internal/provider/gitlab.go`
- Modify: `internal/provider/gitlab_test.go`

**GitLab API specifics:**
- Project identifier (`pid`) = `namespace + "/" + repo` (e.g. `"my-group/repo-a"`). This is the URL-encoded path used by the go-gitlab client as project ID.
- `p.client.Labels.ListLabels(pid, *gitlab.ListLabelsOptions, ...Option)` → `([]*gitlab.Label, *Response, error)`. `l.Color` returns WITH `#` → strip it.
- `p.client.Labels.CreateLabel(pid, *gitlab.CreateLabelOptions, ...Option)` → `(*gitlab.Label, *Response, error)`. `Color` field requires WITH `#` → prepend it.
- `p.client.Milestones.ListMilestones(pid, *gitlab.ListMilestonesOptions, ...Option)` → `([]*gitlab.Milestone, *Response, error)`. No state filter needed to get all. GitLab state `"active"` maps to `MilestoneStateOpen`; `"closed"` maps to `MilestoneStateClosed`.
- `p.client.Milestones.CreateMilestone(pid, *gitlab.CreateMilestoneOptions, ...Option)` → `(*gitlab.Milestone, *Response, error)`. Always creates as "active".
- To close a GitLab milestone after creation: `p.client.Milestones.UpdateMilestone(pid, created.ID, *gitlab.UpdateMilestoneOptions, ...Option)` with `StateEvent: gitlab.Ptr("close")`. Verify field name via `grep StateEvent` in the go-gitlab milestone source.
- `gitlab.ISOTime` wraps `time.Time`. Convert: `time.Time(*m.DueDate)` when non-nil.
- `listPerPage = 100` is already a package-level const in `gitlab.go` — reuse it.
- `"strings"` and `"time"` may need to be added to `gitlab.go` imports if not already present.

- [ ] **Step 1: Write failing tests for the 4 new GitLab methods**

Study the existing httptest setup in `internal/provider/gitlab_test.go` to find how the test provider is constructed, then add:

```go
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
	// Color must be stripped of '#'
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
	// Verify '#' was prepended before sending to GitLab
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
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/provider/... -run "TestGitLabProviderListLabels|TestGitLabProviderCreateLabel|TestGitLabProviderListMilestones|TestGitLabProviderCreateMilestone" -v
```
Expected: compile error — methods not defined on `*GitLabProvider`.

- [ ] **Step 3: Implement the 4 methods in `internal/provider/gitlab.go`**

Add `"strings"` and `"time"` to the imports in `gitlab.go` if not already present.

Add these functions BEFORE `var _ RepositoryProvider = (*GitLabProvider)(nil)`:

```go
// gitlabPID returns the GitLab project path identifier used in API calls.
func gitlabPID(namespace, repo string) string {
	return namespace + "/" + repo
}

// ListLabels returns all labels for the repository.
// Colors are normalized to hex without '#' prefix.
func (p *GitLabProvider) ListLabels(ctx context.Context, namespace, repo string) ([]Label, error) {
	pid := gitlabPID(namespace, repo)
	var labels []Label
	opts := &gitlab.ListLabelsOptions{
		ListOptions: gitlab.ListOptions{PerPage: listPerPage},
	}
	for {
		page, resp, err := p.client.Labels.ListLabels(pid, opts, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list labels for %s: %w", pid, err)
		}
		for _, l := range page {
			labels = append(labels, Label{
				Name:        l.Name,
				Description: l.Description,
				Color:       strings.TrimPrefix(l.Color, "#"),
			})
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return labels, nil
}

// CreateLabel creates a new label on the repository.
// Color must be provided without '#'; this method prepends '#' for the GitLab API.
func (p *GitLabProvider) CreateLabel(ctx context.Context, namespace, repo string, label Label) (Label, error) {
	pid := gitlabPID(namespace, repo)
	color := "#" + label.Color
	opts := &gitlab.CreateLabelOptions{
		Name:        gitlab.Ptr(label.Name),
		Color:       gitlab.Ptr(color),
		Description: gitlab.Ptr(label.Description),
	}
	created, _, err := p.client.Labels.CreateLabel(pid, opts, gitlab.WithContext(ctx))
	if err != nil {
		return Label{}, fmt.Errorf("create label %q in %s: %w", label.Name, pid, err)
	}
	return Label{
		Name:        created.Name,
		Description: created.Description,
		Color:       strings.TrimPrefix(created.Color, "#"),
	}, nil
}

// ListMilestones returns all milestones (active and closed) for the repository.
func (p *GitLabProvider) ListMilestones(ctx context.Context, namespace, repo string) ([]Milestone, error) {
	pid := gitlabPID(namespace, repo)
	var milestones []Milestone
	opts := &gitlab.ListMilestonesOptions{
		ListOptions: gitlab.ListOptions{PerPage: listPerPage},
	}
	for {
		page, resp, err := p.client.Milestones.ListMilestones(pid, opts, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list milestones for %s: %w", pid, err)
		}
		for _, m := range page {
			ms := Milestone{
				ExternalID:  int64(m.ID),
				Title:       m.Title,
				Description: m.Description,
				State:       MilestoneStateOpen,
			}
			if m.State == "closed" {
				ms.State = MilestoneStateClosed
			}
			if m.DueDate != nil {
				t := time.Time(*m.DueDate)
				ms.DueDate = &t
			}
			milestones = append(milestones, ms)
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return milestones, nil
}

// CreateMilestone creates a new milestone on the repository.
// If the source milestone is closed, a second API call closes it on GitLab
// (GitLab always creates milestones as "active").
// ExternalID in the returned Milestone reflects the GitLab-assigned ID.
func (p *GitLabProvider) CreateMilestone(ctx context.Context, namespace, repo string, m Milestone) (Milestone, error) {
	pid := gitlabPID(namespace, repo)
	opts := &gitlab.CreateMilestoneOptions{
		Title:       gitlab.Ptr(m.Title),
		Description: gitlab.Ptr(m.Description),
	}
	if m.DueDate != nil {
		iso := gitlab.ISOTime(*m.DueDate)
		opts.DueDate = &iso
	}
	created, _, err := p.client.Milestones.CreateMilestone(pid, opts, gitlab.WithContext(ctx))
	if err != nil {
		return Milestone{}, fmt.Errorf("create milestone %q in %s: %w", m.Title, pid, err)
	}

	if m.State == MilestoneStateClosed {
		_, _, err := p.client.Milestones.UpdateMilestone(pid, created.ID, &gitlab.UpdateMilestoneOptions{
			StateEvent: gitlab.Ptr("close"),
		}, gitlab.WithContext(ctx))
		if err != nil {
			return Milestone{}, fmt.Errorf("close milestone %q in %s: %w", m.Title, pid, err)
		}
	}

	result := Milestone{
		ExternalID:  int64(created.ID),
		Title:       created.Title,
		Description: created.Description,
		State:       MilestoneStateOpen,
	}
	if created.State == "closed" || m.State == MilestoneStateClosed {
		result.State = MilestoneStateClosed
	}
	if created.DueDate != nil {
		t := time.Time(*created.DueDate)
		result.DueDate = &t
	}
	return result, nil
}
```

If `gitlab.UpdateMilestoneOptions` does not compile (wrong field name or type), run:
```bash
grep -r "StateEvent\|UpdateMilestoneOptions" \
  "$(go env GOPATH)/pkg/mod/gitlab.com/gitlab-org/api/client-go@v1.46.0/milestones.go" | head -20
```
and adjust accordingly.

- [ ] **Step 4: Run the new tests to verify they pass**

```bash
go test ./internal/provider/... -run "TestGitLabProviderListLabels|TestGitLabProviderCreateLabel|TestGitLabProviderListMilestones|TestGitLabProviderCreateMilestone" -v
```
Expected: all 4 tests PASS.

- [ ] **Step 5: Run all tests**

```bash
go test ./...
```
Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/provider/gitlab.go internal/provider/gitlab_test.go
git commit -m "feat(provider/gitlab): implement ListLabels, CreateLabel, ListMilestones, CreateMilestone"
```

---

## Task 4: Core Logic

**Files:**
- Modify: `internal/core/artifacts.go`
- Create: `internal/core/labels.go`
- Create: `internal/core/labels_test.go`

- [ ] **Step 1: Write failing tests for `MigrateLabelsAndMilestones`**

Create `internal/core/labels_test.go`:

```go
package core

import (
	"context"
	"errors"
	"testing"

	"github.com/arijunior2020/tfrepo/internal/provider"
)

// fakeLabelsProvider controls provider responses for testing MigrateLabelsAndMilestones.
type fakeLabelsProvider struct {
	name string

	listLabels     []provider.Label
	listLabelsErr  error
	createLabelErr error
	createdLabels  []provider.Label

	listMilestones     []provider.Milestone
	listMilestonesErr  error
	createMilestoneErr error
	nextMilestoneID    int64
	createdMilestones  []provider.Milestone
}

func (f *fakeLabelsProvider) Name() string { return f.name }
func (f *fakeLabelsProvider) ListNamespaces(_ context.Context) ([]provider.Namespace, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeLabelsProvider) ListRepositories(_ context.Context, _ string) ([]provider.RepositorySummary, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeLabelsProvider) GetRepositoryDetails(_ context.Context, _, _ string) (provider.RepositoryDetails, error) {
	return provider.RepositoryDetails{}, errors.New("not implemented")
}
func (f *fakeLabelsProvider) CreateRepository(_ context.Context, _ string, _ provider.CreateRepositoryInput) (provider.RepositorySummary, error) {
	return provider.RepositorySummary{}, errors.New("not implemented")
}
func (f *fakeLabelsProvider) GetAuthenticatedCloneURL(_, _ string) string { return "" }
func (f *fakeLabelsProvider) GetRepositoryState(_ context.Context, _, _ string) (provider.RepositoryState, error) {
	return provider.RepositoryState{}, errors.New("not implemented")
}
func (f *fakeLabelsProvider) ListLabels(_ context.Context, _, _ string) ([]provider.Label, error) {
	return f.listLabels, f.listLabelsErr
}
func (f *fakeLabelsProvider) CreateLabel(_ context.Context, _, _ string, label provider.Label) (provider.Label, error) {
	if f.createLabelErr != nil {
		return provider.Label{}, f.createLabelErr
	}
	f.createdLabels = append(f.createdLabels, label)
	return label, nil
}
func (f *fakeLabelsProvider) ListMilestones(_ context.Context, _, _ string) ([]provider.Milestone, error) {
	return f.listMilestones, f.listMilestonesErr
}
func (f *fakeLabelsProvider) CreateMilestone(_ context.Context, _, _ string, m provider.Milestone) (provider.Milestone, error) {
	if f.createMilestoneErr != nil {
		return provider.Milestone{}, f.createMilestoneErr
	}
	f.nextMilestoneID++
	created := m
	created.ExternalID = f.nextMilestoneID
	f.createdMilestones = append(f.createdMilestones, created)
	return created, nil
}

var _ provider.RepositoryProvider = (*fakeLabelsProvider)(nil)

func baseLabelsPlan() MigrationPlan {
	return MigrationPlan{
		Source: ProviderRef{Provider: "github", Namespace: "my-org"},
		Target: ProviderRef{Provider: "gitlab", Namespace: "my-group"},
		Tasks: []MigrationTask{
			{
				ID:     "repo-a",
				Source: TaskEndpoint{Namespace: "my-org", Repo: "repo-a"},
				Target: TaskEndpoint{Namespace: "my-group", Repo: "repo-a"},
			},
		},
	}
}

func TestMigrateLabelsAndMilestones_Success(t *testing.T) {
	source := &fakeLabelsProvider{
		name:       "github",
		listLabels: []provider.Label{{Name: "bug", Color: "d73a4a", Description: "A bug"}},
		listMilestones: []provider.Milestone{
			{ExternalID: 1, Title: "v1.0", State: provider.MilestoneStateOpen},
		},
	}
	target := &fakeLabelsProvider{name: "gitlab", nextMilestoneID: 99}

	report, err := MigrateLabelsAndMilestones(context.Background(), baseLabelsPlan(), LabelsMigrateProviders{Source: source, Target: target})
	if err != nil {
		t.Fatalf("MigrateLabelsAndMilestones: %v", err)
	}
	if len(report.Results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(report.Results))
	}
	r := report.Results[0]
	if r.Status != "success" {
		t.Errorf("status = %q, want success (errors: %v)", r.Status, r.Errors)
	}
	if r.LabelsCreated != 1 {
		t.Errorf("LabelsCreated = %d, want 1", r.LabelsCreated)
	}
	if r.MilestonesCreated != 1 {
		t.Errorf("MilestonesCreated = %d, want 1", r.MilestonesCreated)
	}
	// source ExternalID 1 → target ExternalID 100 (nextMilestoneID was 99, increments to 100)
	if r.MilestoneIDMap["1"] != 100 {
		t.Errorf("MilestoneIDMap[\"1\"] = %d, want 100", r.MilestoneIDMap["1"])
	}
	if len(target.createdLabels) != 1 || target.createdLabels[0].Name != "bug" {
		t.Errorf("createdLabels = %+v", target.createdLabels)
	}
}

func TestMigrateLabelsAndMilestones_SkipsExistingLabels(t *testing.T) {
	source := &fakeLabelsProvider{
		name: "github",
		listLabels: []provider.Label{
			{Name: "bug", Color: "d73a4a"},
			{Name: "enhancement", Color: "a2eeef"},
		},
	}
	target := &fakeLabelsProvider{
		name:       "gitlab",
		listLabels: []provider.Label{{Name: "bug", Color: "d73a4a"}}, // already exists
	}

	report, err := MigrateLabelsAndMilestones(context.Background(), baseLabelsPlan(), LabelsMigrateProviders{Source: source, Target: target})
	if err != nil {
		t.Fatalf("MigrateLabelsAndMilestones: %v", err)
	}
	r := report.Results[0]
	if r.LabelsCreated != 1 {
		t.Errorf("LabelsCreated = %d, want 1 (only 'enhancement' should be created)", r.LabelsCreated)
	}
	if len(target.createdLabels) != 1 || target.createdLabels[0].Name != "enhancement" {
		t.Errorf("createdLabels = %+v, want [{Name:enhancement ...}]", target.createdLabels)
	}
}

func TestMigrateLabelsAndMilestones_SkipsExistingMilestonesButMapsID(t *testing.T) {
	source := &fakeLabelsProvider{
		name: "github",
		listMilestones: []provider.Milestone{
			{ExternalID: 1, Title: "v1.0", State: provider.MilestoneStateOpen},
		},
	}
	target := &fakeLabelsProvider{
		name: "gitlab",
		listMilestones: []provider.Milestone{
			{ExternalID: 42, Title: "v1.0", State: provider.MilestoneStateOpen}, // already exists
		},
	}

	report, err := MigrateLabelsAndMilestones(context.Background(), baseLabelsPlan(), LabelsMigrateProviders{Source: source, Target: target})
	if err != nil {
		t.Fatalf("MigrateLabelsAndMilestones: %v", err)
	}
	r := report.Results[0]
	if r.MilestonesCreated != 0 {
		t.Errorf("MilestonesCreated = %d, want 0 (milestone exists)", r.MilestonesCreated)
	}
	// Even skipped milestones must appear in the map
	if r.MilestoneIDMap["1"] != 42 {
		t.Errorf("MilestoneIDMap[\"1\"] = %d, want 42", r.MilestoneIDMap["1"])
	}
}

func TestMigrateLabelsAndMilestones_SourceListLabelsError(t *testing.T) {
	source := &fakeLabelsProvider{name: "github", listLabelsErr: errors.New("API timeout")}
	target := &fakeLabelsProvider{name: "gitlab"}

	report, err := MigrateLabelsAndMilestones(context.Background(), baseLabelsPlan(), LabelsMigrateProviders{Source: source, Target: target})
	if err != nil {
		t.Fatalf("unexpected top-level error: %v", err)
	}
	r := report.Results[0]
	if r.Status != "failed" {
		t.Errorf("status = %q, want failed", r.Status)
	}
	if len(r.Errors) == 0 {
		t.Error("expected at least one error in result")
	}
}

func TestMigrateLabelsAndMilestones_CreateLabelErrors(t *testing.T) {
	source := &fakeLabelsProvider{
		name:       "github",
		listLabels: []provider.Label{{Name: "bug"}, {Name: "enhancement"}},
	}
	target := &fakeLabelsProvider{
		name:           "gitlab",
		createLabelErr: errors.New("conflict"),
	}

	report, err := MigrateLabelsAndMilestones(context.Background(), baseLabelsPlan(), LabelsMigrateProviders{Source: source, Target: target})
	if err != nil {
		t.Fatalf("unexpected top-level error: %v", err)
	}
	r := report.Results[0]
	if r.Status != "failed" {
		t.Errorf("status = %q, want failed", r.Status)
	}
	// Both label create attempts should contribute errors
	if len(r.Errors) != 2 {
		t.Errorf("len(errors) = %d, want 2", len(r.Errors))
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail (types undefined)**

```bash
go test ./internal/core/... -run "TestMigrateLabels" -v
```
Expected: compile error — `MigrateLabelsAndMilestones`, `LabelsMigrateProviders` undefined.

- [ ] **Step 3: Add report types to `internal/core/artifacts.go`**

Append after the `ValidationReport` block (before the `WriteJSON`/`ReadJSON` functions):

```go
// LabelsMigrateResult is the outcome of migrating labels and milestones for
// a single repository pair. MilestoneIDMap maps source milestone ExternalID
// (as decimal string) to the target ExternalID — used by tfrepo migrate-issues
// when rewriting milestone references on cloned issues.
type LabelsMigrateResult struct {
	ID                string           `json:"id"`
	Status            string           `json:"status"` // "success" | "failed"
	LabelsCreated     int              `json:"labelsCreated"`
	MilestonesCreated int              `json:"milestonesCreated"`
	MilestoneIDMap    map[string]int64 `json:"milestoneIdMap"`
	Errors            []string         `json:"errors,omitempty"`
}

// LabelsReport is the contents of labels-report.json, produced by
// tfrepo migrate-labels.
type LabelsReport struct {
	GeneratedAt time.Time              `json:"generatedAt"`
	Results     []LabelsMigrateResult  `json:"results"`
}
```

- [ ] **Step 4: Create `internal/core/labels.go`**

```go
package core

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/arijunior2020/tfrepo/internal/provider"
)

// LabelsMigrateProviders holds the source and target providers for
// label and milestone migration.
type LabelsMigrateProviders struct {
	Source provider.RepositoryProvider
	Target provider.RepositoryProvider
}

// MigrateLabelsAndMilestones mirrors labels and milestones from every source
// repository to its target counterpart as defined in plan. Existing items on
// the target are skipped (idempotent). Errors are collected per-repository;
// the function itself only returns an error for context cancellation or other
// top-level failures.
func MigrateLabelsAndMilestones(ctx context.Context, plan MigrationPlan, providers LabelsMigrateProviders) (LabelsReport, error) {
	results := make([]LabelsMigrateResult, 0, len(plan.Tasks))
	for _, task := range plan.Tasks {
		results = append(results, migrateRepoLabelsAndMilestones(ctx, task, providers))
	}
	return LabelsReport{
		GeneratedAt: time.Now().UTC(),
		Results:     results,
	}, nil
}

func migrateRepoLabelsAndMilestones(ctx context.Context, task MigrationTask, providers LabelsMigrateProviders) LabelsMigrateResult {
	result := LabelsMigrateResult{
		ID:             task.ID,
		MilestoneIDMap: make(map[string]int64),
	}

	labelsCreated, labelsErrs := migrateLabels(ctx, task, providers)
	result.LabelsCreated = labelsCreated
	result.Errors = append(result.Errors, labelsErrs...)

	milestonesCreated, idMap, msErrs := migrateMilestones(ctx, task, providers)
	result.MilestonesCreated = milestonesCreated
	for k, v := range idMap {
		result.MilestoneIDMap[k] = v
	}
	result.Errors = append(result.Errors, msErrs...)

	if len(result.Errors) > 0 {
		result.Status = "failed"
	} else {
		result.Status = "success"
	}
	return result
}

func migrateLabels(ctx context.Context, task MigrationTask, providers LabelsMigrateProviders) (created int, errs []string) {
	sourceLabels, err := providers.Source.ListLabels(ctx, task.Source.Namespace, task.Source.Repo)
	if err != nil {
		return 0, []string{fmt.Sprintf("listar labels de origem %s: %v", task.ID, err)}
	}
	targetLabels, err := providers.Target.ListLabels(ctx, task.Target.Namespace, task.Target.Repo)
	if err != nil {
		return 0, []string{fmt.Sprintf("listar labels de destino %s: %v", task.ID, err)}
	}

	existing := make(map[string]bool, len(targetLabels))
	for _, l := range targetLabels {
		existing[l.Name] = true
	}

	for _, label := range sourceLabels {
		if existing[label.Name] {
			continue
		}
		if _, err := providers.Target.CreateLabel(ctx, task.Target.Namespace, task.Target.Repo, label); err != nil {
			errs = append(errs, fmt.Sprintf("criar label %q em %s: %v", label.Name, task.ID, err))
			continue
		}
		created++
	}
	return
}

func migrateMilestones(ctx context.Context, task MigrationTask, providers LabelsMigrateProviders) (createdCount int, idMap map[string]int64, errs []string) {
	idMap = make(map[string]int64)

	sourceMilestones, err := providers.Source.ListMilestones(ctx, task.Source.Namespace, task.Source.Repo)
	if err != nil {
		errs = []string{fmt.Sprintf("listar milestones de origem %s: %v", task.ID, err)}
		return
	}
	targetMilestones, err := providers.Target.ListMilestones(ctx, task.Target.Namespace, task.Target.Repo)
	if err != nil {
		errs = []string{fmt.Sprintf("listar milestones de destino %s: %v", task.ID, err)}
		return
	}

	existing := make(map[string]provider.Milestone, len(targetMilestones))
	for _, m := range targetMilestones {
		existing[m.Title] = m
	}

	for _, ms := range sourceMilestones {
		sourceKey := strconv.FormatInt(ms.ExternalID, 10)
		if target, ok := existing[ms.Title]; ok {
			idMap[sourceKey] = target.ExternalID
			continue
		}
		created, err := providers.Target.CreateMilestone(ctx, task.Target.Namespace, task.Target.Repo, ms)
		if err != nil {
			errs = append(errs, fmt.Sprintf("criar milestone %q em %s: %v", ms.Title, task.ID, err))
			continue
		}
		idMap[sourceKey] = created.ExternalID
		createdCount++
	}
	return
}
```

- [ ] **Step 5: Run core tests**

```bash
go test ./internal/core/... -run "TestMigrateLabels" -v
```
Expected: all 5 tests PASS.

- [ ] **Step 6: Run all tests**

```bash
go test ./...
```
Expected: all tests PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/core/artifacts.go internal/core/labels.go internal/core/labels_test.go
git commit -m "feat(core): implement MigrateLabelsAndMilestones with skip-if-exists and MilestoneIDMap"
```

---

## Task 5: CLI Command, Destroy Update, README

**Files:**
- Create: `internal/cli/migrate_labels.go`
- Create: `internal/cli/migrate_labels_test.go`
- Modify: `internal/cli/destroy.go`
- Modify: `internal/cli/destroy_test.go`
- Modify: `internal/cli/root.go`
- Modify: `README.md`

**Context:**
- Follow `internal/cli/migrate.go` exactly for the CLI layer pattern.
- `configFlagName`, `defaultConfigPath`, `countLabel`, `migrationPlanPath` are package-level identifiers already defined in the `cli` package.
- Signal handling (`signal.NotifyContext`) goes in `newMigrateLabelsCommand`, NOT in `runMigrateLabels` — same as all other commands.
- `labelsReportPath` is the new constant; `destroy.go` references it by name (same `cli` package).

- [ ] **Step 1: Write failing CLI tests**

Create `internal/cli/migrate_labels_test.go`:

```go
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestNewRootCommandRegistersMigrateLabels(t *testing.T) {
	exitCode := 0
	root := newRootCommand(&exitCode)

	cmd, _, err := root.Find([]string{"migrate-labels"})
	if err != nil {
		t.Fatalf("Find(migrate-labels): %v", err)
	}
	if cmd.Use != "migrate-labels" {
		t.Fatalf("Use = %q, want %q", cmd.Use, "migrate-labels")
	}
}

func TestRunMigrateLabelsExitCode1WhenConfigMissing(t *testing.T) {
	dir := t.TempDir()

	exitCode := 0
	root := newRootCommand(&exitCode)

	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"migrate-labels", "--config", filepath.Join(dir, "does-not-exist.yaml")})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if exitCode != 1 {
		t.Fatalf("exitCode = %d, want 1", exitCode)
	}
}

func TestRunMigrateLabelsExitCode1WhenPlanMissing(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	configPath := filepath.Join(dir, "transferepo.config.yaml")
	if err := os.WriteFile(configPath, []byte("source:\n  provider: github\n  namespace: my-org\ntarget:\n  provider: gitlab\n  namespace: my-group\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	exitCode := 0
	root := newRootCommand(&exitCode)

	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"migrate-labels", "--config", configPath})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if exitCode != 1 {
		t.Fatalf("exitCode = %d, want 1", exitCode)
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail (command not registered)**

```bash
go test ./internal/cli/... -run "TestNewRootCommandRegistersMigrateLabels|TestRunMigrateLabels" -v
```
Expected: `TestNewRootCommandRegistersMigrateLabels` fails (finds root command instead of `migrate-labels`). The other two may fail similarly.

- [ ] **Step 3: Create `internal/cli/migrate_labels.go`**

```go
package cli

import (
	"context"
	"fmt"
	"io"
	"os/signal"
	"syscall"

	"github.com/arijunior2020/tfrepo/internal/config"
	"github.com/arijunior2020/tfrepo/internal/core"
	"github.com/spf13/cobra"
)

const labelsReportPath = "labels-report.json"

func newMigrateLabelsCommand(exitCode *int) *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "migrate-labels",
		Short: "Migra labels e milestones dos repositórios de origem para o destino",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			*exitCode = runMigrateLabels(ctx, configPath, cmd.OutOrStdout(), cmd.ErrOrStderr())
			return nil
		},
	}

	cmd.Flags().StringVarP(&configPath, configFlagName, "c", defaultConfigPath, "arquivo de configuração")
	return cmd
}

func runMigrateLabels(ctx context.Context, configPath string, stdout, stderr io.Writer) int {
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	var plan core.MigrationPlan
	if err := core.ReadJSON(migrationPlanPath, &plan); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	source, err := NewProvider(cfg.Source)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	target, err := NewProvider(cfg.Target)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	report, err := core.MigrateLabelsAndMilestones(ctx, plan, core.LabelsMigrateProviders{Source: source, Target: target})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if err := core.WriteJSON(labelsReportPath, report); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	var failed int
	for _, r := range report.Results {
		if r.Status == "failed" {
			failed++
		}
	}

	label := countLabel(len(report.Results), "repositório", "repositórios")
	suffix := ""
	if failed > 0 {
		suffix = fmt.Sprintf(", %d com falha", failed)
	}
	fmt.Fprintf(stdout, "Wrote %s (%s%s)\n", labelsReportPath, label, suffix)

	if failed > 0 {
		return 1
	}
	return 0
}
```

- [ ] **Step 4: Register the command in `internal/cli/root.go`**

Find the block that registers subcommands (contains `root.AddCommand(newMigrateCommand(exitCode))`). Add:

```go
root.AddCommand(newMigrateLabelsCommand(exitCode))
```

Place it directly after `root.AddCommand(newMigrateCommand(exitCode))` for logical ordering.

- [ ] **Step 5: Add `labelsReportPath` to `migrationArtifactPaths` in `internal/cli/destroy.go`**

Change:
```go
var migrationArtifactPaths = []string{
	inventoryPath,
	migrationPlanPath,
	migrationReportPath,
	validationReportPath,
}
```
to:
```go
var migrationArtifactPaths = []string{
	inventoryPath,
	migrationPlanPath,
	migrationReportPath,
	validationReportPath,
	labelsReportPath,
}
```

- [ ] **Step 6: Update the hardcoded count in `internal/cli/destroy_test.go`**

In `TestRunDestroyRemovesMigrationArtifacts`, change:
```go
if !strings.Contains(stdout.String(), "Removidos 4 arquivos.") {
```
to:
```go
if !strings.Contains(stdout.String(), "Removidos 5 arquivos.") {
```

- [ ] **Step 7: Run all CLI tests**

```bash
go test ./internal/cli/... -v
```
Expected: all tests PASS, including the 3 new tests.

- [ ] **Step 8: Run all tests**

```bash
go test ./...
```
Expected: all tests PASS.

- [ ] **Step 9: Update `README.md`**

**In "Início rápido" section** (the pipeline block at step 4), add `migrate-labels` between `migrate` and `validate`:

```bash
tfrepo scan              # lista repositórios → inventory.json
tfrepo plan              # gera plano de migração → migration-plan.json
tfrepo migrate           # executa migração → migration-report.json
tfrepo migrate-labels    # migra labels e milestones → labels-report.json
tfrepo validate          # verifica integridade → validation-report.json
```

**Add a new command section** in the Commands area, after `### tfrepo migrate` and before `### tfrepo validate`:

```markdown
### `tfrepo migrate-labels`

Migra labels e milestones de cada repositório de origem para o repositório de destino correspondente, conforme definido em `migration-plan.json`. O resultado é gravado em `labels-report.json`.

Requer que `migration-plan.json` exista (gerado por `tfrepo plan`). Itens já existentes no destino são ignorados (operação idempotente). O `labels-report.json` gerado contém um mapeamento de IDs de milestones necessário pelo plano de migração de issues (Plan 9).

```
Flags:
  -c, --config string   arquivo de configuração (padrão: transferepo.config.yaml)
```
```

**Update the Artefatos table** to include `labels-report.json`:

```markdown
| `labels-report.json`     | `migrate-labels` | `migrate-issues` (em breve)           |
```

Also update the destroy section list of artifacts to mention `labels-report.json`:

In `### tfrepo destroy`, add `labels-report.json` to the bullet list of artifacts it removes.

- [ ] **Step 10: Commit**

```bash
git add internal/cli/migrate_labels.go internal/cli/migrate_labels_test.go \
        internal/cli/destroy.go internal/cli/destroy_test.go \
        internal/cli/root.go README.md
git commit -m "feat(cli): add tfrepo migrate-labels command; update destroy and README"
```

---

## Self-Review Checklist

After all tasks are complete, verify:

- [ ] `RepositoryProvider` has 4 new methods: `ListLabels`, `CreateLabel`, `ListMilestones`, `CreateMilestone`
- [ ] `fakeMigrateProvider` in `migrate_test.go` has stub implementations for all 4 new methods
- [ ] GitHub colors: `GetColor()` returns without `#`, stored as-is — no transformation
- [ ] GitLab colors: `l.Color` arrives WITH `#` → `strings.TrimPrefix`; outgoing color gets `"#" +` prepended
- [ ] GitHub milestone ExternalID = `int64(m.GetNumber())`; GitLab = `int64(m.ID)`
- [ ] `ListMilestones` for GitHub uses `State: "all"` query parameter
- [ ] `MilestoneIDMap` keys are `strconv.FormatInt(sourceExternalID, 10)`
- [ ] Skipped (already-existing) milestones appear in `MilestoneIDMap` with their target ExternalID
- [ ] `labelsReportPath = "labels-report.json"` defined in `migrate_labels.go`
- [ ] `labelsReportPath` present in `migrationArtifactPaths` in `destroy.go`
- [ ] `destroy_test.go` updated from `"Removidos 4 arquivos."` to `"Removidos 5 arquivos."`
- [ ] `newMigrateLabelsCommand` registered in `newRootCommand`
- [ ] `go test ./...` passes with zero failures
