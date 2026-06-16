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

Requer `GITHUB_TOKEN` e/ou `GITLAB_TOKEN` definidos antes de rodar.

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

### `tfrepo validate`

Compara branches e tags entre origem e destino usando `migration-plan.json`
(e `migration-report.json`, se presente, para pular tarefas com falha).
Grava `validation-report.json`.

Exit code `0` se tudo ok/ignorado; `1` se houver divergências — adequado para CI.

```
Flags:
  -c, --config string   arquivo de configuração (padrão: transferepo.config.yaml)
```

### `tfrepo update`

Verifica se há uma versão mais recente disponível no GitHub e, se houver,
baixa e substitui o binário instalado automaticamente.

```bash
tfrepo update
```

Não requer nenhuma flag. O binário substituído é o mesmo que está em execução
(`os.Executable()`). Em caso de erro de permissão, execute com `sudo`.

## Variáveis de ambiente

| Variável        | Quando obrigatória                                          |
|-----------------|-------------------------------------------------------------|
| `GITHUB_TOKEN`  | `source.provider: github` ou `target.provider: github`     |
| `GITLAB_TOKEN`  | `source.provider: gitlab` ou `target.provider: gitlab`     |

Tokens **nunca** são lidos de arquivos de configuração, logs ou artefatos.

## Artefatos

Todos os artefatos são JSON e ficam no diretório de trabalho atual:

| Arquivo                  | Gerado por | Lido por                              |
|--------------------------|------------|---------------------------------------|
| `inventory.json`         | `scan`     | `plan`                                |
| `migration-plan.json`    | `plan`     | `migrate`, `validate`                 |
| `migration-report.json`  | `migrate`  | `validate` (opcional — pula falhas)   |
| `validation-report.json` | `validate` | —                                     |

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
