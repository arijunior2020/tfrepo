# tfrepo Plano 6 — Comando `update` (self-update)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Adicionar `tfrepo update`, que consulta a última release no GitHub, compara com a versão instalada e, se houver novidade, baixa e substitui o próprio binário automaticamente.

**Architecture:** Um pacote `internal/updater` expõe funções puras testáveis (`LatestVersion`, `IsNewer`, `DownloadURL`, `Install`) que dependem de um `*http.Client` injetado — sem chamadas globais, sem nada estático difícil de testar. O comando `runUpdate` em `internal/cli/update.go` orquestra essas funções e segue o padrão dos outros comandos (exit code via `*int`, signal handling na camada Cobra, mensagens em português).

**Tech Stack:** Go 1.24, `archive/tar`, `compress/gzip`, `archive/zip`, `net/http`, `net/http/httptest` (testes), Cobra v1.10.2. Sem novas dependências externas.

---

## Estrutura de arquivos

| Arquivo | Ação | Responsabilidade |
|---|---|---|
| `internal/updater/updater.go` | Criar | `LatestVersion`, `IsNewer`, `DownloadURL`, `Install` + helpers de extração |
| `internal/updater/updater_test.go` | Criar | Testes com `httptest.NewServer` e arquivos em memória |
| `internal/cli/update.go` | Criar | `runUpdate`, `newUpdateCommand` |
| `internal/cli/root.go` | Modificar | Registrar `newUpdateCommand` |
| `internal/cli/root_test.go` | Modificar | `TestNewRootCommandRegistersUpdate` |
| `README.md` | Modificar | Seção `### \`tfrepo update\`` em Comandos |

---

### Task 1: Pacote `internal/updater`

Toda a lógica de verificação e instalação, testável sem terminal ou rede real.

**Files:**
- Create: `internal/updater/updater.go`
- Create: `internal/updater/updater_test.go`

- [ ] **Step 1: Escrever os testes**

Arquivo: `internal/updater/updater_test.go`

```go
package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// makeTarGZ cria um tar.gz em memória com um único arquivo name/content.
func makeTarGZ(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	hdr := &tar.Header{Name: name, Mode: 0755, Size: int64(len(content))}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("WriteHeader: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("tw.Write: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tw.Close: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("gw.Close: %v", err)
	}
	return buf.Bytes()
}

// makeZip cria um zip em memória com um único arquivo name/content.
func makeZip(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create(name)
	if err != nil {
		t.Fatalf("zip.Create: %v", err)
	}
	if _, err := f.Write(content); err != nil {
		t.Fatalf("zip.Write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip.Close: %v", err)
	}
	return buf.Bytes()
}

func TestLatestVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/releases/latest" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"tag_name": "v1.2.3"}`)
	}))
	defer srv.Close()

	got, err := LatestVersion(context.Background(), http.DefaultClient, srv.URL, "owner/repo")
	if err != nil {
		t.Fatalf("LatestVersion: %v", err)
	}
	if got != "1.2.3" {
		t.Errorf("LatestVersion = %q, want %q", got, "1.2.3")
	}
}

func TestLatestVersionHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := LatestVersion(context.Background(), http.DefaultClient, srv.URL, "owner/repo")
	if err == nil {
		t.Fatal("LatestVersion() error = nil, want error")
	}
}

func TestIsNewer(t *testing.T) {
	tests := []struct {
		current   string
		candidate string
		want      bool
	}{
		{"0.1.0", "0.2.0", true},
		{"0.1.0", "1.0.0", true},
		{"0.1.0", "0.1.1", true},
		{"0.2.0", "0.1.0", false},
		{"0.1.0", "0.1.0", false},
		{"1.0.0", "0.9.9", false},
		// com prefixo "v" deve funcionar também
		{"v0.1.0", "v0.2.0", true},
		{"v0.1.0", "v0.1.0", false},
	}
	for _, tt := range tests {
		got, err := IsNewer(tt.current, tt.candidate)
		if err != nil {
			t.Errorf("IsNewer(%q, %q): %v", tt.current, tt.candidate, err)
			continue
		}
		if got != tt.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", tt.current, tt.candidate, got, tt.want)
		}
	}
}

