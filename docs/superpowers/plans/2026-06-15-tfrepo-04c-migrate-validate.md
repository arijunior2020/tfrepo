# tfrepo Plano 4c — Comandos migrate e validate

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implementar `tfrepo migrate [--dry-run] [--concurrency N]` e `tfrepo validate` no pacote `internal/cli`, escrevendo `migration-report.json` e `validation-report.json` respectivamente, com testes unitários e integração no comando root Cobra.

**Architecture:** Dois novos arquivos de implementação (`migrate.go`, `validate.go`) + dois de teste (`migrate_test.go`, `validate_test.go`) seguem o mesmo padrão de Plan 4b: `runXxx` (carrega config + disco + constrói providers) delega para `xxxAndWrite` (lógica testável com providers fake). SIGINT/SIGTERM é capturado via `signal.NotifyContext` no `RunE` do Cobra em `root.go`; o context cancelado propaga naturalmente até `core.Migrate` e seus goroutines. O `security.Manager` garante cleanup dos workspaces temporários mesmo em caso de interrupção.

**Tech Stack:** Go 1.24, `github.com/spf13/cobra` v1.10.2, `os/signal` + `syscall` (stdlib), `github.com/arijunior2020/tfrepo/internal/core` (Migrate, Validate, ReadJSON, WriteJSON, MigrationPlan, MigrationReport, ValidationReport, MigrateProviders, MigrateOptions), `github.com/arijunior2020/tfrepo/internal/security` (Manager, NewManager), `github.com/arijunior2020/tfrepo/internal/config`, `github.com/arijunior2020/tfrepo/internal/provider` (RepositoryProvider, RepositoryState, RepositorySummary, CreateRepositoryInput, Namespace, RepositoryDetails), pacote `cli` interno (NewProvider, countLabel, migrationPlanPath).

---

## Mapeamento de arquivos

| Arquivo | Ação | Responsabilidade |
|---------|------|-----------------|
| `internal/cli/migrate.go` | criar | `const migrationReportPath`, `runMigrate`, `migrateAndWrite` |
| `internal/cli/migrate_test.go` | criar | `fakeMigrateCliProvider`, 5 testes para migrate |
| `internal/cli/validate.go` | criar | `const validationReportPath`, `runValidate`, `validateAndWrite` |
| `internal/cli/validate_test.go` | criar | `fakeValidateCliProvider`, 6 testes para validate |
| `internal/cli/root.go` | modificar | `const dryRunFlagName`, `newMigrateCommand`, `newValidateCommand`, registrar ambos |
| `internal/cli/root_test.go` | modificar | 4 testes novos |
| `docs/superpowers/plans/2026-06-15-tfrepo-04c-migrate-validate.md` | criar | este documento (commitado no final) |

**Contexto de tipos** (não mudar estas assinaturas — são definidas em `internal/core` e `internal/provider`):

```go
// internal/core/migrate.go
type MigrateProviders struct { Source, Target provider.RepositoryProvider }
type MigrateOptions  struct { DryRun bool; Concurrency int; Workspaces *security.Manager }
func Migrate(ctx, plan MigrationPlan, providers MigrateProviders, opts MigrateOptions) (MigrationReport, error)

// internal/core/validate.go
func Validate(ctx, plan MigrationPlan, providers MigrateProviders, report *MigrationReport) (ValidationReport, error)

// internal/core/artifacts.go
type MigrationReport  struct { GeneratedAt time.Time; Results []MigrationResult }
type MigrationResult  struct { ID, Status string; ... }  // Status: "success"|"failed"|"dry-run"
type ValidationReport struct { GeneratedAt time.Time; Results []ValidationResult }
type ValidationResult struct { ID, Status string; Divergences []RefDivergence; Reason string }
// Status: "ok"|"diverged"|"skipped"
func WriteJSON(path string, v any) error
func ReadJSON(path string, v any) error

// internal/security/workspace.go
func NewManager() *Manager
func (m *Manager) CleanupAll() error

// internal/provider/types.go
type RepositoryProvider interface {
    Name() string
    ListNamespaces(ctx) ([]Namespace, error)
    ListRepositories(ctx, namespace string) ([]RepositorySummary, error)
    GetRepositoryDetails(ctx, namespace, repo string) (RepositoryDetails, error)
    CreateRepository(ctx, namespace string, input CreateRepositoryInput) (RepositorySummary, error)
    GetAuthenticatedCloneURL(namespace, repo string) string
    GetRepositoryState(ctx, namespace, repo string) (RepositoryState, error)
}
type RepositoryState struct { Branches, Tags map[string]string }
```

