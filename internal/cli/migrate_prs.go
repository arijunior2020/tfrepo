package cli

import (
	"context"
	"fmt"
	"io"
	"os/signal"
	"syscall"

	"github.com/arijunior2020/tfrepo/internal/config"
	"github.com/arijunior2020/tfrepo/internal/core"
	"github.com/arijunior2020/tfrepo/internal/provider"
	"github.com/spf13/cobra"
)

const prsReportPath = "prs-report.json"

func newMigratePRsCommand(exitCode *int) *cobra.Command {
	return &cobra.Command{
		Use:   "migrate-prs",
		Short: "Migra pull requests abertos dos repositórios de origem para o destino",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := cmd.Flags().GetString(configFlagName)
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			*exitCode = runMigratePRs(ctx, configPath, cmd.OutOrStdout(), cmd.ErrOrStderr())
			return nil
		},
	}
}

func runMigratePRs(ctx context.Context, configPath string, stdout, stderr io.Writer) int {
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	source, err := NewProvider(cfg.Source)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	target, err := NewProvider(cfg.Target)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return runMigratePRsWithProviders(ctx, configPath, stdout, stderr, source, target)
}

func runMigratePRsWithProviders(ctx context.Context, _ string, stdout, stderr io.Writer, source, target provider.RepositoryProvider) int {
	var plan core.MigrationPlan
	if err := core.ReadJSON(migrationPlanPath, &plan); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	report, err := core.MigratePullRequests(ctx, plan, core.PRsMigrateProviders{Source: source, Target: target})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if err := core.WriteJSON(prsReportPath, report); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	var failed int
	for _, r := range report.Results {
		if r.Status == "failed" {
			failed++
		}
	}

	label := countLabel(len(report.Results), "repositório", "repositórios")
	suffix := ""
	if failed > 0 {
		suffix = fmt.Sprintf(", %d com falha", failed)
	}
	fmt.Fprintf(stdout, "Wrote %s (%s%s)\n", prsReportPath, label, suffix)

	if failed > 0 {
		return 1
	}
	return 0
}
