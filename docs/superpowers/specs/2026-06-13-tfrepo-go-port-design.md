# tfrepo — Port para Go do TransfeRepo — Design

## Contexto

TransfeRepo (este repositório, Node/TypeScript) já tem a v0.1.0 completa,
validada ponta a ponta contra contas reais do GitHub e do GitLab
(`init` → `scan` → `plan` → `migrate` → `validate`, migração de repositório
privado com histórico completo). Este documento descreve **tfrepo**, um novo
projeto/repositório (`github.com/arijunior2020/tfrepo`, privado) que
reimplementa o **mesmo objetivo** em Go, com paridade funcional completa em
relação à v0.1.0 do TransfeRepo, sem reduzir escopo para um MVP.

Os dois projetos são independentes: nenhum código é compartilhado, e este
repositório (`transferepo`) não é alterado por este trabalho.

## Objetivo

Paridade funcional completa com a v0.1.0 do TransfeRepo:

- Mesmos comandos: `init`, `scan`, `plan`, `migrate` (com `--dry-run`),
  `validate`.
- Mesmos artefatos JSON (`inventory.json`, `migration-plan.json`,
  `migration-report.json`, `validation-report.json`), campo a campo
  compatíveis.
- Mesmo arquivo de configuração `transferepo.config.yaml` (mesmo formato
  YAML).
- Mesmos princípios de segurança: tokens só via env var
  (`GITHUB_TOKEN`/`GITLAB_TOKEN`), nunca em config/logs/artefatos;
  `Redact()` mascara qualquer valor de token.
- **Correção incluída** (não presente no Node): gap conhecido do
  `GitLabProvider` em que `listRepositories` retornava `[]` silenciosamente
  para um namespace pessoal do GitLab que não é o dono do token (ver seção
  "internal/provider" e memória `project_providers_gitlab_namespace_gap`).
- **Diferencial incluído** (não presente no Node): `scan` e `migrate`
  processam repositórios em paralelo, com limite configurável
  (`--concurrency`, padrão `4`).

### Motivação (registro para referência futura)

- Performance: concorrência real em `scan`/`migrate`, startup instantâneo
  (sem runtime Node).
- Distribuição: binário único por plataforma via GoReleaser — sem exigir
  Node/pnpm instalados.
- Expectativa da comunidade open source de ferramentas devops/CLI: Go é o
  padrão de fato (`terraform`, `kubectl`, `gh`, `docker`, `hugo`).

### Fora de escopo

- Qualquer funcionalidade que também esteja fora de escopo na v0.1.0 do
  TransfeRepo: Issues/PRs/MRs/Wiki/Releases/Webhooks/Pipelines/Secrets,
  Bitbucket/Azure DevOps, interface web/API, OAuth (autenticação é só PAT via
  env var).
- Importar/migrar configs ou estado de instalações existentes do
  `transferepo` (Node) — `tfrepo` lê o mesmo formato de
  `transferepo.config.yaml`, mas não há ferramenta de migração de instalação.
- Suporte a subgrupos do GitLab com navegação recursiva (mesma limitação da
  v0.1.0: `namespace` é tratado como string opaca, incluindo paths tipo
  `group/subgroup`).

## Arquitetura — módulo Go único

