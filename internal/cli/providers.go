package cli

import (
	"github.com/arijunior2020/tfrepo/internal/config"
	"github.com/arijunior2020/tfrepo/internal/provider"
	"github.com/arijunior2020/tfrepo/internal/security"
)

// NewProvider resolves cfg.Provider's API token from the environment
// (GITHUB_TOKEN or GITLAB_TOKEN, via security.ResolveToken) and constructs
// the corresponding provider.RepositoryProvider. cfg.BaseURL is passed
// through verbatim (empty means the public API).
func NewProvider(cfg config.ProviderConfig) (provider.RepositoryProvider, error) {
	token, err := security.ResolveToken(cfg.Provider)
	if err != nil {
		return nil, err
	}

	if cfg.Provider == "gitlab" {
		return provider.NewGitLabProvider(token, cfg.BaseURL)
	}
	return provider.NewGitHubProvider(token, cfg.BaseURL)
}
