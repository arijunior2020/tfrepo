package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arijunior2020/tfrepo/internal/core"
	"github.com/arijunior2020/tfrepo/internal/provider"
)

// fakeScanProvider is a minimal provider.RepositoryProvider used to test
// scanAndWrite without making network calls. Only the methods core.Scan
// calls (Name, ListNamespaces, ListRepositories, GetRepositoryDetails)
// return real data; the rest satisfy the interface but are not exercised
// here.
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
		return provider.RepositoryDetails{}, errors.New("no details for " + repo)
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

var _ provider.RepositoryProvider = (*fakeScanProvider)(nil)

func TestScanAndWriteWritesInventoryAndPrintsSummary(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	p := &fakeScanProvider{
		name: "github",
		namespaces: []provider.Namespace{
			{Slug: "my-org", Name: "My Org", Kind: provider.NamespaceOrganization},
		},
		repositories: []provider.RepositorySummary{
			{Name: "repo-a", Namespace: "my-org", DefaultBranch: "main", Visibility: provider.VisibilityPrivate},
		},
		details: map[string]provider.RepositoryDetails{
			"repo-a": {
				RepositorySummary: provider.RepositorySummary{Name: "repo-a", Namespace: "my-org", DefaultBranch: "main", Visibility: provider.VisibilityPrivate},
				Branches:          []string{"main"},
				Tags:              []string{"v1.0.0"},
			},
		},
	}

	var stdout, stderr bytes.Buffer
	code := scanAndWrite(context.Background(), p, "my-org", 1, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("scanAndWrite() = %d, want 0 (stderr: %s)", code, stderr.String())
	}

	var inventory core.Inventory
	if err := core.ReadJSON(inventoryPath, &inventory); err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
	if len(inventory.Namespaces) != 1 || len(inventory.Namespaces[0].Repositories) != 1 {
		t.Fatalf("inventory = %+v, want 1 namespace with 1 repository", inventory)
	}
	if inventory.Namespaces[0].Repositories[0].Name != "repo-a" {
		t.Errorf("Repositories[0].Name = %q, want %q", inventory.Namespaces[0].Repositories[0].Name, "repo-a")
	}

	if !strings.Contains(stdout.String(), "Wrote inventory.json (1 repositório)") {
		t.Errorf("stdout = %q, want it to mention the repository count", stdout.String())
	}
}

func TestScanAndWriteReturnsErrorWhenNamespaceNotFound(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	p := &fakeScanProvider{name: "github"}

	var stdout, stderr bytes.Buffer
	code := scanAndWrite(context.Background(), p, "my-org", 1, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("scanAndWrite() = %d, want 1", code)
	}
	if stderr.String() == "" {
		t.Error("stderr is empty, want an error message")
	}
	if _, err := os.Stat(inventoryPath); !os.IsNotExist(err) {
		t.Error("inventory.json was created on error, want no file")
	}
}

func TestRunScanReturnsErrorWhenTokenMissing(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	configPath := filepath.Join(dir, "transferepo.config.yaml")
	configYAML := `source:
  provider: github
  namespace: my-org
target:
  provider: gitlab
  namespace: my-group
`
	if err := os.WriteFile(configPath, []byte(configYAML), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	t.Setenv("GITHUB_TOKEN", "")

	var stdout, stderr bytes.Buffer
	code := runScan(context.Background(), configPath, 1, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("runScan() = %d, want 1", code)
	}
	if stderr.String() == "" {
		t.Error("stderr is empty, want an error message")
	}
}

func TestRunScanReturnsErrorWhenConfigMissing(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	var stdout, stderr bytes.Buffer
	code := runScan(context.Background(), filepath.Join(dir, "does-not-exist.yaml"), 1, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("runScan() = %d, want 1", code)
	}
	if stderr.String() == "" {
		t.Error("stderr is empty, want an error message")
	}
}
