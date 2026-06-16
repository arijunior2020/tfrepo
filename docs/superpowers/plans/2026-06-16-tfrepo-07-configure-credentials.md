# Plan 7: `tfrepo configure` — Gerenciamento de Credenciais

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Adicionar `tfrepo configure` para armazenar tokens de provedores em `~/.tfrepo/credentials`, com fallback automático a partir de variáveis de ambiente, escalável para novos providers sem mudanças estruturais.

**Architecture:** Um novo pacote `internal/credentials` (sem deps internas) cuida de I/O e registra providers conhecidos. `internal/security/tokens.go` importa `credentials` e cai no arquivo quando a env var está vazia. O comando `configure` usa `huh` para coletar tokens interativamente, skippando providers que já têm env var definida.

**Tech Stack:** Go 1.24, `gopkg.in/yaml.v3` (já no go.mod), `charmbracelet/huh` v1.0.0, `cobra` v1.10.2.

---

## Mapa de Arquivos

| Arquivo | Ação | Responsabilidade |
|---|---|---|
| `internal/credentials/credentials.go` | Criar | Registry `KnownProviders`, structs, `Load`, `Save`, `DefaultPath`, método `Token` |
| `internal/credentials/credentials_test.go` | Criar | Testes de `Load`/`Save`/`Token`/`DefaultPath` |
| `internal/security/tokens.go` | Modificar | Adiciona `credentialsPathFn`, `EnvVarForProvider`, fallback no arquivo; atualiza `MissingTokenError` |
| `internal/security/tokens_test.go` | Modificar | Testes de fallback via arquivo; sobrescreve `credentialsPathFn` em todos os testes |
| `internal/cli/configure.go` | Criar | `newConfigureCommand` + `runConfigure` + `resolveProviderList` |
| `internal/cli/configure_test.go` | Criar | Testes sem TTY: provider inválido, env var set, registro no root |
| `internal/cli/root.go` | Modificar | `root.AddCommand(newConfigureCommand(exitCode))` |
| `README.md` | Modificar | Atualiza "Início rápido" passo 3; adiciona seção `tfrepo configure` |

---

## Task 1: Pacote `internal/credentials`

**Files:**
- Create: `internal/credentials/credentials.go`
- Create: `internal/credentials/credentials_test.go`

- [ ] **Step 1: Escrever os testes que falham**

Crie `internal/credentials/credentials_test.go`:

