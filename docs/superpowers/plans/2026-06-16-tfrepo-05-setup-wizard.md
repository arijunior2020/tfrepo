# tfrepo Plano 5 — Comando `setup` (wizard interativo)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Adicionar `tfrepo setup`, um wizard interativo que guia o usuário pelo processo de configuração — pergunta provider/namespace de origem, executa scan, exibe os repositórios encontrados para seleção, pergunta provider/namespace de destino e gera o `transferepo.config.yaml` pronto para uso.

**Architecture:** A lógica pura (`WizardInput`, `buildConfigYAML`, `setupAndWrite`) é separada dos prompts interativos (`runSetup`), permitindo testes unitários sem terminal. Os prompts usam `charmbracelet/huh` v1.0.0 (TUI forms). O padrão de código segue exatamente os outros comandos do pacote `internal/cli`: `run*` para entry point, função testável separada, `*int exitCode` via Cobra.

**Tech Stack:** Go 1.24, Cobra v1.10.2, `github.com/charmbracelet/huh` v1.0.0, `gopkg.in/yaml.v3`

---

## Estrutura de arquivos

| Arquivo | Ação | Responsabilidade |
|---|---|---|
| `internal/cli/setup.go` | Criar | `WizardInput`, `buildConfigYAML`, `setupAndWrite`, `runSetup` |
| `internal/cli/setup_test.go` | Criar | Testes de `buildConfigYAML` e `setupAndWrite` |
| `internal/cli/root.go` | Modificar | Registrar `newSetupCommand` |
| `internal/cli/root_test.go` | Modificar | Teste de registro do comando `setup` |
| `go.mod` / `go.sum` | Modificar | Adicionar `github.com/charmbracelet/huh` |
| `README.md` | Modificar | Documentar o comando `setup` |

---

### Task 1: Lógica core (não-interativa)

Implementa `WizardInput`, `buildConfigYAML` e `setupAndWrite` — toda a lógica testável sem terminal.

**Files:**
- Create: `internal/cli/setup.go`
- Create: `internal/cli/setup_test.go`

- [ ] **Step 1: Escrever os testes de `buildConfigYAML` e `setupAndWrite`**

Arquivo: `internal/cli/setup_test.go`

