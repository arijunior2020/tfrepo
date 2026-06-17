package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestNewRootCommandRegistersMigrateLabels(t *testing.T) {
	exitCode := 0
	root := newRootCommand(&exitCode)

	cmd, _, err := root.Find([]string{"migrate-labels"})
	if err != nil {
		t.Fatalf("Find(migrate-labels): %v", err)
	}
	if cmd.Use != "migrate-labels" {
		t.Fatalf("Use = %q, want %q", cmd.Use, "migrate-labels")
	}
}

func TestRunMigrateLabelsExitCode1WhenConfigMissing(t *testing.T) {
	dir := t.TempDir()

	exitCode := 0
	root := newRootCommand(&exitCode)

	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"migrate-labels", "--config", filepath.Join(dir, "does-not-exist.yaml")})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if exitCode != 1 {
		t.Fatalf("exitCode = %d, want 1", exitCode)
	}
}

func TestRunMigrateLabelsExitCode1WhenPlanMissing(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	configPath := filepath.Join(dir, "transferepo.config.yaml")
	if err := os.WriteFile(configPath, []byte("source:\n  provider: github\n  namespace: my-org\ntarget:\n  provider: gitlab\n  namespace: my-group\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	exitCode := 0
	root := newRootCommand(&exitCode)

	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"migrate-labels", "--config", configPath})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if exitCode != 1 {
		t.Fatalf("exitCode = %d, want 1", exitCode)
	}
}
