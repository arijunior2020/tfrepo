package provider

import (
	"context"
	"fmt"
	"net/url"

	"github.com/google/go-github/v74/github"
)

const (
	defaultGitHubBaseURL = "https://api.github.com/"
	defaultGitHubHost    = "github.com"
)

// GitHubProvider implements RepositoryProvider for GitHub (github.com and
// GitHub Enterprise Server).
type GitHubProvider struct {
	client  *github.Client
	token   string
	baseURL string
}

// NewGitHubProvider constructs a GitHubProvider authenticated with token.
//
// If baseURL is empty, the provider talks to the public GitHub API
// (https://api.github.com/). If baseURL is non-empty, it is used verbatim as
// the API base URL (e.g. for GitHub Enterprise Server, something like
// "https://github.example.com/api/v3/").
func NewGitHubProvider(token, baseURL string) (*GitHubProvider, error) {
	client := github.NewClient(nil).WithAuthToken(token)

	if baseURL != "" {
		parsed, err := url.Parse(baseURL)
		if err != nil {
			return nil, fmt.Errorf("parse github base url %q: %w", baseURL, err)
		}
		if parsed.Path == "" || parsed.Path[len(parsed.Path)-1] != '/' {
			parsed.Path += "/"
		}
		client.BaseURL = parsed
	}

	effectiveBaseURL := baseURL
	if effectiveBaseURL == "" {
		effectiveBaseURL = defaultGitHubBaseURL
	}

	return &GitHubProvider{
		client:  client,
		token:   token,
		baseURL: effectiveBaseURL,
	}, nil
}

// Name returns the provider identifier.
func (p *GitHubProvider) Name() string {
	return "github"
}

// GetAuthenticatedCloneURL returns an HTTPS clone URL with the provider
// token embedded for non-interactive authentication.
func (p *GitHubProvider) GetAuthenticatedCloneURL(namespace, repo string) string {
	return fmt.Sprintf("https://x-access-token:%s@%s/%s/%s.git", p.token, p.gitHost(), namespace, repo)
}

// gitHost returns the git hostname used to build authenticated clone URLs:
// github.com for the default API base URL, or the host parsed from a custom
// (GitHub Enterprise) base URL.
func (p *GitHubProvider) gitHost() string {
	if p.baseURL == defaultGitHubBaseURL {
		return defaultGitHubHost
	}
	if parsed, err := url.Parse(p.baseURL); err == nil && parsed.Host != "" {
		return parsed.Host
	}
	return defaultGitHubHost
}

