package core

import (
	"context"
	"errors"
	"testing"

	"github.com/arijunior2020/tfrepo/internal/provider"
)

type fakeLabelsProvider struct {
	name string

	listLabels     []provider.Label
	listLabelsErr  error
	createLabelErr error
	createdLabels  []provider.Label

	listMilestones     []provider.Milestone
	listMilestonesErr  error
	createMilestoneErr error
	nextMilestoneID    int64
	createdMilestones  []provider.Milestone
}

func (f *fakeLabelsProvider) Name() string { return f.name }
func (f *fakeLabelsProvider) ListNamespaces(_ context.Context) ([]provider.Namespace, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeLabelsProvider) ListRepositories(_ context.Context, _ string) ([]provider.RepositorySummary, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeLabelsProvider) GetRepositoryDetails(_ context.Context, _, _ string) (provider.RepositoryDetails, error) {
	return provider.RepositoryDetails{}, errors.New("not implemented")
}
func (f *fakeLabelsProvider) CreateRepository(_ context.Context, _ string, _ provider.CreateRepositoryInput) (provider.RepositorySummary, error) {
	return provider.RepositorySummary{}, errors.New("not implemented")
}
func (f *fakeLabelsProvider) GetAuthenticatedCloneURL(_, _ string) string { return "" }
func (f *fakeLabelsProvider) GetRepositoryState(_ context.Context, _, _ string) (provider.RepositoryState, error) {
	return provider.RepositoryState{}, errors.New("not implemented")
}
func (f *fakeLabelsProvider) ListLabels(_ context.Context, _, _ string) ([]provider.Label, error) {
	return f.listLabels, f.listLabelsErr
}
func (f *fakeLabelsProvider) CreateLabel(_ context.Context, _, _ string, label provider.Label) (provider.Label, error) {
	if f.createLabelErr != nil {
		return provider.Label{}, f.createLabelErr
	}
	f.createdLabels = append(f.createdLabels, label)
	return label, nil
}
func (f *fakeLabelsProvider) ListMilestones(_ context.Context, _, _ string) ([]provider.Milestone, error) {
	return f.listMilestones, f.listMilestonesErr
}
func (f *fakeLabelsProvider) CreateMilestone(_ context.Context, _, _ string, m provider.Milestone) (provider.Milestone, error) {
	if f.createMilestoneErr != nil {
		return provider.Milestone{}, f.createMilestoneErr
	}
	f.nextMilestoneID++
	created := m
	created.ExternalID = f.nextMilestoneID
	f.createdMilestones = append(f.createdMilestones, created)
	return created, nil
}

var _ provider.RepositoryProvider = (*fakeLabelsProvider)(nil)

func baseLabelsPlan() MigrationPlan {
	return MigrationPlan{
		Source: ProviderRef{Provider: "github", Namespace: "my-org"},
		Target: ProviderRef{Provider: "gitlab", Namespace: "my-group"},
		Tasks: []MigrationTask{
			{
				ID:     "repo-a",
				Source: TaskEndpoint{Namespace: "my-org", Repo: "repo-a"},
				Target: TaskEndpoint{Namespace: "my-group", Repo: "repo-a"},
			},
		},
	}
}

func TestMigrateLabelsAndMilestones_Success(t *testing.T) {
	source := &fakeLabelsProvider{
		name:       "github",
		listLabels: []provider.Label{{Name: "bug", Color: "d73a4a", Description: "A bug"}},
		listMilestones: []provider.Milestone{
			{ExternalID: 1, Title: "v1.0", State: provider.MilestoneStateOpen},
		},
	}
	target := &fakeLabelsProvider{name: "gitlab", nextMilestoneID: 99}

	report, err := MigrateLabelsAndMilestones(context.Background(), baseLabelsPlan(), LabelsMigrateProviders{Source: source, Target: target})
	if err != nil {
		t.Fatalf("MigrateLabelsAndMilestones: %v", err)
	}
	if len(report.Results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(report.Results))
	}
	r := report.Results[0]
	if r.Status != "success" {
		t.Errorf("status = %q, want success (errors: %v)", r.Status, r.Errors)
	}
	if r.LabelsCreated != 1 {
		t.Errorf("LabelsCreated = %d, want 1", r.LabelsCreated)
	}
	if r.MilestonesCreated != 1 {
		t.Errorf("MilestonesCreated = %d, want 1", r.MilestonesCreated)
	}
	// source ExternalID 1 → target ExternalID 100 (nextMilestoneID was 99, increments to 100)
	if r.MilestoneIDMap["1"] != 100 {
		t.Errorf("MilestoneIDMap[\"1\"] = %d, want 100", r.MilestoneIDMap["1"])
	}
	if len(target.createdLabels) != 1 || target.createdLabels[0].Name != "bug" {
		t.Errorf("createdLabels = %+v", target.createdLabels)
	}
}

