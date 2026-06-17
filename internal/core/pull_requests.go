package core

import (
	"context"
	"fmt"
	"time"

	"github.com/arijunior2020/tfrepo/internal/provider"
	"github.com/arijunior2020/tfrepo/internal/security"
)

// PRsMigrateProviders holds the source and target providers for PR migration.
type PRsMigrateProviders struct {
	Source provider.RepositoryProvider
	Target provider.RepositoryProvider
}

// MigratePullRequests migrates open pull requests from source to target for
// each task in plan. Closed and merged PRs are skipped — they are historical
// artifacts preserved in git history. Existing PRs in the target with the
// same title are skipped (idempotent).
func MigratePullRequests(ctx context.Context, plan MigrationPlan, providers PRsMigrateProviders) (PRsReport, error) {
	results := make([]PRsMigrateResult, 0, len(plan.Tasks))
	for _, task := range plan.Tasks {
		results = append(results, migrateRepoPullRequests(ctx, task, providers))
	}
	return PRsReport{GeneratedAt: time.Now().UTC(), Results: results}, nil
}

func migrateRepoPullRequests(ctx context.Context, task MigrationTask, providers PRsMigrateProviders) PRsMigrateResult {
	result := PRsMigrateResult{ID: task.ID, Status: "success"}

	sourcePRs, err := providers.Source.ListPullRequests(ctx, task.Source.Namespace, task.Source.Repo)
	if err != nil {
		result.Status = "failed"
		result.Errors = append(result.Errors, security.Redact(fmt.Sprintf("list source pull requests: %v", err)))
		return result
	}

	targetPRs, err := providers.Target.ListPullRequests(ctx, task.Target.Namespace, task.Target.Repo)
	if err != nil {
		result.Status = "failed"
		result.Errors = append(result.Errors, security.Redact(fmt.Sprintf("list target pull requests: %v", err)))
		return result
	}

	existing := make(map[string]bool, len(targetPRs))
	for _, pr := range targetPRs {
		existing[pr.Title] = true
	}

	for _, src := range sourcePRs {
		if existing[src.Title] {
			continue
		}
		if _, err := providers.Target.CreatePullRequest(ctx, task.Target.Namespace, task.Target.Repo, src); err != nil {
			result.Status = "failed"
			result.Errors = append(result.Errors, security.Redact(fmt.Sprintf("create pull request %q: %v", src.Title, err)))
			continue
		}
		result.PRsCreated++
	}

	return result
}
