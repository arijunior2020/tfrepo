# tfrepo Plan 1: Scaffold + internal/security Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Initialize the `tfrepo` Go module and implement `internal/security` — token resolution, log/error redaction, temporary workspace management, and git mirror clone/push — the foundation every later plan builds on.

**Architecture:** Single Go module `github.com/arijunior2020/tfrepo`. `internal/security` depends only on the standard library and has no dependency on any other `internal/*` package; `internal/provider`, `internal/core`, and `internal/cli` (Plans 2-4) will import it. This plan ends with a working `go.mod`, a fully tested `internal/security` package, and CI running `go vet`/`go test -race`.

**Tech Stack:** Go 1.23, standard library only (`os`, `os/exec`, `context`, `regexp`, `sort`, `strings`, `sync`), `go test -race`, GitHub Actions.

---

### Task 1: Initialize the Go module

**Files:**
- Create: `go.mod`
- Create: `.gitignore`

- [ ] **Step 1: Initialize the module**

Run (from the repo root `/media/arimateia-junior/Dados3/projetos/tfrepo`):

```bash
go mod init github.com/arijunior2020/tfrepo
```

Expected output:

```
go: creating new go.mod: module github.com/arijunior2020/tfrepo
```

This creates `go.mod` with:

```
module github.com/arijunior2020/tfrepo

go 1.23.6
```

