package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/arijunior2020/tfrepo/internal/core"
	"github.com/arijunior2020/tfrepo/internal/provider"
)

func writePlanFixtures(t *testing.T, configYAML string, inventory core.Inventory) string {
	t.Helper()

	dir := t.TempDir()
	t.Chdir(dir)

	configPath := filepath.Join(dir, "transferepo.config.yaml")
	if err := os.WriteFile(configPath, []byte(configYAML), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := core.WriteJSON(inventoryPath, inventory); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	return configPath
}

func basePlanInventory() core.Inventory {
	return core.Inventory{
		Source: core.ProviderRef{Provider: "github", Namespace: "my-org"},
		Namespaces: []core.InventoryNamespace{
			{
				Slug: "my-org",
				Name: "My Org",
				Kind: provider.NamespaceOrganization,
				Repositories: []core.RepositoryInventory{
					{Name: "repo-a", DefaultBranch: "main", Visibility: provider.VisibilityPrivate, Branches: []string{"main", "develop"}, Tags: []string{"v1.0.0"}},
					{Name: "repo-b", DefaultBranch: "main", Visibility: provider.VisibilityPublic, Branches: []string{"main"}, Tags: []string{}},
				},
			},
		},
	}
}

const basePlanConfigYAML = `source:
  provider: github
  namespace: my-org
target:
  provider: gitlab
  namespace: my-group
`

func TestRunPlanWritesMigrationPlanAndPrintsSummary(t *testing.T) {
	configPath := writePlanFixtures(t, basePlanConfigYAML, basePlanInventory())

	var stdout, stderr bytes.Buffer
	code := runPlan(configPath, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("runPlan() = %d, want 0 (stderr: %s)", code, stderr.String())
	}

	var plan core.MigrationPlan
	if err := core.ReadJSON(migrationPlanPath, &plan); err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
	if len(plan.Tasks) != 2 {
		t.Fatalf("len(Tasks) = %d, want 2", len(plan.Tasks))
	}

	want := "Wrote migration-plan.json (2 repositórios, 3 branches, 1 tag)\n"
	if stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestRunPlanAppliesFiltersFromConfig(t *testing.T) {
	configYAML := basePlanConfigYAML + `filters:
  include: ["repo-a"]
  exclude: []
`
	configPath := writePlanFixtures(t, configYAML, basePlanInventory())

	var stdout, stderr bytes.Buffer
	code := runPlan(configPath, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("runPlan() = %d, want 0 (stderr: %s)", code, stderr.String())
	}

	var plan core.MigrationPlan
	if err := core.ReadJSON(migrationPlanPath, &plan); err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
	if len(plan.Tasks) != 1 || plan.Tasks[0].ID != "repo-a" {
		t.Fatalf("Tasks = %+v, want only repo-a", plan.Tasks)
	}

	want := "Wrote migration-plan.json (1 repositório, 2 branches, 1 tag)\n"
	if stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestRunPlanReturnsErrorWhenInventoryMissing(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	configPath := filepath.Join(dir, "transferepo.config.yaml")
	if err := os.WriteFile(configPath, []byte(basePlanConfigYAML), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := runPlan(configPath, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("runPlan() = %d, want 1", code)
	}
	if stderr.String() == "" {
		t.Error("stderr is empty, want an error message")
	}
	if _, err := os.Stat(migrationPlanPath); !os.IsNotExist(err) {
		t.Error("migration-plan.json was created on error, want no file")
	}
}

func TestRunPlanReturnsErrorWhenConfigMissing(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	var stdout, stderr bytes.Buffer
	code := runPlan(filepath.Join(dir, "does-not-exist.yaml"), &stdout, &stderr)

	if code != 1 {
		t.Fatalf("runPlan() = %d, want 1", code)
	}
	if stderr.String() == "" {
		t.Error("stderr is empty, want an error message")
	}
}
