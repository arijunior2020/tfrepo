package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunInitWritesConfigTemplate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "transferepo.config.yaml")

	var stdout, stderr bytes.Buffer
	code := runInit(path, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("runInit() = %d, want 0 (stderr: %s)", code, stderr.String())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != configTemplate {
		t.Errorf("config file content = %q, want configTemplate", data)
	}
	if !strings.Contains(stdout.String(), "Wrote "+path) {
		t.Errorf("stdout = %q, want it to mention %q", stdout.String(), path)
	}
}

func TestRunInitDoesNotOverwriteExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "transferepo.config.yaml")
	if err := os.WriteFile(path, []byte("existing content"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := runInit(path, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("runInit() = %d, want 1", code)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "existing content" {
		t.Errorf("file was overwritten: %q", data)
	}
	if !strings.Contains(stderr.String(), path) {
		t.Errorf("stderr = %q, want it to mention %q", stderr.String(), path)
	}
}
