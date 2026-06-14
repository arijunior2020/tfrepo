package provider

import "testing"

func TestGitLabName(t *testing.T) {
	p, err := NewGitLabProvider("my-token", "")
	if err != nil {
		t.Fatalf("NewGitLabProvider: %v", err)
	}

	if got := p.Name(); got != "gitlab" {
		t.Errorf("Name() = %q, want %q", got, "gitlab")
	}
}

func TestGitLabGetAuthenticatedCloneURL_DefaultHost(t *testing.T) {
	p, err := NewGitLabProvider("my-token", "")
	if err != nil {
		t.Fatalf("NewGitLabProvider: %v", err)
	}

	got := p.GetAuthenticatedCloneURL("my-group", "repo-one")
	want := "https://oauth2:my-token@gitlab.com/my-group/repo-one.git"

	if got != want {
		t.Errorf("GetAuthenticatedCloneURL() = %q, want %q", got, want)
	}
}

func TestGitLabGetAuthenticatedCloneURL_CustomHost(t *testing.T) {
	p, err := NewGitLabProvider("my-token", "https://gitlab.example.com")
	if err != nil {
		t.Fatalf("NewGitLabProvider: %v", err)
	}

	got := p.GetAuthenticatedCloneURL("my-group", "repo-one")
	want := "https://oauth2:my-token@gitlab.example.com/my-group/repo-one.git"

	if got != want {
		t.Errorf("GetAuthenticatedCloneURL() = %q, want %q", got, want)
	}
}
