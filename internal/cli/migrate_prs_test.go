package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/arijunior2020/tfrepo/internal/core"
	"github.com/arijunior2020/tfrepo/internal/provider"
)

// fakeMigratePRsProvider implementa apenas os métodos usados pelo migrate-prs.
type fakeMigratePRsProvider struct {
	prs       map[string][]provider.PullRequest
	createErr error
}

func (f *fakeMigratePRsProvider) Name() string { return "fake" }
func (f *fakeMigratePRsProvider) ListNamespaces(_ context.Context) ([]provider.Namespace, error) {
	panic("not implemented")
}
func (f *fakeMigratePRsProvider) ListRepositories(_ context.Context, _ string) ([]provider.RepositorySummary, error) {
	panic("not implemented")
}
func (f *fakeMigratePRsProvider) GetRepositoryDetails(_ context.Context, _, _ string) (provider.RepositoryDetails, error) {
	panic("not implemented")
}
func (f *fakeMigratePRsProvider) CreateRepository(_ context.Context, _ string, _ provider.CreateRepositoryInput) (provider.RepositorySummary, error) {
	panic("not implemented")
}
func (f *fakeMigratePRsProvider) GetAuthenticatedCloneURL(_, _ string) string {
	panic("not implemented")
}
func (f *fakeMigratePRsProvider) GetRepositoryState(_ context.Context, _, _ string) (provider.RepositoryState, error) {
	panic("not implemented")
}
func (f *fakeMigratePRsProvider) ListLabels(_ context.Context, _, _ string) ([]provider.Label, error) {
	panic("not implemented")
}
func (f *fakeMigratePRsProvider) CreateLabel(_ context.Context, _, _ string, _ provider.Label) (provider.Label, error) {
	panic("not implemented")
}
func (f *fakeMigratePRsProvider) ListMilestones(_ context.Context, _, _ string) ([]provider.Milestone, error) {
	panic("not implemented")
}
func (f *fakeMigratePRsProvider) CreateMilestone(_ context.Context, _, _ string, _ provider.Milestone) (provider.Milestone, error) {
	panic("not implemented")
}
func (f *fakeMigratePRsProvider) ListIssues(_ context.Context, _, _ string) ([]provider.Issue, error) {
	panic("not implemented")
}
func (f *fakeMigratePRsProvider) CreateIssue(_ context.Context, _, _ string, _ provider.Issue) (provider.Issue, error) {
	panic("not implemented")
}
func (f *fakeMigratePRsProvider) ListPullRequests(_ context.Context, namespace, repo string) ([]provider.PullRequest, error) {
	return f.prs[namespace+"/"+repo], nil
}
func (f *fakeMigratePRsProvider) CreatePullRequest(_ context.Context, _, _ string, pr provider.PullRequest) (provider.PullRequest, error) {
	if f.createErr != nil {
		return provider.PullRequest{}, f.createErr
	}
	pr.ExternalID = 99
	return pr, nil
}

func TestRunMigratePRsWithProviders_WritesReport(t *testing.T) {
	dir := t.TempDir()

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(origDir); err != nil {
			t.Logf("cleanup: failed to restore directory: %v", err)
		}
	})
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	plan := core.MigrationPlan{
		Tasks: []core.MigrationTask{
			{
				ID:     "t1",
				Source: core.TaskEndpoint{Namespace: "src", Repo: "repo"},
				Target: core.TaskEndpoint{Namespace: "tgt", Repo: "repo"},
			},
		},
	}
	if err := core.WriteJSON(migrationPlanPath, plan); err != nil {
		t.Fatal(err)
	}

	src := &fakeMigratePRsProvider{
		prs: map[string][]provider.PullRequest{
			"src/repo": {
				{ExternalID: 1, Title: "PR 1", State: "open", SourceBranch: "feat/1", TargetBranch: "main"},
			},
		},
	}
	tgt := &fakeMigratePRsProvider{
		prs: map[string][]provider.PullRequest{"tgt/repo": {}},
	}

	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"source":{"provider":"github","namespace":"src"},"target":{"provider":"github","namespace":"tgt"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	exitCode := runMigratePRsWithProviders(t.Context(), configPath, &stdout, &stderr, src, tgt)

	if exitCode != 0 {
		t.Errorf("exitCode = %d, want 0; stderr = %s", exitCode, stderr.String())
	}

	reportPath := filepath.Join(dir, prsReportPath)
	if _, err := os.Stat(reportPath); err != nil {
		t.Errorf("%s não foi criado: %v", prsReportPath, err)
	}

	var report core.PRsReport
	if err := core.ReadJSON(prsReportPath, &report); err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
	if len(report.Results) != 1 || report.Results[0].PRsCreated != 1 {
		t.Errorf("Report inesperado: %+v", report.Results)
	}
}