func TestIsNewerInvalidVersion(t *testing.T) {
	_, err := IsNewer("not-a-version", "0.1.0")
	if err == nil {
		t.Fatal("IsNewer() error = nil, want error")
	}
}

func TestDownloadURL(t *testing.T) {
	got := DownloadURL("owner/repo", "0.2.0", "linux", "amd64")
	want := "https://github.com/owner/repo/releases/download/v0.2.0/repo_0.2.0_linux_amd64.tar.gz"
	if got != want {
		t.Errorf("DownloadURL = %q, want %q", got, want)
	}
}

func TestDownloadURLWindows(t *testing.T) {
	got := DownloadURL("owner/repo", "0.2.0", "windows", "amd64")
	want := "https://github.com/owner/repo/releases/download/v0.2.0/repo_0.2.0_windows_amd64.zip"
	if got != want {
		t.Errorf("DownloadURL = %q, want %q", got, want)
	}
}

func TestInstallTarGZ(t *testing.T) {
	content := []byte("#!/bin/sh\necho hello")
	archive := makeTarGZ(t, "tfrepo", content)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(archive)
	}))
	defer srv.Close()

	dir := t.TempDir()
	binaryPath := filepath.Join(dir, "tfrepo")

	err := Install(context.Background(), http.DefaultClient, srv.URL+"/tfrepo_0.2.0_linux_amd64.tar.gz", binaryPath)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	got, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("content = %q, want %q", got, content)
	}

	info, err := os.Stat(binaryPath)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode()&0111 == 0 {
		t.Error("binary is not executable")
	}
}

func TestInstallZip(t *testing.T) {
	content := []byte("fake windows binary")
	archive := makeZip(t, "tfrepo.exe", content)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(archive)
	}))
	defer srv.Close()

	dir := t.TempDir()
	binaryPath := filepath.Join(dir, "tfrepo.exe")

	err := Install(context.Background(), http.DefaultClient, srv.URL+"/tfrepo_0.2.0_windows_amd64.zip", binaryPath)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	got, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("content = %q, want %q", got, content)
	}
}

func TestInstallBinaryNotInArchive(t *testing.T) {
	// tar.gz com nome de arquivo diferente do esperado
	archive := makeTarGZ(t, "other-binary", []byte("content"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer srv.Close()

	dir := t.TempDir()
	binaryPath := filepath.Join(dir, "tfrepo")

	err := Install(context.Background(), http.DefaultClient, srv.URL+"/tfrepo_0.2.0_linux_amd64.tar.gz", binaryPath)
	if err == nil {
		t.Fatal("Install() error = nil, want error")
	}
}
```

- [ ] **Step 2: Rodar os testes para confirmar que falham**

```bash
cd /media/arimateia-junior/Dados4/projetos/tfrepo
go test ./internal/updater/... -v 2>&1 | head -10
```

Esperado: erro de compilação (`no Go files in .../updater`).

- [ ] **Step 3: Implementar `internal/updater/updater.go`**

```go
package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// LatestVersion queries the GitHub releases API and returns the latest
// version string without the "v" prefix (e.g. "0.2.0").
// baseURL should be "https://api.github.com" in production; tests inject
// an httptest.Server URL.
func LatestVersion(ctx context.Context, client *http.Client, baseURL, repo string) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", baseURL, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}

	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}

	version := strings.TrimPrefix(payload.TagName, "v")
	if version == "" {
		return "", fmt.Errorf("GitHub API returned empty tag_name")
	}
	return version, nil
}

// IsNewer reports whether candidate is strictly newer than current.
// Both strings may optionally have a "v" prefix (e.g. "0.1.0" or "v0.1.0").
func IsNewer(current, candidate string) (bool, error) {
	cMaj, cMin, cPat, err := parseVersion(current)
	if err != nil {
		return false, fmt.Errorf("current %q: %w", current, err)
	}
	nMaj, nMin, nPat, err := parseVersion(candidate)
	if err != nil {
		return false, fmt.Errorf("candidate %q: %w", candidate, err)
	}
	if nMaj != cMaj {
		return nMaj > cMaj, nil
	}
	if nMin != cMin {
		return nMin > cMin, nil
	}
	return nPat > cPat, nil
}

