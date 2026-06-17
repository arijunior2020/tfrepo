package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
)

var migrationArtifactPaths = []string{
	inventoryPath,
	migrationPlanPath,
	migrationReportPath,
	labelsReportPath,
	validationReportPath,
}

func runDestroy(configPath string, includeConfig bool, stdout, stderr io.Writer) int {
	removed, err := destroyFiles(destroyPaths(configPath, includeConfig))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if len(removed) == 0 {
		fmt.Fprintln(stdout, "Nenhum arquivo de migração encontrado.")
		return 0
	}

	for _, path := range removed {
		fmt.Fprintf(stdout, "Removido %s\n", path)
	}
	fmt.Fprintf(stdout, "Removidos %s.\n", countLabel(len(removed), "arquivo", "arquivos"))
	return 0
}

func destroyPaths(configPath string, includeConfig bool) []string {
	paths := make([]string, 0, len(migrationArtifactPaths)+1)
	seen := map[string]bool{}

	for _, path := range migrationArtifactPaths {
		if !seen[path] {
			paths = append(paths, path)
			seen[path] = true
		}
	}

	if includeConfig && configPath != "" && !seen[configPath] {
		paths = append(paths, configPath)
	}

	return paths
}

func destroyFiles(paths []string) ([]string, error) {
	removed := make([]string, 0, len(paths))

	for _, path := range paths {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return removed, err
		}
		if info.IsDir() {
			return removed, fmt.Errorf("%s é um diretório; remoção recusada", path)
		}
		if err := os.Remove(path); err != nil {
			return removed, err
		}
		removed = append(removed, path)
	}

	return removed, nil
}
