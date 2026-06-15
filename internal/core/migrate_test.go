package core

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/arijunior2020/tfrepo/internal/provider"
	"github.com/arijunior2020/tfrepo/internal/security"
)

// createRepositoryCall records a single CreateRepository invocation.
type createRepositoryCall struct {
	namespace string
	input     provider.CreateRepositoryInput
}

// fakeMigrateProvider is a minimal provider.RepositoryProvider used to test
// Migrate without making network calls (other than local git operations via
// security.Clone/Push, which accept filesystem paths as "URLs").
type fakeMigrateProvider struct {
	name string

	listRepositoriesResult []provider.RepositorySummary
	listRepositoriesErr    error

	createRepositoryErr   error
	createRepositoryCalls []createRepositoryCall

	cloneURL string
}

func (f *fakeMigrateProvider) Name() string { return f.name }

func (f *fakeMigrateProvider) ListNamespaces(ctx context.Context) ([]provider.Namespace, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeMigrateProvider) ListRepositories(ctx context.Context, namespace string) ([]provider.RepositorySummary, error) {
	return f.listRepositoriesResult, f.listRepositoriesErr
}

func (f *fakeMigrateProvider) GetRepositoryDetails(ctx context.Context, namespace, repo string) (provider.RepositoryDetails, error) {
	return provider.RepositoryDetails{}, errors.New("not implemented")
}

func (f *fakeMigrateProvider) CreateRepository(ctx context.Context, namespace string, input provider.CreateRepositoryInput) (provider.RepositorySummary, error) {
	f.createRepositoryCalls = append(f.createRepositoryCalls, createRepositoryCall{namespace: namespace, input: input})
	if f.createRepositoryErr != nil {
		return provider.RepositorySummary{}, f.createRepositoryErr
	}
	return provider.RepositorySummary{Name: input.Name, Namespace: namespace, Visibility: input.Visibility}, nil
}

func (f *fakeMigrateProvider) GetAuthenticatedCloneURL(namespace, repo string) string {
	return f.cloneURL
}

func (f *fakeMigrateProvider) GetRepositoryState(ctx context.Context, namespace, repo string) (provider.RepositoryState, error) {
	return provider.RepositoryState{}, errors.New("not implemented")
}

var _ provider.RepositoryProvider = (*fakeMigrateProvider)(nil)

func baseMigratePlan() MigrationPlan {
	return MigrationPlan{
		GeneratedAt: time.Now().UTC(),
		Source:      ProviderRef{Provider: "github", Namespace: "my-org"},
		Target:      ProviderRef{Provider: "gitlab", Namespace: "my-group"},
		Tasks: []MigrationTask{
			{
				ID:       "repo-a",
				Source:   TaskEndpoint{Namespace: "my-org", Repo: "repo-a"},
				Target:   TaskEndpoint{Namespace: "my-group", Repo: "repo-a"},
				Branches: []string{"main"},
				Tags:     []string{"v1.0.0"},
			},
		},
	}
}

// initGitRepo initializes a git repository in dir with one commit on "main"
// and returns that commit's SHA.
func initGitRepo(t *testing.T, dir string) string {
	t.Helper()

	runGit(t, dir, "init", "--initial-branch=main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "initial commit")

	return strings.TrimSpace(runGit(t, dir, "rev-parse", "main"))
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}

	return string(output)
}

func TestMigrateClonesAndPushesMirror(t *testing.T) {
	sourceDir := t.TempDir()
	sourceHead := initGitRepo(t, sourceDir)

	targetDir := t.TempDir()
	runGit(t, targetDir, "init", "--bare")

	source := &fakeMigrateProvider{name: "github", cloneURL: sourceDir}
	target := &fakeMigrateProvider{
		name:                   "gitlab",
		listRepositoriesResult: []provider.RepositorySummary{},
		cloneURL:               targetDir,
	}

	report, err := Migrate(context.Background(), baseMigratePlan(), MigrateProviders{Source: source, Target: target}, MigrateOptions{Concurrency: 1, Workspaces: security.NewManager()})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	if len(report.Results) != 1 {
		t.Fatalf("len(Results) = %d, want 1", len(report.Results))
	}
	if report.Results[0].Status != "success" {
		t.Errorf("Status = %q, want %q (error: %s)", report.Results[0].Status, "success", report.Results[0].Error)
	}

	if len(target.createRepositoryCalls) != 1 {
		t.Fatalf("CreateRepository calls = %d, want 1", len(target.createRepositoryCalls))
	}
	call := target.createRepositoryCalls[0]
	if call.namespace != "my-group" {
		t.Errorf("namespace = %q, want %q", call.namespace, "my-group")
	}
	wantInput := provider.CreateRepositoryInput{Name: "repo-a", Visibility: provider.VisibilityPrivate}
	if call.input != wantInput {
		t.Errorf("input = %+v, want %+v", call.input, wantInput)
	}

	targetHead := strings.TrimSpace(runGit(t, targetDir, "rev-parse", "main"))
	if targetHead != sourceHead {
		t.Errorf("target HEAD = %q, want %q", targetHead, sourceHead)
	}
}

