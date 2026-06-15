# tfrepo Plano 4a — CLI scaffold (banner, provider factory, init, root) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Criar o esqueleto executável do binário `tfrepo` (Cobra), com o comando `init` funcionando, o banner ASCII exibido quando chamado sem argumentos, e a fábrica de providers compartilhada pelos comandos `scan`/`plan`/`migrate`/`validate` dos próximos planos.

**Architecture:** `cmd/tfrepo/main.go` chama `cli.Execute()`, que monta a árvore de comandos Cobra em `internal/cli/root.go` e devolve o exit code do processo. Cada subcomando escreve seu próprio exit code em um `*int` compartilhado (passado a `newRootCommand`), espelhando `process.exitCode = await deps.runXxx(...)` do `transferepo` (Node) sem depender do mecanismo de erro do Cobra para exit codes "de negócio". `internal/cli/providers.go` resolve o token via `internal/security.ResolveToken` e instancia `provider.NewGitHubProvider`/`provider.NewGitLabProvider`, pronto para ser reutilizado por `scan`/`plan`/`migrate`/`validate` (Planos 4b/4c).

**Tech Stack:** Go 1.24, Cobra (`github.com/spf13/cobra`), pacotes internos já existentes `internal/config`, `internal/provider`, `internal/security`.

---

## Contexto para quem for executar este plano

Este é o **Plano 4a** de uma série (4a/4b/4c/4d) que implementa o Plano 4
("internal/cli") descrito em
`docs/superpowers/specs/2026-06-13-tfrepo-go-port-design.md`. Os Planos
1-3c já foram implementados e estão na `main`:

- `internal/security` — `ResolveToken`, `Redact`, `Manager`/`Workspace`,
  `Clone`/`Push` (mirror via `git`).
- `internal/provider` — `RepositoryProvider` interface,
  `NewGitHubProvider(token, baseURL string) (*GitHubProvider, error)`,
  `NewGitLabProvider(token, baseURL string) (*GitLabProvider, error)`.
- `internal/config` — `Load(path string) (*Config, error)`,
  `Config{Source, Target config.ProviderConfig; Filters; Mapping}`.
- `internal/core` — `Scan`, `Plan`, `Migrate`, `Validate`,
  `WriteJSON`/`ReadJSON`, tipos `Inventory`/`MigrationPlan`/
  `MigrationReport`/`ValidationReport`.

Este plano (4a) **não** implementa `scan`/`plan`/`migrate`/`validate` —
isso fica para os Planos 4b (scan+plan) e 4c (migrate+validate). Também não
cobre o `README.md` final nem `.goreleaser.yaml`/release workflow — isso
fica para o Plano 4d. Ao final deste plano, `tfrepo init` funciona de ponta
a ponta e `tfrepo` (sem argumentos) mostra o banner + ajuda.

A referência de comportamento (mensagens exatas, exit codes) é o CLI Node
em `apps/cli/src/` do repositório `arijunior2020/transferepo` (privado),
especificamente `cli.ts`, `index.ts`, `banner.ts` e
`commands/init.ts`.

---

## Task 1: Banner (`internal/cli/banner.go`)

**Files:**
- Create: `internal/cli/banner.go`
- Test: `internal/cli/banner_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/cli/banner_test.go`:

```go
package cli

import (
	"strings"
	"testing"
)

func TestRenderBannerIncludesSubtitleAndArt(t *testing.T) {
	got := renderBanner()

	if !strings.Contains(got, bannerSubtitle) {
		t.Errorf("renderBanner() missing subtitle: %q", got)
	}
	if strings.Count(got, "\n") < 6 {
		t.Errorf("renderBanner() has too few lines: %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/... -run TestRenderBannerIncludesSubtitleAndArt -v`

