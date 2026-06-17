package provider_test

import (
	"encoding/json"
	"testing"

	"github.com/arijunior2020/tfrepo/internal/provider"
)

func TestPullRequestJSONRoundTrip(t *testing.T) {
	orig := provider.PullRequest{
		ExternalID:   int64(5),
		Title:        "Add feature X",
		Body:         "This PR adds feature X",
		State:        "open",
		SourceBranch: "feature/x",
		TargetBranch: "main",
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got provider.PullRequest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.ExternalID != orig.ExternalID {
		t.Errorf("ExternalID = %d, want %d", got.ExternalID, orig.ExternalID)
	}
	if got.Title != orig.Title {
		t.Errorf("Title = %q, want %q", got.Title, orig.Title)
	}
	if got.SourceBranch != orig.SourceBranch {
		t.Errorf("SourceBranch = %q, want %q", got.SourceBranch, orig.SourceBranch)
	}
	if got.TargetBranch != orig.TargetBranch {
		t.Errorf("TargetBranch = %q, want %q", got.TargetBranch, orig.TargetBranch)
	}
	if got.State != orig.State {
		t.Errorf("State = %q, want %q", got.State, orig.State)
	}
}

func TestPullRequestJSONEmptyBody(t *testing.T) {
	orig := provider.PullRequest{
		ExternalID:   int64(1),
		Title:        "No body",
		State:        "open",
		SourceBranch: "feat",
		TargetBranch: "main",
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got provider.PullRequest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Body != "" {
		t.Errorf("Body = %q, want empty string", got.Body)
	}
}
