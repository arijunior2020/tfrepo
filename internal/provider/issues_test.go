package provider_test

import (
	"encoding/json"
	"testing"

	"github.com/arijunior2020/tfrepo/internal/provider"
)

func TestIssueJSONRoundTrip(t *testing.T) {
	milestoneID := int64(42)
	orig := provider.Issue{
		ExternalID:          int64(7),
		Title:               "Fix segfault",
		Body:                "Reproduces on Linux",
		State:               "closed",
		Labels:              []string{"bug", "priority"},
		MilestoneExternalID: &milestoneID,
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got provider.Issue
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.ExternalID != orig.ExternalID {
		t.Errorf("ExternalID = %d, want %d", got.ExternalID, orig.ExternalID)
	}
	if got.Title != orig.Title {
		t.Errorf("Title = %q, want %q", got.Title, orig.Title)
	}
	if got.State != orig.State {
		t.Errorf("State = %q, want %q", got.State, orig.State)
	}
	if got.Body != orig.Body {
		t.Errorf("Body = %q, want %q", got.Body, orig.Body)
	}
	if len(got.Labels) != 2 || got.Labels[0] != "bug" || got.Labels[1] != "priority" {
		t.Errorf("Labels = %v, want [bug priority]", got.Labels)
	}
	if got.MilestoneExternalID == nil || *got.MilestoneExternalID != 42 {
		t.Errorf("MilestoneExternalID = %v, want &42", got.MilestoneExternalID)
	}
}

func TestIssueJSONNoMilestone(t *testing.T) {
	orig := provider.Issue{
		ExternalID: int64(1),
		Title:      "No milestone",
		State:      "open",
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got provider.Issue
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.MilestoneExternalID != nil {
		t.Errorf("MilestoneExternalID = %v, want nil", got.MilestoneExternalID)
	}
}
