package core

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/arijunior2020/tfrepo/internal/provider"
	"github.com/arijunior2020/tfrepo/internal/security"
)

type LabelsMigrateProviders struct {
	Source provider.RepositoryProvider
	Target provider.RepositoryProvider
}

func MigrateLabelsAndMilestones(ctx context.Context, plan MigrationPlan, providers LabelsMigrateProviders) (LabelsReport, error) {
	results := make([]LabelsMigrateResult, 0, len(plan.Tasks))
	for _, task := range plan.Tasks {
		results = append(results, migrateRepoLabelsAndMilestones(ctx, task, providers))
	}
	return LabelsReport{
		GeneratedAt: time.Now().UTC(),
		Results:     results,
	}, nil
}

func migrateRepoLabelsAndMilestones(ctx context.Context, task MigrationTask, providers LabelsMigrateProviders) LabelsMigrateResult {
	result := LabelsMigrateResult{
		ID:             task.ID,
		MilestoneIDMap: make(map[string]int64),
	}

	labelsCreated, labelsErrs := migrateLabels(ctx, task, providers)
	result.LabelsCreated = labelsCreated
	result.Errors = append(result.Errors, labelsErrs...)

	milestonesCreated, idMap, msErrs := migrateMilestones(ctx, task, providers)
	result.MilestonesCreated = milestonesCreated
	for k, v := range idMap {
		result.MilestoneIDMap[k] = v
	}
	result.Errors = append(result.Errors, msErrs...)

	if len(result.Errors) > 0 {
		result.Status = "failed"
	} else {
		result.Status = "success"
	}
	return result
}

func migrateLabels(ctx context.Context, task MigrationTask, providers LabelsMigrateProviders) (created int, errs []string) {
	sourceLabels, err := providers.Source.ListLabels(ctx, task.Source.Namespace, task.Source.Repo)
	if err != nil {
		return 0, []string{fmt.Sprintf("listar labels de origem %s: %s", task.ID, security.Redact(err.Error()))}
	}
	targetLabels, err := providers.Target.ListLabels(ctx, task.Target.Namespace, task.Target.Repo)
	if err != nil {
		return 0, []string{fmt.Sprintf("listar labels de destino %s: %s", task.ID, security.Redact(err.Error()))}
	}

	existing := make(map[string]bool, len(targetLabels))
	for _, l := range targetLabels {
		existing[l.Name] = true
	}

	for _, label := range sourceLabels {
		if existing[label.Name] {
			continue
		}
		if _, err := providers.Target.CreateLabel(ctx, task.Target.Namespace, task.Target.Repo, label); err != nil {
			errs = append(errs, fmt.Sprintf("criar label %q em %s: %s", label.Name, task.ID, security.Redact(err.Error())))
			continue
		}
		created++
	}
	return
}

func migrateMilestones(ctx context.Context, task MigrationTask, providers LabelsMigrateProviders) (createdCount int, idMap map[string]int64, errs []string) {
	idMap = make(map[string]int64)

	sourceMilestones, err := providers.Source.ListMilestones(ctx, task.Source.Namespace, task.Source.Repo)
	if err != nil {
		errs = []string{fmt.Sprintf("listar milestones de origem %s: %s", task.ID, security.Redact(err.Error()))}
		return
	}
	targetMilestones, err := providers.Target.ListMilestones(ctx, task.Target.Namespace, task.Target.Repo)
	if err != nil {
		errs = []string{fmt.Sprintf("listar milestones de destino %s: %s", task.ID, security.Redact(err.Error()))}
		return
	}

	existing := make(map[string]provider.Milestone, len(targetMilestones))
	for _, m := range targetMilestones {
		existing[m.Title] = m
	}

	for _, ms := range sourceMilestones {
		sourceKey := strconv.FormatInt(ms.ExternalID, 10)
		if target, ok := existing[ms.Title]; ok {
			idMap[sourceKey] = target.ExternalID
			continue
		}
		created, err := providers.Target.CreateMilestone(ctx, task.Target.Namespace, task.Target.Repo, ms)
		if err != nil {
			errs = append(errs, fmt.Sprintf("criar milestone %q em %s: %s", ms.Title, task.ID, security.Redact(err.Error())))
			continue
		}
		idMap[sourceKey] = created.ExternalID
		createdCount++
	}
	return
}
