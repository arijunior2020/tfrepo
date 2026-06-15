package cli

import (
	"testing"

	"github.com/arijunior2020/tfrepo/internal/config"
)

func TestNewProviderGitHub(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "test-token")

	p, err := NewProvider(config.ProviderConfig{Provider: "github", Namespace: "my-org"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if p.Name() != "github" {
		t.Errorf("Name() = %q, want %q", p.Name(), "github")
	}
}

func TestNewProviderGitLab(t *testing.T) {
	t.Setenv("GITLAB_TOKEN", "test-token")

	p, err := NewProvider(config.ProviderConfig{Provider: "gitlab", Namespace: "my-group"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if p.Name() != "gitlab" {
		t.Errorf("Name() = %q, want %q", p.Name(), "gitlab")
	}
}

func TestNewProviderMissingTokenReturnsError(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")

	_, err := NewProvider(config.ProviderConfig{Provider: "github", Namespace: "my-org"})
	if err == nil {
		t.Fatal("NewProvider() error = nil, want error")
	}
}