**Constantes já definidas** em arquivos existentes do pacote `cli`:
- `migrationPlanPath = "migration-plan.json"` — em `internal/cli/plan.go`
- `configFlagName`, `concurrencyFlagName`, `defaultScanConcurrency` — em `internal/cli/root.go`
- `countLabel(count int, singular, plural string) string` — em `internal/cli/format.go`
- `NewProvider(cfg config.ProviderConfig) (provider.RepositoryProvider, error)` — em `internal/cli/providers.go`

---

## Tarefa 1 — `internal/cli/migrate.go` + `internal/cli/migrate_test.go`

**Arquivos:**
- Criar: `internal/cli/migrate.go`
- Criar: `internal/cli/migrate_test.go`

### Passo 1.1 — Escrever os testes que falham

- [ ] Criar `internal/cli/migrate_test.go` com o conteúdo abaixo:

```go
package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/arijunior2020/tfrepo/internal/core"
	"github.com/arijunior2020/tfrepo/internal/provider"
)

// fakeMigrateCliProvider é um stub mínimo de provider.RepositoryProvider para
// testar migrateAndWrite em modo dry-run. Apenas ListRepositories é chamado
// pelo core.Migrate no modo dry-run (para verificar se o namespace existe).
type fakeMigrateCliProvider struct {
	name         string
	listRepos    []provider.RepositorySummary
	listReposErr error
}

var _ provider.RepositoryProvider = (*fakeMigrateCliProvider)(nil)

func (f *fakeMigrateCliProvider) Name() string { return f.name }
func (f *fakeMigrateCliProvider) ListNamespaces(_ context.Context) ([]provider.Namespace, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeMigrateCliProvider) ListRepositories(_ context.Context, _ string) ([]provider.RepositorySummary, error) {
	return f.listRepos, f.listReposErr
}
func (f *fakeMigrateCliProvider) GetRepositoryDetails(_ context.Context, _, _ string) (provider.RepositoryDetails, error) {
	return provider.RepositoryDetails{}, errors.New("not implemented")
}
func (f *fakeMigrateCliProvider) CreateRepository(_ context.Context, _ string, _ provider.CreateRepositoryInput) (provider.RepositorySummary, error) {
	return provider.RepositorySummary{}, errors.New("not implemented")
}
func (f *fakeMigrateCliProvider) GetAuthenticatedCloneURL(_, _ string) string { return "" }
func (f *fakeMigrateCliProvider) GetRepositoryState(_ context.Context, _, _ string) (provider.RepositoryState, error) {
	return provider.RepositoryState{}, errors.New("not implemented")
}

const baseMigrateConfigYAML = `source:
  provider: github
  namespace: my-org
target:
  provider: gitlab
  namespace: my-group
