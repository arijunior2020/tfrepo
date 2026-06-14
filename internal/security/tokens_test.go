package security

import (
	"errors"
	"testing"
)

func TestResolveToken(t *testing.T) {
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

func TestResolveTokenMissingTokenError(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")

	_, err := ResolveToken("github")

	var missingErr *MissingTokenError
	if !errors.As(err, &missingErr) {
		t.Fatalf("ResolveToken() error = %v, want *MissingTokenError", err)
	}

	if missingErr.EnvVar != "GITHUB_TOKEN" {
		t.Errorf("MissingTokenError.EnvVar = %q, want %q", missingErr.EnvVar, "GITHUB_TOKEN")
	}
}
