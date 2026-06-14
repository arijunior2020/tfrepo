package security

import (
	"os"
	"testing"
)

func TestManagerCreate(t *testing.T) {
	m := NewManager()

	ws, err := m.Create()
	if err != nil {
		t.Fatalf("Create() returned error: %v", err)
	}

	info, err := os.Stat(ws.Path)
	if err != nil {
		t.Fatalf("workspace directory does not exist: %v", err)
	}

	if !info.IsDir() {
		t.Fatalf("workspace path %q is not a directory", ws.Path)
	}

	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("workspace permissions = %o, want %o", perm, 0o700)
	}
}

func TestWorkspaceCleanup(t *testing.T) {
	m := NewManager()

	ws, err := m.Create()
	if err != nil {
		t.Fatalf("Create() returned error: %v", err)
	}

	if err := ws.Cleanup(); err != nil {
		t.Fatalf("Cleanup() returned error: %v", err)
	}

	if _, err := os.Stat(ws.Path); !os.IsNotExist(err) {
		t.Errorf("workspace directory still exists after Cleanup()")
	}

	if err := ws.Cleanup(); err != nil {
		t.Errorf("second Cleanup() returned error: %v", err)
	}
}

func TestManagerCleanupAll(t *testing.T) {
	m := NewManager()

	ws1, err := m.Create()
	if err != nil {
		t.Fatalf("Create() returned error: %v", err)
	}

	ws2, err := m.Create()
	if err != nil {
		t.Fatalf("Create() returned error: %v", err)
	}

	if err := m.CleanupAll(); err != nil {
		t.Fatalf("CleanupAll() returned error: %v", err)
	}

	for _, ws := range []*Workspace{ws1, ws2} {
		if _, err := os.Stat(ws.Path); !os.IsNotExist(err) {
			t.Errorf("workspace %q still exists after CleanupAll()", ws.Path)
		}
	}
}
