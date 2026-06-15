# tfrepo Plan 3a: internal/config + internal/core/artifacts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port `packages/core/src/config.ts` and `packages/core/src/artifacts.ts` from the Node reference implementation to Go, producing `internal/config` (load/validate `transferepo.config.yaml`) and `internal/core` (typed structs + JSON I/O for `inventory.json`, `migration-plan.json`, `migration-report.json`, `validation-report.json`).

**Architecture:** Two new, independent packages. `internal/config` reads and validates `transferepo.config.yaml` with `gopkg.in/yaml.v3`, replicating `TransferepoConfigSchema` from the Node source field-by-field (including its default values and error-message format). `internal/core` defines the four JSON artifact types consumed/produced by the `scan`/`plan`/`migrate`/`validate` commands (built in Plans 3b/3c), plus generic `WriteJSON`/`ReadJSON` helpers used to persist them to the working directory. `internal/core/artifacts.go` imports `internal/provider` for `Visibility`/`NamespaceKind` types, ensuring a single source of truth for those enums.

**Tech Stack:** Go 1.24, `gopkg.in/yaml.v3` (new dependency), `encoding/json` (stdlib), `testing` (stdlib).

---

## Context for implementers

- Reference Node source (read-only, for field-by-field parity):
  - `/media/arimateia-junior/Dados4/projetos/app-transferepo/packages/core/src/config.ts`
  - `/media/arimateia-junior/Dados4/projetos/app-transferepo/packages/core/src/config.test.ts`
  - `/media/arimateia-junior/Dados4/projetos/app-transferepo/packages/core/src/artifacts.ts`
  - `/media/arimateia-junior/Dados4/projetos/app-transferepo/packages/core/src/artifacts.test.ts`
- Existing Go types this plan builds on: `/media/arimateia-junior/Dados4/projetos/tfrepo/internal/provider/types.go` (defines `provider.Visibility`, `provider.NamespaceKind` and their constants — do not redefine these, import them).
- A real example config file (for reference, not to be copied into the repo): `/media/arimateia-junior/Dados4/projetos/migration-teste/transferepo.config.yaml`.
- Module path: `github.com/arijunior2020/tfrepo`. All commands below run from the repo root: `/media/arimateia-junior/Dados4/projetos/tfrepo`.

### Known, deliberate divergences from the design spec's Go sketch (resolved during planning)

The design spec (`docs/superpowers/specs/2026-06-13-tfrepo-go-port-design.md`, lines ~244-313) sketches `internal/core/artifacts.go`, but three details there don't match the **authoritative** Node zod schemas in `artifacts.ts`. This plan follows `artifacts.ts` exactly:

1. **`MigrationResult.StartedAt` / `FinishedAt`**: Node's `MigrationResultSchema` declares `startedAt: z.string()` and `finishedAt: z.string()` — both **required**, not optional. This plan uses `time.Time` (not `*time.Time`, no `omitempty`), which always serializes to an RFC3339 string.
2. **`ValidationResult`**: Node's `ValidationResultSchema` has an additional optional field `reason: z.string().optional()` (used when `status: "skipped"`). This plan adds `Reason string \`json:"reason,omitempty"\`` to `ValidationResult`.
3. **`RefDivergence.SourceSHA` / `TargetSHA`**: Node's `RefDivergenceSchema` declares `sourceSha: z.string().nullable()` and `targetSha: z.string().nullable()` — the keys are always present but the value may be `null`. This plan uses `*string` **without** `omitempty`, so a `nil` pointer serializes as JSON `null` and a non-nil pointer serializes as the string value.
4. **`InventoryNamespace.Repositories`**: Node's `NamespaceInventorySchema.repositories` is `RepositoryInventorySchema[]`, which has **no `namespace` field** (unlike `provider.RepositoryDetails`, which embeds `provider.RepositorySummary.Namespace`). This plan defines a dedicated `core.RepositoryInventory` type with exactly the fields from `RepositoryInventorySchema` (`name`, `defaultBranch`, `visibility`, `sizeKb`, `branches`, `tags`) — no `namespace` key. Plan 3b's `scan` implementation will be responsible for converting `provider.RepositoryDetails` → `core.RepositoryInventory` (dropping `Namespace`).

---

## File Structure