func TestMigrateLabelsAndMilestones_SkipsExistingLabels(t *testing.T) {
	source := &fakeLabelsProvider{
		name: "github",
		listLabels: []provider.Label{
			{Name: "bug", Color: "d73a4a"},
			{Name: "enhancement", Color: "a2eeef"},
		},
	}
	target := &fakeLabelsProvider{
		name:       "gitlab",
		listLabels: []provider.Label{{Name: "bug", Color: "d73a4a"}}, // already exists
	}

	report, err := MigrateLabelsAndMilestones(context.Background(), baseLabelsPlan(), LabelsMigrateProviders{Source: source, Target: target})
	if err != nil {
		t.Fatalf("MigrateLabelsAndMilestones: %v", err)
	}
	r := report.Results[0]
	if r.LabelsCreated != 1 {
		t.Errorf("LabelsCreated = %d, want 1 (only 'enhancement' should be created)", r.LabelsCreated)
	}
	if len(target.createdLabels) != 1 || target.createdLabels[0].Name != "enhancement" {
		t.Errorf("createdLabels = %+v, want [{Name:enhancement ...}]", target.createdLabels)
	}
}

func TestMigrateLabelsAndMilestones_SkipsExistingMilestonesButMapsID(t *testing.T) {
	source := &fakeLabelsProvider{
		name: "github",
		listMilestones: []provider.Milestone{
			{ExternalID: 1, Title: "v1.0", State: provider.MilestoneStateOpen},
		},
	}
	target := &fakeLabelsProvider{
		name: "gitlab",
		listMilestones: []provider.Milestone{
			{ExternalID: 42, Title: "v1.0", State: provider.MilestoneStateOpen}, // already exists
		},
	}

	report, err := MigrateLabelsAndMilestones(context.Background(), baseLabelsPlan(), LabelsMigrateProviders{Source: source, Target: target})
	if err != nil {
		t.Fatalf("MigrateLabelsAndMilestones: %v", err)
	}
	r := report.Results[0]
	if r.MilestonesCreated != 0 {
		t.Errorf("MilestonesCreated = %d, want 0 (milestone exists)", r.MilestonesCreated)
	}
	// Even skipped milestones must appear in the map
	if r.MilestoneIDMap["1"] != 42 {
		t.Errorf("MilestoneIDMap[\"1\"] = %d, want 42", r.MilestoneIDMap["1"])
	}
}

func TestMigrateLabelsAndMilestones_SourceListLabelsError(t *testing.T) {
	source := &fakeLabelsProvider{name: "github", listLabelsErr: errors.New("API timeout")}
	target := &fakeLabelsProvider{name: "gitlab"}

	report, err := MigrateLabelsAndMilestones(context.Background(), baseLabelsPlan(), LabelsMigrateProviders{Source: source, Target: target})
	if err != nil {
		t.Fatalf("unexpected top-level error: %v", err)
	}
	r := report.Results[0]
	if r.Status != "failed" {
		t.Errorf("status = %q, want failed", r.Status)
	}
	if len(r.Errors) == 0 {
		t.Error("expected at least one error in result")
	}
}

func TestMigrateLabelsAndMilestones_CreateLabelErrors(t *testing.T) {
	source := &fakeLabelsProvider{
		name:       "github",
		listLabels: []provider.Label{{Name: "bug"}, {Name: "enhancement"}},
	}
	target := &fakeLabelsProvider{
		name:           "gitlab",
		createLabelErr: errors.New("conflict"),
	}

	report, err := MigrateLabelsAndMilestones(context.Background(), baseLabelsPlan(), LabelsMigrateProviders{Source: source, Target: target})
	if err != nil {
		t.Fatalf("unexpected top-level error: %v", err)
	}
	r := report.Results[0]
	if r.Status != "failed" {
		t.Errorf("status = %q, want failed", r.Status)
	}
	if len(r.Errors) != 2 {
		t.Errorf("len(errors) = %d, want 2", len(r.Errors))
	}
}
