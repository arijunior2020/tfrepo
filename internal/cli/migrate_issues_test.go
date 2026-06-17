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

// fakeMigrateIssuesProvider implements provider.RepositoryProvider for
// migrate-issues CLI tests. Only ListIssues and CreateIssue are functional;
// all other methods return errors.
type fakeMigrateIssuesProvider struct {
	issues    map[string][]provider.Issue // key: "namespace/repo"
	createErr error
	created   []provider.Issue
}

func (f *fakeMigrateIssuesProvider) Name() string { return "fake" }
func (f *fakeMigrateIssuesProvider) ListNamespaces(_ context.Context) ([]provider.Namespace, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeMigrateIssuesProvider) ListRepositories(_ context.Context, _ string) ([]provider.RepositorySummary, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeMigrateIssuesProvider) GetRepositoryDetails(_ context.Context, _, _ string) (provider.RepositoryDetails, error) {
	return provider.RepositoryDetails{}, errors.New("not implemented")
}
func (f *fakeMigrateIssuesProvider) CreateRepository(_ context.Context, _ string, _ provider.CreateRepositoryInput) (provider.RepositorySummary, error) {
	return provider.RepositorySummary{}, errors.New("not implemented")
}
func (f *fakeMigrateIssuesProvider) GetAuthenticatedCloneURL(_, _ string) string { return "" }
func (f *fakeMigrateIssuesProvider) GetRepositoryState(_ context.Context, _, _ string) (provider.RepositoryState, error) {
	return provider.RepositoryState{}, errors.New("not implemented")
}
func (f *fakeMigrateIssuesProvider) ListLabels(_ context.Context, _, _ string) ([]provider.Label, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeMigrateIssuesProvider) CreateLabel(_ context.Context, _, _ string, _ provider.Label) (provider.Label, error) {
	return provider.Label{}, errors.New("not implemented")
}
func (f *fakeMigrateIssuesProvider) ListMilestones(_ context.Context, _, _ string) ([]provider.Milestone, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeMigrateIssuesProvider) CreateMilestone(_ context.Context, _, _ string, _ provider.Milestone) (provider.Milestone, error) {
	return provider.Milestone{}, errors.New("not implemented")
}
func (f *fakeMigrateIssuesProvider) ListIssues(_ context.Context, namespace, repo string) ([]provider.Issue, error) {
	return f.issues[namespace+"/"+repo], nil
}
func (f *fakeMigrateIssuesProvider) CreateIssue(_ context.Context, _, _ string, issue provider.Issue) (provider.Issue, error) {
	if f.createErr != nil {
		return provider.Issue{}, f.createErr
	}
	created := issue
	created.ExternalID = int64(len(f.created) + 100)
	f.created = append(f.created, created)
	return created, nil
}

var _ provider.RepositoryProvider = (*fakeMigrateIssuesProvider)(nil)

func TestRunMigrateIssuesWithProviders_WritesReport(t *testing.T) {
	dir := t.TempDir()

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })
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

	src := &fakeMigrateIssuesProvider{
		issues: map[string][]provider.Issue{
			"src/repo": {{ExternalID: 1, Title: "Issue 1", State: "open"}},
		},
	}
	tgt := &fakeMigrateIssuesProvider{
		issues: map[string][]provider.Issue{"tgt/repo": {}},
	}

	var stdout, stderr bytes.Buffer
	exitCode := runMigrateIssuesWithProviders(context.Background(), "", &stdout, &stderr, src, tgt)

	if exitCode != 0 {
		t.Errorf("exitCode = %d, want 0; stderr = %s", exitCode, stderr.String())
	}
	reportPath := filepath.Join(dir, issuesReportPath)
	if _, err := os.Stat(reportPath); err != nil {
		t.Errorf("issues-report.json not created: %v", err)
	}
	var report core.IssuesReport
	if err := core.ReadJSON(issuesReportPath, &report); err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
	if len(report.Results) != 1 || report.Results[0].IssuesCreated != 1 {
		t.Errorf("Unexpected report: %+v", report.Results)
	}
}
