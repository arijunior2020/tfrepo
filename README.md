# tfrepo

CLI em Go para migração de repositórios Git entre provedores (GitHub, GitLab),
com mirror completo (branches, tags, histórico), foco em segurança,
integridade e auditabilidade.

> Port em Go do [TransfeRepo](https://github.com/arijunior2020/transferepo)
> (Node/TypeScript), com paridade funcional completa em relação à v0.1.0.

## Instalação

### Script de instalação (recomendado)

**Linux / macOS:**

```bash
curl -fsSL https://raw.githubusercontent.com/arijunior2020/tfrepo/main/install.sh | bash
```

**Windows (PowerShell):**

```powershell
irm https://raw.githubusercontent.com/arijunior2020/tfrepo/main/install.ps1 | iex
```

O script detecta o sistema operacional e a arquitetura automaticamente, baixa o binário correto da última release e instala em `/usr/local/bin` (Linux/macOS) ou `%LOCALAPPDATA%\Programs\tfrepo` (Windows).

Para instalar em um diretório personalizado, defina `TFREPO_INSTALL_DIR` antes de executar.

### Binário pré-compilado (manual)

Baixe o binário para sua plataforma na página de
[Releases](https://github.com/arijunior2020/tfrepo/releases/latest),
descompacte e mova para um diretório no `$PATH`:

```bash
# Linux x86_64 (ajuste VERSION para a versão desejada, ex: 0.1.0)
VERSION=0.1.0
curl -Lo tfrepo.tar.gz \
  "https://github.com/arijunior2020/tfrepo/releases/download/v${VERSION}/tfrepo_${VERSION}_linux_amd64.tar.gz"
tar xzf tfrepo.tar.gz tfrepo
chmod +x tfrepo && sudo mv tfrepo /usr/local/bin/

# macOS Apple Silicon
curl -Lo tfrepo.tar.gz \
  "https://github.com/arijunior2020/tfrepo/releases/download/v${VERSION}/tfrepo_${VERSION}_darwin_arm64.tar.gz"
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

**3. Configure os tokens de acesso:**

```bash
tfrepo configure
```

Ou via variáveis de ambiente (têm prioridade sobre o arquivo de credenciais):

```bash
export GITHUB_TOKEN=ghp_...
export GITLAB_TOKEN=glpat-...
```

**4. Execute o pipeline:**

```bash
tfrepo scan              # lista repositórios → inventory.json
tfrepo plan              # gera plano de migração → migration-plan.json
tfrepo migrate           # executa migração → migration-report.json
tfrepo migrate-labels    # migra labels e milestones → labels-report.json
tfrepo migrate-issues    # migra issues → issues-report.json
tfrepo migrate-prs       # migra PRs abertos → prs-report.json
tfrepo validate          # verifica integridade → validation-report.json
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

### `tfrepo setup` (recomendado para novos usuários)

Wizard interativo que guia a configuração completa da migração:

1. Pergunta provider e namespace de **origem**
2. Conecta na API e lista os repositórios disponíveis
3. Permite migrar todos os repositórios ou selecionar repositórios específicos
4. Pergunta provider e namespace de **destino**
5. Gera o `transferepo.config.yaml` pronto para uso

```
Flags:
  -c, --config string   arquivo de configuração a gerar (padrão: transferepo.config.yaml)
```

Requer tokens configurados via `tfrepo configure` ou variáveis de ambiente `GITHUB_TOKEN` / `GITLAB_TOKEN` antes de rodar.

### `tfrepo init`

Gera um `transferepo.config.yaml` de exemplo no diretório atual.
Exit code `1` se o arquivo já existir.

### `tfrepo scan [--concurrency N]`

Lista os repositórios do namespace de origem e grava `inventory.json`.

```
Flags:
  --concurrency int   repositórios processados em paralelo (padrão: 4)
  -c, --config string   arquivo de configuração (padrão: transferepo.config.yaml)
```

### `tfrepo plan`

Aplica filtros e mapeamento sobre `inventory.json` e grava `migration-plan.json`.

```
Flags:
  -c, --config string   arquivo de configuração (padrão: transferepo.config.yaml)
```

### `tfrepo migrate [--dry-run] [--concurrency N]`

Mirror-clona cada repositório da origem e empurra para o destino, gravando
`migration-report.json`. Exit code `1` se alguma tarefa falhar.

```
Flags:
  --dry-run           valida conectividade sem clonar nem empurrar (padrão: false)
  --concurrency int   repositórios migrados em paralelo (padrão: 4)
  -c, --config string   arquivo de configuração (padrão: transferepo.config.yaml)
```

### `tfrepo migrate-labels`

Migra labels e milestones de cada repositório de origem para o repositório de destino correspondente, conforme definido em `migration-plan.json`. O resultado é gravado em `labels-report.json`.

Requer que `migration-plan.json` exista (gerado por `tfrepo plan`). Itens já existentes no destino são ignorados (operação idempotente). O `labels-report.json` gerado contém um mapeamento de IDs de milestones necessário pelo plano de migração de issues (Plan 9).

```
Flags:
  -c, --config string   arquivo de configuração (padrão: transferepo.config.yaml)
```

### `tfrepo migrate-issues`

Migra issues (abertas e fechadas) dos repositórios de origem para o destino.
Lê `labels-report.json` (se existir) para traduzir referências de milestone.
Issues já existentes no destino (por título) são ignoradas — operação idempotente.

```bash
tfrepo migrate-issues
```

```
Flags:
  -c, --config string   arquivo de configuração (padrão: transferepo.config.yaml)
```

### `tfrepo migrate-prs`

Migra pull requests abertos dos repositórios de origem para o destino.
Apenas PRs abertos são migrados — PRs fechados e mesclados são artefatos históricos preservados no histórico git.
PRs já existentes no destino (por título) são ignorados — a operação é idempotente.

```bash
tfrepo migrate-prs
```

### `tfrepo validate`

Compara branches e tags entre origem e destino usando `migration-plan.json`
(e `migration-report.json`, se presente, para pular tarefas com falha).
Grava `validation-report.json`.

Exit code `0` se tudo ok/ignorado; `1` se houver divergências — adequado para CI.

```
Flags:
  -c, --config string   arquivo de configuração (padrão: transferepo.config.yaml)
```

### `tfrepo destroy`

Remove os artefatos locais gerados pela migração atual para iniciar uma nova
migração no mesmo diretório:

- `inventory.json`
- `migration-plan.json`
- `migration-report.json`
- `labels-report.json`
- `issues-report.json`
- `prs-report.json`
- `validation-report.json`

Por padrão, preserva o arquivo de configuração. Use `--include-config` para
remover também o arquivo informado por `--config`.

```
Flags:
  --include-config     também remove o arquivo de configuração
  -c, --config string  arquivo de configuração (padrão: transferepo.config.yaml)
```

### `tfrepo update`

Verifica se há uma versão mais recente disponível no GitHub e, se houver,
baixa e substitui o binário instalado automaticamente.

```bash
tfrepo update
```

Não requer nenhuma flag. O binário substituído é o mesmo que está em execução
(`os.Executable()`). Se ele estiver em um diretório protegido, como
`/usr/local/bin`, o comando solicita `sudo` automaticamente.

## Variáveis de ambiente

| Variável        | Quando obrigatória                                          |
|-----------------|-------------------------------------------------------------|
| `GITHUB_TOKEN`  | `source.provider: github` ou `target.provider: github`     |
| `GITLAB_TOKEN`  | `source.provider: gitlab` ou `target.provider: gitlab`     |

Tokens **nunca** são lidos de arquivos de configuração, logs ou artefatos.

## Artefatos

Todos os artefatos são JSON e ficam no diretório de trabalho atual:

| Arquivo                  | Gerado por        | Lido por                              |
|--------------------------|-------------------|---------------------------------------|
| `inventory.json`         | `scan`            | `plan`                                |
| `migration-plan.json`    | `plan`            | `migrate`, `validate`                 |
| `migration-report.json`  | `migrate`         | `validate` (opcional — pula falhas)   |
| `labels-report.json`     | `migrate-labels`  | `migrate-issues` (milestone mapping)  |
| `issues-report.json`     | `migrate-issues`  | —                                     |
| `prs-report.json`        | `migrate-prs`     | —                                     |
| `validation-report.json` | `validate`        | —                                     |

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
