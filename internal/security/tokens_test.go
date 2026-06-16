package security

// credentialsPathFn é mutada pelos testes para injetar caminhos de arquivo temporários.
// Não use t.Parallel() neste pacote — os testes compartilham este estado global.

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func setNoCredentialsFile(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	old := credentialsPathFn
	credentialsPathFn = func() string { return filepath.Join(dir, "credentials") }
	t.Cleanup(func() { credentialsPathFn = old })
}

func TestResolveToken(t *testing.T) {
	setNoCredentialsFile(t)

	tests := []struct {
		name      string
		provider  string
		envVar    string
		envValue  string
		setEnv    bool
		wantToken string
		wantErr   bool
	}{
		{
			name:      "github token set",
			provider:  "github",
			envVar:    "GITHUB_TOKEN",
			envValue:  "ghp_test123",
			setEnv:    true,
			wantToken: "ghp_test123",
		},
		{
			name:      "gitlab token set",
			provider:  "gitlab",
			envVar:    "GITLAB_TOKEN",
			envValue:  "glpat-test123",
			setEnv:    true,
			wantToken: "glpat-test123",
		},
		{
			name:     "github token missing",
			provider: "github",
			envVar:   "GITHUB_TOKEN",
			envValue: "",
			setEnv:   true,
			wantErr:  true,
		},
		{
			name:     "github token blank",
			provider: "github",
			envVar:   "GITHUB_TOKEN",
			envValue: "   ",
			setEnv:   true,
			wantErr:  true,
		},
		{
			name:     "unknown provider",
			provider: "bitbucket",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				t.Setenv(tt.envVar, tt.envValue)
			}

			got, err := ResolveToken(tt.provider)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("ResolveToken(%q) = nil error, want error", tt.provider)
				}
				return
			}

			if err != nil {
				t.Fatalf("ResolveToken(%q) returned unexpected error: %v", tt.provider, err)
			}

			if got != tt.wantToken {
				t.Errorf("ResolveToken(%q) = %q, want %q", tt.provider, got, tt.wantToken)
			}
		})
	}
}

func TestResolveTokenFromCredentialsFile(t *testing.T) {
	dir := t.TempDir()
	credPath := filepath.Join(dir, "credentials")

	credYAML := "providers:\n  github:\n    token: ghp_from_file\n"
	if err := os.WriteFile(credPath, []byte(credYAML), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	old := credentialsPathFn
	credentialsPathFn = func() string { return credPath }
	t.Cleanup(func() { credentialsPathFn = old })

	t.Setenv("GITHUB_TOKEN", "")

	got, err := ResolveToken("github")
	if err != nil {
		t.Fatalf("ResolveToken from file: %v", err)
	}
	if got != "ghp_from_file" {
		t.Errorf("ResolveToken = %q, want %q", got, "ghp_from_file")
	}
}

func TestResolveTokenEnvVarTakesPriorityOverFile(t *testing.T) {
	dir := t.TempDir()
	credPath := filepath.Join(dir, "credentials")

	credYAML := "providers:\n  github:\n    token: ghp_from_file\n"
	if err := os.WriteFile(credPath, []byte(credYAML), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	old := credentialsPathFn
	credentialsPathFn = func() string { return credPath }
	t.Cleanup(func() { credentialsPathFn = old })

	t.Setenv("GITHUB_TOKEN", "ghp_from_env")

	got, err := ResolveToken("github")
	if err != nil {
		t.Fatalf("ResolveToken: %v", err)
	}
	if got != "ghp_from_env" {
		t.Errorf("ResolveToken = %q, want %q (env var should take priority)", got, "ghp_from_env")
	}
}

func TestResolveTokenMissingTokenError(t *testing.T) {
	setNoCredentialsFile(t)
	t.Setenv("GITHUB_TOKEN", "")

	_, err := ResolveToken("github")

	var missingErr *MissingTokenError
	if !errors.As(err, &missingErr) {
		t.Fatalf("ResolveToken() error = %v, want *MissingTokenError", err)
	}

	if missingErr.EnvVar != "GITHUB_TOKEN" {
		t.Errorf("MissingTokenError.EnvVar = %q, want %q", missingErr.EnvVar, "GITHUB_TOKEN")
	}
	if missingErr.Provider != "github" {
		t.Errorf("MissingTokenError.Provider = %q, want %q", missingErr.Provider, "github")
	}
}

func TestEnvVarForProvider(t *testing.T) {
	tests := []struct {
		provider string
		want     string
	}{
		{"github", "GITHUB_TOKEN"},
		{"gitlab", "GITLAB_TOKEN"},
		{"unknown", ""},
	}
	for _, tt := range tests {
		got := EnvVarForProvider(tt.provider)
		if got != tt.want {
			t.Errorf("EnvVarForProvider(%q) = %q, want %q", tt.provider, got, tt.want)
		}
	}
}
