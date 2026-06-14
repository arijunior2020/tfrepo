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