`

func baseMigratePlan() core.MigrationPlan {
	return core.MigrationPlan{
		Source: core.ProviderRef{Provider: "github", Namespace: "my-org"},
		Target: core.ProviderRef{Provider: "gitlab", Namespace: "my-group"},
		Tasks: []core.MigrationTask{
			{
				ID:     "repo-a",
				Source: core.TaskEndpoint{Namespace: "my-org", Repo: "repo-a"},
				Target: core.TaskEndpoint{Namespace: "my-group", Repo: "repo-a"},
			},
		},
	}
}

// writeMigrateFixtures cria um dir temporário, entra nele via t.Chdir,
// grava o config YAML em caminho absoluto e grava migrationPlanPath como JSON.
// Retorna o caminho absoluto do config.
func writeMigrateFixtures(t *testing.T, plan core.MigrationPlan) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	configPath := filepath.Join(dir, "transferepo.config.yaml")
	if err := os.WriteFile(configPath, []byte(baseMigrateConfigYAML), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := core.WriteJSON(migrationPlanPath, plan); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	return configPath
}

// TestMigrateAndWriteDryRunWritesReportAndPrintsSummary verifica que
// migrateAndWrite em modo dry-run (target.ListRepositories retorna lista vazia)
// grava migration-report.json com status "dry-run" e imprime o resumo correto.
func TestMigrateAndWriteDryRunWritesReportAndPrintsSummary(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	plan := baseMigratePlan()
	providers := core.MigrateProviders{
		Source: &fakeMigrateCliProvider{name: "github"},
		Target: &fakeMigrateCliProvider{name: "gitlab", listRepos: []provider.RepositorySummary{}},
	}

	var stdout, stderr bytes.Buffer
	code := migrateAndWrite(context.Background(), plan, providers, true, 1, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("migrateAndWrite() = %d, want 0 (stderr: %s)", code, stderr.String())
	}

	var report core.MigrationReport
	if err := core.ReadJSON(migrationReportPath, &report); err != nil {
		t.Fatalf("ReadJSON(%s): %v", migrationReportPath, err)
	}
	if len(report.Results) != 1 || report.Results[0].Status != "dry-run" {
		t.Fatalf("Results = %+v, want 1 result with status dry-run", report.Results)
	}

	want := "Wrote migration-report.json (1 repositório, dry-run)\n"
	if stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
}

// TestMigrateAndWriteExitsWithCode1WhenTaskFails verifica que migrateAndWrite
// retorna 1 e ainda grava o relatório quando ao menos uma task falha (aqui,
// target.ListRepositories retorna erro em modo dry-run).
func TestMigrateAndWriteExitsWithCode1WhenTaskFails(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	plan := baseMigratePlan()
	providers := core.MigrateProviders{
		Source: &fakeMigrateCliProvider{name: "github"},
		Target: &fakeMigrateCliProvider{name: "gitlab", listReposErr: errors.New("namespace not found")},
	}

	var stdout, stderr bytes.Buffer
	code := migrateAndWrite(context.Background(), plan, providers, true, 1, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("migrateAndWrite() = %d, want 1", code)
	}

	var report core.MigrationReport
	if err := core.ReadJSON(migrationReportPath, &report); err != nil {
		t.Fatalf("ReadJSON(%s): %v", migrationReportPath, err)
	}
	if len(report.Results) != 1 || report.Results[0].Status != "failed" {
		t.Fatalf("Results = %+v, want 1 result with status failed", report.Results)
	}

	want := "Wrote migration-report.json (1 repositório, 1 com falha, dry-run)\n"
	if stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
}

// TestRunMigrateReturnsErrorWhenConfigMissing verifica que runMigrate retorna
// 1 se o config YAML não existe (config.Load falha antes de ler o plano).
func TestRunMigrateReturnsErrorWhenConfigMissing(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runMigrate(context.Background(), "does-not-exist.yaml", false, 1, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("runMigrate() = %d, want 1 (stderr: %s)", code, stderr.String())
	}
}

// TestRunMigrateReturnsErrorWhenPlanMissing verifica que runMigrate retorna 1
// quando migration-plan.json não existe no diretório de trabalho.
func TestRunMigrateReturnsErrorWhenPlanMissing(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	configPath := filepath.Join(dir, "transferepo.config.yaml")
	if err := os.WriteFile(configPath, []byte(baseMigrateConfigYAML), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := runMigrate(context.Background(), configPath, false, 1, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("runMigrate() = %d, want 1 (stderr: %s)", code, stderr.String())
	}
}

// TestRunMigrateReturnsErrorWhenTokenMissing verifica que runMigrate retorna 1
// quando GITHUB_TOKEN está vazio (NewProvider falha antes de executar migrate).
func TestRunMigrateReturnsErrorWhenTokenMissing(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	configPath := writeMigrateFixtures(t, baseMigratePlan())

	var stdout, stderr bytes.Buffer
	code := runMigrate(context.Background(), configPath, false, 1, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("runMigrate() = %d, want 1 (stderr: %s)", code, stderr.String())
	}
}
```

### Passo 1.2 — Verificar que os testes falham

- [ ] Rodar:
  ```bash
  cd /media/arimateia-junior/Dados4/projetos/tfrepo
  go test ./internal/cli/ -run "TestMigrateAndWrite|TestRunMigrate" -v 2>&1 | head -30
  ```
  Esperado: falha com `undefined: migrateAndWrite` (ou similar — o arquivo ainda não existe).

### Passo 1.3 — Criar `internal/cli/migrate.go`

- [ ] Criar o arquivo com o conteúdo abaixo:

```go
package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/arijunior2020/tfrepo/internal/config"
	"github.com/arijunior2020/tfrepo/internal/core"
	"github.com/arijunior2020/tfrepo/internal/security"
)

// migrationReportPath é o arquivo gravado por "tfrepo migrate", relativo ao
// diretório de trabalho atual. Lido opcionalmente por "tfrepo validate".
const migrationReportPath = "migration-report.json"

// runMigrate carrega o config em configPath, lê migrationPlanPath, constrói
// os providers de origem e destino e delega para migrateAndWrite. Retorna o
// exit code do processo (0 em sucesso, 1 em qualquer erro).
func runMigrate(ctx context.Context, configPath string, dryRun bool, concurrency int, stdout, stderr io.Writer) int {
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

	return migrateAndWrite(ctx, plan, core.MigrateProviders{Source: source, Target: target}, dryRun, concurrency, stdout, stderr)
}

// migrateAndWrite executa plan contra providers, grava migration-report.json e
// imprime um resumo de uma linha. Retorna 1 se alguma task tiver status "failed".
// defer workspaces.CleanupAll() é belt-and-suspenders: cada goroutine já faz
// defer workspace.Cleanup() via core.Migrate, mas CleanupAll garante que
// workspaces criados e não limpos no cancelamento do context sejam removidos.
func migrateAndWrite(ctx context.Context, plan core.MigrationPlan, providers core.MigrateProviders, dryRun bool, concurrency int, stdout, stderr io.Writer) int {
	workspaces := security.NewManager()
	defer workspaces.CleanupAll()

	report, err := core.Migrate(ctx, plan, providers, core.MigrateOptions{
		DryRun:      dryRun,
		Concurrency: concurrency,
		Workspaces:  workspaces,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if err := core.WriteJSON(migrationReportPath, report); err != nil {
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
	if dryRun {
		suffix += ", dry-run"
	}
	fmt.Fprintf(stdout, "Wrote %s (%s%s)\n", migrationReportPath, label, suffix)

	if failed > 0 {
		return 1
	}
	return 0
}
```

