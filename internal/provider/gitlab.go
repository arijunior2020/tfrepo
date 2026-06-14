package provider

import (
	"fmt"
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
