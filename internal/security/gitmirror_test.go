package security

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCloneAndPushMirror(t *testing.T) {
	ctx := context.Background()

	sourceDir := t.TempDir()
	runGit(t, sourceDir, "init", "--initial-branch=main")
	runGit(t, sourceDir, "config", "user.email", "test@example.com")
	runGit(t, sourceDir, "config", "user.name", "Test")

	if err := os.WriteFile(filepath.Join(sourceDir, "README.md"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	runGit(t, sourceDir, "add", "README.md")
	runGit(t, sourceDir, "commit", "-m", "initial commit")

	mirrorDir := filepath.Join(t.TempDir(), "mirror")
	if err := Clone(ctx, sourceDir, mirrorDir); err != nil {
		t.Fatalf("Clone() returned error: %v", err)
	}

	targetDir := t.TempDir()
	runGit(t, targetDir, "init", "--bare")

	if err := Push(ctx, mirrorDir, targetDir); err != nil {
		t.Fatalf("Push() returned error: %v", err)
	}

	sourceHead := strings.TrimSpace(runGit(t, sourceDir, "rev-parse", "main"))
	targetHead := strings.TrimSpace(runGit(t, targetDir, "rev-parse", "main"))

	if sourceHead != targetHead {
		t.Errorf("target HEAD = %q, want %q", targetHead, sourceHead)
	}
}

func TestCloneError(t *testing.T) {
	ctx := context.Background()

	err := Clone(ctx, filepath.Join(t.TempDir(), "does-not-exist"), filepath.Join(t.TempDir(), "dest"))

	if err == nil {
		t.Fatal("Clone() with a non-existent source returned nil error, want error")
	}
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}

	return string(output)
}
