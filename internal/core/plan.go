package core

import (
	"time"

	"github.com/arijunior2020/tfrepo/internal/config"
)

// Plan applies cfg's include/exclude filters and repository name mapping to
// the repositories recorded in inventory, producing the tasks for
// migration-plan.json.
func Plan(inventory Inventory, cfg *config.Config) (MigrationPlan, error) {
	filterer, err := NewFilterer(cfg.Filters.Include, cfg.Filters.Exclude)
	if err != nil {
		return MigrationPlan{}, err
	}

	tasks := []MigrationTask{}
	for _, ns := range inventory.Namespaces {
		for _, repo := range ns.Repositories {
			if !filterer.Match(repo.Name) {
				continue
			}

			targetRepo := repo.Name
			if mapped, ok := cfg.Mapping[repo.Name]; ok {
				targetRepo = mapped
			}

			tasks = append(tasks, MigrationTask{
				ID:       repo.Name,
				Source:   TaskEndpoint{Namespace: ns.Slug, Repo: repo.Name},
				Target:   TaskEndpoint{Namespace: cfg.Target.Namespace, Repo: targetRepo},
				Branches: repo.Branches,
				Tags:     repo.Tags,
			})
		}
	}

	return MigrationPlan{
		GeneratedAt: time.Now().UTC(),
		Source:      ProviderRef{Provider: cfg.Source.Provider, Namespace: cfg.Source.Namespace},
		Target:      ProviderRef{Provider: cfg.Target.Provider, Namespace: cfg.Target.Namespace},
		Tasks:       tasks,
	}, nil
}
