package security

import "testing"

func TestRedact(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		secrets []string
		want    string
	}{
		{
			name:  "redacts credentials in an authenticated URL",
			value: "fatal: unable to access 'https://oauth2:glpat-abc123@gitlab.com/group/repo.git/'",
			want:  "fatal: unable to access 'https://***@gitlab.com/group/repo.git/'",
		},
		{
			name:    "redacts a known secret value anywhere in the string",
			value:   "token ghp_secret123 is invalid",
			secrets: []string{"ghp_secret123"},
			want:    "token *** is invalid",
		},
		{
			name:  "leaves plain URLs untouched",
			value: "cloning from https://github.com/org/repo.git",
			want:  "cloning from https://github.com/org/repo.git",
		},
		{
			name:    "ignores empty secrets",
			value:   "no secrets here",
			secrets: []string{""},
			want:    "no secrets here",
		},
		{
			name:    "redacts longer secrets before shorter ones that are substrings",
			value:   "value is ghp_secret123extra and ghp_secret123",
			secrets: []string{"ghp_secret123", "ghp_secret123extra"},
			want:    "value is *** and ***",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Redact(tt.value, tt.secrets...)
			if got != tt.want {
				t.Errorf("Redact(%q, %v) = %q, want %q", tt.value, tt.secrets, got, tt.want)
			}
		})
	}
}
