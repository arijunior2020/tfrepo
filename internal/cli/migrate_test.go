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
func (f *fakeMigrateCliProvider) ListLabels(_ context.Context, _, _ string) ([]provider.Label, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeMigrateCliProvider) CreateLabel(_ context.Context, _, _ string, _ provider.Label) (provider.Label, error) {
	return provider.Label{}, errors.New("not implemented")
}
func (f *fakeMigrateCliProvider) ListMilestones(_ context.Context, _, _ string) ([]provider.Milestone, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeMigrateCliProvider) CreateMilestone(_ context.Context, _, _ string, _ provider.Milestone) (provider.Milestone, error) {
	return provider.Milestone{}, errors.New("not implemented")
}

func (f *fakeMigrateCliProvider) ListIssues(_ context.Context, _, _ string) ([]provider.Issue, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeMigrateCliProvider) CreateIssue(_ context.Context, _, _ string, _ provider.Issue) (provider.Issue, error) {
	return provider.Issue{}, errors.New("not implemented")
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
