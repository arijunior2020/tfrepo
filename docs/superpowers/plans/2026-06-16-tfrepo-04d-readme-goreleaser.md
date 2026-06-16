# tfrepo Plano 4d — README, GoReleaser e release workflow

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Completar a distribuição do tfrepo: `.goreleaser.yaml` cross-platform, workflow de release em tag `v*`, golangci-lint no CI e README completo com instruções de instalação e uso.

**Architecture:** Três arquivos independentes — configuração do GoReleaser v2 (builds para linux/darwin/windows × amd64/arm64), workflow do GitHub Actions para release, e README expandido. O CI existente (`.github/workflows/ci.yml`) recebe apenas uma linha extra de golangci-lint-action.

**Tech Stack:** Go 1.24, GoReleaser v2, GitHub Actions, golangci-lint

---

## Estrutura de arquivos

| Arquivo | Ação | Responsabilidade |
|---|---|---|
| `.goreleaser.yaml` | Criar | Config GoReleaser v2: builds cross-platform + archives + checksum |
| `.github/workflows/release.yml` | Criar | Workflow de release acionado em tag `v*` |
| `.github/workflows/ci.yml` | Modificar | Adicionar step de golangci-lint-action |
| `README.md` | Reescrever | Instalação, quick start, comandos, config, artefatos, segurança |

---

### Task 1: GoReleaser config

**Files:**
- Create: `.goreleaser.yaml`

- [ ] **Step 1: Criar `.goreleaser.yaml`**

```yaml
version: 2

project_name: tfrepo

before:
  hooks:
    - go mod tidy

builds:
  - id: tfrepo
    main: ./cmd/tfrepo
    binary: tfrepo
    env:
      - CGO_ENABLED=0
    ldflags:
      - -s -w
    goos:
      - linux
      - darwin
      - windows
    goarch:
      - amd64
      - arm64

archives:
  - id: default
    formats:
      - tar.gz
    name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    format_overrides:
      - goos: windows
        formats:
          - zip

checksum:
  name_template: "{{ .ProjectName }}_{{ .Version }}_checksums.txt"

changelog:
  sort: asc
  filters:
    exclude:
      - "^docs:"
      - "^test:"
      - "^chore:"
```

- [ ] **Step 2: Validar sintaxe com goreleaser check**

```bash
go run github.com/goreleaser/goreleaser/v2@latest check
```

Esperado: `• config is valid` (sem erros).

- [ ] **Step 3: Fazer build de snapshot para a plataforma local**

```bash
go run github.com/goreleaser/goreleaser/v2@latest build --snapshot --single-target --clean
```

Esperado: saída terminando com `• build succeeded after N` e presença do binário em `dist/`.

```bash
ls dist/tfrepo_*/tfrepo* | head -5
```

Deve listar pelo menos um binário.

- [ ] **Step 4: Remover artefatos locais do snapshot**

```bash
rm -rf dist/
```

- [ ] **Step 5: Commit**

```bash
git add .goreleaser.yaml
git commit -m "chore: add GoReleaser v2 config for cross-platform builds"
```

---

### Task 2: Workflows de CI e release

**Files:**
- Create: `.github/workflows/release.yml`
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Criar `.github/workflows/release.yml`**

```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

jobs:
  release:
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v5
        with:
          go-version: '1.24'
      - uses: goreleaser/goreleaser-action@v6
        with:
          distribution: goreleaser
          version: '~> v2'
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

- [ ] **Step 2: Atualizar `.github/workflows/ci.yml` para incluir golangci-lint**

Conteúdo completo do arquivo atualizado (o `go vet` e `go test` já estão presentes; adicionar o step de lint entre eles):

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
          go-version: '1.24'
      - run: go vet ./...
      - uses: golangci/golangci-lint-action@v6
        with:
          version: latest
      - run: go test ./... -race
```

- [ ] **Step 3: Verificar que os YAMLs são sintaxe válida**

```bash
python3 -c "
import yaml, sys
for f in ['.github/workflows/ci.yml', '.github/workflows/release.yml']:
    yaml.safe_load(open(f))
    print(f'OK: {f}')
"
```

Esperado:
```
OK: .github/workflows/ci.yml
OK: .github/workflows/release.yml
```

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/release.yml .github/workflows/ci.yml
git commit -m "ci: add release workflow and golangci-lint to CI"
```

---

### Task 3: README

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Reescrever `README.md` com conteúdo completo**

Conteúdo final do arquivo:

````markdown
# tfrepo

CLI em Go para migração de repositórios Git entre provedores (GitHub, GitLab),
com mirror completo (branches, tags, histórico), foco em segurança,
integridade e auditabilidade.

> Port em Go do [TransfeRepo](https://github.com/arijunior2020/transferepo)
> (Node/TypeScript), com paridade funcional completa em relação à v0.1.0.

## Instalação

### Binário pré-compilado (recomendado)

Baixe o binário para sua plataforma na página de
[Releases](https://github.com/arijunior2020/tfrepo/releases/latest),
descompacte e mova para um diretório no `$PATH`:

```bash
# Linux x86_64
curl -Lo tfrepo.tar.gz \
  https://github.com/arijunior2020/tfrepo/releases/latest/download/tfrepo_Linux_x86_64.tar.gz
tar xzf tfrepo.tar.gz tfrepo
chmod +x tfrepo && sudo mv tfrepo /usr/local/bin/

# macOS Apple Silicon
curl -Lo tfrepo.tar.gz \
  https://github.com/arijunior2020/tfrepo/releases/latest/download/tfrepo_Darwin_arm64.tar.gz
