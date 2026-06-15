# tfrepo Plan 3c — Migrate, Validate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `internal/core.Migrate` (mirror-clones each source repository and pushes it to the target, with `--dry-run` and bounded concurrency) and `internal/core.Validate` (compares branch/tag SHAs between source and target) — the business logic behind the future `tfrepo migrate` and `tfrepo validate` CLI commands.

**Architecture:** Two small, focused files added to the existing `internal/core` package (alongside `artifacts.go`, `glob.go`, `scan.go`, `plan.go` from Plans 3a/3b): `migrate.go` (`MigrationPlan` → `MigrationReport`, using `internal/security` for git mirror operations and workspace management) and `validate.go` (`MigrationPlan` + optional `MigrationReport` → `ValidationReport`). Each is a direct, field-by-field Go port of the corresponding Node module (`packages/core/src/migrate.ts`, `packages/core/src/validate.ts`), adapted to Go idioms (errors instead of exceptions, `context.Context`, `errgroup` instead of `Promise.all`/sequential loops).

**Tech Stack:** Go 1.24, `golang.org/x/sync/errgroup` (already a direct dependency from Plan 3b), `internal/security` (already implemented: `Clone`, `Push`, `Manager`/`Workspace`, `Redact`).

---

## Context for implementers

This plan builds directly on Plan 3b (`docs/superpowers/plans/2026-06-15-tfrepo-03b-scan-plan.md`), which is already merged to `main`. You'll be working in the existing `internal/core` package (currently `artifacts.go`, `glob.go`, `scan.go`, `plan.go` + their `_test.go` files) and will newly import `internal/security` (already implemented and committed, not part of this plan).

**Reference Node sources** (read-only, for parity — do not modify):
- `/media/arimateia-junior/Dados4/projetos/app-transferepo/packages/core/src/migrate.ts` + `migrate.test.ts`
- `/media/arimateia-junior/Dados4/projetos/app-transferepo/packages/core/src/validate.ts` + `validate.test.ts`

**Existing Go types you'll use** (all already defined, do not redefine):

From `internal/core/artifacts.go`:
- `TaskEndpoint{Namespace, Repo string}`
- `MigrationTask{ID string, Source, Target TaskEndpoint, Branches, Tags []string}`
- `MigrationPlan{GeneratedAt time.Time, Source, Target ProviderRef, Tasks []MigrationTask}`
- `MigrationResult{ID, Status string, StartedAt, FinishedAt time.Time, Error string `json:"error,omitempty"`}` — `Status` is `"success"` | `"failed"` | `"dry-run"`
- `MigrationReport{GeneratedAt time.Time, Results []MigrationResult}`
- `RefDivergence{Type, Name string, SourceSHA, TargetSHA *string}` — `Type` is `"branch"` | `"tag"`; `SourceSHA`/`TargetSHA` are `nil` (not empty string) when a ref is missing on one side
- `ValidationResult{ID, Status string, Divergences []RefDivergence, Reason string `json:"reason,omitempty"`}` — `Status` is `"ok"` | `"diverged"` | `"skipped"`
- `ValidationReport{GeneratedAt time.Time, Results []ValidationResult}`

From `internal/provider/types.go`:
- `Visibility` (`type Visibility string`), constant `VisibilityPrivate Visibility = "private"`
- `RepositorySummary{Name, Namespace, DefaultBranch string, Visibility Visibility, SizeKB int64}`
- `RepositoryState{Branches, Tags map[string]string}` — ref name → commit SHA
- `CreateRepositoryInput{Name string, Visibility Visibility, Description string}`
- `RepositoryProvider` interface: `Name() string`, `ListNamespaces(ctx) ([]Namespace, error)`, `ListRepositories(ctx, namespace) ([]RepositorySummary, error)`, `GetRepositoryDetails(ctx, namespace, repo) (RepositoryDetails, error)`, `CreateRepository(ctx, namespace, input) (RepositorySummary, error)`, `GetAuthenticatedCloneURL(namespace, repo) string`, `GetRepositoryState(ctx, namespace, repo) (RepositoryState, error)`

