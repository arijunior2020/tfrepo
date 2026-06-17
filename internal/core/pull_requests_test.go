package core_test

import (
	"context"
	"errors"
	"testing"

	"github.com/arijunior2020/tfrepo/internal/core"
	"github.com/arijunior2020/tfrepo/internal/provider"
)

// fakePRsProvider implementa provider.RepositoryProvider para testes de PRs.
type fakePRsProvider struct {
	prs       map[string][]provider.PullRequest // chave: "namespace/repo"
	createErr error
	created   []provider.PullRequest
}

func (f *fakePRsProvider) Name() string { return "fake" }
func (f *fakePRsProvider) ListNamespaces(_ context.Context) ([]provider.Namespace, error) {
	panic("not implemented")
}
func (f *fakePRsProvider) ListRepositories(_ context.Context, _ string) ([]provider.RepositorySummary, error) {
	panic("not implemented")
}
func (f *fakePRsProvider) GetRepositoryDetails(_ context.Context, _, _ string) (provider.RepositoryDetails, error) {
	panic("not implemented")
}
func (f *fakePRsProvider) CreateRepository(_ context.Context, _ string, _ provider.CreateRepositoryInput) (provider.RepositorySummary, error) {
	panic("not implemented")
}
func (f *fakePRsProvider) GetAuthenticatedCloneURL(_, _ string) string { panic("not implemented") }
func (f *fakePRsProvider) GetRepositoryState(_ context.Context, _, _ string) (provider.RepositoryState, error) {
	panic("not implemented")
}
func (f *fakePRsProvider) ListLabels(_ context.Context, _, _ string) ([]provider.Label, error) {
	panic("not implemented")
}
func (f *fakePRsProvider) CreateLabel(_ context.Context, _, _ string, _ provider.Label) (provider.Label, error) {
	panic("not implemented")
}
func (f *fakePRsProvider) ListMilestones(_ context.Context, _, _ string) ([]provider.Milestone, error) {
	panic("not implemented")
}
func (f *fakePRsProvider) CreateMilestone(_ context.Context, _, _ string, _ provider.Milestone) (provider.Milestone, error) {
	panic("not implemented")
}
func (f *fakePRsProvider) ListIssues(_ context.Context, _, _ string) ([]provider.Issue, error) {
	panic("not implemented")
}
func (f *fakePRsProvider) CreateIssue(_ context.Context, _, _ string, _ provider.Issue) (provider.Issue, error) {
	panic("not implemented")
}
func (f *fakePRsProvider) ListPullRequests(_ context.Context, namespace, repo string) ([]provider.PullRequest, error) {
	return f.prs[namespace+"/"+repo], nil
}
func (f *fakePRsProvider) CreatePullRequest(_ context.Context, _, _ string, pr provider.PullRequest) (provider.PullRequest, error) {
	if f.createErr != nil {
		return provider.PullRequest{}, f.createErr
	}
	created := pr
	created.ExternalID = int64(len(f.created) + 100)
	f.created = append(f.created, created)
	return created, nil
}

func TestMigratePullRequests_HappyPath(t *testing.T) {
	source := &fakePRsProvider{
		prs: map[string][]provider.PullRequest{
			"srcorg/repo1": {
				{ExternalID: 1, Title: "PR A", State: "open", SourceBranch: "feat/a", TargetBranch: "main"},
				{ExternalID: 2, Title: "PR B", State: "open", SourceBranch: "feat/b", TargetBranch: "main"},
			},
		},
	}
	target := &fakePRsProvider{
		prs: map[string][]provider.PullRequest{
			"tgtorg/repo1": {},
		},
	}

	plan := core.MigrationPlan{
		Tasks: []core.MigrationTask{
			{ID: "t1", Source: core.TaskEndpoint{Namespace: "srcorg", Repo: "repo1"}, Target: core.TaskEndpoint{Namespace: "tgtorg", Repo: "repo1"}},
		},
	}
	report, err := core.MigratePullRequests(t.Context(), plan, core.PRsMigrateProviders{Source: source, Target: target})
	if err != nil {
		t.Fatalf("MigratePullRequests: %v", err)
	}
	if len(report.Results) != 1 {
		t.Fatalf("len(Results) = %d, want 1", len(report.Results))
	}
	r := report.Results[0]
	if r.Status != "success" {
		t.Errorf("Status = %q, want success", r.Status)
	}
	if r.PRsCreated != 2 {
		t.Errorf("PRsCreated = %d, want 2", r.PRsCreated)
	}
}

func TestMigratePullRequests_SkipsExistingByTitle(t *testing.T) {
	source := &fakePRsProvider{
		prs: map[string][]provider.PullRequest{
			"srcorg/repo1": {
				{ExternalID: 1, Title: "Existing PR", State: "open", SourceBranch: "feat/x", TargetBranch: "main"},
				{ExternalID: 2, Title: "New PR", State: "open", SourceBranch: "feat/y", TargetBranch: "main"},
			},
		},
	}
	target := &fakePRsProvider{
		prs: map[string][]provider.PullRequest{
			"tgtorg/repo1": {
				{ExternalID: 50, Title: "Existing PR", State: "open", SourceBranch: "feat/x", TargetBranch: "main"},
			},
		},
	}

	plan := core.MigrationPlan{
		Tasks: []core.MigrationTask{
			{ID: "t1", Source: core.TaskEndpoint{Namespace: "srcorg", Repo: "repo1"}, Target: core.TaskEndpoint{Namespace: "tgtorg", Repo: "repo1"}},
		},
	}
	report, err := core.MigratePullRequests(t.Context(), plan, core.PRsMigrateProviders{Source: source, Target: target})
	if err != nil {
		t.Fatalf("MigratePullRequests: %v", err)
	}
	r := report.Results[0]
	if r.PRsCreated != 1 {
		t.Errorf("PRsCreated = %d, want 1 (PR existente deve ser ignorado)", r.PRsCreated)
	}
}

func TestMigratePullRequests_CreateError(t *testing.T) {
	source := &fakePRsProvider{
		prs: map[string][]provider.PullRequest{
			"srcorg/repo1": {
				{ExternalID: 1, Title: "Failing PR", State: "open", SourceBranch: "feat/err", TargetBranch: "main"},
			},
		},
	}
	target := &fakePRsProvider{
		prs:       map[string][]provider.PullRequest{"tgtorg/repo1": {}},
		createErr: errors.New("branch not found"),
	}

	plan := core.MigrationPlan{
		Tasks: []core.MigrationTask{
			{ID: "t1", Source: core.TaskEndpoint{Namespace: "srcorg", Repo: "repo1"}, Target: core.TaskEndpoint{Namespace: "tgtorg", Repo: "repo1"}},
		},
	}
	report, err := core.MigratePullRequests(t.Context(), plan, core.PRsMigrateProviders{Source: source, Target: target})
	if err != nil {
		t.Fatalf("MigratePullRequests returned unexpected error: %v", err)
	}
	r := report.Results[0]
	if r.Status != "failed" {
		t.Errorf("Status = %q, want failed", r.Status)
	}
	if len(r.Errors) == 0 {
		t.Error("Errors deve ter ao menos uma entrada")
	}
}
