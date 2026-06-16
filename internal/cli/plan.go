package cli

import (
	"fmt"
	"io"

	"github.com/arijunior2020/tfrepo/internal/config"
	"github.com/arijunior2020/tfrepo/internal/core"
)

// migrationPlanPath is the file written by "tfrepo plan" and read by
// "tfrepo migrate"/"tfrepo validate", relative to the current working
// directory.
const migrationPlanPath = "migration-plan.json"

// runPlan loads the configuration at configPath and the inventory at
// inventoryPath, applies cfg's filters and repository name mapping via
// core.Plan, and writes the result to migrationPlanPath. It returns the
// process exit code (0 on success, 1 on error).
func runPlan(configPath string, stdout, stderr io.Writer) int {
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	var inventory core.Inventory
	if err := core.ReadJSON(inventoryPath, &inventory); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	plan, err := core.Plan(inventory, cfg)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if err := core.WriteJSON(migrationPlanPath, plan); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	var branches, tags int
	for _, task := range plan.Tasks {
		branches += len(task.Branches)
		tags += len(task.Tags)
	}

	fmt.Fprintf(stdout, "Wrote %s (%s, %s, %s)\n", migrationPlanPath,
		countLabel(len(plan.Tasks), "repositório", "repositórios"),
		countLabel(branches, "branch", "branches"),
		countLabel(tags, "tag", "tags"),
	)
	return 0
}