```go
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arijunior2020/tfrepo/internal/config"
)

func TestBuildConfigYAMLAllRepos(t *testing.T) {
	input := WizardInput{
		SourceProvider:  "github",
		SourceNamespace: "my-org",
		TargetProvider:  "gitlab",
		TargetNamespace: "my-group",
	}
	yml, err := buildConfigYAML(input)
	if err != nil {
		t.Fatalf("buildConfigYAML: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "transferepo.config.yaml")
	if err := os.WriteFile(path, yml, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg.Source.Provider != "github" {
		t.Errorf("source.provider = %q, want %q", cfg.Source.Provider, "github")
	}
	if cfg.Source.Namespace != "my-org" {
		t.Errorf("source.namespace = %q, want %q", cfg.Source.Namespace, "my-org")
	}
	if cfg.Target.Provider != "gitlab" {
		t.Errorf("target.provider = %q, want %q", cfg.Target.Provider, "gitlab")
	}
	if cfg.Target.Namespace != "my-group" {
		t.Errorf("target.namespace = %q, want %q", cfg.Target.Namespace, "my-group")
	}
	if len(cfg.Filters.Include) != 1 || cfg.Filters.Include[0] != "*" {
		t.Errorf("filters.include = %v, want [*]", cfg.Filters.Include)
	}
}

func TestBuildConfigYAMLSelectedRepos(t *testing.T) {
	input := WizardInput{
		SourceProvider:  "github",
		SourceNamespace: "my-org",
		TargetProvider:  "gitlab",
		TargetNamespace: "my-group",
		SelectedRepos:   []string{"repo-b", "repo-a"}, // ordem inversa → deve ser ordenado
	}
	yml, err := buildConfigYAML(input)
	if err != nil {
		t.Fatalf("buildConfigYAML: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "transferepo.config.yaml")
	if err := os.WriteFile(path, yml, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	want := []string{"repo-a", "repo-b"} // ordenados alfabeticamente
	if len(cfg.Filters.Include) != 2 ||
		cfg.Filters.Include[0] != want[0] ||
		cfg.Filters.Include[1] != want[1] {
		t.Errorf("filters.include = %v, want %v", cfg.Filters.Include, want)
	}
}

func TestBuildConfigYAMLWithBaseURL(t *testing.T) {
	input := WizardInput{
		SourceProvider:  "gitlab",
		SourceNamespace: "my-group",
		SourceBaseURL:   "https://gitlab.example.com",
		TargetProvider:  "github",
		TargetNamespace: "my-org",
	}
	yml, err := buildConfigYAML(input)
	if err != nil {
		t.Fatalf("buildConfigYAML: %v", err)
	}
	if !strings.Contains(string(yml), "gitlab.example.com") {
		t.Errorf("YAML não contém baseUrl: %s", yml)
	}
}

func TestSetupAndWriteCreatesConfigFile(t *testing.T) {
	t.Chdir(t.TempDir())
	dir := t.TempDir()
	configPath := filepath.Join(dir, "transferepo.config.yaml")

	input := WizardInput{
		SourceProvider:  "github",
		SourceNamespace: "my-org",
		TargetProvider:  "gitlab",
		TargetNamespace: "my-group",
	}
	var stdout, stderr bytes.Buffer
	code := setupAndWrite(input, configPath, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("setupAndWrite = %d, want 0 (stderr: %s)", code, stderr.String())
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("config file not created: %v", err)
	}
	if !strings.Contains(stdout.String(), configPath) {
		t.Errorf("stdout = %q, want to contain %q", stdout.String(), configPath)
	}
}

func TestSetupAndWriteOverwritesExistingFile(t *testing.T) {
	t.Chdir(t.TempDir())
	dir := t.TempDir()
	configPath := filepath.Join(dir, "transferepo.config.yaml")
	if err := os.WriteFile(configPath, []byte("old content"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	input := WizardInput{
		SourceProvider:  "github",
		SourceNamespace: "my-org",
		TargetProvider:  "gitlab",
		TargetNamespace: "my-group",
	}
	var stdout, stderr bytes.Buffer
	code := setupAndWrite(input, configPath, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("setupAndWrite = %d, want 0", code)
	}
	got, _ := os.ReadFile(configPath)
	if strings.Contains(string(got), "old content") {
		t.Error("config file was not overwritten")
	}
}
```

- [ ] **Step 2: Executar os testes para confirmar que falham**

```bash
cd /media/arimateia-junior/Dados4/projetos/tfrepo
go test ./internal/cli/... -run "TestBuild|TestSetupAndWrite" -v 2>&1 | head -20
```

Esperado: erro de compilação (`WizardInput undefined`).

- [ ] **Step 3: Implementar `setup.go` com a lógica core**

Arquivo: `internal/cli/setup.go`

```go
package cli

import (
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/arijunior2020/tfrepo/internal/config"
	"gopkg.in/yaml.v3"
)

// WizardInput holds the answers collected by the setup wizard.
// SelectedRepos, if non-empty, becomes filters.include in the generated
// config. An empty SelectedRepos means "all repos" (filters.include = ["*"]).
type WizardInput struct {
	SourceProvider  string
	SourceNamespace string
	SourceBaseURL   string // empty = public API
	TargetProvider  string
	TargetNamespace string
	TargetBaseURL   string // empty = public API
	SelectedRepos   []string
}

// buildConfigYAML converts wizard answers into transferepo.config.yaml
// content. Repos in SelectedRepos are sorted alphabetically for
// determinism; an empty slice produces filters.include: ["*"].
func buildConfigYAML(input WizardInput) ([]byte, error) {
	include := []string{"*"}
	if len(input.SelectedRepos) > 0 {
		sorted := make([]string, len(input.SelectedRepos))
		copy(sorted, input.SelectedRepos)
		sort.Strings(sorted)
		include = sorted
	}

	cfg := config.Config{
		Source: config.ProviderConfig{
			Provider:  input.SourceProvider,
			Namespace: input.SourceNamespace,
			BaseURL:   input.SourceBaseURL,
		},
		Target: config.ProviderConfig{
			Provider:  input.TargetProvider,
			Namespace: input.TargetNamespace,
			BaseURL:   input.TargetBaseURL,
		},
		Filters: config.Filters{
			Include: include,
			Exclude: []string{},
		},
		Mapping: map[string]string{},
	}

	return yaml.Marshal(cfg)
}

// setupAndWrite writes the config generated from input to configPath,
// overwriting any existing file, and prints a one-line summary to stdout.
func setupAndWrite(input WizardInput, configPath string, stdout, stderr io.Writer) int {
	yml, err := buildConfigYAML(input)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := os.WriteFile(configPath, yml, 0o644); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "Wrote %s\n", configPath)
	return 0
}
```

