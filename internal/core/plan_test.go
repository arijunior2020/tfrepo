package core

import (
	"reflect"
	"testing"

	"github.com/arijunior2020/tfrepo/internal/config"
	"github.com/arijunior2020/tfrepo/internal/provider"
)

func basePlanInventory() Inventory {
	return Inventory{
		Source: ProviderRef{Provider: "github", Namespace: "my-org"},
		Namespaces: []InventoryNamespace{
			{
				Slug: "my-org",
				Name: "My Org",
				Kind: provider.NamespaceOrganization,
				Repositories: []RepositoryInventory{
					{Name: "repo-a", DefaultBranch: "main", Visibility: provider.VisibilityPrivate, SizeKB: 100, Branches: []string{"main"}, Tags: []string{"v1.0.0"}},
					{Name: "repo-b", DefaultBranch: "main", Visibility: provider.VisibilityPublic, SizeKB: 50, Branches: []string{"main"}, Tags: []string{}},
					{Name: "legacy-repo", DefaultBranch: "main", Visibility: provider.VisibilityPrivate, SizeKB: 10, Branches: []string{"main"}, Tags: []string{}},
				},
			},
		},
	}
}

func basePlanConfig() *config.Config {
	return &config.Config{
		Source:  config.ProviderConfig{Provider: "github", Namespace: "my-org"},
		Target:  config.ProviderConfig{Provider: "gitlab", Namespace: "my-group"},
		Filters: config.Filters{Include: []string{"*"}, Exclude: []string{}},
		Mapping: map[string]string{},
	}
}

func taskIDs(p MigrationPlan) []string {
	ids := make([]string, len(p.Tasks))
	for i, task := range p.Tasks {
		ids[i] = task.ID
	}
	return ids
}

func TestPlanIncludesEveryRepositoryByDefault(t *testing.T) {
	got, err := Plan(basePlanInventory(), basePlanConfig())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	want := []string{"repo-a", "repo-b", "legacy-repo"}
	if !reflect.DeepEqual(taskIDs(got), want) {
		t.Errorf("task IDs = %v, want %v", taskIDs(got), want)
	}

	if got.Source != (ProviderRef{Provider: "github", Namespace: "my-org"}) {
		t.Errorf("Source = %+v, want {github my-org}", got.Source)
	}
	if got.Target != (ProviderRef{Provider: "gitlab", Namespace: "my-group"}) {
		t.Errorf("Target = %+v, want {gitlab my-group}", got.Target)
	}
}

func TestPlanAppliesIncludePatterns(t *testing.T) {
	cfg := basePlanConfig()
	cfg.Filters = config.Filters{Include: []string{"repo-*"}, Exclude: []string{}}

	got, err := Plan(basePlanInventory(), cfg)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	want := []string{"repo-a", "repo-b"}
	if !reflect.DeepEqual(taskIDs(got), want) {
		t.Errorf("task IDs = %v, want %v", taskIDs(got), want)
	}
}

func TestPlanAppliesExcludePatternsEvenWhenIncluded(t *testing.T) {
	cfg := basePlanConfig()
	cfg.Filters = config.Filters{Include: []string{"*"}, Exclude: []string{"legacy-*"}}

	got, err := Plan(basePlanInventory(), cfg)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	want := []string{"repo-a", "repo-b"}
	if !reflect.DeepEqual(taskIDs(got), want) {
		t.Errorf("task IDs = %v, want %v", taskIDs(got), want)
	}
}

func TestPlanRenamesTargetRepositoryAccordingToMapping(t *testing.T) {
	cfg := basePlanConfig()
	cfg.Mapping = map[string]string{"repo-a": "renamed-repo"}

	got, err := Plan(basePlanInventory(), cfg)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	for _, task := range got.Tasks {
		if task.ID == "repo-a" {
			want := TaskEndpoint{Namespace: "my-group", Repo: "renamed-repo"}
			if task.Target != want {
				t.Errorf("Target = %+v, want %+v", task.Target, want)
			}
			return
		}
	}
	t.Fatal("task repo-a not found")
}

func TestPlanCopiesBranchesAndTagsFromInventory(t *testing.T) {
	got, err := Plan(basePlanInventory(), basePlanConfig())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	for _, task := range got.Tasks {
		if task.ID == "repo-a" {
			want := MigrationTask{
				ID:       "repo-a",
				Source:   TaskEndpoint{Namespace: "my-org", Repo: "repo-a"},
				Target:   TaskEndpoint{Namespace: "my-group", Repo: "repo-a"},
				Branches: []string{"main"},
				Tags:     []string{"v1.0.0"},
			}
			if !reflect.DeepEqual(task, want) {
				t.Errorf("task = %+v, want %+v", task, want)
			}
			return
		}
	}
	t.Fatal("task repo-a not found")
}