```
tfrepo/
├── cmd/
│   └── tfrepo/
│       └── main.go            # entrypoint; monta o comando raiz (Cobra) e executa
├── internal/
│   ├── cli/                    # comandos Cobra
│   │   ├── root.go             # comando raiz, flags globais, banner
│   │   ├── init.go             # tfrepo init
│   │   ├── scan.go             # tfrepo scan
│   │   ├── plan.go             # tfrepo plan
│   │   ├── migrate.go          # tfrepo migrate [--dry-run] [--concurrency]
│   │   ├── validate.go         # tfrepo validate
│   │   └── banner.go           # ASCII art (paridade com commit 4bdbca7 do Node)
│   ├── config/
│   │   ├── config.go           # structs + Load() + Validate()
│   │   └── config_test.go
│   ├── provider/
│   │   ├── types.go            # Namespace, RepositorySummary, RepositoryDetails,
│   │   │                        # RepositoryState, CreateRepositoryInput, RepositoryProvider
│   │   ├── github.go            # GitHubProvider (google/go-github)
│   │   ├── github_test.go
│   │   ├── gitlab.go            # GitLabProvider (gitlab.com/gitlab-org/api/client-go)
│   │   └── gitlab_test.go
│   ├── core/
│   │   ├── artifacts.go         # tipos + leitura/escrita JSON dos 4 artefatos
│   │   ├── scan.go               # Scan()
│   │   ├── plan.go                # Plan()
│   │   ├── migrate.go             # Migrate() — worker pool com --concurrency
│   │   ├── validate.go            # Validate()
│   │   ├── glob.go                 # aplica filters.include/exclude (gobwas/glob)
│   │   └── *_test.go
│   └── security/
│       ├── tokens.go            # resolve GITHUB_TOKEN/GITLAB_TOKEN de env vars
│       ├── redact.go             # Redact(string) string
│       ├── workspace.go           # dir temporário + cleanup (defer + SIGINT/SIGTERM)
│       ├── gitmirror.go            # clone/push --mirror via os/exec
│       └── *_test.go
├── docs/
│   └── superpowers/
│       ├── specs/               # mesma convenção do transferepo
│       └── plans/
├── .github/
│   └── workflows/
│       ├── ci.yml               # go vet + golangci-lint + go test -race
│       └── release.yml          # GoReleaser em tag v*
├── .goreleaser.yaml
├── go.mod
├── go.sum
├── LICENSE                       # MIT, igual ao transferepo
└── README.md
```

Módulo: `github.com/arijunior2020/tfrepo`. Binário: `tfrepo`. As 4 fases do
roadmap original (security → providers → core → cli) continuam existindo,
mas como **planos de implementação** dentro de uma única spec/módulo, não
como módulos Go separados (ver "Plano de execução" abaixo).

## internal/provider — Provider Interface

```go
package provider

type NamespaceKind string

const (
    NamespaceOrganization NamespaceKind = "organization"
    NamespaceUser         NamespaceKind = "user"
    NamespaceGroup        NamespaceKind = "group"
)

type Visibility string

const (
    VisibilityPublic   Visibility = "public"
    VisibilityPrivate  Visibility = "private"
    VisibilityInternal Visibility = "internal"
)

type Namespace struct {
    Slug string        `json:"slug"` // login da org (GitHub) ou path do group (GitLab)
    Name string        `json:"name"`
    Kind NamespaceKind `json:"kind"`
}

type RepositorySummary struct {
    Name          string     `json:"name"`
    Namespace     string     `json:"namespace"`
    DefaultBranch string     `json:"defaultBranch"`
    Visibility    Visibility `json:"visibility"`
    SizeKB        int64      `json:"sizeKb"`
}

type RepositoryDetails struct {
    RepositorySummary
    Branches []string `json:"branches"`
    Tags     []string `json:"tags"`
}

type RepositoryState struct {
    Branches map[string]string `json:"branches"` // branch -> commit SHA
    Tags     map[string]string `json:"tags"`     // tag -> commit SHA
}

type CreateRepositoryInput struct {
    Name        string
    Visibility  Visibility
    Description string
}

type RepositoryProvider interface {
    Name() string // "github" | "gitlab"

    // Scan
    ListNamespaces(ctx context.Context) ([]Namespace, error)
    ListRepositories(ctx context.Context, namespace string) ([]RepositorySummary, error)
    GetRepositoryDetails(ctx context.Context, namespace, repo string) (RepositoryDetails, error)

    // Migrate (lado destino)
    CreateRepository(ctx context.Context, namespace string, input CreateRepositoryInput) (RepositorySummary, error)

    // Transporte Git — URL com token embutido, usada só em memória, nunca logada
    GetAuthenticatedCloneURL(namespace, repo string) string

    // Validate
    GetRepositoryState(ctx context.Context, namespace, repo string) (RepositoryState, error)
}
```

### GitHubProvider (`google/go-github`)

Replica `packages/providers/src/github-provider.ts`, incluindo a correção do
commit `d515343`: em `ListRepositories`, resolve a conta via
`Users.Get(ctx, namespace)`:

- `type == "Organization"` → `Repositories.ListByOrg`.
- `namespace` igual ao login do usuário autenticado (`Users.Get(ctx, "")`) →
  `Repositories.List(ctx, "", &RepositoryListOptions{Affiliation: "owner"})`
  (equivalente a `GET /user/repos`, inclui privados).