- [ ] **Step 4: Executar os testes para confirmar que passam**

```bash
cd /media/arimateia-junior/Dados4/projetos/tfrepo
go test ./internal/cli/... -run "TestBuild|TestSetupAndWrite" -v
```

Esperado: 5 testes PASS.

- [ ] **Step 5: Commit**

```bash
cd /media/arimateia-junior/Dados4/projetos/tfrepo
git add internal/cli/setup.go internal/cli/setup_test.go
git commit -m "feat(cli): add WizardInput, buildConfigYAML and setupAndWrite for setup wizard"
```

---

### Task 2: Wizard interativo (`runSetup` com `huh`)

Adiciona a dependência `huh` e implementa o fluxo interativo completo.

**Files:**
- Modify: `internal/cli/setup.go` (adicionar `runSetup` e helpers de prompt)
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Adicionar dependência `huh`**

```bash
cd /media/arimateia-junior/Dados4/projetos/tfrepo
go get github.com/charmbracelet/huh@latest
go mod tidy
```

Esperado: `go.mod` passa a incluir `github.com/charmbracelet/huh v1.0.0` (ou versão mais recente).

- [ ] **Step 2: Verificar que o build ainda compila**

```bash
go build ./...
```

Esperado: sem erros.

- [ ] **Step 3: Adicionar `runSetup` ao `setup.go`**

Adicione ao final de `internal/cli/setup.go` (após `setupAndWrite`), substituindo os imports pelo bloco completo:

