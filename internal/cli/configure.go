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

var configureCredPathFn = credentials.DefaultPath

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

	credPath := configureCredPathFn()
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