From `internal/security` (already implemented, committed — read-only):
- `Clone(ctx context.Context, authenticatedURL, dir string) error` — `git clone --mirror -- <authenticatedURL> <dir>`. `dir` must not already exist; git creates it. On error, the returned error already has `authenticatedURL` and any `https://user:pass@...` pattern redacted from the git output.
- `Push(ctx context.Context, dir, authenticatedURL string) error` — `git -C <dir> push --mirror -- <authenticatedURL>`. Same redaction behavior on error.
- `NewManager() *Manager`, `(*Manager) Create() (*Workspace, error)` — creates a `0700` temp dir, returns `&Workspace{Path string}`. `(*Workspace) Cleanup() error` — removes the directory, safe to call more than once.
- `Redact(value string, secrets ...string) string` — replaces `https://user:pass@host` patterns with `https://***@host`, plus any explicitly-passed secret strings, with `***`.

**Three deliberate design decisions** (all already resolved — implement as specified, don't second-guess):

1. **`Migrate` uses the same `errgroup.SetLimit(concurrency)` worker-pool pattern as `Scan`** (Plan 3b), writing into a pre-allocated `results []MigrationResult` slice indexed by task position to preserve order. `concurrency` values below 1 are treated as 1. Unlike Node's sequential `for` loop, this gives `tfrepo migrate` real concurrency (per the design spec: "worker pool de até `--concurrency` goroutines"). Per-task failures are captured into `results[i]` as `"failed"` — the `g.Go` functions always return `nil`, so `Migrate` itself never errors because of a single task's failure.

2. **`Validate` processes tasks sequentially (in plan order), but fetches source and target `GetRepositoryState` concurrently within each task** via a 2-goroutine `errgroup` (no `SetLimit` needed — exactly 2 goroutines), mirroring Node's `Promise.all([source.getRepositoryState(...), target.getRepositoryState(...)])`. There is no `--concurrency` flag for `validate` in the CLI design, so no cross-task worker pool is needed (avoids unbounded fan-out against the provider APIs). If either `GetRepositoryState` call errors, `Validate` returns `(ValidationReport{}, err)` immediately — matching Node's unhandled-rejection behavior (no per-task error capture for `Validate`, unlike `Migrate`).

3. **`compareRefs` sorts ref names before comparing**, producing `[]RefDivergence` in alphabetical order by name (branches first, then tags, each internally sorted). Node's `compareRefs` iterates a `Set` built from `[...Object.keys(source), ...Object.keys(target)]`, whose order is incidental. Sorting makes Go's output deterministic and reproducible across runs — a safe, idiomatic divergence (the Node tests only ever have one ref per type, so order is never asserted).

**Workspace layout:** each migration task gets its own temporary directory via `workspaces.Create()` (returns `*security.Workspace{Path string}`). The mirror clone goes into `filepath.Join(workspace.Path, "repo.git")` (matching the design spec's `<workspace>/repo.git`), then `security.Push` is called with that same directory as its `dir` argument. `workspace.Cleanup()` runs via `defer`, even on error.

**Target repository creation:** if `task.Target.Repo` is not present in `target.ListRepositories(ctx, task.Target.Namespace)`, call `target.CreateRepository(ctx, task.Target.Namespace, provider.CreateRepositoryInput{Name: task.Target.Repo, Visibility: provider.VisibilityPrivate})` — matching Node's `DEFAULT_TARGET_VISIBILITY = 'private'`.

**Error redaction:** every `MigrationResult.Error` must be set via `security.Redact(err.Error())` — matching the `artifacts.go` comment "always passed through security.Redact() before being set" and Node's `redact(toErrorMessage(error))`. `security.Clone`/`security.Push` already redact their own URL from their returned errors; calling `Redact` again on an already-redacted string is harmless (idempotent for `***`).

---

## Task 1: internal/core/migrate.go — Migrate()

**Files:**
- Create: `internal/core/migrate.go`
- Create: `internal/core/migrate_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/core/migrate_test.go`:

```go
package core

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/arijunior2020/tfrepo/internal/provider"
	"github.com/arijunior2020/tfrepo/internal/security"
)

// createRepositoryCall records a single CreateRepository invocation.
type createRepositoryCall struct {
	namespace string
	input     provider.CreateRepositoryInput
}

// fakeMigrateProvider is a minimal provider.RepositoryProvider used to test
// Migrate without making network calls (other than local git operations via
// security.Clone/Push, which accept filesystem paths as "URLs").
type fakeMigrateProvider struct {
	name string

	listRepositoriesResult []provider.RepositorySummary
	listRepositoriesErr    error

	createRepositoryErr   error
	createRepositoryCalls []createRepositoryCall

	cloneURL string
}

func (f *fakeMigrateProvider) Name() string { return f.name }

func (f *fakeMigrateProvider) ListNamespaces(ctx context.Context) ([]provider.Namespace, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeMigrateProvider) ListRepositories(ctx context.Context, namespace string) ([]provider.RepositorySummary, error) {
	return f.listRepositoriesResult, f.listRepositoriesErr
}

func (f *fakeMigrateProvider) GetRepositoryDetails(ctx context.Context, namespace, repo string) (provider.RepositoryDetails, error) {
	return provider.RepositoryDetails{}, errors.New("not implemented")
}

func (f *fakeMigrateProvider) CreateRepository(ctx context.Context, namespace string, input provider.CreateRepositoryInput) (provider.RepositorySummary, error) {
	f.createRepositoryCalls = append(f.createRepositoryCalls, createRepositoryCall{namespace: namespace, input: input})
	if f.createRepositoryErr != nil {
		return provider.RepositorySummary{}, f.createRepositoryErr
	}
	return provider.RepositorySummary{Name: input.Name, Namespace: namespace, Visibility: input.Visibility}, nil
}

func (f *fakeMigrateProvider) GetAuthenticatedCloneURL(namespace, repo string) string {
	return f.cloneURL
}

func (f *fakeMigrateProvider) GetRepositoryState(ctx context.Context, namespace, repo string) (provider.RepositoryState, error) {
	return provider.RepositoryState{}, errors.New("not implemented")
}

var _ provider.RepositoryProvider = (*fakeMigrateProvider)(nil)

func baseMigratePlan() MigrationPlan {
	return MigrationPlan{
		GeneratedAt: time.Now().UTC(),
		Source:      ProviderRef{Provider: "github", Namespace: "my-org"},
		Target:      ProviderRef{Provider: "gitlab", Namespace: "my-group"},
		Tasks: []MigrationTask{
			{
				ID:       "repo-a",
				Source:   TaskEndpoint{Namespace: "my-org", Repo: "repo-a"},
				Target:   TaskEndpoint{Namespace: "my-group", Repo: "repo-a"},
				Branches: []string{"main"},
				Tags:     []string{"v1.0.0"},
			},
		},
	}
}

// initGitRepo initializes a git repository in dir with one commit on "main"
// and returns that commit's SHA.
func initGitRepo(t *testing.T, dir string) string {
	t.Helper()

	runGit(t, dir, "init", "--initial-branch=main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "initial commit")

	return strings.TrimSpace(runGit(t, dir, "rev-parse", "main"))
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}

	return string(output)
}

func TestMigrateClonesAndPushesMirror(t *testing.T) {
	sourceDir := t.TempDir()
	sourceHead := initGitRepo(t, sourceDir)

	targetDir := t.TempDir()
	runGit(t, targetDir, "init", "--bare")

	source := &fakeMigrateProvider{name: "github", cloneURL: sourceDir}
	target := &fakeMigrateProvider{
		name:                    "gitlab",
		listRepositoriesResult: []provider.RepositorySummary{},
		cloneURL:                targetDir,
	}

	report, err := Migrate(context.Background(), baseMigratePlan(), MigrateProviders{Source: source, Target: target}, MigrateOptions{Concurrency: 1, Workspaces: security.NewManager()})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	if len(report.Results) != 1 {
		t.Fatalf("len(Results) = %d, want 1", len(report.Results))
	}
	if report.Results[0].Status != "success" {
		t.Errorf("Status = %q, want %q (error: %s)", report.Results[0].Status, "success", report.Results[0].Error)
	}

	if len(target.createRepositoryCalls) != 1 {
		t.Fatalf("CreateRepository calls = %d, want 1", len(target.createRepositoryCalls))
	}
	call := target.createRepositoryCalls[0]
	if call.namespace != "my-group" {
		t.Errorf("namespace = %q, want %q", call.namespace, "my-group")
	}
	wantInput := provider.CreateRepositoryInput{Name: "repo-a", Visibility: provider.VisibilityPrivate}
	if call.input != wantInput {
		t.Errorf("input = %+v, want %+v", call.input, wantInput)
	}

	targetHead := strings.TrimSpace(runGit(t, targetDir, "rev-parse", "main"))
	if targetHead != sourceHead {
		t.Errorf("target HEAD = %q, want %q", targetHead, sourceHead)
	}
}

func TestMigrateDoesNotCreateTargetRepositoryIfExists(t *testing.T) {
	sourceDir := t.TempDir()
	initGitRepo(t, sourceDir)

	targetDir := t.TempDir()
	runGit(t, targetDir, "init", "--bare")

	source := &fakeMigrateProvider{name: "github", cloneURL: sourceDir}
	target := &fakeMigrateProvider{
		name: "gitlab",
		listRepositoriesResult: []provider.RepositorySummary{
			{Name: "repo-a", Namespace: "my-group", DefaultBranch: "main", Visibility: provider.VisibilityPrivate},
		},
		cloneURL: targetDir,
	}

	report, err := Migrate(context.Background(), baseMigratePlan(), MigrateProviders{Source: source, Target: target}, MigrateOptions{Concurrency: 1, Workspaces: security.NewManager()})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	if report.Results[0].Status != "success" {
		t.Errorf("Status = %q, want %q (error: %s)", report.Results[0].Status, "success", report.Results[0].Error)
	}
	if len(target.createRepositoryCalls) != 0 {
		t.Errorf("CreateRepository calls = %d, want 0", len(target.createRepositoryCalls))
	}
}

func TestMigrateRecordsFailedResultAndRedactsCredentialsOnCloneFailure(t *testing.T) {
	source := &fakeMigrateProvider{
		name:     "github",
		cloneURL: "https://x-access-token:ghp_secret@127.0.0.1/does-not-exist/repo-a.git",
	}
	target := &fakeMigrateProvider{name: "gitlab", listRepositoriesResult: []provider.RepositorySummary{}}

	report, err := Migrate(context.Background(), baseMigratePlan(), MigrateProviders{Source: source, Target: target}, MigrateOptions{Concurrency: 1, Workspaces: security.NewManager()})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	result := report.Results[0]
	if result.Status != "failed" {
		t.Fatalf("Status = %q, want %q (error: %s)", result.Status, "failed", result.Error)
	}
	if strings.Contains(result.Error, "ghp_secret") {
		t.Errorf("Error contains secret: %q", result.Error)
	}
	if !strings.Contains(result.Error, "https://***@127.0.0.1") {
		t.Errorf("Error = %q, want to contain %q", result.Error, "https://***@127.0.0.1")
	}
}

func TestMigrateDryRun(t *testing.T) {
	source := &fakeMigrateProvider{name: "github"}
	target := &fakeMigrateProvider{name: "gitlab", listRepositoriesResult: []provider.RepositorySummary{}}

	report, err := Migrate(context.Background(), baseMigratePlan(), MigrateProviders{Source: source, Target: target}, MigrateOptions{DryRun: true, Concurrency: 1})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	if report.Results[0].Status != "dry-run" {
		t.Errorf("Status = %q, want %q (error: %s)", report.Results[0].Status, "dry-run", report.Results[0].Error)
	}
	if len(target.createRepositoryCalls) != 0 {
		t.Errorf("CreateRepository calls = %d, want 0", len(target.createRepositoryCalls))
	}
}

func TestMigrateDryRunRecordsFailedResultWhenTargetNamespaceCheckFails(t *testing.T) {
	source := &fakeMigrateProvider{name: "github"}
	target := &fakeMigrateProvider{name: "gitlab", listRepositoriesErr: errors.New("404 Not Found")}

	report, err := Migrate(context.Background(), baseMigratePlan(), MigrateProviders{Source: source, Target: target}, MigrateOptions{DryRun: true, Concurrency: 1})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	result := report.Results[0]
	if result.Status != "failed" {
		t.Errorf("Status = %q, want %q", result.Status, "failed")
	}
	if result.Error != "404 Not Found" {
		t.Errorf("Error = %q, want %q", result.Error, "404 Not Found")
	}
}

func TestMigratePreservesTaskOrderUnderConcurrency(t *testing.T) {
	plan := MigrationPlan{
		Tasks: []MigrationTask{
			{ID: "repo-a", Source: TaskEndpoint{Namespace: "my-org", Repo: "repo-a"}, Target: TaskEndpoint{Namespace: "my-group", Repo: "repo-a"}},
			{ID: "repo-b", Source: TaskEndpoint{Namespace: "my-org", Repo: "repo-b"}, Target: TaskEndpoint{Namespace: "my-group", Repo: "repo-b"}},
			{ID: "repo-c", Source: TaskEndpoint{Namespace: "my-org", Repo: "repo-c"}, Target: TaskEndpoint{Namespace: "my-group", Repo: "repo-c"}},
		},
	}

	source := &fakeMigrateProvider{name: "github"}
	target := &fakeMigrateProvider{name: "gitlab", listRepositoriesResult: []provider.RepositorySummary{}}

	report, err := Migrate(context.Background(), plan, MigrateProviders{Source: source, Target: target}, MigrateOptions{DryRun: true, Concurrency: 2})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	var ids []string
	for _, r := range report.Results {
		ids = append(ids, r.ID)
	}
	want := []string{"repo-a", "repo-b", "repo-c"}
	if !reflect.DeepEqual(ids, want) {
		t.Errorf("Results order = %v, want %v", ids, want)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/core/...`

Expected: build failure — `Migrate`, `MigrateProviders`, `MigrateOptions` are undefined.

- [ ] **Step 3: Implement Migrate**

Create `internal/core/migrate.go`:

```go
package core

import (
	"context"
	"path/filepath"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/arijunior2020/tfrepo/internal/provider"
	"github.com/arijunior2020/tfrepo/internal/security"
)

// migrateRepoDirName is the directory created inside each task's workspace
// by security.Clone, and pushed from by security.Push.
const migrateRepoDirName = "repo.git"

// migrateTargetVisibility is the visibility assigned to repositories created
// on the target provider during migration.
const migrateTargetVisibility = provider.VisibilityPrivate

// MigrateProviders groups the source and target providers used by Migrate
// and Validate.
type MigrateProviders struct {
	Source provider.RepositoryProvider
	Target provider.RepositoryProvider
}

// MigrateOptions configures Migrate.
type MigrateOptions struct {
	// DryRun validates that each task's target namespace is reachable
	// without cloning, pushing, or creating a workspace.
	DryRun bool

	// Concurrency is the maximum number of tasks migrated at once. Values
	// below 1 are treated as 1.
	Concurrency int

	// Workspaces creates and cleans up the temporary directory used for
	// each task's mirror clone. Required unless DryRun is true.
	Workspaces *security.Manager
}

// Migrate executes migrationPlan, mirror-cloning each source repository and
// pushing it to the target using up to opts.Concurrency goroutines. Per-task
// failures are recorded as "failed" results rather than aborting the run.
func Migrate(ctx context.Context, migrationPlan MigrationPlan, providers MigrateProviders, opts MigrateOptions) (MigrationReport, error) {
	concurrency := opts.Concurrency
	if concurrency < 1 {
		concurrency = 1
	}

	results := make([]MigrationResult, len(migrationPlan.Tasks))

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)

	for i, task := range migrationPlan.Tasks {
		g.Go(func() error {
			if opts.DryRun {
				results[i] = runDryRun(gctx, task, providers.Target)
			} else {
				results[i] = runMigration(gctx, task, providers, opts.Workspaces)
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return MigrationReport{}, err
	}

	return MigrationReport{GeneratedAt: time.Now().UTC(), Results: results}, nil
}

// runDryRun validates that task's target namespace is reachable without
// migrating anything.
func runDryRun(ctx context.Context, task MigrationTask, target provider.RepositoryProvider) MigrationResult {
	startedAt := time.Now().UTC()

	if _, err := target.ListRepositories(ctx, task.Target.Namespace); err != nil {
		return MigrationResult{
			ID:         task.ID,
			Status:     "failed",
			StartedAt:  startedAt,
			FinishedAt: time.Now().UTC(),
			Error:      security.Redact(err.Error()),
		}
	}

	return MigrationResult{
		ID:         task.ID,
		Status:     "dry-run",
		StartedAt:  startedAt,
		FinishedAt: time.Now().UTC(),
	}
}

// runMigration mirror-clones task's source repository into a temporary
// workspace, ensures the target repository exists, and pushes the mirror to
// the target.
func runMigration(ctx context.Context, task MigrationTask, providers MigrateProviders, workspaces *security.Manager) MigrationResult {
	startedAt := time.Now().UTC()

	if err := migrateTask(ctx, task, providers, workspaces); err != nil {
		return MigrationResult{
			ID:         task.ID,
			Status:     "failed",
			StartedAt:  startedAt,
			FinishedAt: time.Now().UTC(),
			Error:      security.Redact(err.Error()),
		}
	}

	return MigrationResult{
		ID:         task.ID,
		Status:     "success",
		StartedAt:  startedAt,
		FinishedAt: time.Now().UTC(),
	}
}

func migrateTask(ctx context.Context, task MigrationTask, providers MigrateProviders, workspaces *security.Manager) error {
	workspace, err := workspaces.Create()
	if err != nil {
		return err
	}
	defer workspace.Cleanup()

	repoDir := filepath.Join(workspace.Path, migrateRepoDirName)

	sourceURL := providers.Source.GetAuthenticatedCloneURL(task.Source.Namespace, task.Source.Repo)
	if err := security.Clone(ctx, sourceURL, repoDir); err != nil {
		return err
	}

	if err := ensureTargetRepository(ctx, providers.Target, task); err != nil {
		return err
	}

	targetURL := providers.Target.GetAuthenticatedCloneURL(task.Target.Namespace, task.Target.Repo)
	return security.Push(ctx, repoDir, targetURL)
}

// ensureTargetRepository creates task's target repository if it doesn't
// already exist.
func ensureTargetRepository(ctx context.Context, target provider.RepositoryProvider, task MigrationTask) error {
	existing, err := target.ListRepositories(ctx, task.Target.Namespace)
	if err != nil {
		return err
	}

	for _, repo := range existing {
		if repo.Name == task.Target.Repo {
			return nil
		}
	}

	_, err = target.CreateRepository(ctx, task.Target.Namespace, provider.CreateRepositoryInput{
		Name:       task.Target.Repo,
		Visibility: migrateTargetVisibility,
	})
	return err
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/core/... -race`

Expected: `PASS` — all 6 migrate tests pass, plus all tests from Plans 3a/3b.

- [ ] **Step 5: Commit**

```bash
git add internal/core/migrate.go internal/core/migrate_test.go
git commit -m "feat(core): add Migrate to mirror-clone and push repositories"
```

---

## Task 2: internal/core/validate.go — Validate()

**Files:**
- Create: `internal/core/validate.go`
- Create: `internal/core/validate_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/core/validate_test.go`:

```go
package core

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/arijunior2020/tfrepo/internal/provider"
)

// fakeValidateProvider is a minimal provider.RepositoryProvider used to test
// Validate without making network calls.
type fakeValidateProvider struct {
	name string

	state    provider.RepositoryState
	stateErr error

	stateCalls int
}

func (f *fakeValidateProvider) Name() string { return f.name }

func (f *fakeValidateProvider) ListNamespaces(ctx context.Context) ([]provider.Namespace, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeValidateProvider) ListRepositories(ctx context.Context, namespace string) ([]provider.RepositorySummary, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeValidateProvider) GetRepositoryDetails(ctx context.Context, namespace, repo string) (provider.RepositoryDetails, error) {
	return provider.RepositoryDetails{}, errors.New("not implemented")
}

func (f *fakeValidateProvider) CreateRepository(ctx context.Context, namespace string, input provider.CreateRepositoryInput) (provider.RepositorySummary, error) {
	return provider.RepositorySummary{}, errors.New("not implemented")
}

func (f *fakeValidateProvider) GetAuthenticatedCloneURL(namespace, repo string) string {
	return ""
}

func (f *fakeValidateProvider) GetRepositoryState(ctx context.Context, namespace, repo string) (provider.RepositoryState, error) {
	f.stateCalls++
	return f.state, f.stateErr
}

var _ provider.RepositoryProvider = (*fakeValidateProvider)(nil)

func baseValidatePlan() MigrationPlan {
	return MigrationPlan{
		Source: ProviderRef{Provider: "github", Namespace: "my-org"},
		Target: ProviderRef{Provider: "gitlab", Namespace: "my-group"},
		Tasks: []MigrationTask{
			{
				ID:       "repo-a",
				Source:   TaskEndpoint{Namespace: "my-org", Repo: "repo-a"},
				Target:   TaskEndpoint{Namespace: "my-group", Repo: "repo-a"},
				Branches: []string{"main"},
				Tags:     []string{"v1.0.0"},
			},
		},
	}
}

func strPtr(s string) *string { return &s }

func TestValidateReportsOkWhenBranchesAndTagsMatch(t *testing.T) {
	source := &fakeValidateProvider{state: provider.RepositoryState{
		Branches: map[string]string{"main": "abc123"},
		Tags:     map[string]string{"v1.0.0": "def456"},
	}}
	target := &fakeValidateProvider{state: provider.RepositoryState{
		Branches: map[string]string{"main": "abc123"},
		Tags:     map[string]string{"v1.0.0": "def456"},
	}}

	report, err := Validate(context.Background(), baseValidatePlan(), MigrateProviders{Source: source, Target: target}, nil)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}

	want := []ValidationResult{{ID: "repo-a", Status: "ok", Divergences: []RefDivergence{}}}
	if !reflect.DeepEqual(report.Results, want) {
		t.Errorf("Results = %+v, want %+v", report.Results, want)
	}
}

func TestValidateReportsADivergedBranchSHA(t *testing.T) {
	source := &fakeValidateProvider{state: provider.RepositoryState{
		Branches: map[string]string{"main": "abc123"},
		Tags:     map[string]string{},
	}}
	target := &fakeValidateProvider{state: provider.RepositoryState{
		Branches: map[string]string{"main": "zzz999"},
		Tags:     map[string]string{},
	}}

	report, err := Validate(context.Background(), baseValidatePlan(), MigrateProviders{Source: source, Target: target}, nil)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}

	want := []ValidationResult{{
		ID:     "repo-a",
		Status: "diverged",
		Divergences: []RefDivergence{
			{Type: "branch", Name: "main", SourceSHA: strPtr("abc123"), TargetSHA: strPtr("zzz999")},
		},
	}}
	if !reflect.DeepEqual(report.Results, want) {
		t.Errorf("Results = %+v, want %+v", report.Results, want)
	}
}

func TestValidateReportsAMissingBranchOnTargetAsADivergence(t *testing.T) {
	source := &fakeValidateProvider{state: provider.RepositoryState{
		Branches: map[string]string{"main": "abc123", "develop": "ddd"},
		Tags:     map[string]string{},
	}}
	target := &fakeValidateProvider{state: provider.RepositoryState{
		Branches: map[string]string{"main": "abc123"},
		Tags:     map[string]string{},
	}}

	report, err := Validate(context.Background(), baseValidatePlan(), MigrateProviders{Source: source, Target: target}, nil)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}

	want := []ValidationResult{{
		ID:     "repo-a",
		Status: "diverged",
		Divergences: []RefDivergence{
			{Type: "branch", Name: "develop", SourceSHA: strPtr("ddd"), TargetSHA: nil},
		},
	}}
	if !reflect.DeepEqual(report.Results, want) {
		t.Errorf("Results = %+v, want %+v", report.Results, want)
	}
}

func TestValidateReportsADivergedTagSHA(t *testing.T) {
	source := &fakeValidateProvider{state: provider.RepositoryState{
		Branches: map[string]string{},
		Tags:     map[string]string{"v1.0.0": "def456"},
	}}
	target := &fakeValidateProvider{state: provider.RepositoryState{
		Branches: map[string]string{},
		Tags:     map[string]string{},
	}}

	report, err := Validate(context.Background(), baseValidatePlan(), MigrateProviders{Source: source, Target: target}, nil)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}

	want := []ValidationResult{{
		ID:     "repo-a",
		Status: "diverged",
		Divergences: []RefDivergence{
			{Type: "tag", Name: "v1.0.0", SourceSHA: strPtr("def456"), TargetSHA: nil},
		},
	}}
	if !reflect.DeepEqual(report.Results, want) {
		t.Errorf("Results = %+v, want %+v", report.Results, want)
	}
}

func TestValidateSkipsTasksThatFailedMigration(t *testing.T) {
	source := &fakeValidateProvider{}
	target := &fakeValidateProvider{}

	migrationReport := &MigrationReport{
		Results: []MigrationResult{
			{ID: "repo-a", Status: "failed", Error: "boom"},
		},
	}

	report, err := Validate(context.Background(), baseValidatePlan(), MigrateProviders{Source: source, Target: target}, migrationReport)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}

	want := []ValidationResult{{ID: "repo-a", Status: "skipped", Divergences: []RefDivergence{}, Reason: "migration failed"}}
	if !reflect.DeepEqual(report.Results, want) {
		t.Errorf("Results = %+v, want %+v", report.Results, want)
	}

	if source.stateCalls != 0 {
		t.Errorf("source.GetRepositoryState calls = %d, want 0", source.stateCalls)
	}
	if target.stateCalls != 0 {
		t.Errorf("target.GetRepositoryState calls = %d, want 0", target.stateCalls)
	}
}

func TestValidatePropagatesProviderError(t *testing.T) {
	source := &fakeValidateProvider{stateErr: errors.New("boom")}
	target := &fakeValidateProvider{}

	_, err := Validate(context.Background(), baseValidatePlan(), MigrateProviders{Source: source, Target: target}, nil)
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/core/...`

Expected: build failure — `Validate` is undefined.

- [ ] **Step 3: Implement Validate**

Create `internal/core/validate.go`:

```go
package core

import (
	"context"
	"sort"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/arijunior2020/tfrepo/internal/provider"
)

// Validate compares GetRepositoryState between the source and target for
// each task in migrationPlan. If report is non-nil, tasks whose result has
// status "failed" are reported as "skipped" without calling either
// provider.
func Validate(ctx context.Context, migrationPlan MigrationPlan, providers MigrateProviders, report *MigrationReport) (ValidationReport, error) {
	failedIDs := make(map[string]struct{})
	if report != nil {
		for _, result := range report.Results {
			if result.Status == "failed" {
				failedIDs[result.ID] = struct{}{}
			}
		}
	}

	results := make([]ValidationResult, 0, len(migrationPlan.Tasks))

	for _, task := range migrationPlan.Tasks {
		if _, failed := failedIDs[task.ID]; failed {
			results = append(results, ValidationResult{
				ID:          task.ID,
				Status:      "skipped",
				Divergences: []RefDivergence{},
				Reason:      "migration failed",
			})
			continue
		}

		var sourceState, targetState provider.RepositoryState

		g, gctx := errgroup.WithContext(ctx)
		g.Go(func() error {
			var err error
			sourceState, err = providers.Source.GetRepositoryState(gctx, task.Source.Namespace, task.Source.Repo)
			return err
		})
		g.Go(func() error {
			var err error
			targetState, err = providers.Target.GetRepositoryState(gctx, task.Target.Namespace, task.Target.Repo)
			return err
		})

		if err := g.Wait(); err != nil {
			return ValidationReport{}, err
		}

		divergences := append(
			compareRefs("branch", sourceState.Branches, targetState.Branches),
			compareRefs("tag", sourceState.Tags, targetState.Tags)...,
		)

		status := "ok"
		if len(divergences) > 0 {
			status = "diverged"
		}

		results = append(results, ValidationResult{ID: task.ID, Status: status, Divergences: divergences})
	}

	return ValidationReport{GeneratedAt: time.Now().UTC(), Results: results}, nil
}

// compareRefs returns one RefDivergence for every ref name in source or
// target whose commit SHA differs, including names present in only one of
// the two maps. Names are sorted for deterministic output.
func compareRefs(refType string, source, target map[string]string) []RefDivergence {
	names := make(map[string]struct{}, len(source)+len(target))
	for name := range source {
		names[name] = struct{}{}
	}
	for name := range target {
		names[name] = struct{}{}
	}

	sorted := make([]string, 0, len(names))
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)

	divergences := []RefDivergence{}
	for _, name := range sorted {
		sourceSHA, sourceOK := source[name]
		targetSHA, targetOK := target[name]

		if sourceOK && targetOK && sourceSHA == targetSHA {
			continue
		}

		var sourcePtr, targetPtr *string
		if sourceOK {
			sourcePtr = &sourceSHA
		}
		if targetOK {
			targetPtr = &targetSHA
		}

		divergences = append(divergences, RefDivergence{Type: refType, Name: name, SourceSHA: sourcePtr, TargetSHA: targetPtr})
	}

	return divergences
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/core/... -race`

Expected: `PASS` — all 6 validate tests pass, plus all tests from Task 1 and Plans 3a/3b.

- [ ] **Step 5: Commit**

```bash
git add internal/core/validate.go internal/core/validate_test.go
git commit -m "feat(core): add Validate to compare branch/tag SHAs between source and target"
```

---

## Final Verification

After Task 2, run the full test suite to confirm nothing else broke:

```bash
go build ./...
go vet ./...
go test ./... -race
gofmt -l .
```

Expected: all packages build, `go vet` is clean, `gofmt -l .` prints nothing, and all tests pass — including the pre-existing `internal/config`, `internal/provider`, and `internal/security` suites, and `internal/core`'s artifact/glob/scan/plan tests from Plans 3a/3b plus the new `Migrate`/`Validate` tests from this plan (12 new test functions total: 6 from Task 1, 6 from Task 2).
