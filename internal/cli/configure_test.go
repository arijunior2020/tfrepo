package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunConfigureUnknownProvider(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runConfigure(context.Background(), "bitbucket", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("runConfigure(bitbucket) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "bitbucket") {
		t.Errorf("stderr = %q, want provider name in error", stderr.String())
	}
	if !strings.Contains(stderr.String(), "github") {
		t.Errorf("stderr = %q, want available providers listed", stderr.String())
	}
}

func TestRunConfigureSkipsAllEnvVarProviders(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_test")
	t.Setenv("GITLAB_TOKEN", "glpat_test")

	var stdout, stderr bytes.Buffer
	code := runConfigure(context.Background(), "", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runConfigure = %d, want 0 (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "GITHUB_TOKEN") {
		t.Errorf("stdout = %q, want skip message mentioning GITHUB_TOKEN", stdout.String())
	}
	if !strings.Contains(stdout.String(), "GITLAB_TOKEN") {
		t.Errorf("stdout = %q, want skip message mentioning GITLAB_TOKEN", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Nenhuma credencial alterada.") {
		t.Errorf("stdout = %q, want no-change message", stdout.String())
	}
}

func TestRunConfigureSkipsEnvVarProviderSingleArg(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_test")

	var stdout, stderr bytes.Buffer
	code := runConfigure(context.Background(), "github", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runConfigure(github) = %d, want 0 (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "GITHUB_TOKEN") {
		t.Errorf("stdout = %q, want skip message mentioning GITHUB_TOKEN", stdout.String())
	}
}

func TestRunConfigureFailsWhenCredentialsFileCorrupted(t *testing.T) {
	dir := t.TempDir()
	credPath := filepath.Join(dir, "credentials")
	if err := os.WriteFile(credPath, []byte(":::invalid:::yaml:"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	old := configureCredPathFn
	configureCredPathFn = func() string { return credPath }
	t.Cleanup(func() { configureCredPathFn = old })

	var stdout, stderr bytes.Buffer
	// credentials.Load falha antes de chegar no loop de providers ou no huh
	code := runConfigure(context.Background(), "github", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("runConfigure with corrupted credentials = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "credenciais") {
		t.Errorf("stderr = %q, want error mentioning credentials", stderr.String())
	}
}

func TestNewRootCommandRegistersConfigure(t *testing.T) {
	exitCode := 0
	root := newRootCommand(&exitCode)

	configureCmd, _, err := root.Find([]string{"configure"})
	if err != nil {
		t.Fatalf("Find(configure): %v", err)
	}
	if configureCmd.Use != "configure [provider]" {
		t.Fatalf("Find(configure).Use = %q, want %q", configureCmd.Use, "configure [provider]")
	}
}