// DownloadURL returns the GitHub release asset URL for the given
// repo/version/goos/goarch combination, matching the archive name template
// in .goreleaser.yaml: <project>_<version>_<os>_<arch>.tar.gz (or .zip on
// Windows).
func DownloadURL(repo, version, goos, goarch string) string {
	project := filepath.Base(repo)
	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}
	filename := fmt.Sprintf("%s_%s_%s_%s.%s", project, version, goos, goarch, ext)
	return fmt.Sprintf("https://github.com/%s/releases/download/v%s/%s", repo, version, filename)
}

// Install downloads the archive at downloadURL, extracts the binary whose
// base name matches filepath.Base(binaryPath), and atomically replaces
// binaryPath. The replacement is atomic on Unix (os.Rename); on Windows it
// works when the file is not locked by another process.
func Install(ctx context.Context, client *http.Client, downloadURL, binaryPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}

	binaryName := filepath.Base(binaryPath)
	tmpPath := binaryPath + ".new"

	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}

	var found bool
	if strings.HasSuffix(downloadURL, ".tar.gz") {
		found, err = extractTarGZ(resp.Body, binaryName, f)
	} else {
		data, rerr := io.ReadAll(resp.Body)
		if rerr != nil {
			_ = f.Close()
			_ = os.Remove(tmpPath)
			return rerr
		}
		found, err = extractZip(data, binaryName, f)
	}
	_ = f.Close()
	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if !found {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("binary %q not found in archive", binaryName)
	}
	return os.Rename(tmpPath, binaryPath)
}

func extractTarGZ(r io.Reader, name string, dst io.Writer) (bool, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return false, err
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if filepath.Base(hdr.Name) == name {
			if _, err := io.Copy(dst, tr); err != nil {
				return false, err
			}
			return true, nil
		}
	}
}

func extractZip(data []byte, name string, dst io.Writer) (bool, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false, err
	}
	for _, f := range zr.File {
		if filepath.Base(f.Name) == name {
			rc, err := f.Open()
			if err != nil {
				return false, err
			}
			defer func() { _ = rc.Close() }()
			if _, err := io.Copy(dst, rc); err != nil {
				return false, err
			}
			return true, nil
		}
	}
	return false, nil
}

func parseVersion(v string) (major, minor, patch int, err error) {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("expected MAJOR.MINOR.PATCH, got %q", v)
	}
	major, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid major: %w", err)
	}
	minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid minor: %w", err)
	}
	patch, err = strconv.Atoi(parts[2])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid patch: %w", err)
	}
	return
}
```

- [ ] **Step 4: Rodar os testes para confirmar que passam**

```bash
cd /media/arimateia-junior/Dados4/projetos/tfrepo
go test ./internal/updater/... -v
```

Esperado: todos os testes PASS (9 funções de teste).

- [ ] **Step 5: Rodar a suite completa**

```bash
go test ./... -race
```

Esperado: todos os pacotes `ok`.

- [ ] **Step 6: Commit**

```bash
git add internal/updater/updater.go internal/updater/updater_test.go
git commit -m "feat(updater): add LatestVersion, IsNewer, DownloadURL and Install"
```

---

### Task 2: Comando `update` + wiring + README

Implementa `runUpdate` e `newUpdateCommand`, registra em `root.go` e documenta.

**Files:**
- Create: `internal/cli/update.go`
- Modify: `internal/cli/root.go`
- Modify: `internal/cli/root_test.go`
- Modify: `README.md`

- [ ] **Step 1: Escrever o teste de registro do comando**

Adicione ao final de `internal/cli/root_test.go`:

```go
func TestNewRootCommandRegistersUpdate(t *testing.T) {
	exitCode := 0
	root := newRootCommand(&exitCode)

	updateCmd, _, err := root.Find([]string{"update"})
	if err != nil {
		t.Fatalf("Find(update): %v", err)
	}
	if updateCmd.Use != "update" {
		t.Fatalf("Find(update).Use = %q, want %q", updateCmd.Use, "update")
	}
}
```

- [ ] **Step 2: Rodar o teste para confirmar que falha**

```bash
cd /media/arimateia-junior/Dados4/projetos/tfrepo
go test ./internal/cli/... -run TestNewRootCommandRegistersUpdate -v
```

Esperado: FAIL (update not found / Use != "update").

- [ ] **Step 3: Criar `internal/cli/update.go`**

```go
package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/arijunior2020/tfrepo/internal/updater"
	"github.com/spf13/cobra"
	"os/signal"
	"syscall"
)