// ListNamespaces returns the authenticated user's namespace plus every
// organization the authenticated user belongs to.
func (p *GitHubProvider) ListNamespaces(ctx context.Context) ([]Namespace, error) {
	user, _, err := p.client.Users.Get(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("get authenticated user: %w", err)
	}

	namespaces := make([]Namespace, 0, 1)

	name := user.GetName()
	if name == "" {
		name = user.GetLogin()
	}
	namespaces = append(namespaces, Namespace{
		Slug: user.GetLogin(),
		Name: name,
		Kind: NamespaceUser,
	})

	opts := &github.ListOptions{PerPage: 100}
	for {
		orgs, resp, err := p.client.Organizations.List(ctx, "", opts)
		if err != nil {
			return nil, fmt.Errorf("list organizations for authenticated user: %w", err)
		}
		for _, org := range orgs {
			namespaces = append(namespaces, Namespace{
				Slug: org.GetLogin(),
				Name: org.GetLogin(),
				Kind: NamespaceOrganization,
			})
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return namespaces, nil
}

// ListRepositories lists the repositories visible to the authenticated user
// within the given namespace.
func (p *GitHubProvider) ListRepositories(ctx context.Context, namespace string) ([]RepositorySummary, error) {
	repos, err := p.listReposForNamespace(ctx, namespace)
	if err != nil {
		return nil, err
	}

	summaries := make([]RepositorySummary, 0, len(repos))
	for _, repo := range repos {
		summaries = append(summaries, p.toRepositorySummary(repo, namespace))
	}
	return summaries, nil
}

// listReposForNamespace resolves which listing to use based on the
// namespace's account type: organization namespaces use the org repository
// listing, the authenticated user's own namespace uses the "owner
// affiliation" listing (which includes private repositories), and any other
// namespace falls back to the public-repositories-only listing for that
// user.
func (p *GitHubProvider) listReposForNamespace(ctx context.Context, namespace string) ([]*github.Repository, error) {
	account, _, err := p.client.Users.Get(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("get account %s: %w", namespace, err)
	}

	if account.GetType() == "Organization" {
		var repos []*github.Repository
		opts := &github.RepositoryListByOrgOptions{
			ListOptions: github.ListOptions{PerPage: 100},
		}
		for {
			page, resp, err := p.client.Repositories.ListByOrg(ctx, namespace, opts)
			if err != nil {
				return nil, fmt.Errorf("list repositories for org %s: %w", namespace, err)
			}
			repos = append(repos, page...)
			if resp.NextPage == 0 {
				break
			}
			opts.Page = resp.NextPage
		}
		return repos, nil
	}

	// GET /users/{username}/repos only returns PUBLIC repos, even for the
	// token owner. To include private repos for the token owner's own
	// namespace, use GET /user/repos.
	authenticatedUser, _, err := p.client.Users.Get(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("get authenticated user: %w", err)
	}

	if authenticatedUser.GetLogin() == namespace {
		var repos []*github.Repository
		opts := &github.RepositoryListByAuthenticatedUserOptions{
			Affiliation: "owner",
			ListOptions: github.ListOptions{PerPage: 100},
		}
		for {
			page, resp, err := p.client.Repositories.ListByAuthenticatedUser(ctx, opts)
			if err != nil {
				return nil, fmt.Errorf("list repositories for authenticated user: %w", err)
			}
			repos = append(repos, page...)
			if resp.NextPage == 0 {
				break
			}
			opts.Page = resp.NextPage
		}
		return repos, nil
	}

	var repos []*github.Repository
	opts := &github.RepositoryListByUserOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}
	for {
		page, resp, err := p.client.Repositories.ListByUser(ctx, namespace, opts)
		if err != nil {
			return nil, fmt.Errorf("list repositories for user %s: %w", namespace, err)
		}
		repos = append(repos, page...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return repos, nil
}

// toRepositorySummary maps a go-github Repository onto the provider-neutral
// RepositorySummary.
func (p *GitHubProvider) toRepositorySummary(repo *github.Repository, namespace string) RepositorySummary {
	ns := namespace
	if owner := repo.GetOwner(); owner != nil && owner.GetLogin() != "" {
		ns = owner.GetLogin()
	}

	var visibility Visibility
	switch {
	case repo.GetVisibility() == "internal":
		visibility = VisibilityInternal
	case repo.GetPrivate():
		visibility = VisibilityPrivate
	default:
		visibility = VisibilityPublic
	}

	return RepositorySummary{
		Name:          repo.GetName(),
		Namespace:     ns,
		DefaultBranch: repo.GetDefaultBranch(),
		Visibility:    visibility,
		SizeKB:        int64(repo.GetSize()),
	}
}

// GetRepositoryDetails returns repository metadata together with the full
// list of branch and tag names.
func (p *GitHubProvider) GetRepositoryDetails(ctx context.Context, namespace, repo string) (RepositoryDetails, error) {
	data, _, err := p.client.Repositories.Get(ctx, namespace, repo)
	if err != nil {
		return RepositoryDetails{}, fmt.Errorf("get repository %s/%s: %w", namespace, repo, err)
	}

	branches, err := p.listAllBranches(ctx, namespace, repo)
	if err != nil {
		return RepositoryDetails{}, err
	}

	tags, err := p.listAllTags(ctx, namespace, repo)
	if err != nil {
		return RepositoryDetails{}, err
	}

	branchNames := make([]string, 0, len(branches))
	for _, branch := range branches {
		branchNames = append(branchNames, branch.GetName())
	}

	tagNames := make([]string, 0, len(tags))
	for _, tag := range tags {
		tagNames = append(tagNames, tag.GetName())
	}

	return RepositoryDetails{
		RepositorySummary: p.toRepositorySummary(data, namespace),
		Branches:          branchNames,
		Tags:              tagNames,
	}, nil
}

// GetRepositoryState returns every branch and tag mapped to its current
// commit SHA.
func (p *GitHubProvider) GetRepositoryState(ctx context.Context, namespace, repo string) (RepositoryState, error) {
	branches, err := p.listAllBranches(ctx, namespace, repo)
	if err != nil {
		return RepositoryState{}, err
	}

	tags, err := p.listAllTags(ctx, namespace, repo)
	if err != nil {
		return RepositoryState{}, err
	}

	branchMap := make(map[string]string, len(branches))
	for _, branch := range branches {
		branchMap[branch.GetName()] = branch.GetCommit().GetSHA()
	}

	tagMap := make(map[string]string, len(tags))
	for _, tag := range tags {
		tagMap[tag.GetName()] = tag.GetCommit().GetSHA()
	}

	return RepositoryState{
		Branches: branchMap,
		Tags:     tagMap,
	}, nil
}

// listAllBranches collects every branch across all pages.
func (p *GitHubProvider) listAllBranches(ctx context.Context, namespace, repo string) ([]*github.Branch, error) {
	var branches []*github.Branch
	opts := &github.BranchListOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}
	for {
		page, resp, err := p.client.Repositories.ListBranches(ctx, namespace, repo, opts)
		if err != nil {
			return nil, fmt.Errorf("list branches for %s/%s: %w", namespace, repo, err)
		}
		branches = append(branches, page...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return branches, nil
}

// listAllTags collects every tag across all pages.
func (p *GitHubProvider) listAllTags(ctx context.Context, namespace, repo string) ([]*github.RepositoryTag, error) {
	var tags []*github.RepositoryTag
	opts := &github.ListOptions{PerPage: 100}
	for {
		page, resp, err := p.client.Repositories.ListTags(ctx, namespace, repo, opts)
		if err != nil {
			return nil, fmt.Errorf("list tags for %s/%s: %w", namespace, repo, err)
		}
		tags = append(tags, page...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return tags, nil
}

// CreateRepository creates a new repository within namespace. If namespace
// resolves to a GitHub organization, the repository is created in that
// organization; otherwise it is created for the authenticated user.
func (p *GitHubProvider) CreateRepository(ctx context.Context, namespace string, input CreateRepositoryInput) (RepositorySummary, error) {
	account, _, err := p.client.Users.Get(ctx, namespace)
	if err != nil {
		return RepositorySummary{}, fmt.Errorf("get account %s: %w", namespace, err)
	}

	repo := &github.Repository{
		Name:        github.Ptr(input.Name),
		Description: github.Ptr(input.Description),
	}

	org := ""
	if account.GetType() == "Organization" {
		org = namespace
		repo.Visibility = github.Ptr(string(input.Visibility))
	} else {
		repo.Private = github.Ptr(input.Visibility != VisibilityPublic)
	}

	data, _, err := p.client.Repositories.Create(ctx, org, repo)
	if err != nil {
		return RepositorySummary{}, fmt.Errorf("create repository %s/%s: %w", namespace, input.Name, err)
	}

	return p.toRepositorySummary(data, namespace), nil
}

// ListLabels returns all labels defined on the repository.
func (p *GitHubProvider) ListLabels(ctx context.Context, namespace, repo string) ([]Label, error) {
	var labels []Label
	opts := &github.ListOptions{PerPage: 100}
	for {
		page, resp, err := p.client.Issues.ListLabels(ctx, namespace, repo, opts)
		if err != nil {
			return nil, fmt.Errorf("list labels for %s/%s: %w", namespace, repo, err)
		}
		for _, l := range page {
			labels = append(labels, Label{
				Name:        l.GetName(),
				Description: l.GetDescription(),
				Color:       l.GetColor(),
			})
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return labels, nil
}

// CreateLabel creates a new label on the repository.
func (p *GitHubProvider) CreateLabel(ctx context.Context, namespace, repo string, label Label) (Label, error) {
	input := &github.Label{
		Name:        github.Ptr(label.Name),
		Description: github.Ptr(label.Description),
		Color:       github.Ptr(label.Color),
	}
	created, _, err := p.client.Issues.CreateLabel(ctx, namespace, repo, input)
	if err != nil {
		return Label{}, fmt.Errorf("create label %q in %s/%s: %w", label.Name, namespace, repo, err)
	}
	return Label{
		Name:        created.GetName(),
		Description: created.GetDescription(),
		Color:       created.GetColor(),
	}, nil
}

// ListMilestones returns all milestones (open and closed) for the repository.
func (p *GitHubProvider) ListMilestones(ctx context.Context, namespace, repo string) ([]Milestone, error) {
	var milestones []Milestone
	opts := &github.MilestoneListOptions{
		State:       "all",
		ListOptions: github.ListOptions{PerPage: 100},
	}
	for {
		page, resp, err := p.client.Issues.ListMilestones(ctx, namespace, repo, opts)
		if err != nil {
			return nil, fmt.Errorf("list milestones for %s/%s: %w", namespace, repo, err)
		}
		for _, m := range page {
			ms := Milestone{
				ExternalID:  int64(m.GetNumber()),
				Title:       m.GetTitle(),
				Description: m.GetDescription(),
				State:       MilestoneStateOpen,
			}
			if m.GetState() == "closed" {
				ms.State = MilestoneStateClosed
			}
			if duo := m.GetDueOn(); !duo.IsZero() {
				t := duo.Time
				ms.DueDate = &t
			}
			milestones = append(milestones, ms)
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return milestones, nil
}

// CreateMilestone creates a new milestone on the repository.
// ExternalID in the returned Milestone reflects the newly assigned milestone number.
func (p *GitHubProvider) CreateMilestone(ctx context.Context, namespace, repo string, m Milestone) (Milestone, error) {
	state := string(m.State)
	input := &github.Milestone{
		Title:       github.Ptr(m.Title),
		Description: github.Ptr(m.Description),
		State:       github.Ptr(state),
	}
	if m.DueDate != nil {
		input.DueOn = &github.Timestamp{Time: *m.DueDate}
	}
	created, _, err := p.client.Issues.CreateMilestone(ctx, namespace, repo, input)
	if err != nil {
		return Milestone{}, fmt.Errorf("create milestone %q in %s/%s: %w", m.Title, namespace, repo, err)
	}
	result := Milestone{
		ExternalID:  int64(created.GetNumber()),
		Title:       created.GetTitle(),
		Description: created.GetDescription(),
		State:       MilestoneStateOpen,
	}
	if created.GetState() == "closed" {
		result.State = MilestoneStateClosed
	}
	if duo := created.GetDueOn(); !duo.IsZero() {
		t := duo.Time
		result.DueDate = &t
	}
	return result, nil
}

// githubIssueToIssue maps a go-github Issue onto the provider-neutral Issue.
func githubIssueToIssue(i *github.Issue) Issue {
	iss := Issue{
		ExternalID: int64(i.GetNumber()),
		Title:      i.GetTitle(),
		Body:       i.GetBody(),
		State:      i.GetState(),
	}
	for _, l := range i.Labels {
		iss.Labels = append(iss.Labels, l.GetName())
	}
	if m := i.GetMilestone(); m != nil {
		id := int64(m.GetNumber())
		iss.MilestoneExternalID = &id
	}
	return iss
}

// ListIssues returns all issues (open and closed) for the repository,
// excluding pull requests.
func (p *GitHubProvider) ListIssues(ctx context.Context, namespace, repo string) ([]Issue, error) {
	var issues []Issue
	opts := &github.IssueListByRepoOptions{
		State:       "all",
		ListOptions: github.ListOptions{PerPage: 100},
	}
	for {
		page, resp, err := p.client.Issues.ListByRepo(ctx, namespace, repo, opts)
		if err != nil {
			return nil, fmt.Errorf("list issues for %s/%s: %w", namespace, repo, err)
		}
		for _, i := range page {
			if i.IsPullRequest() {
				continue
			}
			issues = append(issues, githubIssueToIssue(i))
		}
		if resp.NextPage == 0 {
			break
		}
		opts.ListOptions.Page = resp.NextPage
	}
	return issues, nil
}

// CreateIssue creates a new issue on the repository. If the issue State is
// "closed", an additional Edit call is made to close the issue after creation.
func (p *GitHubProvider) CreateIssue(ctx context.Context, namespace, repo string, issue Issue) (Issue, error) {
	req := &github.IssueRequest{
		Title: github.Ptr(issue.Title),
		Body:  github.Ptr(issue.Body),
	}
	if len(issue.Labels) > 0 {
		l := issue.Labels
		req.Labels = &l
	}
	if issue.MilestoneExternalID != nil {
		m := int(*issue.MilestoneExternalID)
		req.Milestone = &m
	}
	created, _, err := p.client.Issues.Create(ctx, namespace, repo, req)
	if err != nil {
		return Issue{}, fmt.Errorf("create issue %q in %s/%s: %w", issue.Title, namespace, repo, err)
	}
	if issue.State == "closed" {
		edited, _, err := p.client.Issues.Edit(ctx, namespace, repo, created.GetNumber(), &github.IssueRequest{
			State: github.Ptr("closed"),
		})
		if err != nil {
			return Issue{}, fmt.Errorf("close issue %q in %s/%s: %w", issue.Title, namespace, repo, err)
		}
		return githubIssueToIssue(edited), nil
	}
	return githubIssueToIssue(created), nil
}

// Compile-time check that GitHubProvider implements RepositoryProvider.
var _ RepositoryProvider = (*GitHubProvider)(nil)