### Passo 1.4 — Verificar que os testes passam

- [ ] Rodar:
  ```bash
  go test ./internal/cli/ -run "TestMigrateAndWrite|TestRunMigrate" -v
  ```
  Esperado: 5 testes `PASS`.

### Passo 1.5 — Commit

- [ ] Commitar:
  ```bash
  git add internal/cli/migrate.go internal/cli/migrate_test.go
  git commit -m "feat(cli): add migrate command core (migrateAndWrite, runMigrate)"
  ```

---

## Tarefa 2 — `internal/cli/validate.go` + `internal/cli/validate_test.go`

**Arquivos:**
- Criar: `internal/cli/validate.go`
- Criar: `internal/cli/validate_test.go`

**Nota importante:** `migrationPlanPath` (definido em `plan.go`) e `migrationReportPath` (definido em `migrate.go` acima) são usados por `runValidate` — estão no mesmo pacote `cli`, sem necessidade de importação extra.

### Passo 2.1 — Escrever os testes que falham

- [ ] Criar `internal/cli/validate_test.go` com o conteúdo abaixo:

```go
package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/arijunior2020/tfrepo/internal/core"
	"github.com/arijunior2020/tfrepo/internal/provider"
)

// fakeValidateCliProvider é um stub de provider.RepositoryProvider para
// testar validateAndWrite. Apenas GetRepositoryState é chamado por core.Validate.
type fakeValidateCliProvider struct {
	name        string
	stateResult provider.RepositoryState
	stateErr    error
}

var _ provider.RepositoryProvider = (*fakeValidateCliProvider)(nil)

func (f *fakeValidateCliProvider) Name() string { return f.name }
func (f *fakeValidateCliProvider) ListNamespaces(_ context.Context) ([]provider.Namespace, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeValidateCliProvider) ListRepositories(_ context.Context, _ string) ([]provider.RepositorySummary, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeValidateCliProvider) GetRepositoryDetails(_ context.Context, _, _ string) (provider.RepositoryDetails, error) {
	return provider.RepositoryDetails{}, errors.New("not implemented")
}
func (f *fakeValidateCliProvider) CreateRepository(_ context.Context, _ string, _ provider.CreateRepositoryInput) (provider.RepositorySummary, error) {
	return provider.RepositorySummary{}, errors.New("not implemented")
}
func (f *fakeValidateCliProvider) GetAuthenticatedCloneURL(_, _ string) string { return "" }
func (f *fakeValidateCliProvider) GetRepositoryState(_ context.Context, _, _ string) (provider.RepositoryState, error) {
	return f.stateResult, f.stateErr
}

const baseValidateConfigYAML = `source:
  provider: github
  namespace: my-org
target:
  provider: gitlab
  namespace: my-group