```go
package credentials_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/arijunior2020/tfrepo/internal/credentials"
)

func TestDefaultPath(t *testing.T) {
	got := credentials.DefaultPath()
	if !strings.HasSuffix(got, filepath.Join(".tfrepo", "credentials")) {
		t.Errorf("DefaultPath() = %q, want suffix %q", got, filepath.Join(".tfrepo", "credentials"))
	}
	if runtime.GOOS != "windows" && !strings.HasPrefix(got, "/") {
		t.Errorf("DefaultPath() = %q, want absolute path", got)
	}
}

func TestLoadNonExistentFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")

	got, err := credentials.Load(path)
	if err != nil {
		t.Fatalf("Load(non-existent) returned error: %v", err)
	}
	if got == nil {
		t.Fatal("Load(non-existent) returned nil")
	}
	if got.Token("github") != "" {
		t.Errorf("Token(github) = %q on empty creds, want empty", got.Token("github"))
	}
}

func TestLoadValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")

	yaml := "providers:\n  github:\n    token: ghp_abc123\n"
	if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := credentials.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Token("github") != "ghp_abc123" {
		t.Errorf("Token(github) = %q, want %q", got.Token("github"), "ghp_abc123")
	}
	if got.Token("gitlab") != "" {
		t.Errorf("Token(gitlab) = %q, want empty", got.Token("gitlab"))
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")

	if err := os.WriteFile(path, []byte(":::invalid:::"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := credentials.Load(path)
	if err == nil {
		t.Fatal("Load(invalid YAML) returned nil error, want error")
	}
}

func TestTokenTrimsWhitespace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")

	yaml := "providers:\n  github:\n    token: \"  ghp_trimmed  \"\n"
	if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := credentials.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Token("github") != "ghp_trimmed" {
		t.Errorf("Token(github) = %q, want %q (whitespace trimmed)", got.Token("github"), "ghp_trimmed")
	}
}

func TestSaveCreatesFileAndDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".tfrepo", "credentials")

	creds := &credentials.Credentials{
		Providers: map[string]credentials.ProviderCredential{
			"github": {Token: "ghp_saved"},
		},
	}

	if err := credentials.Save(path, creds); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(credentials): %v", err)
	}
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Errorf("credentials perm = %o, want 0600", perm)
		}
	}

	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("Stat(dir): %v", err)
	}
	if runtime.GOOS != "windows" {
		if perm := dirInfo.Mode().Perm(); perm != 0700 {
			t.Errorf("dir perm = %o, want 0700", perm)
		}
	}

	loaded, err := credentials.Load(path)
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if loaded.Token("github") != "ghp_saved" {
		t.Errorf("Token(github) after Save = %q, want %q", loaded.Token("github"), "ghp_saved")
	}
}

func TestSaveOverwritesExistingToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")

	first := &credentials.Credentials{
		Providers: map[string]credentials.ProviderCredential{
			"github": {Token: "ghp_old"},
		},
	}
	if err := credentials.Save(path, first); err != nil {
		t.Fatalf("Save(first): %v", err)
	}

	second := &credentials.Credentials{
		Providers: map[string]credentials.ProviderCredential{
			"github": {Token: "ghp_new"},
		},
	}
	if err := credentials.Save(path, second); err != nil {
		t.Fatalf("Save(second): %v", err)
	}

	loaded, err := credentials.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Token("github") != "ghp_new" {
		t.Errorf("Token(github) = %q, want %q", loaded.Token("github"), "ghp_new")
	}
}

func TestKnownProvidersContainsGithubAndGitlab(t *testing.T) {
	ids := make(map[string]bool)
	for _, p := range credentials.KnownProviders {
		ids[p.ID] = true
		if p.Label == "" {
			t.Errorf("KnownProvider %q has empty Label", p.ID)
		}
	}
	for _, want := range []string{"github", "gitlab"} {
		if !ids[want] {
			t.Errorf("KnownProviders missing %q", want)
		}
	}
}
```

- [ ] **Step 2: Verificar que os testes falham**

```bash
cd /media/arimateia-junior/Dados4/projetos/tfrepo
go test ./internal/credentials/... 2>&1 | head -5
```

Esperado: `cannot find package` ou `no Go files`.

- [ ] **Step 3: Criar `internal/credentials/credentials.go`**

```go
package credentials

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type ProviderCredential struct {
	Token string `yaml:"token"`
}

type Credentials struct {
	Providers map[string]ProviderCredential `yaml:"providers"`
}

type KnownProvider struct {
	ID    string
	Label string
}

var KnownProviders = []KnownProvider{
	{ID: "github", Label: "GitHub"},
	{ID: "gitlab", Label: "GitLab"},
}

func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".tfrepo", "credentials")
	}
	return filepath.Join(home, ".tfrepo", "credentials")
}

func Load(path string) (*Credentials, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Credentials{Providers: make(map[string]ProviderCredential)}, nil
	}
	if err != nil {
		return nil, err
	}
	var c Credentials
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	if c.Providers == nil {
		c.Providers = make(map[string]ProviderCredential)
	}
	return &c, nil
}

func Save(path string, c *Credentials) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func (c *Credentials) Token(provider string) string {
	if c == nil || c.Providers == nil {
		return ""
	}
	return strings.TrimSpace(c.Providers[provider].Token)
}
```

- [ ] **Step 4: Executar os testes**

```bash
go test ./internal/credentials/... -race -v 2>&1
```

Esperado: todos `PASS`.

- [ ] **Step 5: Commit**

```bash
git add internal/credentials/credentials.go internal/credentials/credentials_test.go
git commit -m "feat(credentials): add credentials package with Load, Save, KnownProviders"
```

---

## Task 2: Estender `internal/security/tokens.go` com fallback para arquivo

**Files:**
- Modify: `internal/security/tokens.go`
- Modify: `internal/security/tokens_test.go`

**Context:** `tokens.go` atualmente lê apenas env vars. `MissingTokenError` tem só o campo `EnvVar`. O objetivo é: (1) adicionar fallback para `~/.tfrepo/credentials`; (2) atualizar `MissingTokenError` com campo `Provider` e mensagem mais informativa; (3) exportar `EnvVarForProvider` para uso no comando configure.

