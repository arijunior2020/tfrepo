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
	SourceBaseURL   string
	TargetProvider  string
	TargetNamespace string
	TargetBaseURL   string
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
