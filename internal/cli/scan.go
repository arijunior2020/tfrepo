package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/arijunior2020/tfrepo/internal/config"
	"github.com/arijunior2020/tfrepo/internal/core"
	"github.com/arijunior2020/tfrepo/internal/provider"
)

// inventoryPath is the file written by "tfrepo scan" and read by
// "tfrepo plan", relative to the current working directory.
const inventoryPath = "inventory.json"

// runScan loads the configuration at configPath, scans cfg.Source's
// namespace using up to concurrency goroutines, and writes the result to
// inventoryPath. It returns the process exit code (0 on success, 1 on
// error).
func runScan(ctx context.Context, configPath string, concurrency int, stdout, stderr io.Writer) int {
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	p, err := NewProvider(cfg.Source)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	return scanAndWrite(ctx, p, cfg.Source.Namespace, concurrency, stdout, stderr)
}

// scanAndWrite runs core.Scan against p and writes the resulting inventory
// to inventoryPath, printing a one-line summary on success.
func scanAndWrite(ctx context.Context, p provider.RepositoryProvider, namespace string, concurrency int, stdout, stderr io.Writer) int {
	inventory, err := core.Scan(ctx, p, namespace, concurrency)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if err := core.WriteJSON(inventoryPath, inventory); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	repoCount := 0
	for _, ns := range inventory.Namespaces {
		repoCount += len(ns.Repositories)
	}

	fmt.Fprintf(stdout, "Wrote %s (%s)\n", inventoryPath, countLabel(repoCount, "repositório", "repositórios"))
	return 0
}