- [ ] **Step 1: Atualizar `internal/security/tokens_test.go`**

Substitua o conteúdo completo do arquivo:

```go
package security

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func setNoCredentialsFile(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	old := credentialsPathFn
	credentialsPathFn = func() string { return filepath.Join(dir, "credentials") }
	t.Cleanup(func() { credentialsPathFn = old })
}

func TestResolveToken(t *testing.T) {
	setNoCredentialsFile(t)

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

func TestResolveTokenFromCredentialsFile(t *testing.T) {
	dir := t.TempDir()
	credPath := filepath.Join(dir, "credentials")

	credYAML := "providers:\n  github:\n    token: ghp_from_file\n"
	if err := os.WriteFile(credPath, []byte(credYAML), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	old := credentialsPathFn
	credentialsPathFn = func() string { return credPath }
	t.Cleanup(func() { credentialsPathFn = old })

	t.Setenv("GITHUB_TOKEN", "")

	got, err := ResolveToken("github")
	if err != nil {
		t.Fatalf("ResolveToken from file: %v", err)
	}
	if got != "ghp_from_file" {
		t.Errorf("ResolveToken = %q, want %q", got, "ghp_from_file")
	}
}

func TestResolveTokenEnvVarTakesPriorityOverFile(t *testing.T) {
	dir := t.TempDir()
	credPath := filepath.Join(dir, "credentials")

	credYAML := "providers:\n  github:\n    token: ghp_from_file\n"
	if err := os.WriteFile(credPath, []byte(credYAML), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	old := credentialsPathFn
	credentialsPathFn = func() string { return credPath }
	t.Cleanup(func() { credentialsPathFn = old })

	t.Setenv("GITHUB_TOKEN", "ghp_from_env")

	got, err := ResolveToken("github")
	if err != nil {
		t.Fatalf("ResolveToken: %v", err)
	}
	if got != "ghp_from_env" {
		t.Errorf("ResolveToken = %q, want %q (env var should take priority)", got, "ghp_from_env")
	}
}

func TestResolveTokenMissingTokenError(t *testing.T) {
	setNoCredentialsFile(t)
	t.Setenv("GITHUB_TOKEN", "")

	_, err := ResolveToken("github")

	var missingErr *MissingTokenError
	if !errors.As(err, &missingErr) {
		t.Fatalf("ResolveToken() error = %v, want *MissingTokenError", err)
	}

	if missingErr.EnvVar != "GITHUB_TOKEN" {
		t.Errorf("MissingTokenError.EnvVar = %q, want %q", missingErr.EnvVar, "GITHUB_TOKEN")
	}
	if missingErr.Provider != "github" {
		t.Errorf("MissingTokenError.Provider = %q, want %q", missingErr.Provider, "github")
	}
}

func TestEnvVarForProvider(t *testing.T) {
	tests := []struct {
		provider string
		want     string
	}{
		{"github", "GITHUB_TOKEN"},
		{"gitlab", "GITLAB_TOKEN"},
		{"unknown", ""},
	}
	for _, tt := range tests {
		got := EnvVarForProvider(tt.provider)
		if got != tt.want {
			t.Errorf("EnvVarForProvider(%q) = %q, want %q", tt.provider, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: Verificar que os novos testes falham**

```bash
go test ./internal/security/... -run TestResolveTokenFromCredentialsFile -v 2>&1
```

Esperado: `FAIL` (função/var ainda não existe).

- [ ] **Step 3: Atualizar `internal/security/tokens.go`**

Substitua o conteúdo completo do arquivo:

```go
package security

import (
	"fmt"
	"os"
	"strings"

	"github.com/arijunior2020/tfrepo/internal/credentials"
)

var envVarByProvider = map[string]string{
	"github": "GITHUB_TOKEN",
	"gitlab": "GITLAB_TOKEN",
}

var credentialsPathFn = credentials.DefaultPath

type MissingTokenError struct {
	Provider string
	EnvVar   string
}

func (e *MissingTokenError) Error() string {
	return fmt.Sprintf(
		"token para %q não encontrado (verificado %s e ~/.tfrepo/credentials) — execute: tfrepo configure",
		e.Provider, e.EnvVar,
	)
}

func EnvVarForProvider(provider string) string {
	return envVarByProvider[provider]
}

