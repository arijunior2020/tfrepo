package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunDestroyRemovesMigrationArtifacts(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	for _, path := range migrationArtifactPaths {
		if err := os.WriteFile(path, []byte(path), 0o644); err != nil {
			t.Fatalf("WriteFile(%s): %v", path, err)
		}
	}
	configPath := filepath.Join(dir, defaultConfigPath)
	if err := os.WriteFile(configPath, []byte("source: {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(config): %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := runDestroy(configPath, false, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runDestroy = %d, want 0 (stderr: %s)", code, stderr.String())
	}

	for _, path := range migrationArtifactPaths {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s still exists, err = %v", path, err)
		}
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Errorf("config should remain, got err = %v", err)
	}
	if !strings.Contains(stdout.String(), "Removidos 4 arquivos.") {
		t.Errorf("stdout = %q, want removal summary", stdout.String())
	}
}

func TestRunDestroyIncludesConfigWhenRequested(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := os.WriteFile(inventoryPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile(inventory): %v", err)
	}
	configPath := filepath.Join(dir, "custom.yaml")
	if err := os.WriteFile(configPath, []byte("source: {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(config): %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := runDestroy(configPath, true, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runDestroy = %d, want 0 (stderr: %s)", code, stderr.String())
	}

	if _, err := os.Stat(inventoryPath); !os.IsNotExist(err) {
		t.Errorf("%s still exists, err = %v", inventoryPath, err)
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Errorf("config still exists, err = %v", err)
	}
}

func TestRunDestroyNoFiles(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	var stdout, stderr bytes.Buffer
	code := runDestroy(defaultConfigPath, false, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runDestroy = %d, want 0 (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Nenhum arquivo de migração encontrado.") {
		t.Errorf("stdout = %q, want no-files message", stdout.String())
	}
}

func TestRunDestroyRefusesDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := os.Mkdir(inventoryPath, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := runDestroy(defaultConfigPath, false, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("runDestroy = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "remoção recusada") {
		t.Errorf("stderr = %q, want refusal message", stderr.String())
	}
}
