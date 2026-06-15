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