`

func baseValidatePlan() core.MigrationPlan {
	return core.MigrationPlan{
		Source: core.ProviderRef{Provider: "github", Namespace: "my-org"},
		Target: core.ProviderRef{Provider: "gitlab", Namespace: "my-group"},
		Tasks: []core.MigrationTask{
			{
				ID:     "repo-a",
				Source: core.TaskEndpoint{Namespace: "my-org", Repo: "repo-a"},
				Target: core.TaskEndpoint{Namespace: "my-group", Repo: "repo-a"},
			},
		},
	}
}

// writeValidateFixtures cria um dir temporário, entra nele via t.Chdir,
// grava o config YAML em caminho absoluto e grava migrationPlanPath como JSON.
func writeValidateFixtures(t *testing.T, plan core.MigrationPlan) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	configPath := filepath.Join(dir, "transferepo.config.yaml")
	if err := os.WriteFile(configPath, []byte(baseValidateConfigYAML), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := core.WriteJSON(migrationPlanPath, plan); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	return configPath
}

// TestValidateAndWriteWritesReportAndPrintsSummary verifica que validateAndWrite
// com providers em estado idêntico (source == target) grava validation-report.json
// com status "ok" e imprime o resumo correto.
func TestValidateAndWriteWritesReportAndPrintsSummary(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	state := provider.RepositoryState{
		Branches: map[string]string{"main": "abc123"},
		Tags:     map[string]string{},
	}
	providers := core.MigrateProviders{
		Source: &fakeValidateCliProvider{name: "github", stateResult: state},
		Target: &fakeValidateCliProvider{name: "gitlab", stateResult: state},
	}

	var stdout, stderr bytes.Buffer
	code := validateAndWrite(context.Background(), baseValidatePlan(), providers, nil, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("validateAndWrite() = %d, want 0 (stderr: %s)", code, stderr.String())
	}

	var report core.ValidationReport
	if err := core.ReadJSON(validationReportPath, &report); err != nil {
		t.Fatalf("ReadJSON(%s): %v", validationReportPath, err)
	}
	if len(report.Results) != 1 || report.Results[0].Status != "ok" {
		t.Fatalf("Results = %+v, want 1 result with status ok", report.Results)
	}

	want := "Wrote validation-report.json (1 repositório ok)\n"
	if stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
}

// TestValidateAndWriteExitsWithCode1WhenDiverged verifica que validateAndWrite
// retorna 1 quando source e target têm SHAs diferentes para o mesmo branch.
func TestValidateAndWriteExitsWithCode1WhenDiverged(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	providers := core.MigrateProviders{
		Source: &fakeValidateCliProvider{name: "github", stateResult: provider.RepositoryState{
			Branches: map[string]string{"main": "abc123"},
			Tags:     map[string]string{},
		}},
		Target: &fakeValidateCliProvider{name: "gitlab", stateResult: provider.RepositoryState{
			Branches: map[string]string{"main": "def456"},
			Tags:     map[string]string{},
		}},
	}

	var stdout, stderr bytes.Buffer
	code := validateAndWrite(context.Background(), baseValidatePlan(), providers, nil, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("validateAndWrite() = %d, want 1 (stdout: %s)", code, stdout.String())
	}

	var report core.ValidationReport
	if err := core.ReadJSON(validationReportPath, &report); err != nil {
		t.Fatalf("ReadJSON(%s): %v", validationReportPath, err)
	}
	if len(report.Results) != 1 || report.Results[0].Status != "diverged" {
		t.Fatalf("Results = %+v, want 1 result with status diverged", report.Results)
	}

	want := "Wrote validation-report.json (1 divergido)\n"
	if stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
}

// TestValidateAndWriteSkipsFailedTasksWhenReportProvided verifica que quando
// um MigrationReport é fornecido (não nil), tasks com status "failed" nele
// são marcadas como "skipped" pelo core.Validate sem chamar os providers.
func TestValidateAndWriteSkipsFailedTasksWhenReportProvided(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	// Plano com 2 tasks; repo-b falhou na migração.
	plan := core.MigrationPlan{
		Source: core.ProviderRef{Provider: "github", Namespace: "my-org"},
		Target: core.ProviderRef{Provider: "gitlab", Namespace: "my-group"},
		Tasks: []core.MigrationTask{
			{ID: "repo-a", Source: core.TaskEndpoint{Namespace: "my-org", Repo: "repo-a"}, Target: core.TaskEndpoint{Namespace: "my-group", Repo: "repo-a"}},
			{ID: "repo-b", Source: core.TaskEndpoint{Namespace: "my-org", Repo: "repo-b"}, Target: core.TaskEndpoint{Namespace: "my-group", Repo: "repo-b"}},
		},
	}

	state := provider.RepositoryState{
		Branches: map[string]string{"main": "abc123"},
		Tags:     map[string]string{},
	}
	providers := core.MigrateProviders{
		Source: &fakeValidateCliProvider{name: "github", stateResult: state},
		Target: &fakeValidateCliProvider{name: "gitlab", stateResult: state},
	}

	migrationReport := &core.MigrationReport{
		Results: []core.MigrationResult{
			{ID: "repo-b", Status: "failed"},
		},
	}

	var stdout, stderr bytes.Buffer
	code := validateAndWrite(context.Background(), plan, providers, migrationReport, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("validateAndWrite() = %d, want 0 (stderr: %s)", code, stderr.String())
	}

	var report core.ValidationReport
	if err := core.ReadJSON(validationReportPath, &report); err != nil {
		t.Fatalf("ReadJSON(%s): %v", validationReportPath, err)
	}
	if len(report.Results) != 2 {
		t.Fatalf("len(Results) = %d, want 2", len(report.Results))
	}
	// Construir mapa id→status para verificação independente da ordem.
	statuses := make(map[string]string, 2)
	for _, r := range report.Results {
		statuses[r.ID] = r.Status
	}
	if statuses["repo-a"] != "ok" || statuses["repo-b"] != "skipped" {
		t.Errorf("statuses = %v, want {repo-a: ok, repo-b: skipped}", statuses)
	}

	want := "Wrote validation-report.json (1 repositório ok, 1 ignorado)\n"
	if stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
}

// TestRunValidateReturnsErrorWhenConfigMissing verifica que runValidate retorna
// 1 se o config YAML não existe (config.Load falha imediatamente).
func TestRunValidateReturnsErrorWhenConfigMissing(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runValidate(context.Background(), "does-not-exist.yaml", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("runValidate() = %d, want 1 (stderr: %s)", code, stderr.String())
	}
}

// TestRunValidateReturnsErrorWhenPlanMissing verifica que runValidate retorna
// 1 quando migration-plan.json não existe no diretório de trabalho.
func TestRunValidateReturnsErrorWhenPlanMissing(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	configPath := filepath.Join(dir, "transferepo.config.yaml")
	if err := os.WriteFile(configPath, []byte(baseValidateConfigYAML), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := runValidate(context.Background(), configPath, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("runValidate() = %d, want 1 (stderr: %s)", code, stderr.String())
	}
}

// TestRunValidateReturnsErrorWhenTokenMissing verifica que runValidate retorna
// 1 quando GITHUB_TOKEN está vazio (NewProvider falha antes de validar).
func TestRunValidateReturnsErrorWhenTokenMissing(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	configPath := writeValidateFixtures(t, baseValidatePlan())

	var stdout, stderr bytes.Buffer
	code := runValidate(context.Background(), configPath, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("runValidate() = %d, want 1 (stderr: %s)", code, stderr.String())
	}
}
```