```go
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"syscall"

	"github.com/arijunior2020/tfrepo/internal/config"
	"github.com/arijunior2020/tfrepo/internal/core"
	"github.com/charmbracelet/huh"
	"gopkg.in/yaml.v3"
)

// WizardInput holds the answers collected by the setup wizard.
type WizardInput struct {
	SourceProvider  string
	SourceNamespace string
	SourceBaseURL   string
	TargetProvider  string
	TargetNamespace string
	TargetBaseURL   string
	SelectedRepos   []string
}

// buildConfigYAML converts wizard answers into transferepo.config.yaml content.
func buildConfigYAML(input WizardInput) ([]byte, error) {
	include := []string{"*"}
	if len(input.SelectedRepos) > 0 {
		sorted := make([]string, len(input.SelectedRepos))
		copy(sorted, input.SelectedRepos)
		sort.Strings(sorted)
		include = sorted
	}
	cfg := config.Config{
		Source: config.ProviderConfig{
			Provider:  input.SourceProvider,
			Namespace: input.SourceNamespace,
			BaseURL:   input.SourceBaseURL,
		},
		Target: config.ProviderConfig{
			Provider:  input.TargetProvider,
			Namespace: input.TargetNamespace,
			BaseURL:   input.TargetBaseURL,
		},
		Filters: config.Filters{
			Include: include,
			Exclude: []string{},
		},
		Mapping: map[string]string{},
	}
	return yaml.Marshal(cfg)
}

// setupAndWrite writes the config generated from input to configPath.
func setupAndWrite(input WizardInput, configPath string, stdout, stderr io.Writer) int {
	yml, err := buildConfigYAML(input)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := os.WriteFile(configPath, yml, 0o644); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "Wrote %s\n", configPath)
	return 0
}

// runSetup is the entry point for "tfrepo setup". It runs an interactive
// wizard that asks for source/target provider+namespace, scans the source
// for repositories, lets the user select which ones to migrate, and writes
// a transferepo.config.yaml file.
func runSetup(ctx context.Context, configPath string, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Step 1: source provider + namespace
	var sourceProvider, sourceNamespace, sourceBaseURL string
	sourceForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Provider de origem").
				Options(
					huh.NewOption("GitHub", "github"),
					huh.NewOption("GitLab", "gitlab"),
				).
				Value(&sourceProvider),
			huh.NewInput().
				Title("Namespace de origem").
				Description("Organização, usuário ou grupo").
				Validate(func(s string) error {
					if s == "" {
						return errors.New("namespace é obrigatório")
					}
					return nil
				}).
				Value(&sourceNamespace),
			huh.NewInput().
				Title("Base URL (opcional)").
				Description("Deixe vazio para usar a API pública (GitHub Enterprise / GitLab self-hosted)").
				Value(&sourceBaseURL),
		),
	)
	if err := sourceForm.Run(); err != nil {
		fmt.Fprintln(stderr, "Operação cancelada.")
		return 1
	}

	// Step 2: resolve token + construir provider
	sourceProv, err := NewProvider(config.ProviderConfig{
		Provider:  sourceProvider,
		Namespace: sourceNamespace,
		BaseURL:   sourceBaseURL,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	// Step 3: scan
	fmt.Fprintf(stdout, "Buscando repositórios em %s/%s...\n", sourceProvider, sourceNamespace)
	inventory, err := core.Scan(ctx, sourceProv, sourceNamespace, defaultScanConcurrency)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	// Coletar nomes de todos os repositórios encontrados
	var allRepos []string
	for _, ns := range inventory.Namespaces {
		for _, r := range ns.Repositories {
			allRepos = append(allRepos, r.Name)
		}
	}
	fmt.Fprintf(stdout, "Encontrados: %s\n", countLabel(len(allRepos), "repositório", "repositórios"))

	// Step 4: seleção de repositórios (pula se não há repos)
	var selectedRepos []string
	if len(allRepos) > 0 {
		repoOptions := make([]huh.Option[string], len(allRepos))
		for i, name := range allRepos {
			repoOptions[i] = huh.NewOption(name, name)
		}
		repoForm := huh.NewForm(
			huh.NewGroup(
				huh.NewMultiSelect[string]().
					Title("Repositórios para migrar").
					Description("Espaço = selecionar/desmarcar  ·  Enter = confirmar  ·  Nenhuma seleção = todos").
					Options(repoOptions...).
					Value(&selectedRepos),
			),
		)
		if err := repoForm.Run(); err != nil {
			fmt.Fprintln(stderr, "Operação cancelada.")
			return 1
		}
	}

	// Step 5: target provider + namespace
	var targetProvider, targetNamespace, targetBaseURL string
	targetForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Provider de destino").
				Options(
					huh.NewOption("GitHub", "github"),
					huh.NewOption("GitLab", "gitlab"),
				).
				Value(&targetProvider),
			huh.NewInput().
				Title("Namespace de destino").
				Description("Organização, usuário ou grupo").
				Validate(func(s string) error {
					if s == "" {
						return errors.New("namespace é obrigatório")
					}
					return nil
				}).
				Value(&targetNamespace),
			huh.NewInput().
				Title("Base URL (opcional)").
				Description("Deixe vazio para usar a API pública").
				Value(&targetBaseURL),
		),
	)
	if err := targetForm.Run(); err != nil {
		fmt.Fprintln(stderr, "Operação cancelada.")
		return 1
	}

	// Step 6: confirmação de sobrescrita se o arquivo já existe
	if _, statErr := os.Stat(configPath); statErr == nil {
		var overwrite bool
		confirmForm := huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title(fmt.Sprintf("%s já existe. Sobrescrever?", configPath)).
					Value(&overwrite),
			),
		)
		if err := confirmForm.Run(); err != nil || !overwrite {
			fmt.Fprintln(stderr, "Operação cancelada.")
			return 1
		}
	}

	// Step 7: gravar config
	input := WizardInput{
		SourceProvider:  sourceProvider,
		SourceNamespace: sourceNamespace,
		SourceBaseURL:   sourceBaseURL,
		TargetProvider:  targetProvider,
		TargetNamespace: targetNamespace,
		TargetBaseURL:   targetBaseURL,
		SelectedRepos:   selectedRepos,
	}
	return setupAndWrite(input, configPath, stdout, stderr)
}
```

