package credentials_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/arijunior2020/tfrepo/internal/credentials"
)

func TestDefaultPath(t *testing.T) {
	got := credentials.DefaultPath()
	if !strings.HasSuffix(got, filepath.Join(".tfrepo", "credentials")) {
		t.Errorf("DefaultPath() = %q, want suffix %q", got, filepath.Join(".tfrepo", "credentials"))
	}
	if runtime.GOOS != "windows" && !strings.HasPrefix(got, "/") {
		t.Errorf("DefaultPath() = %q, want absolute path", got)
	}
}

func TestLoadNonExistentFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")

	got, err := credentials.Load(path)
	if err != nil {
		t.Fatalf("Load(non-existent) returned error: %v", err)
	}
	if got == nil {
		t.Fatal("Load(non-existent) returned nil")
	}
	if got.Token("github") != "" {
		t.Errorf("Token(github) = %q on empty creds, want empty", got.Token("github"))
	}
}

func TestLoadValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")

	yaml := "providers:\n  github:\n    token: ghp_abc123\n"
	if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := credentials.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Token("github") != "ghp_abc123" {
		t.Errorf("Token(github) = %q, want %q", got.Token("github"), "ghp_abc123")
	}
	if got.Token("gitlab") != "" {
		t.Errorf("Token(gitlab) = %q, want empty", got.Token("gitlab"))
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")

	if err := os.WriteFile(path, []byte(":::invalid:::"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := credentials.Load(path)
	if err == nil {
		t.Fatal("Load(invalid YAML) returned nil error, want error")
	}
}

func TestTokenTrimsWhitespace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")

	yaml := "providers:\n  github:\n    token: \"  ghp_trimmed  \"\n"
	if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := credentials.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Token("github") != "ghp_trimmed" {
		t.Errorf("Token(github) = %q, want %q (whitespace trimmed)", got.Token("github"), "ghp_trimmed")
	}
}

func TestSaveCreatesFileAndDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".tfrepo", "credentials")

	creds := &credentials.Credentials{
		Providers: map[string]credentials.ProviderCredential{
			"github": {Token: "ghp_saved"},
		},
	}

	if err := credentials.Save(path, creds); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(credentials): %v", err)
	}
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Errorf("credentials perm = %o, want 0600", perm)
		}
	}

	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("Stat(dir): %v", err)
	}
	if runtime.GOOS != "windows" {
		if perm := dirInfo.Mode().Perm(); perm != 0700 {
			t.Errorf("dir perm = %o, want 0700", perm)
		}
	}

	loaded, err := credentials.Load(path)
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if loaded.Token("github") != "ghp_saved" {
		t.Errorf("Token(github) after Save = %q, want %q", loaded.Token("github"), "ghp_saved")
	}
}

func TestSaveOverwritesExistingToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")

	first := &credentials.Credentials{
		Providers: map[string]credentials.ProviderCredential{
			"github": {Token: "ghp_old"},
		},
	}
	if err := credentials.Save(path, first); err != nil {
		t.Fatalf("Save(first): %v", err)
	}

	second := &credentials.Credentials{
		Providers: map[string]credentials.ProviderCredential{
			"github": {Token: "ghp_new"},
		},
	}
	if err := credentials.Save(path, second); err != nil {
		t.Fatalf("Save(second): %v", err)
	}

	loaded, err := credentials.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Token("github") != "ghp_new" {
		t.Errorf("Token(github) = %q, want %q", loaded.Token("github"), "ghp_new")
	}
}

func TestKnownProvidersContainsGithubAndGitlab(t *testing.T) {
	ids := make(map[string]bool)
	for _, p := range credentials.KnownProviders {
		ids[p.ID] = true
		if p.Label == "" {
			t.Errorf("KnownProvider %q has empty Label", p.ID)
		}
	}
	for _, want := range []string{"github", "gitlab"} {
		if !ids[want] {
			t.Errorf("KnownProviders missing %q", want)
		}
	}
}