- `internal/config/config.go` — **create**. `Config`, `ProviderConfig`, `Filters`, `ConfigError`, `Load`, `Validate`, default-application logic.
- `internal/config/config_test.go` — **create**. Tests mirroring `config.test.ts`.
- `internal/core/artifacts.go` — **create**. All four JSON artifact type groups (`Inventory`, `MigrationPlan`, `MigrationReport`, `ValidationReport`) plus `WriteJSON`/`ReadJSON`.
- `internal/core/artifacts_test.go` — **create**. Tests mirroring `artifacts.test.ts`, plus I/O round-trip tests.
- `go.mod`, `go.sum` — **modify**. Add `gopkg.in/yaml.v3 v3.0.1`.

---

### Task 1: internal/config — Load and Validate

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add the yaml.v3 dependency**

Run:

```bash
go get gopkg.in/yaml.v3@v3.0.1
go mod tidy
```

Expected: `go.mod` gains `gopkg.in/yaml.v3 v3.0.1` in its `require` block, and `go.sum` gains matching entries.

- [ ] **Step 2: Write the failing test file**

Create `internal/config/config_test.go`:

```go
package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, dir, contents string) string {
	t.Helper()
	path := filepath.Join(dir, "transferepo.config.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func requireConfigError(t *testing.T, err error) *ConfigError {
	t.Helper()
	var configErr *ConfigError
	if !errors.As(err, &configErr) {
		t.Fatalf("error type = %T, want *ConfigError", err)
	}
	return configErr
}

func TestLoadFullConfig(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, `
source:
  provider: github
  namespace: my-org

target:
  provider: gitlab
  baseUrl: https://gitlab.example.com
  namespace: my-group

filters:
  include: ["repo-*"]
  exclude: ["repo-legacy"]

mapping:
  old-repo: new-repo
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	want := &Config{
		Source: ProviderConfig{Provider: "github", Namespace: "my-org"},
		Target: ProviderConfig{Provider: "gitlab", BaseURL: "https://gitlab.example.com", Namespace: "my-group"},
		Filters: Filters{
			Include: []string{"repo-*"},
			Exclude: []string{"repo-legacy"},
		},
		Mapping: map[string]string{"old-repo": "new-repo"},
	}

	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("Load() = %+v, want %+v", cfg, want)
	}
}

func TestLoadAppliesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, `
source:
  provider: github
  namespace: my-org

target:
  provider: gitlab
  namespace: my-group
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	wantFilters := Filters{Include: []string{"*"}, Exclude: []string{}}
	if !reflect.DeepEqual(cfg.Filters, wantFilters) {
		t.Errorf("Filters = %+v, want %+v", cfg.Filters, wantFilters)
	}

	if len(cfg.Mapping) != 0 {
		t.Errorf("Mapping = %+v, want empty map", cfg.Mapping)
	}
}

func TestLoadInvalidProvider(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, `
source:
  provider: bitbucket
  namespace: my-org

target:
  provider: gitlab
  namespace: my-group
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}

	configErr := requireConfigError(t, err)
	if !strings.Contains(configErr.Message, "source.provider") {
		t.Errorf("Load() error = %q, want to contain %q", configErr.Message, "source.provider")
	}
}

func TestLoadMissingNamespace(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, `
source:
  provider: github

target:
  provider: gitlab
  namespace: my-group
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}

	configErr := requireConfigError(t, err)
	if !strings.Contains(configErr.Message, "source.namespace") {
		t.Errorf("Load() error = %q, want to contain %q", configErr.Message, "source.namespace")
	}
}

func TestLoadInvalidBaseURL(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, `
source:
  provider: github
  namespace: my-org

target:
  provider: gitlab
  baseUrl: "not a url"
  namespace: my-group
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}

	configErr := requireConfigError(t, err)
	if !strings.Contains(configErr.Message, "target.baseUrl") {
		t.Errorf("Load() error = %q, want to contain %q", configErr.Message, "target.baseUrl")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, "source: [unterminated")

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}

	requireConfigError(t, err)
}

func TestLoadMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.yaml")

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}

	requireConfigError(t, err)
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/config/...`

Expected: build failure — `config.go` does not exist yet, so `Config`, `ProviderConfig`, `Filters`, `ConfigError`, `Load` are all undefined.

- [ ] **Step 4: Write the implementation**

Create `internal/config/config.go`:

```go
package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// ProviderConfig describes one side (source or target) of a migration.
type ProviderConfig struct {
	Provider  string `yaml:"provider"`           // "github" | "gitlab"
	BaseURL   string `yaml:"baseUrl,omitempty"`  // GitHub Enterprise / GitLab self-managed
	Namespace string `yaml:"namespace"`
}

