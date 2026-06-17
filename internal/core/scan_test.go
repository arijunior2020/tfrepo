package core

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/arijunior2020/tfrepo/internal/provider"
)

// fakeScanProvider is a minimal provider.RepositoryProvider used to test
// Scan without making network calls. Only the methods Scan calls
// (Name, ListNamespaces, ListRepositories, GetRepositoryDetails) return
// real data; the rest satisfy the interface but are not exercised here.
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
		return provider.RepositoryDetails{}, fmt.Errorf("no details for %q", repo)
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

func (f *fakeScanProvider) ListLabels(ctx context.Context, namespace, repo string) ([]provider.Label, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeScanProvider) CreateLabel(ctx context.Context, namespace, repo string, label provider.Label) (provider.Label, error) {
	return provider.Label{}, errors.New("not implemented")
}

func (f *fakeScanProvider) ListMilestones(ctx context.Context, namespace, repo string) ([]provider.Milestone, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeScanProvider) CreateMilestone(ctx context.Context, namespace, repo string, m provider.Milestone) (provider.Milestone, error) {
	return provider.Milestone{}, errors.New("not implemented")
}

func (f *fakeScanProvider) ListIssues(_ context.Context, _, _ string) ([]provider.Issue, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeScanProvider) CreateIssue(_ context.Context, _, _ string, _ provider.Issue) (provider.Issue, error) {
	return provider.Issue{}, errors.New("not implemented")
}

func (f *fakeScanProvider) ListPullRequests(_ context.Context, _, _ string) ([]provider.PullRequest, error) {
	panic("not implemented")
}

func (f *fakeScanProvider) CreatePullRequest(_ context.Context, _, _ string, _ provider.PullRequest) (provider.PullRequest, error) {
	panic("not implemented")
}

var _ provider.RepositoryProvider = (*fakeScanProvider)(nil)

func TestScanBuildsInventory(t *testing.T) {
	p := &fakeScanProvider{
		name: "github",
		namespaces: []provider.Namespace{
			{Slug: "my-org", Name: "My Org", Kind: provider.NamespaceOrganization},
		},
		repositories: []provider.RepositorySummary{
			{Name: "repo-a", Namespace: "my-org", DefaultBranch: "main", Visibility: provider.VisibilityPrivate, SizeKB: 100},
		},
		details: map[string]provider.RepositoryDetails{
			"repo-a": {
				RepositorySummary: provider.RepositorySummary{
					Name: "repo-a", Namespace: "my-org", DefaultBranch: "main", Visibility: provider.VisibilityPrivate, SizeKB: 100,
				},
				Branches: []string{"main", "develop"},
				Tags:     []string{"v1.0.0"},
			},
		},
	}

	inv, err := Scan(context.Background(), p, "my-org", 4)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if inv.Source != (ProviderRef{Provider: "github", Namespace: "my-org"}) {
		t.Errorf("Source = %+v, want {github my-org}", inv.Source)
	}

	want := []InventoryNamespace{
		{
			Slug: "my-org",
			Name: "My Org",
			Kind: provider.NamespaceOrganization,
			Repositories: []RepositoryInventory{
				{
					Name:          "repo-a",
					DefaultBranch: "main",
					Visibility:    provider.VisibilityPrivate,
					SizeKB:        100,
					Branches:      []string{"main", "develop"},
					Tags:          []string{"v1.0.0"},
				},
			},
		},
	}

	if !reflect.DeepEqual(inv.Namespaces, want) {
		t.Errorf("Namespaces = %+v, want %+v", inv.Namespaces, want)
	}

	if inv.GeneratedAt.IsZero() {
		t.Error("GeneratedAt is zero, want a real timestamp")
	}
}

func TestScanEmptyRepositories(t *testing.T) {
	p := &fakeScanProvider{
		name: "github",
		namespaces: []provider.Namespace{
			{Slug: "my-org", Name: "My Org", Kind: provider.NamespaceOrganization},
		},
		details: map[string]provider.RepositoryDetails{},
	}

	inv, err := Scan(context.Background(), p, "my-org", 4)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if len(inv.Namespaces) != 1 {
		t.Fatalf("len(Namespaces) = %d, want 1", len(inv.Namespaces))
	}
	if len(inv.Namespaces[0].Repositories) != 0 {
		t.Errorf("Repositories = %+v, want empty", inv.Namespaces[0].Repositories)
	}
}

func TestScanNamespaceNotFound(t *testing.T) {
	p := &fakeScanProvider{
		name: "github",
		namespaces: []provider.Namespace{
			{Slug: "other-org", Name: "Other", Kind: provider.NamespaceOrganization},
		},
	}

	_, err := Scan(context.Background(), p, "my-org", 4)
	if err == nil {
		t.Fatal("Scan() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "my-org") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "my-org")
	}
}

func TestScanConcurrencyPreservesOrder(t *testing.T) {
	p := &fakeScanProvider{
		name: "github",
		namespaces: []provider.Namespace{
			{Slug: "my-org", Name: "My Org", Kind: provider.NamespaceOrganization},
		},
		repositories: []provider.RepositorySummary{
			{Name: "repo-a"},
			{Name: "repo-b"},
			{Name: "repo-c"},
		},
		details: map[string]provider.RepositoryDetails{
			"repo-a": {RepositorySummary: provider.RepositorySummary{Name: "repo-a"}, Branches: []string{}, Tags: []string{}},
			"repo-b": {RepositorySummary: provider.RepositorySummary{Name: "repo-b"}, Branches: []string{}, Tags: []string{}},
			"repo-c": {RepositorySummary: provider.RepositorySummary{Name: "repo-c"}, Branches: []string{}, Tags: []string{}},
		},
	}

	inv, err := Scan(context.Background(), p, "my-org", 2)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	var names []string
	for _, r := range inv.Namespaces[0].Repositories {
		names = append(names, r.Name)
	}
	want := []string{"repo-a", "repo-b", "repo-c"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("Repositories order = %v, want %v", names, want)
	}
}