Expected: FAIL — `package cli is not in std` / `undefined: renderBanner`,
`undefined: bannerSubtitle` (package `internal/cli` doesn't exist yet).

- [ ] **Step 3: Write minimal implementation**

Create `internal/cli/banner.go`:

```go
package cli

// bannerSubtitle is shown under the ASCII art when tfrepo is invoked
// without any arguments, mirroring SUBTITLE in apps/cli/src/banner.ts
// (TransfeRepo, commit 4bdbca7).
const bannerSubtitle = "Migração de repositórios Git entre provedores (GitHub, GitLab)"

// bannerArt is the ASCII art title shown when tfrepo is invoked without any
// arguments.
const bannerArt = `
████████╗███████╗██████╗ ███████╗██████╗   ██████╗
╚══██╔══╝██╔════╝██╔══██╗██╔════╝██╔══██╗ ██╔═══██╗
   ██║   █████╗  ██████╔╝█████╗  ██████╔╝ ██║   ██║
   ██║   ██╔══╝  ██╔══██╗██╔══╝  ██╔═══╝  ██║   ██║
   ██║   ██║     ██║  ██║███████╗██║      ╚██████╔╝
   ╚═╝   ╚═╝     ╚═╝  ╚═╝╚══════╝╚═╝       ╚═════╝
`

// renderBanner returns the ASCII art banner shown when tfrepo is invoked
// without any arguments.
func renderBanner() string {
	return bannerArt + bannerSubtitle + "\n"
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/... -run TestRenderBannerIncludesSubtitleAndArt -v`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/cli/banner.go internal/cli/banner_test.go
git commit -m "feat(cli): add tfrepo ASCII banner"
```

---

## Task 2: Provider factory (`internal/cli/providers.go`)

**Files:**
- Create: `internal/cli/providers.go`
- Test: `internal/cli/providers_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/cli/providers_test.go`:

```go
package cli

import (
	"testing"

	"github.com/arijunior2020/tfrepo/internal/config"
)

func TestNewProviderGitHub(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "test-token")

	p, err := NewProvider(config.ProviderConfig{Provider: "github", Namespace: "my-org"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if p.Name() != "github" {
		t.Errorf("Name() = %q, want %q", p.Name(), "github")
	}
}

func TestNewProviderGitLab(t *testing.T) {
	t.Setenv("GITLAB_TOKEN", "test-token")

	p, err := NewProvider(config.ProviderConfig{Provider: "gitlab", Namespace: "my-group"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if p.Name() != "gitlab" {
		t.Errorf("Name() = %q, want %q", p.Name(), "gitlab")
	}
}

func TestNewProviderMissingTokenReturnsError(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")

	_, err := NewProvider(config.ProviderConfig{Provider: "github", Namespace: "my-org"})
	if err == nil {
		t.Fatal("NewProvider() error = nil, want error")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cli/... -run TestNewProvider -v`

Expected: FAIL — `undefined: NewProvider`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/cli/providers.go`:

```go
package cli

import (
	"github.com/arijunior2020/tfrepo/internal/config"
	"github.com/arijunior2020/tfrepo/internal/provider"
	"github.com/arijunior2020/tfrepo/internal/security"
)

// NewProvider resolves cfg.Provider's API token from the environment
// (GITHUB_TOKEN or GITLAB_TOKEN, via security.ResolveToken) and constructs
// the corresponding provider.RepositoryProvider. cfg.BaseURL is passed
// through verbatim (empty means the public API).
func NewProvider(cfg config.ProviderConfig) (provider.RepositoryProvider, error) {
	token, err := security.ResolveToken(cfg.Provider)
	if err != nil {
		return nil, err
	}

	if cfg.Provider == "gitlab" {
		return provider.NewGitLabProvider(token, cfg.BaseURL)
	}
	return provider.NewGitHubProvider(token, cfg.BaseURL)
}
```

Note: `security.ResolveToken` only recognizes `"github"` and `"gitlab"` and
returns an error for anything else (see `internal/security/tokens.go`), so
by the time the `if` below is reached, `cfg.Provider` is guaranteed to be
one of those two values — no `default`/unknown-provider branch is needed
here.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cli/... -run TestNewProvider -v`

Expected: PASS (all 3 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/cli/providers.go internal/cli/providers_test.go
git commit -m "feat(cli): add provider factory resolving tokens from env"
```

---

## Task 3: `init` command (`internal/cli/init.go`)

**Files:**
- Create: `internal/cli/init.go`
- Test: `internal/cli/init_test.go`

This task implements `runInit`, the testable core of `tfrepo init`,
independent of Cobra wiring (which is added in Task 4).

- [ ] **Step 1: Write the failing tests**

Create `internal/cli/init_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cli/... -run TestRunInit -v`

Expected: FAIL — `undefined: runInit`, `undefined: configTemplate`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/cli/init.go`:

```go
package cli

import (
	"fmt"
	"io"
	"os"
)

// configTemplate is the content written by "tfrepo init", mirroring
// CONFIG_TEMPLATE in apps/cli/src/commands/init.ts.
const configTemplate = `# Configuração do tfrepo.
#
# Este arquivo foi gerado como um TEMPLATE por "tfrepo init".
# Edite os valores abaixo com as informações da sua migração e depois rode:
#   tfrepo scan

source:
  provider: github       # "github" ou "gitlab"
  namespace: minha-org   # usuário ou organização/grupo de ORIGEM
  # baseUrl: https://github.minhaempresa.com  # opcional: GitHub Enterprise / GitLab self-hosted

target:
  provider: gitlab
  namespace: meu-grupo   # usuário ou grupo/subgrupo de DESTINO
  # baseUrl: https://gitlab.minhaempresa.com  # opcional: GitLab self-hosted

filters:
  include: ["*"]   # padrões glob (estilo .gitignore) de repositórios a incluir
  exclude: []      # padrões glob de repositórios a excluir, mesmo que incluídos acima

mapping: {}        # renomeia repositórios no destino: { "repo-origem": "repo-destino" }
`

// runInit writes configTemplate to configPath, unless a file already exists
// there, in which case it leaves the existing file untouched and reports an
// error. It returns the process exit code (0 on success, 1 otherwise),
// mirroring runInit in apps/cli/src/commands/init.ts.
func runInit(configPath string, stdout, stderr io.Writer) int {
	if _, err := os.Stat(configPath); err == nil {
		fmt.Fprintf(stderr, "%s já existe. Remova-o ou edite-o diretamente antes de rodar \"init\" novamente.\n", configPath)
		return 1
	}

	if err := os.WriteFile(configPath, []byte(configTemplate), 0o644); err != nil {
		fmt.Fprintf(stderr, "%s\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "Wrote %s (edite os valores e rode \"tfrepo scan\")\n", configPath)
	return 0
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cli/... -run TestRunInit -v`

Expected: PASS (both tests)

- [ ] **Step 5: Commit**

```bash
git add internal/cli/init.go internal/cli/init_test.go
git commit -m "feat(cli): add tfrepo init command logic"
```

---

## Task 4: Root command (`internal/cli/root.go`)

**Files:**
- Create: `internal/cli/root.go`
- Test: `internal/cli/root_test.go`
- Modify: `go.mod`, `go.sum` (adds Cobra dependency)

This task adds the Cobra dependency, wires `init` into a root command tree,
and provides `Execute() int` — the function `cmd/tfrepo/main.go` (Task 5)
will call.

- [ ] **Step 1: Add the Cobra dependency**

Run: `go get github.com/spf13/cobra`

Expected: `go.mod` gains a `require github.com/spf13/cobra vX.Y.Z` line
(plus its transitive dependencies in `go.sum`).

- [ ] **Step 2: Write the failing tests**

Create `internal/cli/root_test.go`:

```go
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecuteInitCreatesConfigFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "transferepo.config.yaml")

	exitCode := 0
	root := newRootCommand(&exitCode)

	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"init", "--config", configPath})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v (stderr: %s)", err, stderr.String())
	}
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0 (stdout: %s, stderr: %s)", exitCode, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Errorf("config file not created: %v", err)
	}
}

func TestExecuteInitReturnsExitCode1WhenConfigExists(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "transferepo.config.yaml")
	if err := os.WriteFile(configPath, []byte("existing"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	exitCode := 0
	root := newRootCommand(&exitCode)

	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"init", "--config", configPath})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v (stderr: %s)", err, stderr.String())
	}
	if exitCode != 1 {
		t.Fatalf("exitCode = %d, want 1", exitCode)
	}
}

func TestExecuteNoArgsPrintsBanner(t *testing.T) {
	exitCode := 0
	root := newRootCommand(&exitCode)

	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(stdout.String(), bannerSubtitle) {
		t.Errorf("stdout = %q, want it to contain the banner subtitle", stdout.String())
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/cli/... -run TestExecute -v`

Expected: FAIL — `undefined: newRootCommand`.

- [ ] **Step 4: Write minimal implementation**

Create `internal/cli/root.go`:

```go
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const (
	// defaultConfigPath is the default value of the --config flag,
	// matching DEFAULT_CONFIG_PATH in apps/cli/src/cli.ts.
	defaultConfigPath = "transferepo.config.yaml"

	// programVersion matches PROGRAM_VERSION in apps/cli/src/cli.ts.
	programVersion = "0.1.0"
)

// Execute runs the tfrepo root command against os.Args and returns the
// process exit code.
func Execute() int {
	exitCode := 0
	root := newRootCommand(&exitCode)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return exitCode
}

// newRootCommand builds the tfrepo command tree. Each subcommand writes its
// exit code to *exitCode instead of returning an error for "business" exit
// codes (e.g. migrate returning 1 because a task failed), mirroring
// process.exitCode = await deps.runXxx(...) in apps/cli/src/cli.ts.
func newRootCommand(exitCode *int) *cobra.Command {
	root := &cobra.Command{
		Use:           "tfrepo",
		Short:         "Migra repositórios Git entre provedores (GitHub, GitLab)",
		Version:       programVersion,
		SilenceErrors: true,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprint(cmd.OutOrStdout(), renderBanner())
			fmt.Fprintln(cmd.OutOrStdout())
			_ = cmd.Help()
		},
	}

	root.PersistentFlags().StringP("config", "c", defaultConfigPath, "caminho do arquivo de configuração")

	root.AddCommand(newInitCommand(exitCode))

	return root
}

func newInitCommand(exitCode *int) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Gera um transferepo.config.yaml de exemplo no diretório atual",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := cmd.Flags().GetString("config")
			if err != nil {
				return err
			}
			*exitCode = runInit(configPath, cmd.OutOrStdout(), cmd.ErrOrStderr())
			return nil
		},
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/cli/... -v`

Expected: PASS — all tests in the package (banner, providers, init, root).

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/cli/root.go internal/cli/root_test.go
git commit -m "feat(cli): add root command wiring init, banner and --config flag"
```

---

## Task 5: Entrypoint (`cmd/tfrepo/main.go`)

**Files:**
- Create: `cmd/tfrepo/main.go`

- [ ] **Step 1: Write the entrypoint**

Create `cmd/tfrepo/main.go`:

```go
package main

import (
	"os"

	"github.com/arijunior2020/tfrepo/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
```

- [ ] **Step 2: Build and smoke-test manually**

Run: `go build ./...`

Expected: builds cleanly, produces no errors.

Run (no args — should print banner + help, exit 0):

```bash
go run ./cmd/tfrepo
```

Expected: ASCII art banner, then `bannerSubtitle`, then Cobra's standard
help output (Usage, Available Commands including `init`, Flags including
`-c, --config`).

Run (`--help` — should print help without banner, exit 0):

```bash
go run ./cmd/tfrepo --help
```

Run (`init` in a temp dir):

```bash
cd "$(mktemp -d)" && go run /media/arimateia-junior/Dados4/projetos/tfrepo/cmd/tfrepo init && cat transferepo.config.yaml && cd -
```

Expected: prints `Wrote transferepo.config.yaml (edite os valores e rode "tfrepo scan")`,
and the file's content matches `configTemplate` from Task 3.

Run `init` again in the same temp dir to confirm the exit-code-1 path:

```bash
cd "$(mktemp -d)" && go run /media/arimateia-junior/Dados4/projetos/tfrepo/cmd/tfrepo init && go run /media/arimateia-junior/Dados4/projetos/tfrepo/cmd/tfrepo init; echo "exit code: $?"
```

Expected: second invocation prints the "já existe" message to stderr and
`exit code: 1`.

- [ ] **Step 3: Commit**

```bash
git add cmd/tfrepo/main.go
git commit -m "feat(cli): add cmd/tfrepo entrypoint"
```

---

## Final Verification

Run from the repo root:

```bash
go build ./...
go vet ./...
go test ./... -race
gofmt -l .
```

Expected: all four commands succeed with no output from `gofmt -l .`
(no unformatted files), and `go test ./...` shows `ok` for `internal/cli`
alongside the existing `internal/config`, `internal/core`,
`internal/provider`, and `internal/security` packages.

---

## What's next (Planos 4b/4c/4d — not part of this plan)

- **4b**: `tfrepo scan [--concurrency N]` and `tfrepo plan`, using
  `cli.NewProvider`, `core.Scan`, `core.Plan`, and
  `core.WriteJSON`/`core.ReadJSON` for `inventory.json` /
  `migration-plan.json`.
- **4c**: `tfrepo migrate [--dry-run] [--concurrency N]` and
  `tfrepo validate`, using `core.Migrate`/`core.Validate`,
  `security.Manager` for workspace cleanup, and SIGINT/SIGTERM handling.
- **4d**: rewrite `README.md` with full `tfrepo` usage instructions (ported
  from `transferepo`'s README section 5/6), `.goreleaser.yaml`, and
  `.github/workflows/release.yml`.