### Passo 2.2 — Verificar que os testes falham

- [ ] Rodar:
  ```bash
  go test ./internal/cli/ -run "TestValidateAndWrite|TestRunValidate" -v 2>&1 | head -30
  ```
  Esperado: falha com `undefined: validateAndWrite` (ou similar).

### Passo 2.3 — Criar `internal/cli/validate.go`

- [ ] Criar o arquivo com o conteúdo abaixo:

```go
package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/arijunior2020/tfrepo/internal/config"
	"github.com/arijunior2020/tfrepo/internal/core"
)

// validationReportPath é o arquivo gravado por "tfrepo validate", relativo ao
// diretório de trabalho atual.
const validationReportPath = "validation-report.json"

// runValidate carrega o config em configPath, lê migrationPlanPath, tenta ler
// migrationReportPath (opcional — se ausente, report é nil e nenhuma task é
// ignorada), constrói os providers e delega para validateAndWrite. Retorna o
// exit code do processo (0 se tudo ok/skipped, 1 se houver qualquer divergido).
func runValidate(ctx context.Context, configPath string, stdout, stderr io.Writer) int {
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

	// migration-report.json é opcional: se ausente, nenhuma task é marcada
	// como skipped. O erro de leitura é silenciado intencionalmente.
	var report *core.MigrationReport
	var mr core.MigrationReport
	if err := core.ReadJSON(migrationReportPath, &mr); err == nil {
		report = &mr
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

	return validateAndWrite(ctx, plan, core.MigrateProviders{Source: source, Target: target}, report, stdout, stderr)
}

// validateAndWrite executa core.Validate para plan, grava validation-report.json
// e imprime um resumo de uma linha. Retorna 1 se houver qualquer task "diverged".
func validateAndWrite(ctx context.Context, plan core.MigrationPlan, providers core.MigrateProviders, report *core.MigrationReport, stdout, stderr io.Writer) int {
	validation, err := core.Validate(ctx, plan, providers, report)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if err := core.WriteJSON(validationReportPath, validation); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	var ok, diverged, skipped int
	for _, r := range validation.Results {
		switch r.Status {
		case "ok":
			ok++
		case "diverged":
			diverged++
		case "skipped":
			skipped++
		}
	}

	var parts []string
	if ok > 0 {
		parts = append(parts, countLabel(ok, "repositório ok", "repositórios ok"))
	}
	if diverged > 0 {
		parts = append(parts, countLabel(diverged, "divergido", "divergidos"))
	}
	if skipped > 0 {
		parts = append(parts, countLabel(skipped, "ignorado", "ignorados"))
	}
	if len(parts) == 0 {
		parts = []string{countLabel(0, "repositório ok", "repositórios ok")}
	}
	fmt.Fprintf(stdout, "Wrote %s (%s)\n", validationReportPath, strings.Join(parts, ", "))

	if diverged > 0 {
		return 1
	}
	return 0
}
```