const (
	githubRepo    = "arijunior2020/tfrepo"
	githubAPIBase = "https://api.github.com"
)

func newUpdateCommand(exitCode *int) *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Atualiza o tfrepo para a versão mais recente",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			*exitCode = runUpdate(ctx, cmd.OutOrStdout(), cmd.ErrOrStderr())
			return nil
		},
	}
}

func runUpdate(ctx context.Context, stdout, stderr io.Writer) int {
	client := &http.Client{Timeout: 60 * time.Second}

	fmt.Fprintln(stdout, "Buscando última versão...")
	latest, err := updater.LatestVersion(ctx, client, githubAPIBase, githubRepo)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	newer, err := updater.IsNewer(programVersion, latest)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if !newer {
		fmt.Fprintf(stdout, "Já está na versão mais recente (v%s).\n", programVersion)
		return 0
	}

	fmt.Fprintf(stdout, "Nova versão disponível: v%s → v%s\n", programVersion, latest)

	binaryPath, err := os.Executable()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	// Resolve symlinks para obter o caminho real do binário
	binaryPath, err = filepath.EvalSymlinks(binaryPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	url := updater.DownloadURL(githubRepo, latest, runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(stdout, "Baixando %s...\n", url)

	if err := updater.Install(ctx, client, url, binaryPath); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	fmt.Fprintf(stdout, "Atualizado para v%s com sucesso!\n", latest)
	return 0
}
```

**Nota:** as imports ficam em dois blocos separados pelo `goimports`/`gofmt`. Se o compilador reclamar, agrupe manualmente:

```go
import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/arijunior2020/tfrepo/internal/updater"
	"github.com/spf13/cobra"
)
```

- [ ] **Step 4: Registrar em `root.go`**

Em `newRootCommand`, após `root.AddCommand(newSetupCommand(exitCode))`, adicione:

```go
root.AddCommand(newUpdateCommand(exitCode))
```

- [ ] **Step 5: Rodar o teste para confirmar que passa**

```bash
go test ./internal/cli/... -run TestNewRootCommandRegistersUpdate -v
```

Esperado: PASS.

- [ ] **Step 6: Rodar a suite completa**

```bash
go test ./... -race
```

Esperado: todos os pacotes `ok`.

- [ ] **Step 7: Build para confirmar compilação**

```bash
go build ./...
```

Esperado: sem erros.

- [ ] **Step 8: Atualizar `README.md`**

Localize `### \`tfrepo validate\`` e adicione a seguinte seção **depois** dela (antes de `## Variáveis de ambiente`):

```markdown
### `tfrepo update`

Verifica se há uma versão mais recente disponível no GitHub e, se houver,
baixa e substitui o binário instalado automaticamente.

```bash
tfrepo update
```

Não requer nenhuma flag. O binário substituído é o mesmo que está em execução
(`os.Executable()`). Em caso de erro de permissão, execute com `sudo`.

```

- [ ] **Step 9: Commit**

```bash
cd /media/arimateia-junior/Dados4/projetos/tfrepo
git add internal/cli/update.go internal/cli/root.go internal/cli/root_test.go README.md
git commit -m "feat(cli): add update command for self-update from GitHub releases"
```