- [ ] **Step 4: Verificar que o build compila sem erros**

```bash
cd /media/arimateia-junior/Dados4/projetos/tfrepo
go build ./...
```

Esperado: sem erros.

- [ ] **Step 5: Verificar que os testes existentes ainda passam**

```bash
go test ./... -race
```

Esperado: `ok` em todos os pacotes.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/setup.go go.mod go.sum
git commit -m "feat(cli): add runSetup interactive wizard using charmbracelet/huh"
```

---

### Task 3: Wiring no root + README

Registra o comando `setup` no Cobra e documenta no README.

**Files:**
- Modify: `internal/cli/root.go`
- Modify: `internal/cli/root_test.go`
- Modify: `README.md`

- [ ] **Step 1: Escrever o teste de registro do comando `setup`**

Adicione ao final de `internal/cli/root_test.go`:

```go
func TestNewRootCommandRegistersSetup(t *testing.T) {
	exitCode := 0
	root := newRootCommand(&exitCode)

	setupCmd, _, err := root.Find([]string{"setup"})
	if err != nil {
		t.Fatalf("Find(setup): %v", err)
	}
	if setupCmd.Use != "setup" {
		t.Fatalf("Find(setup).Use = %q, want %q", setupCmd.Use, "setup")
	}
}
```

- [ ] **Step 2: Executar o teste para confirmar que falha**

```bash
cd /media/arimateia-junior/Dados4/projetos/tfrepo
go test ./internal/cli/... -run TestNewRootCommandRegistersSetup -v
```

Esperado: FAIL (`setup` não encontrado).

- [ ] **Step 3: Adicionar `newSetupCommand` ao `root.go`**

Em `internal/cli/root.go`, adicione a função `newSetupCommand` antes da última chave do arquivo:

```go
func newSetupCommand(exitCode *int) *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Wizard interativo: escaneia a origem e gera transferepo.config.yaml",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := cmd.Flags().GetString(configFlagName)
			if err != nil {
				return err
			}
			*exitCode = runSetup(cmd.Context(), configPath, cmd.OutOrStdout(), cmd.ErrOrStderr())
			return nil
		},
	}
}
```

E em `newRootCommand`, após `root.AddCommand(newValidateCommand(exitCode))`, adicione:

```go
root.AddCommand(newSetupCommand(exitCode))
```

- [ ] **Step 4: Executar o teste para confirmar que passa**

```bash
go test ./internal/cli/... -run TestNewRootCommandRegistersSetup -v
```

Esperado: PASS.

- [ ] **Step 5: Suite completa**

```bash
go test ./... -race
```

Esperado: `ok` em todos os pacotes.

- [ ] **Step 6: Atualizar `README.md`**

Localize a seção `### \`tfrepo init\`` e adicione a seguinte seção **antes** dela:

```markdown
### `tfrepo setup` (recomendado para novos usuários)

Wizard interativo que guia a configuração completa da migração:

1. Pergunta provider e namespace de **origem**
2. Conecta na API e lista os repositórios disponíveis
3. Exibe os repositórios para seleção interativa
4. Pergunta provider e namespace de **destino**
5. Gera o `transferepo.config.yaml` pronto para uso

```
Flags:
  -c, --config string   arquivo de configuração a gerar (padrão: transferepo.config.yaml)
```

Requer `GITHUB_TOKEN` e/ou `GITLAB_TOKEN` definidos antes de rodar.

```

- [ ] **Step 7: Commit**

```bash
cd /media/arimateia-junior/Dados4/projetos/tfrepo
git add internal/cli/root.go internal/cli/root_test.go README.md
git commit -m "feat(cli): wire setup command into root and document in README"
```
