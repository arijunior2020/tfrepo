package security

import (
	"os"
	"sync"
)

const (
	tempDirPattern = "tfrepo-*"
	workspaceMode  = 0o700
)

// Manager tracks temporary workspace directories so they can all be removed
// together if the process is interrupted before per-task cleanup runs.
type Manager struct {
	mu     sync.Mutex
	active map[string]struct{}
}

// NewManager returns a Manager with no active workspaces.
func NewManager() *Manager {
	return &Manager{active: make(map[string]struct{})}
}

// Workspace is an isolated temporary directory used for a single git
// clone/push operation.
type Workspace struct {
	Path string

	manager *Manager
}

// Create makes a new temporary directory restricted to the current user and
// registers it with the manager for cleanup.
func (m *Manager) Create() (*Workspace, error) {
	path, err := os.MkdirTemp("", tempDirPattern)
	if err != nil {
		return nil, err
	}

	if err := os.Chmod(path, workspaceMode); err != nil {
		os.RemoveAll(path)
		return nil, err
	}

	m.mu.Lock()
	m.active[path] = struct{}{}
	m.mu.Unlock()

	return &Workspace{Path: path, manager: m}, nil
}

// Cleanup removes the workspace directory. Safe to call more than once.
func (w *Workspace) Cleanup() error {
	return w.manager.cleanup(w.Path)
}

// CleanupAll removes every workspace still active.
func (m *Manager) CleanupAll() error {
	m.mu.Lock()
	paths := make([]string, 0, len(m.active))
	for path := range m.active {
		paths = append(paths, path)
	}
	m.mu.Unlock()

	var firstErr error
	for _, path := range paths {
		if err := m.cleanup(path); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (m *Manager) cleanup(path string) error {
	m.mu.Lock()
	_, active := m.active[path]
	delete(m.active, path)
	m.mu.Unlock()

	if !active {
		return nil
	}

	return os.RemoveAll(path)
}
