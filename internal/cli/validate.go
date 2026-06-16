package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/arijunior2020/tfrepo/internal/config"
	"github.com/arijunior2020/tfrepo/internal/core"
)

// validationReportPath é o arquivo gravado por "tfrepo validate", relativo ao
// diretório de trabalho atual.
const validationReportPath = "validation-report.json"

// runValidate carrega o config em configPath, lê migrationPlanPath, tenta ler
// migrationReportPath (opcional — se ausente, report é nil e nenhuma task é
// ignorada), constrói os providers e delega para validateAndWrite. Retorna o
// exit code do processo (0 se tudo ok/skipped, 1 se houver qualquer divergido).
func runValidate(ctx context.Context, configPath string, stdout, stderr io.Writer) int {
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

	// migration-report.json é opcional: se ausente, nenhuma task é marcada
	// como skipped. O erro de leitura é silenciado intencionalmente.
	var report *core.MigrationReport
	var mr core.MigrationReport
	if err := core.ReadJSON(migrationReportPath, &mr); err == nil {
		report = &mr
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

	return validateAndWrite(ctx, plan, core.MigrateProviders{Source: source, Target: target}, report, stdout, stderr)
}

// validateAndWrite executa core.Validate para plan, grava validation-report.json
// e imprime um resumo de uma linha. Retorna 1 se houver qualquer task "diverged".
func validateAndWrite(ctx context.Context, plan core.MigrationPlan, providers core.MigrateProviders, report *core.MigrationReport, stdout, stderr io.Writer) int {
	validation, err := core.Validate(ctx, plan, providers, report)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if err := core.WriteJSON(validationReportPath, validation); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	var ok, diverged, skipped int
	for _, r := range validation.Results {
		switch r.Status {
		case "ok":
			ok++
		case "diverged":
			diverged++
		case "skipped":
			skipped++
		}
	}

	var parts []string
	if ok > 0 {
		parts = append(parts, countLabel(ok, "repositório ok", "repositórios ok"))
	}
	if diverged > 0 {
		parts = append(parts, countLabel(diverged, "divergido", "divergidos"))
	}
	if skipped > 0 {
		parts = append(parts, countLabel(skipped, "ignorado", "ignorados"))
	}
	if len(parts) == 0 {
		parts = []string{countLabel(0, "repositório ok", "repositórios ok")}
	}
	fmt.Fprintf(stdout, "Wrote %s (%s)\n", validationReportPath, strings.Join(parts, ", "))

	if diverged > 0 {
		return 1
	}
	return 0
}
