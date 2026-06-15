# tfrepo Plan 3b — Glob Filters, Scan, Plan Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `internal/core.Filterer` (glob include/exclude filtering), `internal/core.Scan` (build `Inventory` from a provider), and `internal/core.Plan` (build `MigrationPlan` from an `Inventory` + `Config`) — the business logic behind the future `tfrepo scan` and `tfrepo plan` CLI commands.

**Architecture:** Three small, focused files added to the existing `internal/core` package (alongside `artifacts.go` from Plan 3a): `glob.go` (pattern matching), `scan.go` (provider → `Inventory`, with a bounded worker pool), and `plan.go` (`Inventory` + `internal/config.Config` → `MigrationPlan`, using `Filterer`). Each is a direct, field-by-field Go port of the corresponding Node module (`packages/core/src/scan.ts`, `packages/core/src/plan.ts`), adapted to Go idioms (errors instead of exceptions, `context.Context`, a worker pool instead of `Promise.all`).

**Tech Stack:** Go 1.24, `github.com/gobwas/glob` (glob matching, equivalent to Node's `minimatch`), `golang.org/x/sync/errgroup` (bounded concurrency for `Scan`).

---

## Context for implementers

This plan builds directly on Plan 3a (`docs/superpowers/plans/2026-06-14-tfrepo-03a-config-artifacts.md`), which is already merged to `main`. You'll be working in the existing `internal/core` package (currently just `artifacts.go` + `artifacts_test.go`) and will newly import `internal/config` (from Plan 3a) and `internal/provider` (from Plan 2, already imported by `artifacts.go`).

**Reference Node sources** (read-only, for parity — do not modify):
- `/media/arimateia-junior/Dados4/projetos/app-transferepo/packages/core/src/scan.ts` + `scan.test.ts`
- `/media/arimateia-junior/Dados4/projetos/app-transferepo/packages/core/src/plan.ts` + `plan.test.ts`

**Existing Go types you'll use** (all already defined, do not redefine):

From `internal/core/artifacts.go`:
- `ProviderRef{Provider, Namespace string}`
- `RepositoryInventory{Name, DefaultBranch string, Visibility provider.Visibility, SizeKB int64, Branches, Tags []string}`
- `InventoryNamespace{Slug, Name string, Kind provider.NamespaceKind, Repositories []RepositoryInventory}`
- `Inventory{GeneratedAt time.Time, Source ProviderRef, Namespaces []InventoryNamespace}`
- `TaskEndpoint{Namespace, Repo string}`
- `MigrationTask{ID string, Source, Target TaskEndpoint, Branches, Tags []string}`
- `MigrationPlan{GeneratedAt time.Time, Source, Target ProviderRef, Tasks []MigrationTask}`

From `internal/provider/types.go`:
- `Namespace{Slug, Name string, Kind NamespaceKind}`
- `RepositorySummary{Name, Namespace, DefaultBranch string, Visibility Visibility, SizeKB int64}`
- `RepositoryDetails{RepositorySummary, Branches, Tags []string}` (embeds `RepositorySummary`)
- `RepositoryProvider` interface: `Name() string`, `ListNamespaces(ctx) ([]Namespace, error)`, `ListRepositories(ctx, namespace) ([]RepositorySummary, error)`, `GetRepositoryDetails(ctx, namespace, repo) (RepositoryDetails, error)`, `CreateRepository(ctx, namespace, input) (RepositorySummary, error)`, `GetAuthenticatedCloneURL(namespace, repo) string`, `GetRepositoryState(ctx, namespace, repo) (RepositoryState, error)`

From `internal/config/config.go`:
- `Config{Source, Target ProviderConfig, Filters Filters, Mapping map[string]string}`
- `ProviderConfig{Provider, BaseURL, Namespace string}`
- `Filters{Include, Exclude []string}`

**Three deliberate design decisions** (all already resolved — implement as specified, don't second-guess):

1. **Dependency `golang.org/x/sync` must be pinned to `v0.19.0`, not `@latest`.** Versions `v0.20.0` and `v0.21.0` declare `go 1.25.0` in their own `go.mod`, which would force-upgrade this module's `go` directive (currently `go 1.24.0` in `/media/arimateia-junior/Dados4/projetos/tfrepo/go.mod`) when running `go mod tidy`. `v0.19.0` declares `go 1.24.0` — an exact match, no toolchain bump. Use exactly `go get golang.org/x/sync@v0.19.0`.

2. **Use `errgroup.Group.SetLimit(concurrency)` for the bounded worker pool in `Scan`**, not a hand-rolled semaphore channel. `SetLimit` is the modern, built-in equivalent of "errgroup + semaphore" referenced in the design spec (`docs/superpowers/specs/2026-06-13-tfrepo-go-port-design.md`) — same effect, less code. `concurrency` values below 1 must be treated as 1 (an `errgroup` with `SetLimit(0)` would deadlock forever, since zero goroutines could ever run).

3. **`Scan`'s "namespace not found" error message is lowercase** (`"namespace %q not found for provider %q"`), unlike Node's capitalized `Namespace "${namespace}" not found for provider "${provider.name}"`. This follows Go's convention that error strings should not be capitalized (avoids a `staticcheck`/`golint` ST1005 warning). The Node test only checks the message *contains* the namespace name (`rejects.toThrow(/my-org/)`), so this is a safe, idiomatic divergence — not a parity break.

**New cross-package dependency:** `plan.go` will import `internal/config` (for `*config.Config`). Verify there's no cycle: `internal/config` imports only `fmt`, `net/url`, `os`, `strings`, `gopkg.in/yaml.v3` — it does not import `internal/core`, so this is safe.

**Why `*config.Config` and not separate parameters:** `Plan`'s Node equivalent takes the whole `TransferepoConfig` object and reads `config.filters`, `config.target.namespace`, `config.mapping`, `config.source`. Passing `*config.Config` directly avoids a parameter explosion and matches the 1:1 port.

---

## Task 1: internal/core/glob.go — Filterer (include/exclude glob matching)

**Files:**
- Create: `internal/core/glob.go`
- Create: `internal/core/glob_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/core/glob_test.go`:

```go
package core

import "testing"

func TestFiltererIncludesEverythingByDefault(t *testing.T) {
	f, err := NewFilterer([]string{"*"}, []string{})
	if err != nil {
		t.Fatalf("NewFilterer: %v", err)
	}

	for _, name := range []string{"repo-a", "repo-b", "legacy-repo"} {
		if !f.Match(name) {
			t.Errorf("Match(%q) = false, want true", name)
		}
	}
}

func TestFiltererAppliesIncludePatterns(t *testing.T) {
	f, err := NewFilterer([]string{"repo-*"}, []string{})
	if err != nil {
		t.Fatalf("NewFilterer: %v", err)
	}

	if !f.Match("repo-a") {
		t.Error(`Match("repo-a") = false, want true`)
	}
	if f.Match("legacy-repo") {
		t.Error(`Match("legacy-repo") = true, want false`)
	}
}

func TestFiltererAppliesExcludePatternsEvenWhenIncluded(t *testing.T) {
	f, err := NewFilterer([]string{"*"}, []string{"legacy-*"})
	if err != nil {
		t.Fatalf("NewFilterer: %v", err)
	}

	if !f.Match("repo-a") {
		t.Error(`Match("repo-a") = false, want true`)
	}
	if f.Match("legacy-repo") {
		t.Error(`Match("legacy-repo") = true, want false`)
	}
}

func TestFiltererExcludesEverythingWhenNoIncludePatterns(t *testing.T) {
	f, err := NewFilterer([]string{}, []string{})
	if err != nil {
		t.Fatalf("NewFilterer: %v", err)
	}

	if f.Match("repo-a") {
		t.Error(`Match("repo-a") = true, want false`)
	}
}

func TestNewFiltererInvalidPattern(t *testing.T) {
	if _, err := NewFilterer([]string{"["}, nil); err == nil {
		t.Error("NewFilterer() error = nil, want error for invalid pattern")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/core/...`

Expected: build failure — `NewFilterer` is undefined.

- [ ] **Step 3: Add the gobwas/glob dependency and implement Filterer**

```bash
go get github.com/gobwas/glob@v0.2.3
go mod tidy
```

Create `internal/core/glob.go`:

```go
package core

import (
	"fmt"

	"github.com/gobwas/glob"
)

// Filterer applies filters.include/filters.exclude glob patterns to
// repository names, equivalent to plan.ts's use of minimatch. A name passes
// the filter if it matches at least one include pattern and no exclude
// pattern. Patterns are compiled once by NewFilterer and reused across
// Match calls.
type Filterer struct {
	include []glob.Glob
	exclude []glob.Glob
}

// NewFilterer compiles the include and exclude glob patterns.
func NewFilterer(include, exclude []string) (*Filterer, error) {
	includeGlobs, err := compileGlobs(include)
	if err != nil {
		return nil, err
	}
	excludeGlobs, err := compileGlobs(exclude)
	if err != nil {
		return nil, err
	}
	return &Filterer{include: includeGlobs, exclude: excludeGlobs}, nil
}

func compileGlobs(patterns []string) ([]glob.Glob, error) {
	globs := make([]glob.Glob, len(patterns))
	for i, pattern := range patterns {
		g, err := glob.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid glob pattern %q: %w", pattern, err)
		}
		globs[i] = g
	}
	return globs, nil
}

// Match reports whether name should be included: it must match at least one
// include pattern and must not match any exclude pattern.
func (f *Filterer) Match(name string) bool {
	included := false
	for _, g := range f.include {
		if g.Match(name) {
			included = true
			break
		}
	}
	if !included {
		return false
	}

	for _, g := range f.exclude {
		if g.Match(name) {
			return false
		}
	}
	return true
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/core/...`

Expected: `PASS` — all glob tests pass, plus the pre-existing artifact tests (`TestInventoryMarshal`, `TestMigrationPlanMarshal`, `TestMigrationReportMarshal`, `TestValidationReportMarshal`, `TestWriteAndReadJSON`, `TestReadJSONMissingFile`).

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum internal/core/glob.go internal/core/glob_test.go
git commit -m "feat(core): add Filterer for include/exclude glob patterns"
```

---

## Task 2: internal/core/scan.go — Scan()

**Files:**
- Create: `internal/core/scan.go`
- Create: `internal/core/scan_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/core/scan_test.go`:

```go
package core

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/arijunior2020/tfrepo/internal/provider"
)

// fakeScanProvider is a minimal provider.RepositoryProvider used to test
// Scan without making network calls. Only the methods Scan calls
// (Name, ListNamespaces, ListRepositories, GetRepositoryDetails) return
// real data; the rest satisfy the interface but are not exercised here.
type fakeScanProvider struct {
	name         string
	namespaces   []provider.Namespace
	repositories []provider.RepositorySummary
	details      map[string]provider.RepositoryDetails
}

func (f *fakeScanProvider) Name() string { return f.name }

func (f *fakeScanProvider) ListNamespaces(ctx context.Context) ([]provider.Namespace, error) {
	return f.namespaces, nil
}

func (f *fakeScanProvider) ListRepositories(ctx context.Context, namespace string) ([]provider.RepositorySummary, error) {
	return f.repositories, nil
}

func (f *fakeScanProvider) GetRepositoryDetails(ctx context.Context, namespace, repo string) (provider.RepositoryDetails, error) {
	d, ok := f.details[repo]
	if !ok {
		return provider.RepositoryDetails{}, fmt.Errorf("no details for %q", repo)
	}
	return d, nil
}

func (f *fakeScanProvider) CreateRepository(ctx context.Context, namespace string, input provider.CreateRepositoryInput) (provider.RepositorySummary, error) {
	return provider.RepositorySummary{}, errors.New("not implemented")
}

func (f *fakeScanProvider) GetAuthenticatedCloneURL(namespace, repo string) string {
	return ""
}

func (f *fakeScanProvider) GetRepositoryState(ctx context.Context, namespace, repo string) (provider.RepositoryState, error) {
	return provider.RepositoryState{}, errors.New("not implemented")
}

var _ provider.RepositoryProvider = (*fakeScanProvider)(nil)

func TestScanBuildsInventory(t *testing.T) {
	p := &fakeScanProvider{
		name: "github",
		namespaces: []provider.Namespace{
			{Slug: "my-org", Name: "My Org", Kind: provider.NamespaceOrganization},
		},
		repositories: []provider.RepositorySummary{
			{Name: "repo-a", Namespace: "my-org", DefaultBranch: "main", Visibility: provider.VisibilityPrivate, SizeKB: 100},
		},
		details: map[string]provider.RepositoryDetails{
			"repo-a": {
				RepositorySummary: provider.RepositorySummary{
					Name: "repo-a", Namespace: "my-org", DefaultBranch: "main", Visibility: provider.VisibilityPrivate, SizeKB: 100,
				},
				Branches: []string{"main", "develop"},
				Tags:     []string{"v1.0.0"},
			},
		},
	}

	inv, err := Scan(context.Background(), p, "my-org", 4)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if inv.Source != (ProviderRef{Provider: "github", Namespace: "my-org"}) {
		t.Errorf("Source = %+v, want {github my-org}", inv.Source)
	}

	want := []InventoryNamespace{
		{
			Slug: "my-org",
			Name: "My Org",
			Kind: provider.NamespaceOrganization,
			Repositories: []RepositoryInventory{
				{
					Name:          "repo-a",
					DefaultBranch: "main",
					Visibility:    provider.VisibilityPrivate,
					SizeKB:        100,
					Branches:      []string{"main", "develop"},
					Tags:          []string{"v1.0.0"},
				},
			},
		},
	}

	if !reflect.DeepEqual(inv.Namespaces, want) {
		t.Errorf("Namespaces = %+v, want %+v", inv.Namespaces, want)
	}

	if inv.GeneratedAt.IsZero() {
		t.Error("GeneratedAt is zero, want a real timestamp")
	}
}

func TestScanEmptyRepositories(t *testing.T) {
	p := &fakeScanProvider{
		name: "github",
		namespaces: []provider.Namespace{
			{Slug: "my-org", Name: "My Org", Kind: provider.NamespaceOrganization},
		},
		details: map[string]provider.RepositoryDetails{},
	}

	inv, err := Scan(context.Background(), p, "my-org", 4)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if len(inv.Namespaces) != 1 {
		t.Fatalf("len(Namespaces) = %d, want 1", len(inv.Namespaces))
	}
	if len(inv.Namespaces[0].Repositories) != 0 {
		t.Errorf("Repositories = %+v, want empty", inv.Namespaces[0].Repositories)
	}
}

func TestScanNamespaceNotFound(t *testing.T) {
	p := &fakeScanProvider{
		name: "github",
		namespaces: []provider.Namespace{
			{Slug: "other-org", Name: "Other", Kind: provider.NamespaceOrganization},
		},
	}

	_, err := Scan(context.Background(), p, "my-org", 4)
	if err == nil {
		t.Fatal("Scan() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "my-org") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "my-org")
	}
}

func TestScanConcurrencyPreservesOrder(t *testing.T) {
	p := &fakeScanProvider{
		name: "github",
		namespaces: []provider.Namespace{
			{Slug: "my-org", Name: "My Org", Kind: provider.NamespaceOrganization},
		},
		repositories: []provider.RepositorySummary{
			{Name: "repo-a"},
			{Name: "repo-b"},
			{Name: "repo-c"},
		},
		details: map[string]provider.RepositoryDetails{
			"repo-a": {RepositorySummary: provider.RepositorySummary{Name: "repo-a"}, Branches: []string{}, Tags: []string{}},
			"repo-b": {RepositorySummary: provider.RepositorySummary{Name: "repo-b"}, Branches: []string{}, Tags: []string{}},
			"repo-c": {RepositorySummary: provider.RepositorySummary{Name: "repo-c"}, Branches: []string{}, Tags: []string{}},
		},
	}

	inv, err := Scan(context.Background(), p, "my-org", 2)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	var names []string
	for _, r := range inv.Namespaces[0].Repositories {
		names = append(names, r.Name)
	}
	want := []string{"repo-a", "repo-b", "repo-c"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("Repositories order = %v, want %v", names, want)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/core/...`

Expected: build failure — `Scan` is undefined.

- [ ] **Step 3: Add the golang.org/x/sync dependency and implement Scan**

```bash
go get golang.org/x/sync@v0.19.0
go mod tidy
```

Create `internal/core/scan.go`:

```go
package core

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/arijunior2020/tfrepo/internal/provider"
)

// Scan builds an Inventory for namespace by listing its repositories and
// fetching branch/tag details for each one, using up to concurrency
// goroutines. Values of concurrency below 1 are treated as 1.
func Scan(ctx context.Context, p provider.RepositoryProvider, namespace string, concurrency int) (Inventory, error) {
	if concurrency < 1 {
		concurrency = 1
	}

	namespaces, err := p.ListNamespaces(ctx)
	if err != nil {
		return Inventory{}, err
	}

	var target *provider.Namespace
	for i := range namespaces {
		if namespaces[i].Slug == namespace {
			target = &namespaces[i]
			break
		}
	}
	if target == nil {
		return Inventory{}, fmt.Errorf("namespace %q not found for provider %q", namespace, p.Name())
	}

	repositories, err := p.ListRepositories(ctx, namespace)
	if err != nil {
		return Inventory{}, err
	}

	details := make([]provider.RepositoryDetails, len(repositories))

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)

	for i, repo := range repositories {
		g.Go(func() error {
			d, err := p.GetRepositoryDetails(gctx, namespace, repo.Name)
			if err != nil {
				return err
			}
			details[i] = d
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return Inventory{}, err
	}

	repositoryInventories := make([]RepositoryInventory, len(details))
	for i, d := range details {
		repositoryInventories[i] = RepositoryInventory{
			Name:          d.Name,
			DefaultBranch: d.DefaultBranch,
			Visibility:    d.Visibility,
			SizeKB:        d.SizeKB,
			Branches:      d.Branches,
			Tags:          d.Tags,
		}
	}

	return Inventory{
		GeneratedAt: time.Now().UTC(),
		Source:      ProviderRef{Provider: p.Name(), Namespace: namespace},
		Namespaces: []InventoryNamespace{
			{
				Slug:         target.Slug,
				Name:         target.Name,
				Kind:         target.Kind,
				Repositories: repositoryInventories,
			},
		},
	}, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/core/...`

Expected: `PASS` — all 4 scan tests pass, plus all tests from Task 1 and Plan 3a.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum internal/core/scan.go internal/core/scan_test.go
git commit -m "feat(core): add Scan to build Inventory from a provider"
```

---

## Task 3: internal/core/plan.go — Plan()

**Files:**
- Create: `internal/core/plan.go`
- Create: `internal/core/plan_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/core/plan_test.go`:

```go
package core

import (
	"reflect"
	"testing"

	"github.com/arijunior2020/tfrepo/internal/config"
	"github.com/arijunior2020/tfrepo/internal/provider"
)

func basePlanInventory() Inventory {
	return Inventory{
		Source: ProviderRef{Provider: "github", Namespace: "my-org"},
		Namespaces: []InventoryNamespace{
			{
				Slug: "my-org",
				Name: "My Org",
				Kind: provider.NamespaceOrganization,
				Repositories: []RepositoryInventory{
					{Name: "repo-a", DefaultBranch: "main", Visibility: provider.VisibilityPrivate, SizeKB: 100, Branches: []string{"main"}, Tags: []string{"v1.0.0"}},
					{Name: "repo-b", DefaultBranch: "main", Visibility: provider.VisibilityPublic, SizeKB: 50, Branches: []string{"main"}, Tags: []string{}},
					{Name: "legacy-repo", DefaultBranch: "main", Visibility: provider.VisibilityPrivate, SizeKB: 10, Branches: []string{"main"}, Tags: []string{}},
				},
			},
		},
	}
}

func basePlanConfig() *config.Config {
	return &config.Config{
		Source:  config.ProviderConfig{Provider: "github", Namespace: "my-org"},
		Target:  config.ProviderConfig{Provider: "gitlab", Namespace: "my-group"},
		Filters: config.Filters{Include: []string{"*"}, Exclude: []string{}},
		Mapping: map[string]string{},
	}
}

func taskIDs(p MigrationPlan) []string {
	ids := make([]string, len(p.Tasks))
	for i, task := range p.Tasks {
		ids[i] = task.ID
	}
	return ids
}

func TestPlanIncludesEveryRepositoryByDefault(t *testing.T) {
	got, err := Plan(basePlanInventory(), basePlanConfig())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	want := []string{"repo-a", "repo-b", "legacy-repo"}
	if !reflect.DeepEqual(taskIDs(got), want) {
		t.Errorf("task IDs = %v, want %v", taskIDs(got), want)
	}

	if got.Source != (ProviderRef{Provider: "github", Namespace: "my-org"}) {
		t.Errorf("Source = %+v, want {github my-org}", got.Source)
	}
	if got.Target != (ProviderRef{Provider: "gitlab", Namespace: "my-group"}) {
		t.Errorf("Target = %+v, want {gitlab my-group}", got.Target)
	}
}

func TestPlanAppliesIncludePatterns(t *testing.T) {
	cfg := basePlanConfig()
	cfg.Filters = config.Filters{Include: []string{"repo-*"}, Exclude: []string{}}

	got, err := Plan(basePlanInventory(), cfg)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	want := []string{"repo-a", "repo-b"}
	if !reflect.DeepEqual(taskIDs(got), want) {
		t.Errorf("task IDs = %v, want %v", taskIDs(got), want)
	}
}

func TestPlanAppliesExcludePatternsEvenWhenIncluded(t *testing.T) {
	cfg := basePlanConfig()
	cfg.Filters = config.Filters{Include: []string{"*"}, Exclude: []string{"legacy-*"}}

	got, err := Plan(basePlanInventory(), cfg)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	want := []string{"repo-a", "repo-b"}
	if !reflect.DeepEqual(taskIDs(got), want) {
		t.Errorf("task IDs = %v, want %v", taskIDs(got), want)
	}
}

func TestPlanRenamesTargetRepositoryAccordingToMapping(t *testing.T) {
	cfg := basePlanConfig()
	cfg.Mapping = map[string]string{"repo-a": "renamed-repo"}

	got, err := Plan(basePlanInventory(), cfg)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	for _, task := range got.Tasks {
		if task.ID == "repo-a" {
			want := TaskEndpoint{Namespace: "my-group", Repo: "renamed-repo"}
			if task.Target != want {
				t.Errorf("Target = %+v, want %+v", task.Target, want)
			}
			return
		}
	}
	t.Fatal("task repo-a not found")
}

func TestPlanCopiesBranchesAndTagsFromInventory(t *testing.T) {
	got, err := Plan(basePlanInventory(), basePlanConfig())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	for _, task := range got.Tasks {
		if task.ID == "repo-a" {
			want := MigrationTask{
				ID:       "repo-a",
				Source:   TaskEndpoint{Namespace: "my-org", Repo: "repo-a"},
				Target:   TaskEndpoint{Namespace: "my-group", Repo: "repo-a"},
				Branches: []string{"main"},
				Tags:     []string{"v1.0.0"},
			}
			if !reflect.DeepEqual(task, want) {
				t.Errorf("task = %+v, want %+v", task, want)
			}
			return
		}
	}
	t.Fatal("task repo-a not found")
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/core/...`

Expected: build failure — `Plan` is undefined.

- [ ] **Step 3: Implement Plan**

Create `internal/core/plan.go`:

```go
package core

import (
	"time"

	"github.com/arijunior2020/tfrepo/internal/config"
)

// Plan applies cfg's include/exclude filters and repository name mapping to
// the repositories recorded in inventory, producing the tasks for
// migration-plan.json.
func Plan(inventory Inventory, cfg *config.Config) (MigrationPlan, error) {
	filterer, err := NewFilterer(cfg.Filters.Include, cfg.Filters.Exclude)
	if err != nil {
		return MigrationPlan{}, err
	}

	tasks := []MigrationTask{}
	for _, ns := range inventory.Namespaces {
		for _, repo := range ns.Repositories {
			if !filterer.Match(repo.Name) {
				continue
			}

			targetRepo := repo.Name
			if mapped, ok := cfg.Mapping[repo.Name]; ok {
				targetRepo = mapped
			}

			tasks = append(tasks, MigrationTask{
				ID:       repo.Name,
				Source:   TaskEndpoint{Namespace: ns.Slug, Repo: repo.Name},
				Target:   TaskEndpoint{Namespace: cfg.Target.Namespace, Repo: targetRepo},
				Branches: repo.Branches,
				Tags:     repo.Tags,
			})
		}
	}

	return MigrationPlan{
		GeneratedAt: time.Now().UTC(),
		Source:      ProviderRef{Provider: cfg.Source.Provider, Namespace: cfg.Source.Namespace},
		Target:      ProviderRef{Provider: cfg.Target.Provider, Namespace: cfg.Target.Namespace},
		Tasks:       tasks,
	}, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/core/...`

Expected: `PASS` — all 5 plan tests pass, plus all tests from Tasks 1-2 and Plan 3a.

- [ ] **Step 5: Commit**

```bash
git add internal/core/plan.go internal/core/plan_test.go
git commit -m "feat(core): add Plan to build MigrationPlan from Inventory and Config"
```

---

## Final Verification

After Task 3, run the full test suite to confirm nothing else broke:

```bash
go build ./...
go vet ./...
go test ./...
gofmt -l .
```

Expected: all packages build, `go vet` is clean, `gofmt -l .` prints nothing, and all tests pass — including the pre-existing `internal/config`, `internal/provider`, and `internal/security` suites, and `internal/core`'s artifact tests from Plan 3a plus the new `Filterer`/`Scan`/`Plan` tests from this plan (14 new test functions total: 5 from Task 1, 4 from Task 2, 5 from Task 3).
