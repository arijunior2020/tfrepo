package core

import "testing"

func TestFiltererIncludesEverythingByDefault(t *testing.T) {
	f, err := NewFilterer([]string{"*"}, []string{})
	if err != nil {
		t.Fatalf("NewFilterer: %v", err)
	}

	for _, name := range []string{"repo-a", "repo-b", "legacy-repo"} {
		if !f.Match(name) {
			t.Errorf("Match(%q) = false, want true", name)
		}
	}
}

func TestFiltererAppliesIncludePatterns(t *testing.T) {
	f, err := NewFilterer([]string{"repo-*"}, []string{})
	if err != nil {
		t.Fatalf("NewFilterer: %v", err)
	}

	if !f.Match("repo-a") {
		t.Error(`Match("repo-a") = false, want true`)
	}
	if f.Match("legacy-repo") {
		t.Error(`Match("legacy-repo") = true, want false`)
	}
}

func TestFiltererAppliesExcludePatternsEvenWhenIncluded(t *testing.T) {
	f, err := NewFilterer([]string{"*"}, []string{"legacy-*"})
	if err != nil {
		t.Fatalf("NewFilterer: %v", err)
	}

	if !f.Match("repo-a") {
		t.Error(`Match("repo-a") = false, want true`)
	}
	if f.Match("legacy-repo") {
		t.Error(`Match("legacy-repo") = true, want false`)
	}
}

func TestFiltererExcludesEverythingWhenNoIncludePatterns(t *testing.T) {
	f, err := NewFilterer([]string{}, []string{})
	if err != nil {
		t.Fatalf("NewFilterer: %v", err)
	}

	if f.Match("repo-a") {
		t.Error(`Match("repo-a") = true, want false`)
	}
}

func TestNewFiltererInvalidPattern(t *testing.T) {
	if _, err := NewFilterer([]string{"["}, nil); err == nil {
		t.Error("NewFilterer() error = nil, want error for invalid pattern")
	}
}
