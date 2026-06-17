package core

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/arijunior2020/tfrepo/internal/provider"
)

// fakeIssuesProvider implements provider.RepositoryProvider for issue migration tests.
// Only ListIssues and CreateIssue are functional; all other methods return errors.
type fakeIssuesProvider struct {
	issues    map[string][]provider.Issue // key: "namespace/repo"
	createErr error
	created   []provider.Issue
}

func (f *fakeIssuesProvider) Name() string { return "fake" }
func (f *fakeIssuesProvider) ListNamespaces(_ context.Context) ([]provider.Namespace, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeIssuesProvider) ListRepositories(_ context.Context, _ string) ([]provider.RepositorySummary, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeIssuesProvider) GetRepositoryDetails(_ context.Context, _, _ string) (provider.RepositoryDetails, error) {
	return provider.RepositoryDetails{}, errors.New("not implemented")
}
func (f *fakeIssuesProvider) CreateRepository(_ context.Context, _ string, _ provider.CreateRepositoryInput) (provider.RepositorySummary, error) {
	return provider.RepositorySummary{}, errors.New("not implemented")
}
func (f *fakeIssuesProvider) GetAuthenticatedCloneURL(_, _ string) string { return "" }
func (f *fakeIssuesProvider) GetRepositoryState(_ context.Context, _, _ string) (provider.RepositoryState, error) {
	return provider.RepositoryState{}, errors.New("not implemented")
}
func (f *fakeIssuesProvider) ListLabels(_ context.Context, _, _ string) ([]provider.Label, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeIssuesProvider) CreateLabel(_ context.Context, _, _ string, _ provider.Label) (provider.Label, error) {
	return provider.Label{}, errors.New("not implemented")
}
func (f *fakeIssuesProvider) ListMilestones(_ context.Context, _, _ string) ([]provider.Milestone, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeIssuesProvider) CreateMilestone(_ context.Context, _, _ string, _ provider.Milestone) (provider.Milestone, error) {
	return provider.Milestone{}, errors.New("not implemented")
}
func (f *fakeIssuesProvider) ListIssues(_ context.Context, namespace, repo string) ([]provider.Issue, error) {
	return f.issues[namespace+"/"+repo], nil
}
func (f *fakeIssuesProvider) CreateIssue(_ context.Context, _, _ string, issue provider.Issue) (provider.Issue, error) {
	if f.createErr != nil {
		return provider.Issue{}, f.createErr
	}
	created := issue
	created.ExternalID = int64(len(f.created) + 100)
	f.created = append(f.created, created)
	return created, nil
}

func (f *fakeIssuesProvider) ListPullRequests(_ context.Context, _, _ string) ([]provider.PullRequest, error) {
	panic("not implemented")
}

func (f *fakeIssuesProvider) CreatePullRequest(_ context.Context, _, _ string, _ provider.PullRequest) (provider.PullRequest, error) {
	panic("not implemented")
}

var _ provider.RepositoryProvider = (*fakeIssuesProvider)(nil)

func makeIssuePlan(tasks ...MigrationTask) MigrationPlan {
	return MigrationPlan{Tasks: tasks}
}

func makeIssueTask(id, srcNS, srcRepo, tgtNS, tgtRepo string) MigrationTask {
	return MigrationTask{
		ID:     id,
		Source: TaskEndpoint{Namespace: srcNS, Repo: srcRepo},
		Target: TaskEndpoint{Namespace: tgtNS, Repo: tgtRepo},
	}
}

