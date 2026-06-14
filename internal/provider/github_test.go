package provider

import "testing"

func TestGitHubName(t *testing.T) {
	p, err := NewGitHubProvider("test-token", "")
	if err != nil {
		t.Fatalf("NewGitHubProvider: %v", err)
	}

	if got := p.Name(); got != "github" {
		t.Errorf("Name() = %q, want %q", got, "github")
	}
}

func TestGitHubGetAuthenticatedCloneURL(t *testing.T) {
	t.Run("default github.com", func(t *testing.T) {
		p, err := NewGitHubProvider("my-token", "")
		if err != nil {
			t.Fatalf("NewGitHubProvider: %v", err)
		}

		got := p.GetAuthenticatedCloneURL("acme-corp", "widget-api")
		want := "https://x-access-token:my-token@github.com/acme-corp/widget-api.git"
		if got != want {
			t.Errorf("GetAuthenticatedCloneURL() = %q, want %q", got, want)
		}
	})

	t.Run("custom enterprise base url", func(t *testing.T) {
		p, err := NewGitHubProvider("my-token", "https://github.example.com/api/v3/")
		if err != nil {
			t.Fatalf("NewGitHubProvider: %v", err)
		}

		got := p.GetAuthenticatedCloneURL("acme-corp", "widget-api")
		want := "https://x-access-token:my-token@github.example.com/acme-corp/widget-api.git"
		if got != want {
			t.Errorf("GetAuthenticatedCloneURL() = %q, want %q", got, want)
		}
	})
}