// Filters controls which repositories are included when building a
// migration plan.
type Filters struct {
	Include []string `yaml:"include"` // default: []string{"*"}
	Exclude []string `yaml:"exclude"` // default: []string{}
}

// Config is the parsed and validated contents of transferepo.config.yaml.
type Config struct {
	Source  ProviderConfig    `yaml:"source"`
	Target  ProviderConfig    `yaml:"target"`
	Filters Filters           `yaml:"filters"`
	Mapping map[string]string `yaml:"mapping"`
}

// ConfigError is returned by Load when the configuration file is missing,
// contains invalid YAML, or fails validation. It mirrors the ConfigError
// class in packages/core/src/config.ts.
type ConfigError struct {
	Message string
}

func (e *ConfigError) Error() string {
	return e.Message
}

// Load reads, parses and validates the configuration file at path,
// replicating the behavior of loadConfig in packages/core/src/config.ts.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, &ConfigError{Message: fmt.Sprintf("Could not read configuration file at %s: %s", path, err)}
	}

	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, &ConfigError{Message: fmt.Sprintf("Invalid YAML in %s: %s", path, err)}
	}

	cfg.applyDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, &ConfigError{Message: fmt.Sprintf("Invalid configuration in %s: %s", path, err)}
	}

	return &cfg, nil
}

// applyDefaults fills in Filters and Mapping when absent from the YAML,
// matching FiltersSchema.default(...) and the mapping record's
// .default({}) in packages/core/src/config.ts. A nil slice/map means the
// corresponding YAML key was entirely absent.
func (c *Config) applyDefaults() {
	if c.Filters.Include == nil {
		c.Filters.Include = []string{"*"}
	}
	if c.Filters.Exclude == nil {
		c.Filters.Exclude = []string{}
	}
	if c.Mapping == nil {
		c.Mapping = map[string]string{}
	}
}

// Validate checks the configuration against the rules in
// TransferepoConfigSchema: source and target must each have a known
// provider and a non-empty namespace, and baseUrl, if set, must be a valid
// absolute URL. Errors are formatted as "path.to.field: message" joined by
// "; ", matching the issue formatting in loadConfig.
func (c *Config) Validate() error {
	var issues []string
	issues = append(issues, validateProviderConfig("source", c.Source)...)
	issues = append(issues, validateProviderConfig("target", c.Target)...)

	if len(issues) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(issues, "; "))
}

