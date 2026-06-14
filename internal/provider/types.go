package provider

import "context"

// NamespaceKind identifies what kind of account a Namespace represents.
type NamespaceKind string

const (
	NamespaceOrganization NamespaceKind = "organization"
	NamespaceUser         NamespaceKind = "user"
	NamespaceGroup        NamespaceKind = "group"
)

// Visibility is the access level of a repository.
type Visibility string

const (
	VisibilityPublic   Visibility = "public"
	VisibilityPrivate  Visibility = "private"
	VisibilityInternal Visibility = "internal"
)

// Namespace is an account or group that can own repositories.
type Namespace struct {
	Slug string        `json:"slug"` // org login (GitHub) or group path (GitLab)
	Name string        `json:"name"`
	Kind NamespaceKind `json:"kind"`
}

// RepositorySummary is the provider-neutral view of a repository used when
// listing repositories.
type RepositorySummary struct {
	Name          string     `json:"name"`
	Namespace     string     `json:"namespace"`
	DefaultBranch string     `json:"defaultBranch"`
	Visibility    Visibility `json:"visibility"`
	SizeKB        int64      `json:"sizeKb"`
}

// RepositoryDetails extends RepositorySummary with the full list of branch
// and tag names.
type RepositoryDetails struct {
	RepositorySummary
	Branches []string `json:"branches"`
	Tags     []string `json:"tags"`
}

// RepositoryState maps branch and tag names to their current commit SHAs.
type RepositoryState struct {
	Branches map[string]string `json:"branches"` // branch -> commit SHA
	Tags     map[string]string `json:"tags"`     // tag -> commit SHA
}

// CreateRepositoryInput describes a repository to create on the destination
// provider.
type CreateRepositoryInput struct {
	Name        string
	Visibility  Visibility
	Description string
}

// RepositoryProvider is the interface implemented by GitHubProvider and
// GitLabProvider.
type RepositoryProvider interface {
	Name() string // "github" | "gitlab"

	// Scan
	ListNamespaces(ctx context.Context) ([]Namespace, error)
	ListRepositories(ctx context.Context, namespace string) ([]RepositorySummary, error)
	GetRepositoryDetails(ctx context.Context, namespace, repo string) (RepositoryDetails, error)

	// Migrate (destination side)
	CreateRepository(ctx context.Context, namespace string, input CreateRepositoryInput) (RepositorySummary, error)

	// Git transport — token-embedded URL, used only in memory, never logged
	GetAuthenticatedCloneURL(namespace, repo string) string

	// Validate
	GetRepositoryState(ctx context.Context, namespace, repo string) (RepositoryState, error)
}
