# tfrepo Plano 4b — comandos scan e plan Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implementar os comandos `tfrepo scan [--concurrency N]` e `tfrepo plan`, que geram `inventory.json` (via `core.Scan`) e `migration-plan.json` (via `core.Plan`), imprimindo um resumo em português na saída padrão.

**Architecture:** `internal/cli/scan.go` adiciona `runScan`/`scanAndWrite`: carregam a configuração (`config.Load`), constroem o provider de origem (`cli.NewProvider`), chamam `core.Scan` e gravam `inventory.json` via `core.WriteJSON`. `internal/cli/plan.go` adiciona `runPlan`: lê `inventory.json` (via `core.ReadJSON`), carrega a configuração, chama `core.Plan` e grava `migration-plan.json`. Um helper compartilhado `internal/cli/format.go` (`countLabel`) formata contagens com singular/plural em português, reaproveitado pelos dois comandos. `internal/cli/root.go` ganha `newScanCommand`/`newPlanCommand`, registrados em `newRootCommand`, seguindo o mesmo padrão `*int exitCode` do Plano 4a.

**Tech Stack:** Go 1.24, Cobra (`github.com/spf13/cobra`), pacotes internos já existentes `internal/core` (`Scan`, `Plan`, `WriteJSON`, `ReadJSON`), `internal/config` (`Load`), `internal/provider`. Nenhuma dependência nova em `go.mod`.

---

## Contexto para quem for executar este plano

Este é o **Plano 4b** de uma série (4a/4b/4c/4d) que implementa o Plano 4
("internal/cli") descrito em
`docs/superpowers/specs/2026-06-13-tfrepo-go-port-design.md`. Os Planos
1-4a já foram implementados e estão na `main`:

- `internal/security` — `ResolveToken`, `Redact`, `Manager`/`Workspace`,
  `Clone`/`Push`.
- `internal/provider` — `RepositoryProvider`, `NewGitHubProvider`,
  `NewGitLabProvider`, tipos `Namespace`, `RepositorySummary`,
  `RepositoryDetails`, `NamespaceKind`, `Visibility`.
- `internal/config` — `Load(path string) (*Config, error)`,
  `Config{Source, Target config.ProviderConfig; Filters; Mapping}`.
- `internal/core` — `Scan(ctx, p, namespace, concurrency) (Inventory, error)`,
  `Plan(inventory, cfg) (MigrationPlan, error)`, `WriteJSON`/`ReadJSON`,
  tipos `Inventory`/`InventoryNamespace`/`RepositoryInventory`/
  `MigrationPlan`/`MigrationTask`.
- `internal/cli` — `renderBanner`/`bannerSubtitle` (Task 1 do Plano 4a),
  `NewProvider(cfg config.ProviderConfig) (provider.RepositoryProvider, error)`
  (Task 2), `runInit`/`configTemplate` (Task 3), `newRootCommand`/`Execute`/
  `configFlagName`/`defaultConfigPath` (Task 4), `cmd/tfrepo/main.go`
  (Task 5).

Este plano (4b) implementa `tfrepo scan` e `tfrepo plan`. **Não** cobre
`migrate`/`validate` (Plano 4c) nem o `README.md` final/`.goreleaser.yaml`/
release workflow (Plano 4d).

**Aviso sobre mensagens:** o repositório Node `transferepo` (fonte de
verdade original) não está disponível localmente nesta sessão para conferir
a redação exata das mensagens de `scan`/`plan`. As mensagens de saída abaixo
foram desenhadas seguindo o estilo já estabelecido em `internal/cli/init.go`
(frases curtas em português, formato `Wrote <arquivo> (<resumo>)`). Se mais
tarde alguém tiver acesso ao Node original e as strings exatas forem
diferentes, ajustar apenas os literais de string — isso não muda nenhuma
assinatura de função nem estrutura de dados deste plano.

Padrão de exit code: cada `RunE` escreve em `*exitCode` (0 ou 1) e retorna
`nil`; erros de parsing de flags retornam o `error` diretamente (vira exit
code 1 via `Execute`), igual ao `init` do Plano 4a.

---

## Task 1: Helper de formatação (`internal/cli/format.go`)

**Files:**
- Create: `internal/cli/format.go`
- Test: `internal/cli/format_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/cli/format_test.go`:

```go
package cli

import "testing"

func TestCountLabel(t *testing.T) {
	tests := []struct {
		count            int
		singular, plural string
		want             string
	}{
		{0, "repositório", "repositórios", "0 repositórios"},
		{1, "repositório", "repositórios", "1 repositório"},
		{2, "repositório", "repositórios", "2 repositórios"},
		{1, "branch", "branches", "1 branch"},
		{3, "branch", "branches", "3 branches"},
	}

	for _, tt := range tests {
		if got := countLabel(tt.count, tt.singular, tt.plural); got != tt.want {
			t.Errorf("countLabel(%d, %q, %q) = %q, want %q", tt.count, tt.singular, tt.plural, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/... -run TestCountLabel -v`

Expected: FAIL — `undefined: countLabel` (build error).

- [ ] **Step 3: Write minimal implementation**

Create `internal/cli/format.go`:

```go
package cli

import "fmt"

// countLabel formats count together with the Portuguese singular or plural
// form of a noun, e.g. countLabel(1, "repositório", "repositórios") returns
// "1 repositório" and countLabel(2, "repositório", "repositórios") returns
// "2 repositórios".
func countLabel(count int, singular, plural string) string {
	word := plural
	if count == 1 {
		word = singular
	}
	return fmt.Sprintf("%d %s", count, word)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/... -run TestCountLabel -v`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/cli/format.go internal/cli/format_test.go
git commit -m "feat(cli): add countLabel formatting helper"
```

---

## Task 2: Comando `scan` (`internal/cli/scan.go`)

**Files:**
- Create: `internal/cli/scan.go`
- Test: `internal/cli/scan_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/cli/scan_test.go`:

```go
package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arijunior2020/tfrepo/internal/core"
	"github.com/arijunior2020/tfrepo/internal/provider"
)

// fakeScanProvider is a minimal provider.RepositoryProvider used to test
// scanAndWrite without making network calls. Only the methods core.Scan
// calls (Name, ListNamespaces, ListRepositories, GetRepositoryDetails)
// return real data; the rest satisfy the interface but are not exercised
// here.
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
		return provider.RepositoryDetails{}, errors.New("no details for " + repo)
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

func TestScanAndWriteWritesInventoryAndPrintsSummary(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	p := &fakeScanProvider{
		name: "github",
		namespaces: []provider.Namespace{
			{Slug: "my-org", Name: "My Org", Kind: provider.NamespaceOrganization},
		},
		repositories: []provider.RepositorySummary{
			{Name: "repo-a", Namespace: "my-org", DefaultBranch: "main", Visibility: provider.VisibilityPrivate},
		},
		details: map[string]provider.RepositoryDetails{
			"repo-a": {
				RepositorySummary: provider.RepositorySummary{Name: "repo-a", Namespace: "my-org", DefaultBranch: "main", Visibility: provider.VisibilityPrivate},
				Branches:          []string{"main"},
				Tags:              []string{"v1.0.0"},
			},
		},
	}

	var stdout, stderr bytes.Buffer
	code := scanAndWrite(context.Background(), p, "my-org", 1, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("scanAndWrite() = %d, want 0 (stderr: %s)", code, stderr.String())
	}

	var inventory core.Inventory
	if err := core.ReadJSON(inventoryPath, &inventory); err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
	if len(inventory.Namespaces) != 1 || len(inventory.Namespaces[0].Repositories) != 1 {
		t.Fatalf("inventory = %+v, want 1 namespace with 1 repository", inventory)
	}
	if inventory.Namespaces[0].Repositories[0].Name != "repo-a" {
		t.Errorf("Repositories[0].Name = %q, want %q", inventory.Namespaces[0].Repositories[0].Name, "repo-a")
	}

	if !strings.Contains(stdout.String(), "Wrote inventory.json (1 repositório)") {
		t.Errorf("stdout = %q, want it to mention the repository count", stdout.String())
	}
}

func TestScanAndWriteReturnsErrorWhenNamespaceNotFound(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	p := &fakeScanProvider{name: "github"}

	var stdout, stderr bytes.Buffer
	code := scanAndWrite(context.Background(), p, "my-org", 1, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("scanAndWrite() = %d, want 1", code)
	}
	if stderr.String() == "" {
		t.Error("stderr is empty, want an error message")
	}
	if _, err := os.Stat(inventoryPath); !os.IsNotExist(err) {
		t.Error("inventory.json was created on error, want no file")
	}
}