tar xzf tfrepo.tar.gz tfrepo
chmod +x tfrepo && sudo mv tfrepo /usr/local/bin/
```

### go install

Requer Go 1.24+:

```bash
go install github.com/arijunior2020/tfrepo/cmd/tfrepo@latest
```

## Início rápido

**1. Crie o arquivo de configuração:**

```bash
tfrepo init
```

**2. Edite `transferepo.config.yaml`:**

```yaml
source:
  provider: github
  namespace: minha-org

target:
  provider: gitlab
  namespace: meu-grupo
```

**3. Configure os tokens via variáveis de ambiente:**

```bash
export GITHUB_TOKEN=ghp_...
export GITLAB_TOKEN=glpat-...
```

**4. Execute o pipeline:**

```bash
tfrepo scan      # lista repositórios → inventory.json
tfrepo plan      # gera plano de migração → migration-plan.json
tfrepo migrate   # executa migração → migration-report.json
tfrepo validate  # verifica integridade → validation-report.json
```

## Configuração

O arquivo `transferepo.config.yaml` (gerado por `tfrepo init`) segue o mesmo
formato da v0.1.0 do TransfeRepo:

```yaml
source:
  provider: github       # "github" | "gitlab"
  namespace: minha-org  # organização, usuário ou grupo
  # baseUrl: https://github.example.com  # GitHub Enterprise

target:
  provider: gitlab
  namespace: meu-grupo
  # baseUrl: https://gitlab.example.com  # GitLab auto-hospedado

filters:
  include:               # padrões glob; padrão: ["*"] (todos)
    - "api-*"
    - "shared-*"
  exclude:               # padrões glob para excluir
    - "*-archive"

mapping:                 # renomear repositórios no destino; padrão: mesmo nome
  old-name: new-name
```

## Comandos

### `tfrepo init`

Gera um `transferepo.config.yaml` de exemplo no diretório atual.
Exit code `1` se o arquivo já existir.

### `tfrepo scan [--concurrency N]`

Lista os repositórios do namespace de origem e grava `inventory.json`.

```
Flags:
  --concurrency int   repositórios processados em paralelo (padrão: 4)
  --config string     arquivo de configuração (padrão: transferepo.config.yaml)
```

### `tfrepo plan`

Aplica filtros e mapeamento sobre `inventory.json` e grava `migration-plan.json`.

```
Flags:
  --config string   arquivo de configuração (padrão: transferepo.config.yaml)
```

### `tfrepo migrate [--dry-run] [--concurrency N]`

Mirror-clona cada repositório da origem e empurra para o destino, gravando
`migration-report.json`. Exit code `1` se alguma tarefa falhar.

```
Flags:
  --dry-run           valida conectividade sem clonar nem empurrar (padrão: false)
  --concurrency int   repositórios migrados em paralelo (padrão: 4)
  --config string     arquivo de configuração (padrão: transferepo.config.yaml)
```

### `tfrepo validate`

Compara branches e tags entre origem e destino usando `migration-plan.json`
(e `migration-report.json`, se presente, para pular tarefas com falha).
Grava `validation-report.json`.

Exit code `0` se tudo ok/ignorado; `1` se houver divergências — adequado para CI.

```
Flags:
  --config string   arquivo de configuração (padrão: transferepo.config.yaml)
```

## Variáveis de ambiente

| Variável        | Quando obrigatória                             |
|-----------------|------------------------------------------------|
| `GITHUB_TOKEN`  | `source.provider: github` ou `target.provider: github` |
| `GITLAB_TOKEN`  | `source.provider: gitlab` ou `target.provider: gitlab` |

Tokens **nunca** são lidos de arquivos de configuração, logs ou artefatos.

## Artefatos

Todos os artefatos são JSON e ficam no diretório de trabalho atual:

| Arquivo                  | Gerado por       | Lido por                          |
|--------------------------|------------------|-----------------------------------|
| `inventory.json`         | `scan`           | `plan`                            |
| `migration-plan.json`    | `plan`           | `migrate`, `validate`             |
| `migration-report.json`  | `migrate`        | `validate` (opcional — pula falhas) |
| `validation-report.json` | `validate`       | —                                 |

## Segurança

- Tokens exclusivamente via variáveis de ambiente — nunca em `transferepo.config.yaml`,
  logs ou artefatos.
- Qualquer valor que possa conter um token é substituído por `***` antes de
  ser exibido ou gravado em disco.
- Repositórios clonados durante `migrate` ficam em diretórios temporários
  isolados removidos ao final — inclusive em caso de erro ou `Ctrl+C`.

## Design

Veja [`docs/superpowers/specs/2026-06-13-tfrepo-go-port-design.md`](docs/superpowers/specs/2026-06-13-tfrepo-go-port-design.md)
para a arquitetura completa.

## Licença

MIT
````

- [ ] **Step 2: Verificar que os comandos mencionados no README existem**

```bash
go build ./cmd/tfrepo -o /tmp/tfrepo-check && \
  /tmp/tfrepo-check --help | grep -E "init|scan|plan|migrate|validate" && \
  rm /tmp/tfrepo-check
```

Esperado: as 5 linhas com os subcomandos listados pelo help do Cobra.

- [ ] **Step 3: Verificar suite completa de testes**

```bash
go test ./... -race
```

Esperado: `ok` em todos os pacotes, sem falhas.

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: expand README with install, quickstart, command reference and config docs"
```