### Passo 2.4 — Verificar que os testes passam

- [ ] Rodar:
  ```bash
  go test ./internal/cli/ -run "TestValidateAndWrite|TestRunValidate" -v
  ```
  Esperado: 6 testes `PASS`.

### Passo 2.5 — Commit

- [ ] Commitar:
  ```bash
  git add internal/cli/validate.go internal/cli/validate_test.go
  git commit -m "feat(cli): add validate command core (validateAndWrite, runValidate)"
  ```

---

## Tarefa 3 — Integração no root Cobra (`root.go` + `root_test.go`)

**Arquivos:**
- Modificar: `internal/cli/root.go` (linhas 1–10 para imports, 10–28 para consts, 66–70 para AddCommand)
- Modificar: `internal/cli/root_test.go` (adicionar 4 testes ao final)

### Passo 3.1 — Escrever os testes que falham

- [ ] Adicionar ao **final** de `internal/cli/root_test.go` (após o último teste existente):

```go
func TestNewRootCommandRegistersMigrateWithFlags(t *testing.T) {
	exitCode := 0
	root := newRootCommand(&exitCode)

	migrateCmd, _, err := root.Find([]string{"migrate"})
	if err != nil {
		t.Fatalf("Find(migrate): %v", err)
	}
	// cobra.Find retorna o root silenciosamente quando o subcomando não existe;
	// verificar Use é obrigatório para detectar esse caso.
	if migrateCmd.Use != "migrate" {
		t.Fatalf("Find(migrate).Use = %q, want %q", migrateCmd.Use, "migrate")
	}

	dryRunFlag := migrateCmd.Flags().Lookup(dryRunFlagName)
	if dryRunFlag == nil {
		t.Fatal("migrate command missing --dry-run flag")
	}
	if dryRunFlag.DefValue != "false" {
		t.Errorf("--dry-run default = %q, want %q", dryRunFlag.DefValue, "false")
	}

	concurrencyFlag := migrateCmd.Flags().Lookup(concurrencyFlagName)
	if concurrencyFlag == nil {
		t.Fatal("migrate command missing --concurrency flag")
	}
	if concurrencyFlag.DefValue != strconv.Itoa(defaultScanConcurrency) {
		t.Errorf("--concurrency default = %q, want %q", concurrencyFlag.DefValue, strconv.Itoa(defaultScanConcurrency))
	}
}

func TestNewRootCommandRegistersValidate(t *testing.T) {
	exitCode := 0
	root := newRootCommand(&exitCode)

	validateCmd, _, err := root.Find([]string{"validate"})
	if err != nil {
		t.Fatalf("Find(validate): %v", err)
	}
	if validateCmd.Use != "validate" {
		t.Fatalf("Find(validate).Use = %q, want %q", validateCmd.Use, "validate")
	}
}

func TestExecuteMigrateReturnsExitCode1WhenPlanMissing(t *testing.T) {
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
	root.SetArgs([]string{"migrate", "--config", configPath})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v (stderr: %s)", err, stderr.String())
	}
	if exitCode != 1 {
		t.Fatalf("exitCode = %d, want 1", exitCode)
	}
}

func TestExecuteValidateReturnsExitCode1WhenPlanMissing(t *testing.T) {
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
	root.SetArgs([]string{"validate", "--config", configPath})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v (stderr: %s)", err, stderr.String())
	}
	if exitCode != 1 {
		t.Fatalf("exitCode = %d, want 1", exitCode)
	}
}
```

### Passo 3.2 — Verificar que os testes novos falham

- [ ] Rodar:
  ```bash
  go test ./internal/cli/ -run "TestNewRootCommandRegistersMigrate|TestNewRootCommandRegistersValidate|TestExecuteMigrate|TestExecuteValidate" -v 2>&1 | head -30
  ```
  Esperado: falha com `undefined: dryRunFlagName` ou `undefined: newMigrateCommand`.

### Passo 3.3 — Modificar `internal/cli/root.go`

Três mudanças cirúrgicas. Fazer uma de cada vez.

**3.3a — Adicionar imports `os/signal` e `syscall`**

- [ ] Substituir o bloco de imports em `internal/cli/root.go`:

  Atual (linhas 3–8):
  ```go
  import (
  	"fmt"
  	"os"
  
  	"github.com/spf13/cobra"
  )
  ```

  Novo:
  ```go
  import (
  	"fmt"
  	"os"
  	"os/signal"
  	"syscall"
  
  	"github.com/spf13/cobra"
  )
  ```

**3.3b — Adicionar constante `dryRunFlagName`**

