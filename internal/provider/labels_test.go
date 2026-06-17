package provider

import (
	"encoding/json"
	"testing"
	"time"
)

func TestLabelJSONRoundTrip(t *testing.T) {
	original := Label{
		Name:        "bug",
		Description: "Something is wrong",
		Color:       "d73a4a",
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got Label
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got != original {
		t.Errorf("got %+v, want %+v", got, original)
	}
}

func TestMilestoneJSONRoundTrip(t *testing.T) {
	dueDate := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	original := Milestone{
		ExternalID:  42,
		Title:       "v1.0",
		Description: "First stable release",
		State:       MilestoneStateOpen,
		DueDate:     &dueDate,
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got Milestone
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.ExternalID != original.ExternalID ||
		got.Title != original.Title ||
		got.Description != original.Description ||
		got.State != original.State ||
		got.DueDate == nil || !got.DueDate.Equal(*original.DueDate) {
		t.Errorf("got %+v, want %+v", got, original)
	}
}

func TestMilestoneNilDueDateRoundTrip(t *testing.T) {
	original := Milestone{
		ExternalID: 1,
		Title:      "Sprint 1",
		State:      MilestoneStateClosed,
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got Milestone
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.DueDate != nil {
		t.Errorf("DueDate = %v, want nil", got.DueDate)
	}
}