(`go mod init` writes the exact installed toolchain version — here Go 1.23.6. If a different toolchain is installed, the version will differ; that's fine, it only sets the minimum required language version.)

- [ ] **Step 2: Create `.gitignore`**

Create `.gitignore`:

```
/tfrepo
*.test
/dist
```

- [ ] **Step 3: Commit**

```bash
git add go.mod .gitignore
git commit -m "chore: initialize Go module"
```

---

### Task 2: internal/security — token resolution

**Files:**
- Create: `internal/security/tokens.go`
- Test: `internal/security/tokens_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/security/tokens_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/security/... -run TestResolveToken -v`

Expected: FAIL — compile error, `undefined: ResolveToken` (and `undefined: MissingTokenError`), since `internal/security` has no source files yet.

- [ ] **Step 3: Write minimal implementation**

Create `internal/security/tokens.go`:

```go
package security

import (
	"fmt"
	"os"
	"strings"
)

// envVarByProvider maps a provider name to the environment variable that
// holds its API token.
var envVarByProvider = map[string]string{
	"github": "GITHUB_TOKEN",
	"gitlab": "GITLAB_TOKEN",
}

// MissingTokenError indicates that the environment variable for a
// provider's token is not set or is empty.
type MissingTokenError struct {
	EnvVar string
}

func (e *MissingTokenError) Error() string {
	return fmt.Sprintf("missing required environment variable: %s", e.EnvVar)
}

// ResolveToken returns the API token for the given provider ("github" or
// "gitlab"), read exclusively from GITHUB_TOKEN/GITLAB_TOKEN.
func ResolveToken(provider string) (string, error) {
	envVar, ok := envVarByProvider[provider]
	if !ok {
		return "", fmt.Errorf("unknown provider %q", provider)
	}

	token := strings.TrimSpace(os.Getenv(envVar))
	if token == "" {
		return "", &MissingTokenError{EnvVar: envVar}
	}

	return token, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/security/... -run TestResolveToken -v`

Expected: PASS — `TestResolveToken` (all subtests) and `TestResolveTokenMissingTokenError` both `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/security/tokens.go internal/security/tokens_test.go
git commit -m "feat(security): resolve provider tokens from environment variables"
```

---

### Task 3: internal/security — redaction

**Files:**
- Create: `internal/security/redact.go`
- Test: `internal/security/redact_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/security/redact_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/security/... -run TestRedact -v`

Expected: FAIL — `undefined: Redact`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/security/redact.go`:

```go
package security

import (
	"regexp"
	"sort"
	"strings"
)

// authURLPattern matches the credentials portion of a URL such as
// "https://oauth2:TOKEN@gitlab.com/..." — anything between "://" and "@"
// that contains no whitespace or "/". It cannot catch a bare token that
// appears outside of a URL; pass known secrets explicitly for that.
var authURLPattern = regexp.MustCompile(`(https?://)[^@\s/]+@`)

// Redact replaces credentials embedded in authenticated URLs with "***" and
// replaces any provided secret values wherever they appear. Empty secrets
// are ignored. Longer secrets are replaced first so that one secret being a
// prefix of another doesn't leave a partial value behind.
func Redact(value string, secrets ...string) string {
	result := authURLPattern.ReplaceAllString(value, "${1}***@")

	nonEmpty := make([]string, 0, len(secrets))
	for _, secret := range secrets {
		if secret != "" {
			nonEmpty = append(nonEmpty, secret)
		}
	}

	sort.Slice(nonEmpty, func(i, j int) bool { return len(nonEmpty[i]) > len(nonEmpty[j]) })

	for _, secret := range nonEmpty {
		result = strings.ReplaceAll(result, secret, "***")
	}

	return result
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/security/... -run TestRedact -v`

Expected: PASS — all `TestRedact` subtests `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/security/redact.go internal/security/redact_test.go
git commit -m "feat(security): redact credentials from URLs and secret values"
```

---

### Task 4: internal/security — temporary workspace management

**Files:**
- Create: `internal/security/workspace.go`
- Test: `internal/security/workspace_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/security/workspace_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/security/... -run 'TestManagerCreate|TestWorkspaceCleanup|TestManagerCleanupAll' -v`

Expected: FAIL — `undefined: NewManager` (and `undefined: Workspace`).

- [ ] **Step 3: Write minimal implementation**

Create `internal/security/workspace.go`:

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/security/... -run 'TestManagerCreate|TestWorkspaceCleanup|TestManagerCleanupAll' -v`

Expected: PASS — `TestManagerCreate`, `TestWorkspaceCleanup`, and `TestManagerCleanupAll` all `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/security/workspace.go internal/security/workspace_test.go
git commit -m "feat(security): add temporary workspace manager"
```

---

### Task 5: internal/security — git mirror clone/push

**Files:**
- Create: `internal/security/gitmirror.go`
- Test: `internal/security/gitmirror_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/security/gitmirror_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/security/... -run 'TestCloneAndPushMirror|TestCloneError' -v`

Expected: FAIL — `undefined: Clone` (and `undefined: Push`).

- [ ] **Step 3: Write minimal implementation**

Create `internal/security/gitmirror.go`:

```go
package security

import (
	"context"
	"fmt"
	"os/exec"
)

// Clone performs a full mirror clone of authenticatedURL into dir. dir must
// not already exist — git creates it.
func Clone(ctx context.Context, authenticatedURL, dir string) error {
	cmd := exec.CommandContext(ctx, "git", "clone", "--mirror", authenticatedURL, dir)

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone --mirror: %w: %s", err, Redact(string(output), authenticatedURL))
	}

	return nil
}

// Push mirrors every ref from the repository in dir to authenticatedURL.
func Push(ctx context.Context, dir, authenticatedURL string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "push", "--mirror", authenticatedURL)

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git push --mirror: %w: %s", err, Redact(string(output), authenticatedURL))
	}

	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/security/... -run 'TestCloneAndPushMirror|TestCloneError' -v`

Expected: PASS — `TestCloneAndPushMirror` and `TestCloneError` both `ok`. (Requires the `git` binary on PATH, which is already a hard dependency of `tfrepo` itself.)

- [ ] **Step 5: Commit**

```bash
git add internal/security/gitmirror.go internal/security/gitmirror_test.go
git commit -m "feat(security): add git mirror clone and push"
```

---

### Task 6: CI workflow

**Files:**
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Run the full test suite locally first**

Run: `go vet ./... && go test ./... -race`

Expected: PASS — `ok github.com/arijunior2020/tfrepo/internal/security` with no `vet` issues. This confirms the CI workflow added below will be green immediately.

- [ ] **Step 2: Create the CI workflow**

Create `.github/workflows/ci.yml`:

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      - run: go vet ./...
      - run: go test ./... -race
```

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add go vet and go test -race workflow"
```

---

## Acceptance Check

After Task 6, the following must all hold:

- `go build ./...` succeeds with no source files outside `internal/security` yet (an empty build is fine).
- `go vet ./...` and `go test ./... -race` both pass.
- `internal/security` exports: `ResolveToken`, `MissingTokenError`, `Redact`, `NewManager`/`Manager`/`Workspace` (`Create`, `Cleanup`, `CleanupAll`), `Clone`, `Push` — everything Plans 2-4 will depend on.
- `.github/workflows/ci.yml` runs `go vet` and `go test -race` on push/PR to `main`.
