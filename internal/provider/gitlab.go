package provider

import (
	"context"
	"fmt"
	"math"
	"net/url"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

const gitlabDefaultBaseURL = "https://gitlab.com"

// listPerPage is the page size used for all paginated GitLab API requests.
const listPerPage = 100

// GitLabProvider implements RepositoryProvider against the GitLab REST API
// using gitlab.com/gitlab-org/api/client-go.
type GitLabProvider struct {
	client  *gitlab.Client
	token   string
	baseURL string
}

// NewGitLabProvider creates a GitLabProvider authenticated with token.
// If baseURL is empty, it defaults to https://gitlab.com.
func NewGitLabProvider(token, baseURL string) (*GitLabProvider, error) {
	if baseURL == "" {
		baseURL = gitlabDefaultBaseURL
	}

	client, err := gitlab.NewClient(token, gitlab.WithBaseURL(baseURL))
	if err != nil {
		return nil, fmt.Errorf("create gitlab client: %w", err)
	}

	return &GitLabProvider{
		client:  client,
		token:   token,
		baseURL: baseURL,
	}, nil
}

// Name returns the provider's identifier.
func (p *GitLabProvider) Name() string {
	return "gitlab"
}

// GetAuthenticatedCloneURL returns an HTTPS clone URL that embeds the
// provider token for authentication.
func (p *GitLabProvider) GetAuthenticatedCloneURL(namespace, repo string) string {
	host := "gitlab.com"
	if u, err := url.Parse(p.baseURL); err == nil && u.Host != "" {
		host = u.Host
	}

	return fmt.Sprintf("https://oauth2:%s@%s/%s/%s.git", p.token, host, namespace, repo)
}

// ListNamespaces returns every namespace (user or group, including
// subgroups) visible to the authenticated token.
func (p *GitLabProvider) ListNamespaces(ctx context.Context) ([]Namespace, error) {
	var result []Namespace

	opts := &gitlab.ListNamespacesOptions{
		ListOptions: gitlab.ListOptions{PerPage: listPerPage},
	}

	for {
		namespaces, resp, err := p.client.Namespaces.ListNamespaces(opts, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list namespaces: %w", err)
		}

		for _, ns := range namespaces {
			result = append(result, Namespace{
				Slug: ns.FullPath,
				Name: ns.Name,
				Kind: namespaceKindFromString(ns.Kind),
			})
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return result, nil
}

// namespaceKindFromString maps GitLab's namespace "kind" string to the
// shared NamespaceKind type.
func namespaceKindFromString(kind string) NamespaceKind {
	switch kind {
	case "group":
		return NamespaceGroup
	case "user":
		return NamespaceUser
	default:
		return NamespaceKind(kind)
	}
}

// ListRepositories lists the repositories visible to the token within the
// given namespace.
//
// This fixes a bug present in the v0.1.0 Node implementation: that version
// resolved "another user's personal namespace" by listing the token owner's
// own projects and filtering by namespace, which silently returned an empty
// slice for any namespace that wasn't the token owner's own. This
// implementation instead branches on the namespace kind:
//
//  1. "group"       -> list all projects within that group/subgroup that the
//     token can see (Groups.ListGroupProjects).
//  2. "user", self  -> list the authenticated user's own projects, including
//     private ones (Projects.ListProjects with Owned=true).
//  3. "user", other -> list that user's publicly visible projects
//     (Projects.ListUserProjects).
func (p *GitLabProvider) ListRepositories(ctx context.Context, namespace string) ([]RepositorySummary, error) {
	projects, err := p.listProjectsForNamespace(ctx, namespace)
	if err != nil {
		return nil, err
	}

	summaries := make([]RepositorySummary, 0, len(projects))
	for _, project := range projects {
		summaries = append(summaries, toRepositorySummary(project))
	}
	return summaries, nil
}

// listProjectsForNamespace implements the 3-branch namespace resolution
// described in ListRepositories.
func (p *GitLabProvider) listProjectsForNamespace(ctx context.Context, namespace string) ([]*gitlab.Project, error) {
	ns, _, err := p.client.Namespaces.GetNamespace(namespace, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("get namespace %q: %w", namespace, err)
	}

	switch ns.Kind {
	case "group":
		return p.listAllGroupProjects(ctx, namespace)
	case "user":
		currentUser, _, err := p.client.Users.CurrentUser(gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("get current user: %w", err)
		}

		if currentUser.Username == namespace {
			// Branch 2: the token owner's own personal namespace. List
			// their own projects, including private ones.
			return p.listOwnProjects(ctx)
		}

		// Branch 3: another user's personal namespace. List that user's
		// publicly visible projects.
		return p.listUserProjects(ctx, namespace)
	default:
		return nil, fmt.Errorf("namespace %q has unsupported kind %q", namespace, ns.Kind)
	}
}

// listAllGroupProjects lists every project within the given group/subgroup
// namespace that the authenticated token can see.
func (p *GitLabProvider) listAllGroupProjects(ctx context.Context, namespace string) ([]*gitlab.Project, error) {
	var result []*gitlab.Project

	opts := &gitlab.ListGroupProjectsOptions{
		ListOptions: gitlab.ListOptions{PerPage: listPerPage},
	}

	for {
		projects, resp, err := p.client.Groups.ListGroupProjects(namespace, opts, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list group projects for %q: %w", namespace, err)
		}

		result = append(result, projects...)

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return result, nil
}

// listOwnProjects lists all projects owned by the authenticated user,
// including private ones.
func (p *GitLabProvider) listOwnProjects(ctx context.Context) ([]*gitlab.Project, error) {
	var result []*gitlab.Project

	opts := &gitlab.ListProjectsOptions{
		ListOptions: gitlab.ListOptions{PerPage: listPerPage},
		Owned:       gitlab.Ptr(true),
		Statistics:  gitlab.Ptr(true),
	}

	for {
		projects, resp, err := p.client.Projects.ListProjects(opts, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list own projects: %w", err)
		}

		result = append(result, projects...)

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return result, nil
}

// listUserProjects lists the publicly visible projects belonging to the
// given username's personal namespace.
func (p *GitLabProvider) listUserProjects(ctx context.Context, username string) ([]*gitlab.Project, error) {
	var result []*gitlab.Project

	opts := &gitlab.ListProjectsOptions{
		ListOptions: gitlab.ListOptions{PerPage: listPerPage},
		Statistics:  gitlab.Ptr(true),
	}

	for {
		projects, resp, err := p.client.Projects.ListUserProjects(username, opts, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list projects for user %q: %w", username, err)
		}

		result = append(result, projects...)

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return result, nil
}

// toRepositorySummary maps a GitLab project to a RepositorySummary.
func toRepositorySummary(project *gitlab.Project) RepositorySummary {
	var sizeKB int64
	if project.Statistics != nil {
		sizeKB = int64(math.Round(float64(project.Statistics.RepositorySize) / 1024))
	}

	namespace := ""
	if project.Namespace != nil {
		namespace = project.Namespace.FullPath
	}

	return RepositorySummary{
		Name:          project.Path,
		Namespace:     namespace,
		DefaultBranch: project.DefaultBranch,
		Visibility:    Visibility(project.Visibility),
		SizeKB:        sizeKB,
	}
}

// GetRepositoryDetails returns the repository summary plus its branch and
// tag names.
func (p *GitLabProvider) GetRepositoryDetails(ctx context.Context, namespace, repo string) (RepositoryDetails, error) {
	projectID := namespace + "/" + repo

	project, _, err := p.client.Projects.GetProject(projectID, &gitlab.GetProjectOptions{
		Statistics: gitlab.Ptr(true),
	}, gitlab.WithContext(ctx))
	if err != nil {
		return RepositoryDetails{}, fmt.Errorf("get project %q: %w", projectID, err)
	}

	branchNames, err := p.listAllBranchNames(ctx, projectID)
	if err != nil {
		return RepositoryDetails{}, err
	}

	tagNames, err := p.listAllTagNames(ctx, projectID)
	if err != nil {
		return RepositoryDetails{}, err
	}

	return RepositoryDetails{
		RepositorySummary: toRepositorySummary(project),
		Branches:          branchNames,
		Tags:              tagNames,
	}, nil
}

// GetRepositoryState returns a map of branch and tag names to their current
// commit SHAs.
func (p *GitLabProvider) GetRepositoryState(ctx context.Context, namespace, repo string) (RepositoryState, error) {
	projectID := namespace + "/" + repo

	branches, err := p.listAllBranches(ctx, projectID)
	if err != nil {
		return RepositoryState{}, err
	}

	tags, err := p.listAllTags(ctx, projectID)
	if err != nil {
		return RepositoryState{}, err
	}

	branchSHAs := make(map[string]string, len(branches))
	for _, branch := range branches {
		if branch.Commit != nil {
			branchSHAs[branch.Name] = branch.Commit.ID
		}
	}

	tagSHAs := make(map[string]string, len(tags))
	for _, tag := range tags {
		if tag.Commit != nil {
			tagSHAs[tag.Name] = tag.Commit.ID
		}
	}

	return RepositoryState{
		Branches: branchSHAs,
		Tags:     tagSHAs,
	}, nil
}

// listAllBranches returns every branch for the given project, paginated.
func (p *GitLabProvider) listAllBranches(ctx context.Context, projectID string) ([]*gitlab.Branch, error) {
	var result []*gitlab.Branch

	opts := &gitlab.ListBranchesOptions{
		ListOptions: gitlab.ListOptions{PerPage: listPerPage},
	}

	for {
		branches, resp, err := p.client.Branches.ListBranches(projectID, opts, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list branches for %q: %w", projectID, err)
		}

		result = append(result, branches...)

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return result, nil
}

// listAllTags returns every tag for the given project, paginated.
func (p *GitLabProvider) listAllTags(ctx context.Context, projectID string) ([]*gitlab.Tag, error) {
	var result []*gitlab.Tag

	opts := &gitlab.ListTagsOptions{
		ListOptions: gitlab.ListOptions{PerPage: listPerPage},
	}

	for {
		tags, resp, err := p.client.Tags.ListTags(projectID, opts, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list tags for %q: %w", projectID, err)
		}

		result = append(result, tags...)

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return result, nil
}

// listAllBranchNames returns only the names of every branch for the given
// project.
func (p *GitLabProvider) listAllBranchNames(ctx context.Context, projectID string) ([]string, error) {
	branches, err := p.listAllBranches(ctx, projectID)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(branches))
	for _, branch := range branches {
		names = append(names, branch.Name)
	}
	return names, nil
}

// listAllTagNames returns only the names of every tag for the given project.
func (p *GitLabProvider) listAllTagNames(ctx context.Context, projectID string) ([]string, error) {
	tags, err := p.listAllTags(ctx, projectID)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(tags))
	for _, tag := range tags {
		names = append(names, tag.Name)
	}
	return names, nil
}

// CreateRepository creates a new project within the given namespace.
func (p *GitLabProvider) CreateRepository(ctx context.Context, namespace string, input CreateRepositoryInput) (RepositorySummary, error) {
	ns, _, err := p.client.Namespaces.GetNamespace(namespace, gitlab.WithContext(ctx))
	if err != nil {
		return RepositorySummary{}, fmt.Errorf("get namespace %q: %w", namespace, err)
	}

	project, _, err := p.client.Projects.CreateProject(&gitlab.CreateProjectOptions{
		Name:        gitlab.Ptr(input.Name),
		Path:        gitlab.Ptr(input.Name),
		NamespaceID: gitlab.Ptr(ns.ID),
		Visibility:  gitlab.Ptr(gitlab.VisibilityValue(input.Visibility)),
		Description: gitlab.Ptr(input.Description),
	}, gitlab.WithContext(ctx))
	if err != nil {
		return RepositorySummary{}, fmt.Errorf("create project %q in namespace %q: %w", input.Name, namespace, err)
	}

	return toRepositorySummary(project), nil
}

func (p *GitLabProvider) ListLabels(ctx context.Context, namespace, repo string) ([]Label, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *GitLabProvider) CreateLabel(ctx context.Context, namespace, repo string, label Label) (Label, error) {
	return Label{}, fmt.Errorf("not implemented")
}

func (p *GitLabProvider) ListMilestones(ctx context.Context, namespace, repo string) ([]Milestone, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *GitLabProvider) CreateMilestone(ctx context.Context, namespace, repo string, m Milestone) (Milestone, error) {
	return Milestone{}, fmt.Errorf("not implemented")
}

// Compile-time check that GitLabProvider implements RepositoryProvider.
var _ RepositoryProvider = (*GitLabProvider)(nil)
