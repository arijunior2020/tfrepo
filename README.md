# tfrepo

CLI em Go para migração de repositórios entre provedores Git (GitHub, GitLab),
com mirror completo (branches, tags, histórico), foco em segurança,
integridade e auditabilidade.

> Status: em desenvolvimento — port em Go do
> [TransfeRepo](https://github.com/arijunior2020/transferepo) (Node/TypeScript),
> com paridade funcional completa em relação à v0.1.0.

## Design

Veja [`docs/superpowers/specs/2026-06-13-tfrepo-go-port-design.md`](docs/superpowers/specs/2026-06-13-tfrepo-go-port-design.md)
para a arquitetura completa (módulo Go, providers, motor de migração, CLI,
segurança, testes e distribuição).

## Segurança

- Tokens só via variáveis de ambiente (`GITHUB_TOKEN`, `GITLAB_TOKEN`) — nunca
  em arquivos de configuração, logs ou artefatos. Qualquer valor que possa
  conter um token é substituído por `***` antes de ser exibido ou gravado em
  disco.
- Nenhum código-fonte de repositórios migrados é armazenado permanentemente:
  cada tarefa de `migrate` usa um diretório temporário isolado, removido ao
  final — inclusive em caso de erro ou interrupção do processo (`Ctrl+C`).

## Licença

MIT