- Caso contrário → `Repositories.ListByUser(ctx, namespace, ...)` (só
  públicos).

`GetAuthenticatedCloneURL` monta
`https://x-access-token:<token>@<host>/<namespace>/<repo>.git`, resolvendo
`<host>` a partir de `baseUrl` (GitHub Enterprise) ou `github.com` (default).

### GitLabProvider (`gitlab.com/gitlab-org/api/client-go`)

Replica `packages/providers/src/gitlab-provider.ts`, **com a correção do gap
de namespace incluída desde o início** (não presente no Node):

`ListRepositories(ctx, namespace)`:

1. Resolve o namespace via `Namespaces.GetNamespace(namespace)` → obtém
   `Kind` (`"group"` ou `"user"`).
2. `Kind == "group"` → `Groups.ListGroupProjects(namespace, ...)`, paginado.
   Lista tudo que o token pode ver no grupo, sem depender do usuário ser
   "Owner" (mais robusto que o `owned: true` + filtro do Node).
3. `Kind == "user"`:
   - Se `namespace` é o path do usuário autenticado (`Users.CurrentUser()`)
     → `Projects.ListProjects(&ListProjectsOptions{Owned: gitlab.Bool(true)})`
     (inclui privados).
   - Caso contrário → `Projects.ListUserProjects(userID, ...)` (projetos
     visíveis publicamente daquele usuário — equivalente ao `ListByUser` do
     GitHub).

Essa estrutura de 3 ramos (group / próprio user / outro user) espelha
exatamente o `GitHubProvider.ListRepositories` acima — mesma forma, API
diferente.

`GetAuthenticatedCloneURL` monta
`https://oauth2:<token>@<host>/<namespace>/<repo>.git`.

## internal/core — pipeline de comandos

Cada comando do CLI chama uma função correspondente em `internal/core`, que
recebe um `provider.RepositoryProvider` (injetado pelo CLI a partir da
config) e lê/grava o artefato JSON correspondente no diretório de trabalho
atual (cwd) — mesma convenção do Node, permitindo auditoria e versionamento.

### Artefatos JSON (`internal/core/artifacts.go`)

```go
type Inventory struct {
    GeneratedAt time.Time           `json:"generatedAt"`
    Source      ProviderRef         `json:"source"`
    Namespaces  []InventoryNamespace `json:"namespaces"`
}

type ProviderRef struct {
    Provider  string `json:"provider"`
    Namespace string `json:"namespace"`
}

type InventoryNamespace struct {
    Slug         string                       `json:"slug"`
    Name         string                       `json:"name"`
    Kind         provider.NamespaceKind       `json:"kind"`
    Repositories []provider.RepositoryDetails `json:"repositories"`
}

type MigrationPlan struct {
    GeneratedAt time.Time      `json:"generatedAt"`
    Source      ProviderRef    `json:"source"`
    Target      ProviderRef    `json:"target"`
    Tasks       []MigrationTask `json:"tasks"`
}

type MigrationTask struct {
    ID       string       `json:"id"`
    Source   TaskEndpoint `json:"source"`
    Target   TaskEndpoint `json:"target"`
    Branches []string     `json:"branches"`
    Tags     []string     `json:"tags"`
}

type TaskEndpoint struct {
    Namespace string `json:"namespace"`
    Repo      string `json:"repo"`
}

type MigrationReport struct {
    GeneratedAt time.Time          `json:"generatedAt"`
    Results     []MigrationResult `json:"results"`
}

type MigrationResult struct {
    ID         string     `json:"id"`
    Status     string     `json:"status"` // "success" | "failed" | "dry-run"
    StartedAt  *time.Time `json:"startedAt,omitempty"`
    FinishedAt *time.Time `json:"finishedAt,omitempty"`
    Error      string     `json:"error,omitempty"` // sempre passado por Redact()
}

type ValidationReport struct {
    GeneratedAt time.Time           `json:"generatedAt"`
    Results     []ValidationResult `json:"results"`
}

type ValidationResult struct {
    ID          string       `json:"id"`
    Status      string       `json:"status"` // "ok" | "diverged" | "skipped"
    Divergences []Divergence `json:"divergences"`
}

type Divergence struct {
    Type      string `json:"type"` // "branch" | "tag"
    Name      string `json:"name"`
    SourceSHA string `json:"sourceSha,omitempty"`
    TargetSHA string `json:"targetSha,omitempty"`
}
```