func validateProviderConfig(field string, pc ProviderConfig) []string {
	var issues []string

	if pc.Provider != "github" && pc.Provider != "gitlab" {
		issues = append(issues, fmt.Sprintf(`%s.provider: must be "github" or "gitlab"`, field))
	}

	if pc.Namespace == "" {
		issues = append(issues, fmt.Sprintf("%s.namespace: is required", field))
	}

	if pc.BaseURL != "" {
		u, err := url.Parse(pc.BaseURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			issues = append(issues, fmt.Sprintf("%s.baseUrl: must be a valid URL", field))
		}
	}

	return issues
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/config/...`

Expected: `PASS`, all 7 tests pass (`TestLoadFullConfig`, `TestLoadAppliesDefaults`, `TestLoadInvalidProvider`, `TestLoadMissingNamespace`, `TestLoadInvalidBaseURL`, `TestLoadInvalidYAML`, `TestLoadMissingFile`).

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): load and validate transferepo.config.yaml"
```

---

### Task 2: internal/core/artifacts.go — Inventory types

**Files:**
- Create: `internal/core/artifacts.go`
- Test: `internal/core/artifacts_test.go`

- [ ] **Step 1: Write the failing test file**

Create `internal/core/artifacts_test.go`:

```go
package core

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/arijunior2020/tfrepo/internal/provider"
)

// toMap unmarshals JSON bytes into a generic map for structural comparison,
// so tests don't depend on key ordering.
func toMap(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v\ndata: %s", err, data)
	}
	return m
}

func TestInventoryMarshal(t *testing.T) {
	generatedAt, err := time.Parse(time.RFC3339, "2026-06-11T12:00:00Z")
	if err != nil {
		t.Fatalf("time.Parse: %v", err)
	}

	inv := Inventory{
		GeneratedAt: generatedAt,
		Source:      ProviderRef{Provider: "github", Namespace: "my-org"},
		Namespaces: []InventoryNamespace{
			{
				Slug: "my-org",
				Name: "My Org",
				Kind: provider.NamespaceOrganization,
				Repositories: []RepositoryInventory{
					{
						Name:          "repo-a",
						DefaultBranch: "main",
						Visibility:    provider.VisibilityPrivate,
						SizeKB:        12345,
						Branches:      []string{"main", "develop"},
						Tags:          []string{"v1.0.0", "v1.1.0"},
					},
				},
			},
		},
	}

	got, err := json.Marshal(inv)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	want := []byte(`{
		"generatedAt": "2026-06-11T12:00:00Z",
		"source": {"provider": "github", "namespace": "my-org"},
		"namespaces": [
			{
				"slug": "my-org",
				"name": "My Org",
				"kind": "organization",
				"repositories": [
					{
						"name": "repo-a",
						"defaultBranch": "main",
						"visibility": "private",
						"sizeKb": 12345,
						"branches": ["main", "develop"],
						"tags": ["v1.0.0", "v1.1.0"]
					}
				]
			}
		]
	}`)

	if !reflect.DeepEqual(toMap(t, got), toMap(t, want)) {
		t.Errorf("Marshal(inv) = %s, want %s", got, want)
	}

	var roundTrip Inventory
	if err := json.Unmarshal(got, &roundTrip); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(roundTrip, inv) {
		t.Errorf("round trip = %+v, want %+v", roundTrip, inv)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/core/...`

Expected: build failure — `core` package does not exist yet, so `Inventory`, `ProviderRef`, `InventoryNamespace`, `RepositoryInventory` are all undefined.

- [ ] **Step 3: Write the implementation**

Create `internal/core/artifacts.go`:

```go
package core

import (
	"time"

	"github.com/arijunior2020/tfrepo/internal/provider"
)

// ProviderRef identifies a provider and namespace within an artifact's
// source/target metadata.
type ProviderRef struct {
	Provider  string `json:"provider"` // "github" | "gitlab"
	Namespace string `json:"namespace"`
}

// RepositoryInventory is the snapshot of a single repository recorded in
// inventory.json. Unlike provider.RepositoryDetails, it has no Namespace
// field — the namespace is implied by the enclosing InventoryNamespace.
type RepositoryInventory struct {
	Name          string              `json:"name"`
	DefaultBranch string              `json:"defaultBranch"`
	Visibility    provider.Visibility `json:"visibility"`
	SizeKB        int64               `json:"sizeKb"`
	Branches      []string            `json:"branches"`
	Tags          []string            `json:"tags"`
}

// InventoryNamespace groups the repositories scanned for one namespace.
type InventoryNamespace struct {
	Slug         string                 `json:"slug"`
	Name         string                 `json:"name"`
	Kind         provider.NamespaceKind `json:"kind"`
	Repositories []RepositoryInventory  `json:"repositories"`
}

// Inventory is the contents of inventory.json, produced by `tfrepo scan`.
type Inventory struct {
	GeneratedAt time.Time            `json:"generatedAt"`
	Source      ProviderRef          `json:"source"`
	Namespaces  []InventoryNamespace `json:"namespaces"`
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/core/...`

Expected: `PASS` — `TestInventoryMarshal` passes.

- [ ] **Step 5: Commit**

```bash
git add internal/core/artifacts.go internal/core/artifacts_test.go
git commit -m "feat(core): add Inventory artifact types"
```

---

### Task 3: internal/core/artifacts.go — MigrationPlan types

**Files:**
- Modify: `internal/core/artifacts.go`
- Modify: `internal/core/artifacts_test.go`

- [ ] **Step 1: Append the failing test**

Add to `internal/core/artifacts_test.go` (after `TestInventoryMarshal`):

```go

func TestMigrationPlanMarshal(t *testing.T) {
	generatedAt, err := time.Parse(time.RFC3339, "2026-06-11T12:05:00Z")
	if err != nil {
		t.Fatalf("time.Parse: %v", err)
	}

	plan := MigrationPlan{
		GeneratedAt: generatedAt,
		Source:      ProviderRef{Provider: "github", Namespace: "my-org"},
		Target:      ProviderRef{Provider: "gitlab", Namespace: "my-group"},
		Tasks: []MigrationTask{
			{
				ID:       "repo-a",
				Source:   TaskEndpoint{Namespace: "my-org", Repo: "repo-a"},
				Target:   TaskEndpoint{Namespace: "my-group", Repo: "repo-a"},
				Branches: []string{"main", "develop"},
				Tags:     []string{"v1.0.0", "v1.1.0"},
			},
		},
	}

	got, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	want := []byte(`{
		"generatedAt": "2026-06-11T12:05:00Z",
		"source": {"provider": "github", "namespace": "my-org"},
		"target": {"provider": "gitlab", "namespace": "my-group"},
		"tasks": [
			{
				"id": "repo-a",
				"source": {"namespace": "my-org", "repo": "repo-a"},
				"target": {"namespace": "my-group", "repo": "repo-a"},
				"branches": ["main", "develop"],
				"tags": ["v1.0.0", "v1.1.0"]
			}
		]
	}`)

	if !reflect.DeepEqual(toMap(t, got), toMap(t, want)) {
		t.Errorf("Marshal(plan) = %s, want %s", got, want)
	}

	var roundTrip MigrationPlan
	if err := json.Unmarshal(got, &roundTrip); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(roundTrip, plan) {
		t.Errorf("round trip = %+v, want %+v", roundTrip, plan)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/core/...`

Expected: build failure — `MigrationPlan`, `MigrationTask`, `TaskEndpoint` are undefined.

- [ ] **Step 3: Append the implementation**

Add to `internal/core/artifacts.go` (after the `Inventory` struct):

```go

// TaskEndpoint identifies a repository within a source or target namespace.
type TaskEndpoint struct {
	Namespace string `json:"namespace"`
	Repo      string `json:"repo"`
}

// MigrationTask describes the migration of a single repository from source
// to target.
type MigrationTask struct {
	ID       string       `json:"id"`
	Source   TaskEndpoint `json:"source"`
	Target   TaskEndpoint `json:"target"`
	Branches []string     `json:"branches"`
	Tags     []string     `json:"tags"`
}

// MigrationPlan is the contents of migration-plan.json, produced by
// `tfrepo plan`.
type MigrationPlan struct {
	GeneratedAt time.Time       `json:"generatedAt"`
	Source      ProviderRef     `json:"source"`
	Target      ProviderRef     `json:"target"`
	Tasks       []MigrationTask `json:"tasks"`
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/core/...`

Expected: `PASS` — `TestInventoryMarshal` and `TestMigrationPlanMarshal` both pass.

- [ ] **Step 5: Commit**

```bash
git add internal/core/artifacts.go internal/core/artifacts_test.go
git commit -m "feat(core): add MigrationPlan artifact types"
```

---

### Task 4: internal/core/artifacts.go — MigrationReport types

**Files:**
- Modify: `internal/core/artifacts.go`
- Modify: `internal/core/artifacts_test.go`

- [ ] **Step 1: Append the failing test**

Add to `internal/core/artifacts_test.go` (after `TestMigrationPlanMarshal`):

```go

func TestMigrationReportMarshal(t *testing.T) {
	generatedAt, err := time.Parse(time.RFC3339, "2026-06-11T12:30:00Z")
	if err != nil {
		t.Fatalf("time.Parse: %v", err)
	}
	startedA, err := time.Parse(time.RFC3339, "2026-06-11T12:10:00Z")
	if err != nil {
		t.Fatalf("time.Parse: %v", err)
	}
	finishedA, err := time.Parse(time.RFC3339, "2026-06-11T12:12:00Z")
	if err != nil {
		t.Fatalf("time.Parse: %v", err)
	}
	startedB, err := time.Parse(time.RFC3339, "2026-06-11T12:12:00Z")
	if err != nil {
		t.Fatalf("time.Parse: %v", err)
	}
	finishedB, err := time.Parse(time.RFC3339, "2026-06-11T12:13:00Z")
	if err != nil {
		t.Fatalf("time.Parse: %v", err)
	}

	report := MigrationReport{
		GeneratedAt: generatedAt,
		Results: []MigrationResult{
			{ID: "repo-a", Status: "success", StartedAt: startedA, FinishedAt: finishedA},
			{ID: "repo-b", Status: "failed", StartedAt: startedB, FinishedAt: finishedB, Error: "push rejected"},
		},
	}

	got, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	want := []byte(`{
		"generatedAt": "2026-06-11T12:30:00Z",
		"results": [
			{
				"id": "repo-a",
				"status": "success",
				"startedAt": "2026-06-11T12:10:00Z",
				"finishedAt": "2026-06-11T12:12:00Z"
			},
			{
				"id": "repo-b",
				"status": "failed",
				"startedAt": "2026-06-11T12:12:00Z",
				"finishedAt": "2026-06-11T12:13:00Z",
				"error": "push rejected"
			}
		]
	}`)

	if !reflect.DeepEqual(toMap(t, got), toMap(t, want)) {
		t.Errorf("Marshal(report) = %s, want %s", got, want)
	}

	var roundTrip MigrationReport
	if err := json.Unmarshal(got, &roundTrip); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(roundTrip, report) {
		t.Errorf("round trip = %+v, want %+v", roundTrip, report)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/core/...`

Expected: build failure — `MigrationReport`, `MigrationResult` are undefined.

- [ ] **Step 3: Append the implementation**

Add to `internal/core/artifacts.go` (after the `MigrationPlan` struct):

```go

// MigrationResult is the outcome of migrating a single repository.
type MigrationResult struct {
	ID         string    `json:"id"`
	Status     string    `json:"status"` // "success" | "failed" | "dry-run"
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
	Error      string    `json:"error,omitempty"` // always passed through security.Redact() before being set
}

// MigrationReport is the contents of migration-report.json, produced by
// `tfrepo migrate`.
type MigrationReport struct {
	GeneratedAt time.Time          `json:"generatedAt"`
	Results     []MigrationResult `json:"results"`
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/core/...`

Expected: `PASS` — all three tests so far pass.

- [ ] **Step 5: Commit**

```bash
git add internal/core/artifacts.go internal/core/artifacts_test.go
git commit -m "feat(core): add MigrationReport artifact types"
```

---

### Task 5: internal/core/artifacts.go — ValidationReport types

**Files:**
- Modify: `internal/core/artifacts.go`
- Modify: `internal/core/artifacts_test.go`

- [ ] **Step 1: Append the failing test**

Add to `internal/core/artifacts_test.go` (after `TestMigrationReportMarshal`):

```go

func TestValidationReportMarshal(t *testing.T) {
	generatedAt, err := time.Parse(time.RFC3339, "2026-06-11T13:00:00Z")
	if err != nil {
		t.Fatalf("time.Parse: %v", err)
	}

	sourceSHA := "abc123"
	targetSHA := "def456"
	tagSourceSHA := "aaa"

	report := ValidationReport{
		GeneratedAt: generatedAt,
		Results: []ValidationResult{
			{ID: "repo-a", Status: "ok", Divergences: []RefDivergence{}},
			{
				ID:     "repo-b",
				Status: "diverged",
				Divergences: []RefDivergence{
					{Type: "branch", Name: "main", SourceSHA: &sourceSHA, TargetSHA: &targetSHA},
					{Type: "tag", Name: "v1.0.0", SourceSHA: &tagSourceSHA, TargetSHA: nil},
				},
			},
			{ID: "repo-c", Status: "skipped", Divergences: []RefDivergence{}, Reason: "migration failed"},
		},
	}

	got, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	want := []byte(`{
		"generatedAt": "2026-06-11T13:00:00Z",
		"results": [
			{"id": "repo-a", "status": "ok", "divergences": []},
			{
				"id": "repo-b",
				"status": "diverged",
				"divergences": [
					{"type": "branch", "name": "main", "sourceSha": "abc123", "targetSha": "def456"},
					{"type": "tag", "name": "v1.0.0", "sourceSha": "aaa", "targetSha": null}
				]
			},
			{"id": "repo-c", "status": "skipped", "divergences": [], "reason": "migration failed"}
		]
	}`)

	if !reflect.DeepEqual(toMap(t, got), toMap(t, want)) {
		t.Errorf("Marshal(report) = %s, want %s", got, want)
	}

	var roundTrip ValidationReport
	if err := json.Unmarshal(got, &roundTrip); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(roundTrip, report) {
		t.Errorf("round trip = %+v, want %+v", roundTrip, report)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/core/...`

Expected: build failure — `ValidationReport`, `ValidationResult`, `RefDivergence` are undefined.

- [ ] **Step 3: Append the implementation**

Add to `internal/core/artifacts.go` (after the `MigrationReport` struct):

```go

// RefDivergence describes a single branch or tag whose commit SHA differs
// between source and target. SourceSHA/TargetSHA are pointers (not
// omitempty) so a missing ref serializes as JSON null, matching
// RefDivergenceSchema's z.string().nullable().
type RefDivergence struct {
	Type      string  `json:"type"` // "branch" | "tag"
	Name      string  `json:"name"`
	SourceSHA *string `json:"sourceSha"`
	TargetSHA *string `json:"targetSha"`
}

// ValidationResult is the outcome of validating a single repository.
type ValidationResult struct {
	ID          string          `json:"id"`
	Status      string          `json:"status"` // "ok" | "diverged" | "skipped"
	Divergences []RefDivergence `json:"divergences"`
	Reason      string          `json:"reason,omitempty"` // set when Status == "skipped"
}

// ValidationReport is the contents of validation-report.json, produced by
// `tfrepo validate`.
type ValidationReport struct {
	GeneratedAt time.Time          `json:"generatedAt"`
	Results     []ValidationResult `json:"results"`
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/core/...`

Expected: `PASS` — all four tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/core/artifacts.go internal/core/artifacts_test.go
git commit -m "feat(core): add ValidationReport artifact types"
```

---

### Task 6: internal/core — WriteJSON/ReadJSON helpers

**Files:**
- Modify: `internal/core/artifacts.go`
- Modify: `internal/core/artifacts_test.go`

- [ ] **Step 1: Append the failing tests**

Add to `internal/core/artifacts_test.go` (after `TestValidationReportMarshal`). This adds three new imports (`os`, `path/filepath`, `strings`) to the existing import block — update the import block at the top of the file to:

```go
import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/arijunior2020/tfrepo/internal/provider"
)
```

Then append:

```go

func TestWriteAndReadJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inventory.json")

	generatedAt, err := time.Parse(time.RFC3339, "2026-06-11T12:00:00Z")
	if err != nil {
		t.Fatalf("time.Parse: %v", err)
	}

	inv := Inventory{
		GeneratedAt: generatedAt,
		Source:      ProviderRef{Provider: "github", Namespace: "my-org"},
		Namespaces:  []InventoryNamespace{},
	}

	if err := WriteJSON(path, inv); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), `"generatedAt": "2026-06-11T12:00:00Z"`) {
		t.Errorf("file contents = %s, want to contain generatedAt field", data)
	}

	var got Inventory
	if err := ReadJSON(path, &got); err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
	if !reflect.DeepEqual(got, inv) {
		t.Errorf("ReadJSON() = %+v, want %+v", got, inv)
	}
}

func TestReadJSONMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.json")

	var inv Inventory
	if err := ReadJSON(path, &inv); err == nil {
		t.Error("ReadJSON() error = nil, want error")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/core/...`

Expected: build failure — `WriteJSON`, `ReadJSON` are undefined.

- [ ] **Step 3: Append the implementation**

First, update the import block at the top of `internal/core/artifacts.go` to:

```go
import (
	"encoding/json"
	"os"
	"time"

	"github.com/arijunior2020/tfrepo/internal/provider"
)
```

Then add to `internal/core/artifacts.go` (after the `ValidationReport` struct):

```go

// WriteJSON marshals v as indented JSON and writes it to path, creating or
// truncating the file with mode 0644.
func WriteJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

// ReadJSON reads the JSON file at path and unmarshals it into v.
func ReadJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/core/...`

Expected: `PASS` — all six tests pass (`TestInventoryMarshal`, `TestMigrationPlanMarshal`, `TestMigrationReportMarshal`, `TestValidationReportMarshal`, `TestWriteAndReadJSON`, `TestReadJSONMissingFile`).

- [ ] **Step 5: Commit**

```bash
git add internal/core/artifacts.go internal/core/artifacts_test.go
git commit -m "feat(core): add WriteJSON/ReadJSON artifact I/O helpers"
```

---

## Final Verification

After Task 6, run the full test suite to confirm nothing else broke:

```bash
go build ./...
go vet ./...
go test ./...
```

Expected: all packages build, `go vet` is clean, and all tests pass (including the pre-existing `internal/provider` and `internal/security` suites from Plans 1 and 2).