func TestMigrateIssues_HappyPath(t *testing.T) {
	milestoneID := int64(10)
	source := &fakeIssuesProvider{
		issues: map[string][]provider.Issue{
			"srcorg/repo1": {
				{ExternalID: 1, Title: "Issue A", State: "open", Labels: []string{"bug"}},
				{ExternalID: 2, Title: "Issue B", State: "closed", MilestoneExternalID: &milestoneID},
			},
		},
	}
	target := &fakeIssuesProvider{
		issues: map[string][]provider.Issue{
			"tgtorg/repo1": {},
		},
	}

	milestoneMap := MilestoneIDMap{
		strconv.FormatInt(10, 10): int64(99),
	}

	plan := makeIssuePlan(makeIssueTask("t1", "srcorg", "repo1", "tgtorg", "repo1"))
	report, err := MigrateIssues(context.Background(), plan,
		IssuesMigrateProviders{Source: source, Target: target},
		map[string]MilestoneIDMap{"t1": milestoneMap})
	if err != nil {
		t.Fatalf("MigrateIssues: %v", err)
	}
	if len(report.Results) != 1 {
		t.Fatalf("len(Results) = %d, want 1", len(report.Results))
	}
	r := report.Results[0]
	if r.Status != "success" {
		t.Errorf("Status = %q, want success; errors: %v", r.Status, r.Errors)
	}
	if r.IssuesCreated != 2 {
		t.Errorf("IssuesCreated = %d, want 2", r.IssuesCreated)
	}
	// Milestone on Issue B must be translated: source 10 → target 99
	if target.created[1].MilestoneExternalID == nil || *target.created[1].MilestoneExternalID != 99 {
		t.Errorf("MilestoneExternalID = %v, want &99 (translated via MilestoneIDMap)", target.created[1].MilestoneExternalID)
	}
}

func TestMigrateIssues_SkipsExistingByTitle(t *testing.T) {
	source := &fakeIssuesProvider{
		issues: map[string][]provider.Issue{
			"srcorg/repo1": {
				{ExternalID: 1, Title: "Already there", State: "open"},
				{ExternalID: 2, Title: "New issue", State: "open"},
			},
		},
	}
	target := &fakeIssuesProvider{
		issues: map[string][]provider.Issue{
			"tgtorg/repo1": {
				{ExternalID: 50, Title: "Already there", State: "open"},
			},
		},
	}

	plan := makeIssuePlan(makeIssueTask("t1", "srcorg", "repo1", "tgtorg", "repo1"))
	report, err := MigrateIssues(context.Background(), plan,
		IssuesMigrateProviders{Source: source, Target: target},
		nil)
	if err != nil {
		t.Fatalf("MigrateIssues: %v", err)
	}
	r := report.Results[0]
	if r.IssuesCreated != 1 {
		t.Errorf("IssuesCreated = %d, want 1 (existing issue must be skipped)", r.IssuesCreated)
	}
}

func TestMigrateIssues_CreateError(t *testing.T) {
	source := &fakeIssuesProvider{
		issues: map[string][]provider.Issue{
			"srcorg/repo1": {
				{ExternalID: 1, Title: "Failing issue", State: "open"},
			},
		},
	}
	target := &fakeIssuesProvider{
		issues:    map[string][]provider.Issue{"tgtorg/repo1": {}},
		createErr: errors.New("api error"),
	}

	plan := makeIssuePlan(makeIssueTask("t1", "srcorg", "repo1", "tgtorg", "repo1"))
	report, err := MigrateIssues(context.Background(), plan,
		IssuesMigrateProviders{Source: source, Target: target},
		nil)
	if err != nil {
		t.Fatalf("MigrateIssues returned unexpected top-level error: %v", err)
	}
	r := report.Results[0]
	if r.Status != "failed" {
		t.Errorf("Status = %q, want failed", r.Status)
	}
	if len(r.Errors) == 0 {
		t.Error("Errors must have at least one entry")
	}
}

func TestMigrateIssues_NoMilestoneMap(t *testing.T) {
	milestoneID := int64(10)
	source := &fakeIssuesProvider{
		issues: map[string][]provider.Issue{
			"srcorg/repo1": {
				{ExternalID: 1, Title: "Issue", State: "open", MilestoneExternalID: &milestoneID},
			},
		},
	}
	target := &fakeIssuesProvider{
		issues: map[string][]provider.Issue{"tgtorg/repo1": {}},
	}

	plan := makeIssuePlan(makeIssueTask("t1", "srcorg", "repo1", "tgtorg", "repo1"))
	report, err := MigrateIssues(context.Background(), plan,
		IssuesMigrateProviders{Source: source, Target: target},
		nil) // no map → milestone must be dropped
	if err != nil {
		t.Fatalf("MigrateIssues: %v", err)
	}
	if len(report.Results) != 1 || report.Results[0].Status != "success" {
		t.Errorf("Unexpected result: %+v", report.Results)
	}
	if target.created[0].MilestoneExternalID != nil {
		t.Errorf("MilestoneExternalID = %v, want nil (no translation map)", target.created[0].MilestoneExternalID)
	}
}
