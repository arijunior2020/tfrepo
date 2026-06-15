package core

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/arijunior2020/tfrepo/internal/provider"
)

// toMap unmarshals JSON bytes into a generic map for structural comparison,
// so tests don't depend on key ordering.
func toMap(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v\ndata: %s", err, data)
	}
	return m
}

func TestInventoryMarshal(t *testing.T) {
	generatedAt, err := time.Parse(time.RFC3339, "2026-06-11T12:00:00Z")
	if err != nil {
		t.Fatalf("time.Parse: %v", err)
	}

	inv := Inventory{
		GeneratedAt: generatedAt,
		Source:      ProviderRef{Provider: "github", Namespace: "my-org"},
		Namespaces: []InventoryNamespace{
			{
				Slug: "my-org",
				Name: "My Org",
				Kind: provider.NamespaceOrganization,
				Repositories: []RepositoryInventory{
					{
						Name:          "repo-a",
						DefaultBranch: "main",
						Visibility:    provider.VisibilityPrivate,
						SizeKB:        12345,
						Branches:      []string{"main", "develop"},
						Tags:          []string{"v1.0.0", "v1.1.0"},
					},
				},
			},
		},
	}

	got, err := json.Marshal(inv)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	want := []byte(`{
		"generatedAt": "2026-06-11T12:00:00Z",
		"source": {"provider": "github", "namespace": "my-org"},
		"namespaces": [
			{
				"slug": "my-org",
				"name": "My Org",
				"kind": "organization",
				"repositories": [
					{
						"name": "repo-a",
						"defaultBranch": "main",
						"visibility": "private",
						"sizeKb": 12345,
						"branches": ["main", "develop"],
						"tags": ["v1.0.0", "v1.1.0"]
					}
				]
			}
		]
	}`)

	if !reflect.DeepEqual(toMap(t, got), toMap(t, want)) {
		t.Errorf("Marshal(inv) = %s, want %s", got, want)
	}

	var roundTrip Inventory
	if err := json.Unmarshal(got, &roundTrip); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(roundTrip, inv) {
		t.Errorf("round trip = %+v, want %+v", roundTrip, inv)
	}
}

func TestMigrationPlanMarshal(t *testing.T) {
	generatedAt, err := time.Parse(time.RFC3339, "2026-06-11T12:05:00Z")
	if err != nil {
		t.Fatalf("time.Parse: %v", err)
	}

	plan := MigrationPlan{
		GeneratedAt: generatedAt,
		Source:      ProviderRef{Provider: "github", Namespace: "my-org"},
		Target:      ProviderRef{Provider: "gitlab", Namespace: "my-group"},
		Tasks: []MigrationTask{
			{
				ID:       "repo-a",
				Source:   TaskEndpoint{Namespace: "my-org", Repo: "repo-a"},
				Target:   TaskEndpoint{Namespace: "my-group", Repo: "repo-a"},
				Branches: []string{"main", "develop"},
				Tags:     []string{"v1.0.0", "v1.1.0"},
			},
		},
	}

	got, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	want := []byte(`{
		"generatedAt": "2026-06-11T12:05:00Z",
		"source": {"provider": "github", "namespace": "my-org"},
		"target": {"provider": "gitlab", "namespace": "my-group"},
		"tasks": [
			{
				"id": "repo-a",
				"source": {"namespace": "my-org", "repo": "repo-a"},
				"target": {"namespace": "my-group", "repo": "repo-a"},
				"branches": ["main", "develop"],
				"tags": ["v1.0.0", "v1.1.0"]
			}
		]
	}`)

	if !reflect.DeepEqual(toMap(t, got), toMap(t, want)) {
		t.Errorf("Marshal(plan) = %s, want %s", got, want)
	}

	var roundTrip MigrationPlan
	if err := json.Unmarshal(got, &roundTrip); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(roundTrip, plan) {
		t.Errorf("round trip = %+v, want %+v", roundTrip, plan)
	}
}
