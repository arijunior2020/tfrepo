package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

func TestMigrationReportMarshal(t *testing.T) {
	generatedAt, err := time.Parse(time.RFC3339, "2026-06-11T12:30:00Z")
	if err != nil {
		t.Fatalf("time.Parse: %v", err)
	}
	startedA, err := time.Parse(time.RFC3339, "2026-06-11T12:10:00Z")
	if err != nil {
		t.Fatalf("time.Parse: %v", err)
	}
	finishedA, err := time.Parse(time.RFC3339, "2026-06-11T12:12:00Z")
	if err != nil {
		t.Fatalf("time.Parse: %v", err)
	}
	startedB, err := time.Parse(time.RFC3339, "2026-06-11T12:12:00Z")
	if err != nil {
		t.Fatalf("time.Parse: %v", err)
	}
	finishedB, err := time.Parse(time.RFC3339, "2026-06-11T12:13:00Z")
	if err != nil {
		t.Fatalf("time.Parse: %v", err)
	}

	report := MigrationReport{
		GeneratedAt: generatedAt,
		Results: []MigrationResult{
			{ID: "repo-a", Status: "success", StartedAt: startedA, FinishedAt: finishedA},
			{ID: "repo-b", Status: "failed", StartedAt: startedB, FinishedAt: finishedB, Error: "push rejected"},
		},
	}

	got, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	want := []byte(`{
		"generatedAt": "2026-06-11T12:30:00Z",
		"results": [
			{
				"id": "repo-a",
				"status": "success",
				"startedAt": "2026-06-11T12:10:00Z",
				"finishedAt": "2026-06-11T12:12:00Z"
			},
			{
				"id": "repo-b",
				"status": "failed",
				"startedAt": "2026-06-11T12:12:00Z",
				"finishedAt": "2026-06-11T12:13:00Z",
				"error": "push rejected"
			}
		]
	}`)

	if !reflect.DeepEqual(toMap(t, got), toMap(t, want)) {
		t.Errorf("Marshal(report) = %s, want %s", got, want)
	}

	var roundTrip MigrationReport
	if err := json.Unmarshal(got, &roundTrip); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(roundTrip, report) {
		t.Errorf("round trip = %+v, want %+v", roundTrip, report)
	}
}

func TestValidationReportMarshal(t *testing.T) {
	generatedAt, err := time.Parse(time.RFC3339, "2026-06-11T13:00:00Z")
	if err != nil {
		t.Fatalf("time.Parse: %v", err)
	}

	sourceSHA := "abc123"
	targetSHA := "def456"
	tagSourceSHA := "aaa"

	report := ValidationReport{
		GeneratedAt: generatedAt,
		Results: []ValidationResult{
			{ID: "repo-a", Status: "ok", Divergences: []RefDivergence{}},
			{
				ID:     "repo-b",
				Status: "diverged",
				Divergences: []RefDivergence{
					{Type: "branch", Name: "main", SourceSHA: &sourceSHA, TargetSHA: &targetSHA},
					{Type: "tag", Name: "v1.0.0", SourceSHA: &tagSourceSHA, TargetSHA: nil},
				},
			},
			{ID: "repo-c", Status: "skipped", Divergences: []RefDivergence{}, Reason: "migration failed"},
		},
	}

	got, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	want := []byte(`{
		"generatedAt": "2026-06-11T13:00:00Z",
		"results": [
			{"id": "repo-a", "status": "ok", "divergences": []},
			{
				"id": "repo-b",
				"status": "diverged",
				"divergences": [
					{"type": "branch", "name": "main", "sourceSha": "abc123", "targetSha": "def456"},
					{"type": "tag", "name": "v1.0.0", "sourceSha": "aaa", "targetSha": null}
				]
			},
			{"id": "repo-c", "status": "skipped", "divergences": [], "reason": "migration failed"}
		]
	}`)

	if !reflect.DeepEqual(toMap(t, got), toMap(t, want)) {
		t.Errorf("Marshal(report) = %s, want %s", got, want)
	}

	var roundTrip ValidationReport
	if err := json.Unmarshal(got, &roundTrip); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(roundTrip, report) {
		t.Errorf("round trip = %+v, want %+v", roundTrip, report)
	}
}

func TestWriteAndReadJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inventory.json")

	generatedAt, err := time.Parse(time.RFC3339, "2026-06-11T12:00:00Z")
	if err != nil {
		t.Fatalf("time.Parse: %v", err)
	}

	inv := Inventory{
		GeneratedAt: generatedAt,
		Source:      ProviderRef{Provider: "github", Namespace: "my-org"},
		Namespaces:  []InventoryNamespace{},
	}

	if err := WriteJSON(path, inv); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), `"generatedAt": "2026-06-11T12:00:00Z"`) {
		t.Errorf("file contents = %s, want to contain generatedAt field", data)
	}

	var got Inventory
	if err := ReadJSON(path, &got); err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
	if !reflect.DeepEqual(got, inv) {
		t.Errorf("ReadJSON() = %+v, want %+v", got, inv)
	}
}

func TestReadJSONMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.json")

	var inv Inventory
	if err := ReadJSON(path, &inv); err == nil {
		t.Error("ReadJSON() error = nil, want error")
	}
}