- [ ] No bloco `const` de `root.go`, após `defaultScanConcurrency = 4`, adicionar:

  Atual (linhas 26–28):
  ```go
  	// defaultScanConcurrency is the default value of the --concurrency flag
  	// on "scan".
  	defaultScanConcurrency = 4
  )
  ```

  Novo:
  ```go
  	// defaultScanConcurrency is the default value of the --concurrency flag
  	// on "scan".
  	defaultScanConcurrency = 4

  	// dryRunFlagName is the name of the --dry-run flag on "migrate".
  	dryRunFlagName = "dry-run"
  )
  ```

**3.3c — Registrar migrate e validate em `newRootCommand` e adicionar as funções**

- [ ] Em `newRootCommand`, após `root.AddCommand(newPlanCommand(exitCode))`, adicionar:

  Atual (linhas 66–69):
  ```go
  	root.AddCommand(newInitCommand(exitCode))
  	root.AddCommand(newScanCommand(exitCode))
  	root.AddCommand(newPlanCommand(exitCode))
  
  	return root
  ```

  Novo:
  ```go
  	root.AddCommand(newInitCommand(exitCode))
  	root.AddCommand(newScanCommand(exitCode))
  	root.AddCommand(newPlanCommand(exitCode))
  	root.AddCommand(newMigrateCommand(exitCode))
  	root.AddCommand(newValidateCommand(exitCode))
  
  	return root
  ```

- [ ] Adicionar ao **final** de `internal/cli/root.go` (após `newPlanCommand`) as duas novas funções:

```go
func newMigrateCommand(exitCode *int) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Executa migration-plan.json, mirror-clonando cada repositório de origem e empurrando para o destino",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := cmd.Flags().GetString(configFlagName)
			if err != nil {
				return err
			}
			dryRun, err := cmd.Flags().GetBool(dryRunFlagName)
			if err != nil {
				return err
			}
			concurrency, err := cmd.Flags().GetInt(concurrencyFlagName)
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			*exitCode = runMigrate(ctx, configPath, dryRun, concurrency, cmd.OutOrStdout(), cmd.ErrOrStderr())
			return nil
		},
	}
	cmd.Flags().Bool(dryRunFlagName, false, "valida conectividade com o destino sem clonar nem empurrar repositórios")
	cmd.Flags().Int(concurrencyFlagName, defaultScanConcurrency, "número de repositórios migrados em paralelo")
	return cmd
}

func newValidateCommand(exitCode *int) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Compara branches e tags entre origem e destino usando migration-plan.json",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := cmd.Flags().GetString(configFlagName)
			if err != nil {
				return err
			}
			*exitCode = runValidate(cmd.Context(), configPath, cmd.OutOrStdout(), cmd.ErrOrStderr())
			return nil
		},
	}
}
```

### Passo 3.4 — Verificar que todos os testes do pacote passam

- [ ] Rodar:
  ```bash
  go test ./internal/cli/ -v
  ```
  Esperado: todos os testes do pacote (incluindo os 4 novos) `PASS`.

### Passo 3.5 — Verificar compilação, vet e formato

- [ ] Rodar:
  ```bash
  go build ./...
  go vet ./...
  gofmt -l .
  ```
  Esperado: sem saída (nenhum erro, nenhum arquivo mal formatado).

### Passo 3.6 — Rodar suite completa com race detector

- [ ] Rodar:
  ```bash
  go test ./... -race
  ```
  Esperado: `ok` em todos os pacotes, sem data races.

### Passo 3.7 — Commitar root.go + root_test.go

- [ ] Commitar:
  ```bash
  git add internal/cli/root.go internal/cli/root_test.go
  git commit -m "feat(cli): wire migrate and validate commands into root"
  ```

### Passo 3.8 — Commitar o documento do plano

- [ ] Commitar:
  ```bash
  git add docs/superpowers/plans/2026-06-15-tfrepo-04c-migrate-validate.md
  git commit -m "docs: add Plano 4c (migrate/validate commands) implementation plan"
  ```

---

## Verificação Final

Após os três commits acima, confirmar que o repositório está limpo:

```bash
# Todos os testes passam, sem race conditions
go test ./... -race

# Build limpo
go build ./...

# Sem problemas de análise estática
go vet ./...

# Sem arquivos mal formatados
gofmt -l .

# Binary executável responde ao help mostrando migrate e validate
go run ./cmd/tfrepo help
```

Esperado em `go run ./cmd/tfrepo help`: listar `migrate`, `validate`, `scan`, `plan`, `init` como subcomandos.

---

## O que vem a seguir (Plano 4d)

- `README.md` com instruções de instalação e uso
- Configuração de `goreleaser` para build multiplataforma
- GitHub Actions workflow de release (`on: push: tags: ['v*']`)