### `tfrepo scan`

- Lê `transferepo.config.yaml` (seção `source`), instancia o provider via
  `internal/security` (token resolvido por env var).
- `ListNamespaces()` (filtrado pelo `namespace` configurado) →
  `ListRepositories()` → `GetRepositoryDetails()` para cada repo.
- Com `--concurrency N` (padrão `4`): `GetRepositoryDetails` para os
  repositórios de um namespace roda em um worker pool de N goroutines
  (`golang.org/x/sync/errgroup` + semáforo), preservando a ordem do
  resultado final (resultados coletados em slice pré-alocada por índice, não
  por ordem de chegada).
- Grava `inventory.json`.

### `tfrepo plan`

- Lê `inventory.json` + `transferepo.config.yaml` (`target`, `filters`,
  `mapping`).
- Aplica `filters.include`/`filters.exclude` (glob patterns via
  `gobwas/glob`, compilados uma vez e reutilizados).
- Aplica `mapping` para renomear repositórios no destino (default: mesmo
  nome).
- Grava `migration-plan.json`.
- Imprime resumo no terminal (quantidade de repos, branches, tags totais —
  com pluralização correta de "branch(es)"/"tag(s)").

### `tfrepo migrate [--dry-run] [--concurrency N]`

- Lê `migration-plan.json`.
- Para cada task, em um worker pool de até `--concurrency` goroutines
  (padrão `4`):
  1. `security.NewWorkspace()` → diretório temporário isolado
     (`os.MkdirTemp`, permissão `0700`), um por task.
  2. `git clone --mirror <url-autenticada-origem> <workspace>/repo.git` via
     `internal/security/gitmirror.go` (`os/exec`).
  3. Se o repo não existir no destino: `provider.CreateRepository(...)`.
  4. `git push --mirror <url-autenticada-destino>` a partir do workspace.
  5. `workspace.Cleanup()` via `defer`, executado mesmo em erro.
- `--dry-run`: valida pré-condições (auth, namespace destino existe via
  `ListRepositories`/`GetRepositoryDetails`) sem clonar/empurrar nada;
  resultado gravado com `status: "dry-run"`.
- Grava `migration-report.json`. Qualquer `error` passa por
  `security.Redact()` antes de ser serializado.
- `SIGINT`/`SIGTERM` durante a execução: cancela o `context.Context` raiz
  (interrompendo o worker pool), aguarda os workspaces em andamento serem
  limpos antes de encerrar.

### `tfrepo validate`

- Lê `migration-plan.json` (+ `migration-report.json` se existir, para pular
  tasks com `status: "failed"`, marcando-as `"skipped"`).
- Para cada task: `GetRepositoryState()` no source e no target, compara
  `Branches`/`Tags` (nome + SHA).
- Grava `validation-report.json` com divergências encontradas.
- Exit code `0` se tudo `"ok"`/`"skipped"`, `1` se houver qualquer
  `"diverged"` (uso em CI) — mesma semântica da v0.1.0.

## Config file — `transferepo.config.yaml`

Mesmo formato YAML da v0.1.0, carregado com `gopkg.in/yaml.v3`:

```go
type Config struct {
    Source  ProviderConfig    `yaml:"source"`
    Target  ProviderConfig    `yaml:"target"`
    Filters Filters           `yaml:"filters"`
    Mapping map[string]string `yaml:"mapping"`
}

type ProviderConfig struct {
    Provider  string `yaml:"provider"`          // "github" | "gitlab"
    BaseURL   string `yaml:"baseUrl,omitempty"` // GitHub Enterprise / GitLab self-managed
    Namespace string `yaml:"namespace"`
}

type Filters struct {
    Include []string `yaml:"include"` // default: []string{"*"}
    Exclude []string `yaml:"exclude"` // default: []string{}
}
```

