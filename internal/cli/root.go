package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const (
	// defaultConfigPath is the default value of the --config flag,
	// matching DEFAULT_CONFIG_PATH in apps/cli/src/cli.ts.
	defaultConfigPath = "transferepo.config.yaml"

	// programVersion matches PROGRAM_VERSION in apps/cli/src/cli.ts.
	programVersion = "0.1.0"
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

	root.PersistentFlags().StringP("config", "c", defaultConfigPath, "caminho do arquivo de configuração")

	root.AddCommand(newInitCommand(exitCode))

	return root
}

func newInitCommand(exitCode *int) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Gera um transferepo.config.yaml de exemplo no diretório atual",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := cmd.Flags().GetString("config")
			if err != nil {
				return err
			}
			*exitCode = runInit(configPath, cmd.OutOrStdout(), cmd.ErrOrStderr())
			return nil
		},
	}
}
