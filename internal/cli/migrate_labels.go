package cli

import (
	"context"
	"fmt"
	"io"
	"os/signal"
	"syscall"

	"github.com/arijunior2020/tfrepo/internal/config"
	"github.com/arijunior2020/tfrepo/internal/core"
	"github.com/spf13/cobra"
)

const labelsReportPath = "labels-report.json"

func newMigrateLabelsCommand(exitCode *int) *cobra.Command {
	return &cobra.Command{
		Use:   "migrate-labels",
		Short: "Migra labels e milestones dos repositórios de origem para o destino",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := cmd.Flags().GetString(configFlagName)
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			*exitCode = runMigrateLabels(ctx, configPath, cmd.OutOrStdout(), cmd.ErrOrStderr())
			return nil
		},
	}
}

func runMigrateLabels(ctx context.Context, configPath string, stdout, stderr io.Writer) int {
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	var plan core.MigrationPlan
	if err := core.ReadJSON(migrationPlanPath, &plan); err != nil {
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

	report, err := core.MigrateLabelsAndMilestones(ctx, plan, core.LabelsMigrateProviders{Source: source, Target: target})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if err := core.WriteJSON(labelsReportPath, report); err != nil {
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
	fmt.Fprintf(stdout, "Wrote %s (%s%s)\n", labelsReportPath, label, suffix)

	if failed > 0 {
		return 1
	}
	return 0
}
