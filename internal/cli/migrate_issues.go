package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/arijunior2020/tfrepo/internal/config"
	"github.com/arijunior2020/tfrepo/internal/core"
	"github.com/arijunior2020/tfrepo/internal/provider"
	"github.com/spf13/cobra"
)

const issuesReportPath = "issues-report.json"

func newMigrateIssuesCommand(exitCode *int) *cobra.Command {
	return &cobra.Command{
		Use:   "migrate-issues",
		Short: "Migra issues dos repositórios de origem para o destino",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := cmd.Flags().GetString(configFlagName)
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			*exitCode = runMigrateIssues(ctx, configPath, cmd.OutOrStdout(), cmd.ErrOrStderr())
			return nil
		},
	}
}

func runMigrateIssues(ctx context.Context, configPath string, stdout, stderr io.Writer) int {
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
	return runMigrateIssuesWithProviders(ctx, configPath, stdout, stderr, source, target)
}

func runMigrateIssuesWithProviders(ctx context.Context, _ string, stdout, stderr io.Writer, source, target provider.RepositoryProvider) int {
	var plan core.MigrationPlan
	if err := core.ReadJSON(migrationPlanPath, &plan); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	milestoneMapsByTask := buildMilestoneMapsByTask()

	report, err := core.MigrateIssues(ctx, plan, core.IssuesMigrateProviders{Source: source, Target: target}, milestoneMapsByTask)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if err := core.WriteJSON(issuesReportPath, report); err != nil {
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
	fmt.Fprintf(stdout, "Wrote %s (%s%s)\n", issuesReportPath, label, suffix)

	if failed > 0 {
		return 1
	}
	return 0
}

// buildMilestoneMapsByTask reads labels-report.json (if present) and returns
// a task-ID-keyed map of MilestoneIDMap for milestone translation.
// A missing file is silently ignored.
func buildMilestoneMapsByTask() map[string]core.MilestoneIDMap {
	var labelsReport core.LabelsReport
	if err := core.ReadJSON(labelsReportPath, &labelsReport); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return nil
	}
	m := make(map[string]core.MilestoneIDMap, len(labelsReport.Results))
	for _, r := range labelsReport.Results {
		m[r.ID] = r.MilestoneIDMap
	}
	return m
}
