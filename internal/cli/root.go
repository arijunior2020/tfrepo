package cli

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

// programVersion is set at build time via ldflags -X. Must be var, not const.
var programVersion = "0.1.0"

const (
	// defaultConfigPath is the default value of the --config flag,
	// matching DEFAULT_CONFIG_PATH in apps/cli/src/cli.ts.
	defaultConfigPath = "transferepo.config.yaml"

	// configFlagName is the name of the persistent --config/-c flag shared by
	// every subcommand.
	configFlagName = "config"

	// concurrencyFlagName is the name of the --concurrency flag on "scan" and
	// "migrate".
	concurrencyFlagName = "concurrency"

	// defaultScanConcurrency is the default value of the --concurrency flag
	// on "scan" and "migrate".
	defaultScanConcurrency = 4

	// dryRunFlagName is the name of the --dry-run flag on "migrate".
	dryRunFlagName = "dry-run"
)

// Execute runs the tfrepo root command against os.Args and returns the
// process exit code.
func Execute() int {
	exitCode := 0
	root := newRootCommand(&exitCode)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return exitCode
}

// newRootCommand builds the tfrepo command tree. Each subcommand writes its
// exit code to *exitCode instead of returning an error for "business" exit
// codes (e.g. migrate returning 1 because a task failed), mirroring
// process.exitCode = await deps.runXxx(...) in apps/cli/src/cli.ts.
//
// Contract: every subcommand's RunE must set *exitCode before returning nil.
// Returning a non-nil error from RunE always yields process exit code 1
// (via Execute), regardless of *exitCode.
func newRootCommand(exitCode *int) *cobra.Command {
	root := &cobra.Command{
		Use:           "tfrepo",
		Short:         "Migra repositórios Git entre provedores (GitHub, GitLab)",
		Version:       programVersion,
		SilenceErrors: true,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprint(cmd.OutOrStdout(), renderBanner())
			fmt.Fprintln(cmd.OutOrStdout())
			_ = cmd.Help()
		},
	}

	root.PersistentFlags().StringP(configFlagName, "c", defaultConfigPath, "caminho do arquivo de configuração")

	root.AddCommand(newInitCommand(exitCode))
	root.AddCommand(newScanCommand(exitCode))
	root.AddCommand(newPlanCommand(exitCode))
	root.AddCommand(newMigrateCommand(exitCode))
	root.AddCommand(newValidateCommand(exitCode))
	root.AddCommand(newSetupCommand(exitCode))
	root.AddCommand(newUpdateCommand(exitCode))

	return root
}

func newInitCommand(exitCode *int) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Gera um transferepo.config.yaml de exemplo no diretório atual",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := cmd.Flags().GetString(configFlagName)
			if err != nil {
				return err
			}
			*exitCode = runInit(configPath, cmd.OutOrStdout(), cmd.ErrOrStderr())
			return nil
		},
	}
}

func newScanCommand(exitCode *int) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Lista os repositórios do namespace de origem e grava inventory.json",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := cmd.Flags().GetString(configFlagName)
			if err != nil {
				return err
			}
			concurrency, err := cmd.Flags().GetInt(concurrencyFlagName)
			if err != nil {
				return err
			}
			*exitCode = runScan(cmd.Context(), configPath, concurrency, cmd.OutOrStdout(), cmd.ErrOrStderr())
			return nil
		},
	}

	cmd.Flags().Int(concurrencyFlagName, defaultScanConcurrency, "número de repositórios processados em paralelo")

	return cmd
}

func newPlanCommand(exitCode *int) *cobra.Command {
	return &cobra.Command{
		Use:   "plan",
		Short: "Aplica filtros e mapeamento de inventory.json e grava migration-plan.json",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := cmd.Flags().GetString(configFlagName)
			if err != nil {
				return err
			}
			*exitCode = runPlan(configPath, cmd.OutOrStdout(), cmd.ErrOrStderr())
			return nil
		},
	}
}

func newMigrateCommand(exitCode *int) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Executa migration-plan.json, mirror-clonando cada repositório de origem e empurrando para o destino",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := cmd.Flags().GetString(configFlagName)
			if err != nil {
				return err
			}
			dryRun, err := cmd.Flags().GetBool(dryRunFlagName)
			if err != nil {
				return err
			}
			concurrency, err := cmd.Flags().GetInt(concurrencyFlagName)
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			*exitCode = runMigrate(ctx, configPath, dryRun, concurrency, cmd.OutOrStdout(), cmd.ErrOrStderr())
			return nil
		},
	}
	cmd.Flags().Bool(dryRunFlagName, false, "valida conectividade com o destino sem clonar nem empurrar repositórios")
	cmd.Flags().Int(concurrencyFlagName, defaultScanConcurrency, "número de repositórios migrados em paralelo")
	return cmd
}

func newValidateCommand(exitCode *int) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Compara branches e tags entre origem e destino usando migration-plan.json",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := cmd.Flags().GetString(configFlagName)
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			*exitCode = runValidate(ctx, configPath, cmd.OutOrStdout(), cmd.ErrOrStderr())
			return nil
		},
	}
}

func newSetupCommand(exitCode *int) *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Wizard interativo: escaneia a origem e gera transferepo.config.yaml",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := cmd.Flags().GetString(configFlagName)
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			*exitCode = runSetup(ctx, configPath, cmd.OutOrStdout(), cmd.ErrOrStderr())
			return nil
		},
	}
}
