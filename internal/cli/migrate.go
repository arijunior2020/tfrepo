package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/arijunior2020/tfrepo/internal/config"
	"github.com/arijunior2020/tfrepo/internal/core"
	"github.com/arijunior2020/tfrepo/internal/security"
)

// migrationReportPath é o arquivo gravado por "tfrepo migrate", relativo ao
// diretório de trabalho atual. Lido opcionalmente por "tfrepo validate".
const migrationReportPath = "migration-report.json"

// runMigrate carrega o config em configPath, lê migrationPlanPath, constrói
// os providers de origem e destino e delega para migrateAndWrite. Retorna o
// exit code do processo (0 em sucesso, 1 em qualquer erro).
func runMigrate(ctx context.Context, configPath string, dryRun bool, concurrency int, stdout, stderr io.Writer) int {
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

	return migrateAndWrite(ctx, plan, core.MigrateProviders{Source: source, Target: target}, dryRun, concurrency, stdout, stderr)
}

// migrateAndWrite executa plan contra providers, grava migration-report.json e
// imprime um resumo de uma linha. Retorna 1 se alguma task tiver status "failed".
// defer workspaces.CleanupAll() é belt-and-suspenders: cada goroutine já faz
// defer workspace.Cleanup() via core.Migrate, mas CleanupAll garante que
// workspaces criados e não limpos no cancelamento do context sejam removidos.
func migrateAndWrite(ctx context.Context, plan core.MigrationPlan, providers core.MigrateProviders, dryRun bool, concurrency int, stdout, stderr io.Writer) int {
	workspaces := security.NewManager()
	defer func() { _ = workspaces.CleanupAll() }()

	report, err := core.Migrate(ctx, plan, providers, core.MigrateOptions{
		DryRun:      dryRun,
		Concurrency: concurrency,
		Workspaces:  workspaces,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if err := core.WriteJSON(migrationReportPath, report); err != nil {
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
	if dryRun {
		suffix += ", dry-run"
	}
	fmt.Fprintf(stdout, "Wrote %s (%s%s)\n", migrationReportPath, label, suffix)

	if failed > 0 {
		return 1
	}
	return 0
}
