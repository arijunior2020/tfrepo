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
func (f *fakeValidateCliProvider) ListLabels(_ context.Context, _, _ string) ([]provider.Label, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeValidateCliProvider) CreateLabel(_ context.Context, _, _ string, _ provider.Label) (provider.Label, error) {
	return provider.Label{}, errors.New("not implemented")
}
func (f *fakeValidateCliProvider) ListMilestones(_ context.Context, _, _ string) ([]provider.Milestone, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeValidateCliProvider) CreateMilestone(_ context.Context, _, _ string, _ provider.Milestone) (provider.Milestone, error) {
	return provider.Milestone{}, errors.New("not implemented")
}

func (f *fakeValidateCliProvider) ListIssues(_ context.Context, _, _ string) ([]provider.Issue, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeValidateCliProvider) CreateIssue(_ context.Context, _, _ string, _ provider.Issue) (provider.Issue, error) {
	return provider.Issue{}, errors.New("not implemented")
}

func (f *fakeValidateCliProvider) ListPullRequests(_ context.Context, _, _ string) ([]provider.PullRequest, error) {
	panic("not implemented")
}

func (f *fakeValidateCliProvider) CreatePullRequest(_ context.Context, _, _ string, _ provider.PullRequest) (provider.PullRequest, error) {
	panic("not implemented")
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