func TestRunScanReturnsErrorWhenTokenMissing(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	configPath := filepath.Join(dir, "transferepo.config.yaml")
	configYAML := `source:
  provider: github
  namespace: my-org
target:
  provider: gitlab
  namespace: my-group
`
	if err := os.WriteFile(configPath, []byte(configYAML), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	t.Setenv("GITHUB_TOKEN", "")

	var stdout, stderr bytes.Buffer
	code := runScan(context.Background(), configPath, 1, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("runScan() = %d, want 1", code)
	}
	if stderr.String() == "" {
		t.Error("stderr is empty, want an error message")
	}
}

func TestRunScanReturnsErrorWhenConfigMissing(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	var stdout, stderr bytes.Buffer
	code := runScan(context.Background(), filepath.Join(dir, "does-not-exist.yaml"), 1, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("runScan() = %d, want 1", code)
	}
	if stderr.String() == "" {
		t.Error("stderr is empty, want an error message")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/... -run 'TestScanAndWrite|TestRunScan' -v`

Expected: FAIL — `undefined: scanAndWrite`, `undefined: inventoryPath`,
`undefined: runScan` (build error).

- [ ] **Step 3: Write minimal implementation**

Create `internal/cli/scan.go`:

```go
package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/arijunior2020/tfrepo/internal/config"
	"github.com/arijunior2020/tfrepo/internal/core"
	"github.com/arijunior2020/tfrepo/internal/provider"
)

// inventoryPath is the file written by "tfrepo scan" and read by
// "tfrepo plan", relative to the current working directory.
const inventoryPath = "inventory.json"

// runScan loads the configuration at configPath, scans cfg.Source's
// namespace using up to concurrency goroutines, and writes the result to
// inventoryPath. It returns the process exit code (0 on success, 1 on
// error).
func runScan(ctx context.Context, configPath string, concurrency int, stdout, stderr io.Writer) int {
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	p, err := NewProvider(cfg.Source)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	return scanAndWrite(ctx, p, cfg.Source.Namespace, concurrency, stdout, stderr)
}

// scanAndWrite runs core.Scan against p and writes the resulting inventory
// to inventoryPath, printing a one-line summary on success.
func scanAndWrite(ctx context.Context, p provider.RepositoryProvider, namespace string, concurrency int, stdout, stderr io.Writer) int {
	inventory, err := core.Scan(ctx, p, namespace, concurrency)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if err := core.WriteJSON(inventoryPath, inventory); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	repoCount := 0
	for _, ns := range inventory.Namespaces {
		repoCount += len(ns.Repositories)
	}

	fmt.Fprintf(stdout, "Wrote %s (%s)\n", inventoryPath, countLabel(repoCount, "repositório", "repositórios"))
	return 0
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/... -run 'TestScanAndWrite|TestRunScan' -v`

Expected: PASS (4 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/cli/scan.go internal/cli/scan_test.go
git commit -m "feat(cli): add tfrepo scan command logic"
```

---

## Task 3: Comando `plan` (`internal/cli/plan.go`)

**Files:**
- Create: `internal/cli/plan.go`
- Test: `internal/cli/plan_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/cli/plan_test.go`:

```go
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/arijunior2020/tfrepo/internal/core"
	"github.com/arijunior2020/tfrepo/internal/provider"
)

func writePlanFixtures(t *testing.T, configYAML string, inventory core.Inventory) string {
	t.Helper()

	dir := t.TempDir()
	t.Chdir(dir)

	configPath := filepath.Join(dir, "transferepo.config.yaml")
	if err := os.WriteFile(configPath, []byte(configYAML), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := core.WriteJSON(inventoryPath, inventory); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	return configPath
}

func basePlanInventory() core.Inventory {
	return core.Inventory{
		Source: core.ProviderRef{Provider: "github", Namespace: "my-org"},
		Namespaces: []core.InventoryNamespace{
			{
				Slug: "my-org",
				Name: "My Org",
				Kind: provider.NamespaceOrganization,
				Repositories: []core.RepositoryInventory{
					{Name: "repo-a", DefaultBranch: "main", Visibility: provider.VisibilityPrivate, Branches: []string{"main", "develop"}, Tags: []string{"v1.0.0"}},
					{Name: "repo-b", DefaultBranch: "main", Visibility: provider.VisibilityPublic, Branches: []string{"main"}, Tags: []string{}},
				},
			},
		},
	}
}

const basePlanConfigYAML = `source:
  provider: github
  namespace: my-org
target:
  provider: gitlab
  namespace: my-group
`

func TestRunPlanWritesMigrationPlanAndPrintsSummary(t *testing.T) {
	configPath := writePlanFixtures(t, basePlanConfigYAML, basePlanInventory())

	var stdout, stderr bytes.Buffer
	code := runPlan(configPath, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("runPlan() = %d, want 0 (stderr: %s)", code, stderr.String())
	}

	var plan core.MigrationPlan
	if err := core.ReadJSON(migrationPlanPath, &plan); err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
	if len(plan.Tasks) != 2 {
		t.Fatalf("len(Tasks) = %d, want 2", len(plan.Tasks))
	}

	want := "Wrote migration-plan.json (2 repositórios, 3 branches, 1 tag)\n"
	if stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestRunPlanAppliesFiltersFromConfig(t *testing.T) {
	configYAML := basePlanConfigYAML + `filters:
  include: ["repo-a"]
  exclude: []
`
	configPath := writePlanFixtures(t, configYAML, basePlanInventory())

	var stdout, stderr bytes.Buffer
	code := runPlan(configPath, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("runPlan() = %d, want 0 (stderr: %s)", code, stderr.String())
	}

	var plan core.MigrationPlan
	if err := core.ReadJSON(migrationPlanPath, &plan); err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
	if len(plan.Tasks) != 1 || plan.Tasks[0].ID != "repo-a" {
		t.Fatalf("Tasks = %+v, want only repo-a", plan.Tasks)
	}

	want := "Wrote migration-plan.json (1 repositório, 2 branches, 1 tag)\n"
	if stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestRunPlanReturnsErrorWhenInventoryMissing(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	configPath := filepath.Join(dir, "transferepo.config.yaml")
	if err := os.WriteFile(configPath, []byte(basePlanConfigYAML), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := runPlan(configPath, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("runPlan() = %d, want 1", code)
	}
	if stderr.String() == "" {
		t.Error("stderr is empty, want an error message")
	}
	if _, err := os.Stat(migrationPlanPath); !os.IsNotExist(err) {
		t.Error("migration-plan.json was created on error, want no file")
	}
}

func TestRunPlanReturnsErrorWhenConfigMissing(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	var stdout, stderr bytes.Buffer
	code := runPlan(filepath.Join(dir, "does-not-exist.yaml"), &stdout, &stderr)

	if code != 1 {
		t.Fatalf("runPlan() = %d, want 1", code)
	}
	if stderr.String() == "" {
		t.Error("stderr is empty, want an error message")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/... -run TestRunPlan -v`

Expected: FAIL — `undefined: runPlan`, `undefined: migrationPlanPath`
(build error).

- [ ] **Step 3: Write minimal implementation**

Create `internal/cli/plan.go`:

```go
package cli

import (
	"fmt"
	"io"

	"github.com/arijunior2020/tfrepo/internal/config"
	"github.com/arijunior2020/tfrepo/internal/core"
)

// migrationPlanPath is the file written by "tfrepo plan" and read by
// "tfrepo migrate"/"tfrepo validate", relative to the current working
// directory.
const migrationPlanPath = "migration-plan.json"

// runPlan loads the configuration at configPath and the inventory at
// inventoryPath, applies cfg's filters and repository name mapping via
// core.Plan, and writes the result to migrationPlanPath. It returns the
// process exit code (0 on success, 1 on error).
func runPlan(configPath string, stdout, stderr io.Writer) int {
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	var inventory core.Inventory
	if err := core.ReadJSON(inventoryPath, &inventory); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	plan, err := core.Plan(inventory, cfg)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if err := core.WriteJSON(migrationPlanPath, plan); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	var branches, tags int
	for _, task := range plan.Tasks {
		branches += len(task.Branches)
		tags += len(task.Tags)
	}

	fmt.Fprintf(stdout, "Wrote %s (%s, %s, %s)\n", migrationPlanPath,
		countLabel(len(plan.Tasks), "repositório", "repositórios"),
		countLabel(branches, "branch", "branches"),
		countLabel(tags, "tag", "tags"),
	)
	return 0
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/... -run TestRunPlan -v`

Expected: PASS (4 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/cli/plan.go internal/cli/plan_test.go
git commit -m "feat(cli): add tfrepo plan command logic"
```

---

## Task 4: Registrar `scan`/`plan` em `internal/cli/root.go`

**Files:**
- Modify: `internal/cli/root.go`
- Test: `internal/cli/root_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/cli/root_test.go` (after `TestExecuteNoArgsPrintsBanner`):

```go

func TestNewRootCommandRegistersScanWithConcurrencyFlag(t *testing.T) {
	exitCode := 0
	root := newRootCommand(&exitCode)

	scanCmd, _, err := root.Find([]string{"scan"})
	if err != nil {
		t.Fatalf("Find(scan): %v", err)
	}

	flag := scanCmd.Flags().Lookup(concurrencyFlagName)
	if flag == nil {
		t.Fatal("scan command missing --concurrency flag")
	}
	if flag.DefValue != "4" {
		t.Errorf("--concurrency default = %q, want %q", flag.DefValue, "4")
	}
}

func TestNewRootCommandRegistersPlan(t *testing.T) {
	exitCode := 0
	root := newRootCommand(&exitCode)

	if _, _, err := root.Find([]string{"plan"}); err != nil {
		t.Fatalf("Find(plan): %v", err)
	}
}

func TestExecuteScanReturnsExitCode1WhenConfigMissing(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	exitCode := 0
	root := newRootCommand(&exitCode)

	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"scan", "--config", filepath.Join(dir, "does-not-exist.yaml")})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v (stderr: %s)", err, stderr.String())
	}
	if exitCode != 1 {
		t.Fatalf("exitCode = %d, want 1", exitCode)
	}
}

func TestExecutePlanReturnsExitCode1WhenInventoryMissing(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	configPath := filepath.Join(dir, "transferepo.config.yaml")
	configYAML := "source:\n  provider: github\n  namespace: my-org\ntarget:\n  provider: gitlab\n  namespace: my-group\n"
	if err := os.WriteFile(configPath, []byte(configYAML), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	exitCode := 0
	root := newRootCommand(&exitCode)

	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"plan", "--config", configPath})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v (stderr: %s)", err, stderr.String())
	}
	if exitCode != 1 {
		t.Fatalf("exitCode = %d, want 1", exitCode)
	}
}
```

No new imports are needed — `bytes`, `os`, `path/filepath` and `testing`
are already imported by `internal/cli/root_test.go`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cli/... -run 'TestNewRootCommandRegisters|TestExecuteScan|TestExecutePlan' -v`

