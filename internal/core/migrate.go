package core

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/arijunior2020/tfrepo/internal/provider"
	"github.com/arijunior2020/tfrepo/internal/security"
)

// migrateRepoDirName is the directory created inside each task's workspace
// by security.Clone, and pushed from by security.Push.
const migrateRepoDirName = "repo.git"

// migrateTargetVisibility is the visibility assigned to repositories created
// on the target provider during migration.
const migrateTargetVisibility = provider.VisibilityPrivate

// MigrateProviders groups the source and target providers used by Migrate
// and Validate.
type MigrateProviders struct {
	Source provider.RepositoryProvider
	Target provider.RepositoryProvider
}

// MigrateOptions configures Migrate.
type MigrateOptions struct {
	// DryRun validates that each task's target namespace is reachable
	// without cloning, pushing, or creating a workspace.
	DryRun bool

	// Concurrency is the maximum number of tasks migrated at once. Values
	// below 1 are treated as 1.
	Concurrency int

	// Workspaces creates and cleans up the temporary directory used for
	// each task's mirror clone. Required unless DryRun is true.
	Workspaces *security.Manager
}

// Migrate executes migrationPlan, mirror-cloning each source repository and
// pushing it to the target using up to opts.Concurrency goroutines. Per-task
// failures are recorded as "failed" results rather than aborting the run.
func Migrate(ctx context.Context, migrationPlan MigrationPlan, providers MigrateProviders, opts MigrateOptions) (MigrationReport, error) {
	if !opts.DryRun && opts.Workspaces == nil {
		return MigrationReport{}, errors.New("MigrateOptions.Workspaces is required when DryRun is false")
	}

	concurrency := opts.Concurrency
	if concurrency < 1 {
		concurrency = 1
	}

	results := make([]MigrationResult, len(migrationPlan.Tasks))

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)

	for i, task := range migrationPlan.Tasks {
		g.Go(func() error {
			if opts.DryRun {
				results[i] = runDryRun(gctx, task, providers.Target)
			} else {
				results[i] = runMigration(gctx, task, providers, opts.Workspaces)
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return MigrationReport{}, err
	}

	return MigrationReport{GeneratedAt: time.Now().UTC(), Results: results}, nil
}

// runDryRun validates that task's target namespace is reachable without
// migrating anything.
func runDryRun(ctx context.Context, task MigrationTask, target provider.RepositoryProvider) MigrationResult {
	startedAt := time.Now().UTC()

	if _, err := target.ListRepositories(ctx, task.Target.Namespace); err != nil {
		return MigrationResult{
			ID:         task.ID,
			Status:     "failed",
			StartedAt:  startedAt,
			FinishedAt: time.Now().UTC(),
			Error:      security.Redact(err.Error()),
		}
	}

	return MigrationResult{
		ID:         task.ID,
		Status:     "dry-run",
		StartedAt:  startedAt,
		FinishedAt: time.Now().UTC(),
	}
}

// runMigration mirror-clones task's source repository into a temporary
// workspace, ensures the target repository exists, and pushes the mirror to
// the target.
func runMigration(ctx context.Context, task MigrationTask, providers MigrateProviders, workspaces *security.Manager) MigrationResult {
	startedAt := time.Now().UTC()

	if err := migrateTask(ctx, task, providers, workspaces); err != nil {
		return MigrationResult{
			ID:         task.ID,
			Status:     "failed",
			StartedAt:  startedAt,
			FinishedAt: time.Now().UTC(),
			Error:      security.Redact(err.Error()),
		}
	}

	return MigrationResult{
		ID:         task.ID,
		Status:     "success",
		StartedAt:  startedAt,
		FinishedAt: time.Now().UTC(),
	}
}

func migrateTask(ctx context.Context, task MigrationTask, providers MigrateProviders, workspaces *security.Manager) error {
	workspace, err := workspaces.Create()
	if err != nil {
		return err
	}
	defer workspace.Cleanup()

	repoDir := filepath.Join(workspace.Path, migrateRepoDirName)

	sourceURL := providers.Source.GetAuthenticatedCloneURL(task.Source.Namespace, task.Source.Repo)
	if err := security.Clone(ctx, sourceURL, repoDir); err != nil {
		return err
	}

	if err := ensureTargetRepository(ctx, providers.Target, task); err != nil {
		return err
	}

	targetURL := providers.Target.GetAuthenticatedCloneURL(task.Target.Namespace, task.Target.Repo)
	return security.Push(ctx, repoDir, targetURL)
}

// ensureTargetRepository creates task's target repository if it doesn't
// already exist.
func ensureTargetRepository(ctx context.Context, target provider.RepositoryProvider, task MigrationTask) error {
	existing, err := target.ListRepositories(ctx, task.Target.Namespace)
	if err != nil {
		return err
	}

	for _, repo := range existing {
		if repo.Name == task.Target.Repo {
			return nil
		}
	}

	_, err = target.CreateRepository(ctx, task.Target.Namespace, provider.CreateRepositoryInput{
		Name:       task.Target.Repo,
		Visibility: migrateTargetVisibility,
	})
	return err
}