`(*Config) Validate() error` replica exatamente `TransferepoConfigSchema`
(`packages/core/src/config.ts`): `Provider` ∈ {`github`, `gitlab`} em
`source` e `target`, `Namespace` não vazio em ambos, `BaseURL` (se presente)
é uma URL válida. Após `yaml.Unmarshal`, aplica os mesmos defaults do zod:
`Filters.Include = ["*"]` e `Filters.Exclude = []` se `filters` ausente,
`Mapping = map[string]string{}` se `mapping` ausente. Sem validação
cruzada entre `source`/`target` — mesmo comportamento do Node. Mensagens de
erro apontam o campo inválido.

Tokens **nunca** vão no YAML — apenas `GITHUB_TOKEN`/`GITLAB_TOKEN` (env
vars), resolvidos por `internal/security` de acordo com o `provider`
configurado em `source`/`target`.

## Engine de migração de Git

Igual ao Node: **mirror clone via binário `git` do sistema**, chamado com
`os/exec` (não usa biblioteca go-git):

- `git clone --mirror <url-autenticada-origem> <workspace>` — clona todas as
  refs (branches, tags, notes), garantindo histórico completo.
- `git push --mirror <url-autenticada-destino>` — empurra todas as refs para
  o destino.

A URL autenticada é construída em memória
(`https://x-access-token:<token>@host/...` para GitHub,
`https://oauth2:<token>@host/...` para GitLab), passada ao processo `git`
via argumento (nunca via env var herdada por subprocessos, nunca escrita em
disco/config do git). stdout/stderr do `git` são capturados e passados por
`security.Redact()` antes de qualquer log.

## internal/security

- **`tokens.go`**: `ResolveToken(providerName string) (string, error)` — lê
  `GITHUB_TOKEN`/`GITLAB_TOKEN` exclusivamente de variáveis de ambiente; erro
  claro se ausente.
- **`redact.go`**: `Redact(s string) string` — substitui valores de token
  conhecidos e padrões de URL autenticada (`https://[^@]+@`) por `***`.
  Usado em qualquer string antes de log, erro de artefato JSON, ou saída de
  `git`.
- **`workspace.go`**: `NewWorkspace() (*Workspace, error)` cria
  `os.MkdirTemp("", "tfrepo-*")` com permissão `0700`; `(*Workspace) Cleanup()`
  remove recursivamente. Um `signal.Notify(ch, SIGINT, SIGTERM)` no
  `cmd/tfrepo/main.go` cancela o `context.Context` raiz, e cada goroutine do
  worker pool garante `Cleanup()` via `defer` mesmo em cancelamento.
- **`gitmirror.go`**: `Clone(ctx, authenticatedURL, dir string) error` e
  `Push(ctx, dir, authenticatedURL string) error`, via
  `exec.CommandContext(ctx, "git", ...)`.
- Nenhuma chamada de rede fora dos providers configurados — sem telemetria,
  sem upload de conteúdo a terceiros (mesmo princípio do Node).

## internal/cli — comandos (Cobra)

Mesmo conjunto de comandos da v0.1.0:

- `tfrepo` (sem args) → banner ASCII (paridade com commit `4bdbca7` do Node).
- `tfrepo init` → gera `transferepo.config.yaml` (paridade com `6bd1fb6`).
- `tfrepo scan [--concurrency N]`
- `tfrepo plan`
- `tfrepo migrate [--dry-run] [--concurrency N]`
- `tfrepo validate`

Flags globais: `--config <path>` (default `./transferepo.config.yaml`).

Códigos de saída seguem a seção 6 do README do `transferepo` (sucesso = `0`;
`validate` com divergência = `1`; erros de config/execução com os mesmos
códigos específicos já documentados) — na hora de escrever o plano de
implementação, o `README.md` atual do `transferepo` é a fonte da verdade para
os valores exatos.

## Stack técnica

- Go 1.23+
- Cobra (CLI)
- `google/go-github` (GitHub)
- `gitlab.com/gitlab-org/api/client-go` (GitLab, sucessor oficial do
  `xanzy/go-gitlab`)
- `gopkg.in/yaml.v3` (config)
- `gobwas/glob` (filtros, equivalente ao `minimatch`)
- `golang.org/x/sync/errgroup` (worker pools com `--concurrency`)
- `go test` + `-race` (testes)
- `golangci-lint`
- GoReleaser + GitHub Actions (CI + release)
- Licença: MIT

