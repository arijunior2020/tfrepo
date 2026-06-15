package core

import (
	"context"
	"sort"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/arijunior2020/tfrepo/internal/provider"
)

// Validate compares GetRepositoryState between the source and target for
// each task in migrationPlan. If report is non-nil, tasks whose result has
// status "failed" are reported as "skipped" without calling either
// provider. If either provider's GetRepositoryState call errors for a task,
// Validate returns immediately with that error and no results.
func Validate(ctx context.Context, migrationPlan MigrationPlan, providers MigrateProviders, report *MigrationReport) (ValidationReport, error) {
	failedIDs := make(map[string]struct{})
	if report != nil {
		for _, result := range report.Results {
			if result.Status == "failed" {
				failedIDs[result.ID] = struct{}{}
			}
		}
	}

	results := make([]ValidationResult, 0, len(migrationPlan.Tasks))

	for _, task := range migrationPlan.Tasks {
		if _, failed := failedIDs[task.ID]; failed {
			results = append(results, ValidationResult{
				ID:          task.ID,
				Status:      "skipped",
				Divergences: []RefDivergence{},
				Reason:      "migration failed",
			})
			continue
		}

		var sourceState, targetState provider.RepositoryState

		g, gctx := errgroup.WithContext(ctx)
		g.Go(func() error {
			var err error
			sourceState, err = providers.Source.GetRepositoryState(gctx, task.Source.Namespace, task.Source.Repo)
			return err
		})
		g.Go(func() error {
			var err error
			targetState, err = providers.Target.GetRepositoryState(gctx, task.Target.Namespace, task.Target.Repo)
			return err
		})

		if err := g.Wait(); err != nil {
			return ValidationReport{}, err
		}

		divergences := append(
			compareRefs("branch", sourceState.Branches, targetState.Branches),
			compareRefs("tag", sourceState.Tags, targetState.Tags)...,
		)

		status := "ok"
		if len(divergences) > 0 {
			status = "diverged"
		}

		results = append(results, ValidationResult{ID: task.ID, Status: status, Divergences: divergences})
	}

	return ValidationReport{GeneratedAt: time.Now().UTC(), Results: results}, nil
}

// compareRefs returns one RefDivergence for every ref name in source or
// target whose commit SHA differs, including names present in only one of
// the two maps. Names are sorted for deterministic output.
func compareRefs(refType string, source, target map[string]string) []RefDivergence {
	names := make(map[string]struct{}, len(source)+len(target))
	for name := range source {
		names[name] = struct{}{}
	}
	for name := range target {
		names[name] = struct{}{}
	}

	sorted := make([]string, 0, len(names))
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)

	divergences := []RefDivergence{}
	for _, name := range sorted {
		sourceSHA, sourceOK := source[name]
		targetSHA, targetOK := target[name]

		if sourceOK && targetOK && sourceSHA == targetSHA {
			continue
		}

		var sourcePtr, targetPtr *string
		if sourceOK {
			sourcePtr = &sourceSHA
		}
		if targetOK {
			targetPtr = &targetSHA
		}

		divergences = append(divergences, RefDivergence{Type: refType, Name: name, SourceSHA: sourcePtr, TargetSHA: targetPtr})
	}

	return divergences
}
