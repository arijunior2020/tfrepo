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

// runSetup runs the interactive setup wizard that guides the user through
// configuring source and target providers, scanning available repositories,
// and writing a transferepo.config.yaml file.
func runSetup(ctx context.Context, configPath string, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// --- source form ---
	var sourceProvider, sourceNamespace, sourceBaseURL string

	sourceForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Provedor de origem").
				Options(
					huh.NewOption("GitHub", "github"),
					huh.NewOption("GitLab", "gitlab"),
				).
				Value(&sourceProvider),
			huh.NewInput().
				Title("Namespace de origem (org/usuário)").
				Value(&sourceNamespace).
				Validate(func(s string) error {
					if s == "" {
						return errors.New("namespace não pode ser vazio")
					}
					return nil
				}),
			huh.NewInput().
				Title("Base URL de origem (opcional, deixe em branco para API pública)").
				Value(&sourceBaseURL),
		),
	)

	if err := sourceForm.RunWithContext(ctx); err != nil {
		fmt.Fprintln(stderr, "Operação cancelada.")
		return 1
	}

	// --- scan ---
	sourceProv, err := NewProvider(config.ProviderConfig{
		Provider:  sourceProvider,
		Namespace: sourceNamespace,
		BaseURL:   sourceBaseURL,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	fmt.Fprintf(stdout, "Buscando repositórios em %s/%s...\n", sourceProvider, sourceNamespace)

	inventory, err := core.Scan(ctx, sourceProv, sourceNamespace, defaultScanConcurrency)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	var allRepos []string
	for _, ns := range inventory.Namespaces {
		for _, r := range ns.Repositories {
			allRepos = append(allRepos, r.Name)
		}
	}

	fmt.Fprintf(stdout, "Encontrados: %s\n", countLabel(len(allRepos), "repositório", "repositórios"))

	// --- repo selection ---
	var selectedRepos []string
	if len(allRepos) > 0 {
		repoOptions := make([]huh.Option[string], len(allRepos))
		for i, name := range allRepos {
			repoOptions[i] = huh.NewOption(name, name)
		}

		repoForm := huh.NewForm(
			huh.NewGroup(
				huh.NewMultiSelect[string]().
					Title("Selecione os repositórios a migrar").
					Description("Espaço = selecionar/desmarcar  ·  Enter = confirmar  ·  Nenhuma seleção = todos").
					Options(repoOptions...).
					Value(&selectedRepos),
			),
		)

		if err := repoForm.RunWithContext(ctx); err != nil {
			fmt.Fprintln(stderr, "Operação cancelada.")
			return 1
		}
	}

	// --- target form ---
	var targetProvider, targetNamespace, targetBaseURL string

	targetForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Provedor de destino").
				Options(
					huh.NewOption("GitHub", "github"),
					huh.NewOption("GitLab", "gitlab"),
				).
				Value(&targetProvider),
			huh.NewInput().
				Title("Namespace de destino (org/usuário)").
				Value(&targetNamespace).
				Validate(func(s string) error {
					if s == "" {
						return errors.New("namespace não pode ser vazio")
					}
					return nil
				}),
			huh.NewInput().
				Title("Base URL de destino (opcional, deixe em branco para API pública)").
				Value(&targetBaseURL),
		),
	)

	if err := targetForm.RunWithContext(ctx); err != nil {
		fmt.Fprintln(stderr, "Operação cancelada.")
		return 1
	}

	// --- overwrite check ---
	if _, statErr := os.Stat(configPath); statErr == nil {
		var overwrite bool
		confirmForm := huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title(configPath + " já existe. Sobrescrever?").
					Value(&overwrite),
			),
		)

		if err := confirmForm.RunWithContext(ctx); err != nil {
			fmt.Fprintln(stderr, "Operação cancelada.")
			return 1
		}

		if !overwrite {
			fmt.Fprintln(stderr, "Operação cancelada.")
			return 1
		}
	}

	return setupAndWrite(WizardInput{
		SourceProvider:  sourceProvider,
		SourceNamespace: sourceNamespace,
		SourceBaseURL:   sourceBaseURL,
		TargetProvider:  targetProvider,
		TargetNamespace: targetNamespace,
		TargetBaseURL:   targetBaseURL,
		SelectedRepos:   selectedRepos,
	}, configPath, stdout, stderr)
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
