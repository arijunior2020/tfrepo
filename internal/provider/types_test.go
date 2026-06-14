package provider

import (
	"encoding/json"
	"testing"
)

func TestRepositorySummaryJSON(t *testing.T) {
	summary := RepositorySummary{
		Name:          "widget-api",
		Namespace:     "acme-corp",
		DefaultBranch: "main",
		Visibility:    VisibilityPrivate,
		SizeKB:        1234,
	}

	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	want := `{"name":"widget-api","namespace":"acme-corp","defaultBranch":"main","visibility":"private","sizeKb":1234}`
	if string(data) != want {
		t.Errorf("Marshal() = %s, want %s", data, want)
	}
}

func TestRepositoryDetailsJSON(t *testing.T) {
	details := RepositoryDetails{
		RepositorySummary: RepositorySummary{
			Name:          "widget-api",
			Namespace:     "acme-corp",
			DefaultBranch: "main",
			Visibility:    VisibilityPublic,
			SizeKB:        10,
		},
		Branches: []string{"main", "develop"},
		Tags:     []string{"v1.0.0"},
	}

	data, err := json.Marshal(details)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	want := `{"name":"widget-api","namespace":"acme-corp","defaultBranch":"main","visibility":"public","sizeKb":10,"branches":["main","develop"],"tags":["v1.0.0"]}`
	if string(data) != want {
		t.Errorf("Marshal() = %s, want %s", data, want)
	}
}