func TestMigrateDoesNotCreateTargetRepositoryIfExists(t *testing.T) {
	sourceDir := t.TempDir()
	initGitRepo(t, sourceDir)

	targetDir := t.TempDir()
	runGit(t, targetDir, "init", "--bare")

	source := &fakeMigrateProvider{name: "github", cloneURL: sourceDir}
	target := &fakeMigrateProvider{
		name: "gitlab",
		listRepositoriesResult: []provider.RepositorySummary{
			{Name: "repo-a", Namespace: "my-group", DefaultBranch: "main", Visibility: provider.VisibilityPrivate},
		},
		cloneURL: targetDir,
	}

	report, err := Migrate(context.Background(), baseMigratePlan(), MigrateProviders{Source: source, Target: target}, MigrateOptions{Concurrency: 1, Workspaces: security.NewManager()})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	if report.Results[0].Status != "success" {
		t.Errorf("Status = %q, want %q (error: %s)", report.Results[0].Status, "success", report.Results[0].Error)
	}
	if len(target.createRepositoryCalls) != 0 {
		t.Errorf("CreateRepository calls = %d, want 0", len(target.createRepositoryCalls))
	}
}

func TestMigrateRecordsFailedResultAndRedactsCredentialsOnCloneFailure(t *testing.T) {
	source := &fakeMigrateProvider{
		name:     "github",
		cloneURL: "https://x-access-token:ghp_secret@127.0.0.1/does-not-exist/repo-a.git",
	}
	target := &fakeMigrateProvider{name: "gitlab", listRepositoriesResult: []provider.RepositorySummary{}}

	report, err := Migrate(context.Background(), baseMigratePlan(), MigrateProviders{Source: source, Target: target}, MigrateOptions{Concurrency: 1, Workspaces: security.NewManager()})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	result := report.Results[0]
	if result.Status != "failed" {
		t.Fatalf("Status = %q, want %q (error: %s)", result.Status, "failed", result.Error)
	}
	if result.Error == "" {
		t.Fatal("Error is empty, want a clone failure message")
	}
	if strings.Contains(result.Error, "ghp_secret") {
		t.Errorf("Error contains secret: %q", result.Error)
	}
	if strings.Contains(result.Error, "x-access-token:ghp_secret@") {
		t.Errorf("Error contains embedded credentials: %q", result.Error)
	}
}

func TestMigrateDryRun(t *testing.T) {
	source := &fakeMigrateProvider{name: "github"}
	target := &fakeMigrateProvider{name: "gitlab", listRepositoriesResult: []provider.RepositorySummary{}}

	report, err := Migrate(context.Background(), baseMigratePlan(), MigrateProviders{Source: source, Target: target}, MigrateOptions{DryRun: true, Concurrency: 1})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	if report.Results[0].Status != "dry-run" {
		t.Errorf("Status = %q, want %q (error: %s)", report.Results[0].Status, "dry-run", report.Results[0].Error)
	}
	if len(target.createRepositoryCalls) != 0 {
		t.Errorf("CreateRepository calls = %d, want 0", len(target.createRepositoryCalls))
	}
}

func TestMigrateDryRunRecordsFailedResultWhenTargetNamespaceCheckFails(t *testing.T) {
	source := &fakeMigrateProvider{name: "github"}
	target := &fakeMigrateProvider{name: "gitlab", listRepositoriesErr: errors.New("404 Not Found")}

	report, err := Migrate(context.Background(), baseMigratePlan(), MigrateProviders{Source: source, Target: target}, MigrateOptions{DryRun: true, Concurrency: 1})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	result := report.Results[0]
	if result.Status != "failed" {
		t.Errorf("Status = %q, want %q", result.Status, "failed")
	}
	if result.Error != "404 Not Found" {
		t.Errorf("Error = %q, want %q", result.Error, "404 Not Found")
	}
}

func TestMigratePreservesTaskOrderUnderConcurrency(t *testing.T) {
	plan := MigrationPlan{
		Tasks: []MigrationTask{
			{ID: "repo-a", Source: TaskEndpoint{Namespace: "my-org", Repo: "repo-a"}, Target: TaskEndpoint{Namespace: "my-group", Repo: "repo-a"}},
			{ID: "repo-b", Source: TaskEndpoint{Namespace: "my-org", Repo: "repo-b"}, Target: TaskEndpoint{Namespace: "my-group", Repo: "repo-b"}},
			{ID: "repo-c", Source: TaskEndpoint{Namespace: "my-org", Repo: "repo-c"}, Target: TaskEndpoint{Namespace: "my-group", Repo: "repo-c"}},
		},
	}

	source := &fakeMigrateProvider{name: "github"}
	target := &fakeMigrateProvider{name: "gitlab", listRepositoriesResult: []provider.RepositorySummary{}}

	report, err := Migrate(context.Background(), plan, MigrateProviders{Source: source, Target: target}, MigrateOptions{DryRun: true, Concurrency: 2})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	var ids []string
	for _, r := range report.Results {
		ids = append(ids, r.ID)
	}
	want := []string{"repo-a", "repo-b", "repo-c"}
	if !reflect.DeepEqual(ids, want) {
		t.Errorf("Results order = %v, want %v", ids, want)
	}
}
