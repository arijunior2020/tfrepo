package core

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/arijunior2020/tfrepo/internal/provider"
	"github.com/arijunior2020/tfrepo/internal/security"
)

// IssuesMigrateProviders holds the source and target providers for issue migration.
type IssuesMigrateProviders struct {
	Source provider.RepositoryProvider
	Target provider.RepositoryProvider
}

// MigrateIssues migrates issues from source to target for each task in plan.
// milestoneMapsByTask maps task ID to MilestoneIDMap for milestone ID translation.
// Pass nil to skip milestone translation.
func MigrateIssues(ctx context.Context, plan MigrationPlan, providers IssuesMigrateProviders, milestoneMapsByTask map[string]MilestoneIDMap) (IssuesReport, error) {
	results := make([]IssuesMigrateResult, 0, len(plan.Tasks))
	for _, task := range plan.Tasks {
		results = append(results, migrateRepoIssues(ctx, task, providers, milestoneMapsByTask[task.ID]))
	}
	return IssuesReport{GeneratedAt: time.Now().UTC(), Results: results}, nil
}

func migrateRepoIssues(ctx context.Context, task MigrationTask, providers IssuesMigrateProviders, milestoneMap MilestoneIDMap) IssuesMigrateResult {
	result := IssuesMigrateResult{ID: task.ID, Status: "success"}

	sourceIssues, err := providers.Source.ListIssues(ctx, task.Source.Namespace, task.Source.Repo)
	if err != nil {
		result.Status = "failed"
		result.Errors = append(result.Errors, security.Redact(fmt.Sprintf("list source issues: %v", err)))
		return result
	}

	targetIssues, err := providers.Target.ListIssues(ctx, task.Target.Namespace, task.Target.Repo)
	if err != nil {
		result.Status = "failed"
		result.Errors = append(result.Errors, security.Redact(fmt.Sprintf("list target issues: %v", err)))
		return result
	}

	existing := make(map[string]bool, len(targetIssues))
	for _, i := range targetIssues {
		existing[i.Title] = true
	}

	for _, src := range sourceIssues {
		if existing[src.Title] {
			continue
		}
		toCreate := src
		toCreate.MilestoneExternalID = translateMilestone(src.MilestoneExternalID, milestoneMap)

		if _, err := providers.Target.CreateIssue(ctx, task.Target.Namespace, task.Target.Repo, toCreate); err != nil {
			result.Status = "failed"
			result.Errors = append(result.Errors, security.Redact(fmt.Sprintf("create issue %q: %v", src.Title, err)))
			continue
		}
		result.IssuesCreated++
	}

	return result
}

// translateMilestone looks up sourceID in milestoneMap and returns the target ID.
// Returns nil if sourceID is nil, milestoneMap is nil, or the key is not found.
func translateMilestone(sourceID *int64, milestoneMap MilestoneIDMap) *int64 {
	if sourceID == nil || len(milestoneMap) == 0 {
		return nil
	}
	key := strconv.FormatInt(*sourceID, 10)
	targetID, ok := milestoneMap[key]
	if !ok {
		return nil
	}
	return &targetID
}