Expected: FAIL — `undefined: concurrencyFlagName` (build error in package
`cli`, since `scan`/`plan`/`concurrencyFlagName`/`defaultScanConcurrency`
don't exist yet).

- [ ] **Step 3: Wire the new commands into the root command**

Replace the full contents of `internal/cli/root.go`:

```go
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const (
	// defaultConfigPath is the default value of the --config flag,
	// matching DEFAULT_CONFIG_PATH in apps/cli/src/cli.ts.
	defaultConfigPath = "transferepo.config.yaml"

	// programVersion matches PROGRAM_VERSION in apps/cli/src/cli.ts.
	programVersion = "0.1.0"

	// configFlagName is the name of the persistent --config/-c flag shared by
	// every subcommand.
	configFlagName = "config"

	// concurrencyFlagName is the name of the --concurrency flag on "scan".
	concurrencyFlagName = "concurrency"

	// defaultScanConcurrency is the default value of the --concurrency flag
	// on "scan".
	defaultScanConcurrency = 4
)

// Execute runs the tfrepo root command against os.Args and returns the
// process exit code.
func Execute() int {
	exitCode := 0
	root := newRootCommand(&exitCode)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return exitCode
}

// newRootCommand builds the tfrepo command tree. Each subcommand writes its
// exit code to *exitCode instead of returning an error for "business" exit
// codes (e.g. migrate returning 1 because a task failed), mirroring
// process.exitCode = await deps.runXxx(...) in apps/cli/src/cli.ts.
//
// Contract: every subcommand's RunE must set *exitCode before returning nil.
// Returning a non-nil error from RunE always yields process exit code 1
// (via Execute), regardless of *exitCode.
func newRootCommand(exitCode *int) *cobra.Command {
	root := &cobra.Command{
		Use:           "tfrepo",
		Short:         "Migra repositórios Git entre provedores (GitHub, GitLab)",
		Version:       programVersion,
		SilenceErrors: true,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprint(cmd.OutOrStdout(), renderBanner())
			fmt.Fprintln(cmd.OutOrStdout())
			_ = cmd.Help()
		},
	}

	root.PersistentFlags().StringP(configFlagName, "c", defaultConfigPath, "caminho do arquivo de configuração")

	root.AddCommand(newInitCommand(exitCode))
	root.AddCommand(newScanCommand(exitCode))
	root.AddCommand(newPlanCommand(exitCode))

	return root
}

func newInitCommand(exitCode *int) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Gera um transferepo.config.yaml de exemplo no diretório atual",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := cmd.Flags().GetString(configFlagName)
			if err != nil {
				return err
			}
			*exitCode = runInit(configPath, cmd.OutOrStdout(), cmd.ErrOrStderr())
			return nil
		},
	}
}

func newScanCommand(exitCode *int) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Lista os repositórios do namespace de origem e grava inventory.json",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := cmd.Flags().GetString(configFlagName)
			if err != nil {
				return err
			}
			concurrency, err := cmd.Flags().GetInt(concurrencyFlagName)
			if err != nil {
				return err
			}
			*exitCode = runScan(cmd.Context(), configPath, concurrency, cmd.OutOrStdout(), cmd.ErrOrStderr())
			return nil
		},
	}

	cmd.Flags().Int(concurrencyFlagName, defaultScanConcurrency, "número de repositórios processados em paralelo")

	return cmd
}

func newPlanCommand(exitCode *int) *cobra.Command {
	return &cobra.Command{
		Use:   "plan",
		Short: "Aplica filtros e mapeamento de inventory.json e grava migration-plan.json",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := cmd.Flags().GetString(configFlagName)
			if err != nil {
				return err
			}
			*exitCode = runPlan(configPath, cmd.OutOrStdout(), cmd.ErrOrStderr())
			return nil
		},
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cli/... -v`

Expected: PASS for all tests in `internal/cli` (banner, providers, init,
root, format, scan, plan).

- [ ] **Step 5: Commit**

```bash
git add internal/cli/root.go internal/cli/root_test.go
git commit -m "feat(cli): register scan and plan commands"
```

---

## Final Verification

Run from the repo root:

```bash
go build ./...
go vet ./...
go test ./... -race
gofmt -l .
```

Expected: all four commands succeed with no output from `gofmt -l .`
(no unformatted files), and `go test ./...` shows `ok` for `internal/cli`
alongside `internal/config`, `internal/core`, `internal/provider`, and
`internal/security`.

Manual smoke test (uses fixtures from `internal/cli/init.go`'s
`configTemplate`, requires a valid `GITHUB_TOKEN`/`GITLAB_TOKEN` to reach
the real APIs — otherwise `scan` exits 1 with a "missing required
environment variable" message, which is also acceptable as a smoke check):

```bash
cd "$(mktemp -d)"
go run /media/arimateia-junior/Dados4/projetos/tfrepo/cmd/tfrepo init
# edit transferepo.config.yaml with real source/target namespaces, then:
go run /media/arimateia-junior/Dados4/projetos/tfrepo/cmd/tfrepo scan --concurrency 4
go run /media/arimateia-junior/Dados4/projetos/tfrepo/cmd/tfrepo plan
cat inventory.json migration-plan.json
```

---

## What's next (Planos 4c/4d — not part of this plan)

- **4c**: `tfrepo migrate [--dry-run] [--concurrency N]` e
  `tfrepo validate`, usando `core.Migrate`/`core.Validate`,
  `security.Manager` para limpeza de workspaces, e tratamento de
  SIGINT/SIGTERM.
- **4d**: reescrever `README.md` com instruções completas de uso do
  `tfrepo` (portado da seção 5/6 do README do `transferepo`),
  `.goreleaser.yaml`, e `.github/workflows/release.yml`.
