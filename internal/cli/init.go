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