## Estratégia de testes

- **`internal/core`**: testes de tabela com fakes de `RepositoryProvider`
  (sem rede) — cobre geração de `inventory.json`, `migration-plan.json`
  (filtros/mapping), `migration-report.json`, `validation-report.json` e
  suas comparações, incluindo cenários de divergência, `--dry-run` e
  `--concurrency > 1`.
- **`internal/provider`**: testes com `httptest.NewServer` simulando
  respostas REST do GitHub/GitLab — cobre os 3 ramos de resolução de
  namespace em ambos os providers (organization/group, próprio user, outro
  user), incluindo o caso do gap corrigido do GitLab.
- **`internal/security`**: testes de `Redact`, `ResolveToken` (via env vars
  setadas/não setadas no teste), `Workspace` (criação/cleanup, inclusive em
  cenário de erro e de cancelamento de contexto).
- **`internal/cli`**: execução de comandos Cobra com buffers de
  stdout/stderr, verificando exit codes e mensagens.
- Todos os testes rodam com `go test ./... -race` — importante porque
  `scan`/`migrate` agora têm worker pools.
- **E2E opcional**: pipeline `scan → plan → migrate → validate` contra
  repositórios de teste reais, gated por env var dedicada — não roda no CI
  padrão (exige tokens), mesma convenção do Node.

## Distribuição / CI

- `.github/workflows/ci.yml` — em push/PR: `go vet`, `golangci-lint`,
  `go test ./... -race`.
- `.github/workflows/release.yml` — em tag `v*`: roda GoReleaser.
- `.goreleaser.yaml` — builds cross-platform (`linux`/`darwin`/`windows`,
  `amd64`/`arm64`), publicados como GitHub Release.
- `README.md` com instruções de instalação: binário da Release, ou
  `go install github.com/arijunior2020/tfrepo/cmd/tfrepo@latest`.

## Plano de execução

Mesma decomposição em 4 planos sequenciais da v0.1.0 do Node, adaptada à
estrutura de módulo único:

1. **Scaffold + `internal/security`** — `go.mod`, layout de diretórios,
   `tokens.go`, `redact.go`, `workspace.go`, `gitmirror.go` (sem uso ainda),
   CI básico (`go vet`, `go test -race`).
2. **`internal/provider`** — `types.go`, `RepositoryProvider`,
   `GitHubProvider`, `GitLabProvider` (incluindo a correção do gap de
   namespace desde o início).
3. **`internal/core`** — `config`, `artifacts.go`, `scan`/`plan`/`migrate`/
   `validate`, glob filters, worker pools com `--concurrency`.
4. **`internal/cli`** — comandos Cobra (`init`/`scan`/`plan`/`migrate`/
   `validate`), banner, `README.md`, `.goreleaser.yaml`,
   `.github/workflows/release.yml`.

Cada plano produz software testável de forma independente; o Plano N+1
depende do Plano N estar mergeado em `main`.

## Critérios de aceite

- `tfrepo scan` gera `inventory.json` estruturalmente compatível com o do
  Node, incluindo: repositórios privados do namespace próprio no GitHub, e
  repositórios de um namespace pessoal de **terceiros** no GitLab (gap
  corrigido).
- `tfrepo plan` aplica `filters`/`mapping` e gera `migration-plan.json`
  campo a campo compatível com o do Node.
- `tfrepo migrate` migra repositório de teste (branches, tags, histórico
  completo) GitHub → GitLab e GitLab → GitHub, com `--concurrency` > 1
  processando múltiplos repositórios em paralelo sem condições de corrida
  (`go test -race` limpo, incluindo testes do worker pool).
- `tfrepo validate` detecta repositórios migrados com sucesso (sem
  divergências) e repositórios com divergência proposital (branch faltante,
  SHA diferente), com a mesma semântica de exit code da v0.1.0.
- Nenhum token aparece em log, artefato JSON ou mensagem de erro.
- Workspaces temporários são removidos após cada execução, mesmo em caso de
  erro ou interrupção (Ctrl+C), inclusive com tasks em paralelo.
- Binários `tfrepo` para `linux`/`darwin`/`windows` (`amd64`/`arm64`) são
  publicados em GitHub Release ao criar uma tag `vX.Y.Z`.