func ResolveToken(provider string) (string, error) {
	envVar, ok := envVarByProvider[provider]
	if !ok {
		return "", fmt.Errorf("unknown provider %q", provider)
	}

	if token := strings.TrimSpace(os.Getenv(envVar)); token != "" {
		return token, nil
	}

	creds, err := credentials.Load(credentialsPathFn())
	if err == nil {
		if token := creds.Token(provider); token != "" {
			return token, nil
		}
	}

	return "", &MissingTokenError{Provider: provider, EnvVar: envVar}
}
```

- [ ] **Step 4: Executar todos os testes do pacote security**

```bash
go test ./internal/security/... -race -v 2>&1
```

Esperado: todos `PASS`. Nota: `TestResolveTokenFromCredentialsFile` e `TestResolveTokenEnvVarTakesPriorityOverFile` devem aparecer como `PASS`.

- [ ] **Step 5: Verificar que o resto do projeto ainda compila**

```bash
go build ./... 2>&1
```

Esperado: sem erros.

- [ ] **Step 6: Commit**

```bash
git add internal/security/tokens.go internal/security/tokens_test.go
git commit -m "feat(security): extend ResolveToken with credentials file fallback"
```

---

## Task 3: Comando `tfrepo configure`

**Files:**
- Create: `internal/cli/configure.go`
- Create: `internal/cli/configure_test.go`

**Context:** O comando aceita argumento opcional `[provider]`. Sem argumento, configura todos os providers em `credentials.KnownProviders`. Com argumento, configura apenas aquele provider. Providers com env var definida são pulados com mensagem informativa. Usa `huh.EchoModePassword` para mascarar entrada do token.

- [ ] **Step 1: Escrever os testes que não exigem TTY**

Crie `internal/cli/configure_test.go`:

```go
package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunConfigureUnknownProvider(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runConfigure(context.Background(), "bitbucket", &stdout, &stderr)
	if code != 1 {
		t.Fatalf("runConfigure(bitbucket) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "bitbucket") {
		t.Errorf("stderr = %q, want provider name in error", stderr.String())
	}
	if !strings.Contains(stderr.String(), "github") {
		t.Errorf("stderr = %q, want available providers listed", stderr.String())
	}
}

func TestRunConfigureSkipsAllEnvVarProviders(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_test")
	t.Setenv("GITLAB_TOKEN", "glpat_test")

	var stdout, stderr bytes.Buffer
	// Sem TTY: todos os providers têm env var → nenhum form huh é exibido
	code := runConfigure(context.Background(), "", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runConfigure = %d, want 0 (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "GITHUB_TOKEN") {
		t.Errorf("stdout = %q, want skip message mentioning GITHUB_TOKEN", stdout.String())
	}
	if !strings.Contains(stdout.String(), "GITLAB_TOKEN") {
		t.Errorf("stdout = %q, want skip message mentioning GITLAB_TOKEN", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Nenhuma credencial alterada.") {
		t.Errorf("stdout = %q, want no-change message", stdout.String())
	}
}

func TestRunConfigureSkipsEnvVarProviderSingleArg(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_test")

	var stdout, stderr bytes.Buffer
	code := runConfigure(context.Background(), "github", &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runConfigure(github) = %d, want 0 (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "GITHUB_TOKEN") {
		t.Errorf("stdout = %q, want skip message mentioning GITHUB_TOKEN", stdout.String())
	}
}

func TestNewRootCommandRegistersConfigure(t *testing.T) {
	exitCode := 0
	root := newRootCommand(&exitCode)

	configureCmd, _, err := root.Find([]string{"configure"})
	if err != nil {
		t.Fatalf("Find(configure): %v", err)
	}
	if configureCmd.Use != "configure [provider]" {
		t.Fatalf("Find(configure).Use = %q, want %q", configureCmd.Use, "configure [provider]")
	}
}
```

- [ ] **Step 2: Verificar que os testes falham**

```bash
go test ./internal/cli/... -run TestRunConfigure -v 2>&1 | head -10
```

Esperado: `FAIL` (função `runConfigure` não existe).

- [ ] **Step 3: Criar `internal/cli/configure.go`**

```go
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/arijunior2020/tfrepo/internal/credentials"
	"github.com/arijunior2020/tfrepo/internal/security"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

func newConfigureCommand(exitCode *int) *cobra.Command {
	return &cobra.Command{
		Use:   "configure [provider]",
		Short: "Configura tokens de acesso para os provedores (salva em ~/.tfrepo/credentials)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var provider string
			if len(args) > 0 {
				provider = args[0]
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			*exitCode = runConfigure(ctx, provider, cmd.OutOrStdout(), cmd.ErrOrStderr())
			return nil
		},
	}
}

func runConfigure(ctx context.Context, provider string, stdout, stderr io.Writer) int {
	providerList, err := resolveProviderList(provider, stderr)
	if err != nil {
		return 1
	}

	credPath := credentials.DefaultPath()
	creds, err := credentials.Load(credPath)
	if err != nil {
		fmt.Fprintln(stderr, "erro ao ler credenciais:", err)
		return 1
	}

	changed := false
	for _, p := range providerList {
		envVar := security.EnvVarForProvider(p.ID)
		if strings.TrimSpace(os.Getenv(envVar)) != "" {
			fmt.Fprintf(stdout, "%s: já configurado via %s (pulando)\n", p.Label, envVar)
			continue
		}

		existingToken := creds.Token(p.ID)

		desc := "Não configurado."
		if existingToken != "" {
			desc = "Já configurado no arquivo. Deixe em branco para manter o token atual."
		}

		var newToken string
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title(fmt.Sprintf("Token de acesso do %s", p.Label)).
					Description(desc).
					EchoMode(huh.EchoModePassword).
					Value(&newToken).
					Validate(func(s string) error {
						if strings.TrimSpace(s) == "" && existingToken == "" {
							return errors.New("token obrigatório")
						}
						return nil
					}),
			),
		)

		if err := form.RunWithContext(ctx); err != nil {
			if errors.Is(err, huh.ErrUserAborted) || ctx.Err() != nil {
				return 0
			}
			fmt.Fprintln(stderr, err)
			return 1
		}

		if trimmed := strings.TrimSpace(newToken); trimmed != "" {
			creds.Providers[p.ID] = credentials.ProviderCredential{Token: trimmed}
			changed = true
		}
	}

	if !changed {
		fmt.Fprintln(stdout, "Nenhuma credencial alterada.")
		return 0
	}

	if err := credentials.Save(credPath, creds); err != nil {
		fmt.Fprintln(stderr, "erro ao salvar credenciais:", err)
		return 1
	}
	fmt.Fprintf(stdout, "Credenciais salvas em %s\n", credPath)
	return 0
}

func resolveProviderList(provider string, stderr io.Writer) ([]credentials.KnownProvider, error) {
	if provider == "" {
		return credentials.KnownProviders, nil
	}
	for _, p := range credentials.KnownProviders {
		if p.ID == provider {
			return []credentials.KnownProvider{p}, nil
		}
	}
	ids := make([]string, len(credentials.KnownProviders))
	for i, p := range credentials.KnownProviders {
		ids[i] = p.ID
	}
	fmt.Fprintf(stderr, "provider %q desconhecido. Disponíveis: %s\n", provider, strings.Join(ids, ", "))
	return nil, fmt.Errorf("unknown provider %q", provider)
}
```

- [ ] **Step 4: Executar os testes do configure**

```bash
go test ./internal/cli/... -run "TestRunConfigure|TestNewRootCommandRegistersConfigure" -race -v 2>&1
```

Esperado: `TestNewRootCommandRegistersConfigure` falha (ainda não registrado em root.go). `TestRunConfigure*` devem passar.

- [ ] **Step 5: Commit (só configure.go e configure_test.go)**

```bash
git add internal/cli/configure.go internal/cli/configure_test.go
git commit -m "feat(cli): add configure command for credential management"
```

---

## Task 4: Registrar o comando e atualizar README

**Files:**
- Modify: `internal/cli/root.go:83` (linha da última `AddCommand`)
- Modify: `README.md`

- [ ] **Step 1: Registrar o comando em `internal/cli/root.go`**

No bloco de `AddCommand` em `newRootCommand` (após `newDestroyCommand`), adicionar:

```go
root.AddCommand(newConfigureCommand(exitCode))
```

O bloco final de AddCommand deve ficar assim:

```go
root.AddCommand(newInitCommand(exitCode))
root.AddCommand(newScanCommand(exitCode))
root.AddCommand(newPlanCommand(exitCode))
root.AddCommand(newMigrateCommand(exitCode))
root.AddCommand(newValidateCommand(exitCode))
root.AddCommand(newSetupCommand(exitCode))
root.AddCommand(newUpdateCommand(exitCode))
root.AddCommand(newDestroyCommand(exitCode))
root.AddCommand(newConfigureCommand(exitCode))
```

- [ ] **Step 2: Verificar que todos os testes passam**

```bash
go test ./... -race 2>&1
```

Esperado: `ok` em todos os pacotes. `TestNewRootCommandRegistersConfigure` agora deve passar.

- [ ] **Step 3: Atualizar README.md — "Início rápido" passo 3**

Localizar e substituir o bloco:

```markdown
**3. Configure os tokens via variáveis de ambiente:**

```bash
export GITHUB_TOKEN=ghp_...
export GITLAB_TOKEN=glpat-...
```
```

Substituir por:

```markdown
**3. Configure os tokens de acesso:**

```bash
tfrepo configure
```

Ou via variáveis de ambiente (têm prioridade sobre o arquivo de credenciais):

```bash
export GITHUB_TOKEN=ghp_...
export GITLAB_TOKEN=glpat-...
```
```

- [ ] **Step 4: Atualizar README.md — adicionar seção `tfrepo configure`**

Localizar a linha que começa com `### \`tfrepo setup\`` e inserir a seção abaixo **antes** dela:

```markdown
### `tfrepo configure [provider]`

Wizard interativo para configurar tokens de acesso pessoal. Salva os tokens em
`~/.tfrepo/credentials` com permissões `0600`. Os tokens **nunca** são gravados
no `transferepo.config.yaml` nem em logs.

```
Argumentos opcionais:
  provider   provider a configurar: "github" ou "gitlab"
             (sem argumento: configura todos os providers)
```

**Prioridade de resolução de token:**
1. Variável de ambiente (`GITHUB_TOKEN` / `GITLAB_TOKEN`) — sempre tem prioridade
2. Arquivo `~/.tfrepo/credentials`
3. Erro com instrução para executar `tfrepo configure`

**Exemplos:**

```bash
tfrepo configure           # configura GitHub e GitLab
tfrepo configure github    # configura apenas GitHub
tfrepo configure gitlab    # configura apenas GitLab
```

```

Also, update the note on `tfrepo setup` that says "Requer `GITHUB_TOKEN` e/ou `GITLAB_TOKEN` definidos antes de rodar" to:

```markdown
Requer tokens configurados via `tfrepo configure` ou variáveis de ambiente
`GITHUB_TOKEN` / `GITLAB_TOKEN` antes de rodar.
```

- [ ] **Step 5: Verificar build e testes finais**

```bash
go build ./... && go test ./... -race 2>&1
```

Esperado: sem erros, todos `PASS`.

- [ ] **Step 6: Commit final**

```bash
git add internal/cli/root.go README.md
git commit -m "feat(cli): register configure command and update README"
```

---

## Self-Review

### 1. Spec coverage

| Requisito | Task |
|---|---|
| `tfrepo configure` interativo com huh | Task 3 |
| `tfrepo configure github` (provider opcional) | Task 3 |
| Prioridade: env var → arquivo → erro | Task 2 |
| Arquivo `~/.tfrepo/credentials` YAML | Task 1 |
| Dir 0700, arquivo 0600 | Task 1 (Save) |
| Mensagem de erro aponta para `tfrepo configure` | Task 2 (MissingTokenError) |
| Escalável: adicionar provider = uma linha | Task 1 (KnownProviders) + Task 2 (envVarByProvider) |
| README atualizado | Task 4 |
| Sem tokens em `.yaml` de config ou logs | Task 3 (EchoModePassword + Save) |

### 2. Placeholder scan

Sem placeholders. Todo código está completo e testável.

### 3. Consistência de tipos

- `credentials.KnownProvider`, `credentials.ProviderCredential`, `credentials.Credentials` — definidos em Task 1, usados corretamente em Task 3.
- `security.EnvVarForProvider` — definido em Task 2, usado em Task 3.
- `credentialsPathFn` (não exportada) — definida em Task 2, sobrescrita nos testes de Task 2.
- `resolveProviderList(provider string, stderr io.Writer) ([]credentials.KnownProvider, error)` — definida e usada em Task 3.
- `runConfigure(ctx context.Context, provider string, stdout, stderr io.Writer) int` — consistente com o padrão `runXxx` do projeto.
