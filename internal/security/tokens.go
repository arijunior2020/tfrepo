package security

import (
	"fmt"
	"os"
	"strings"
)

// envVarByProvider maps a provider name to the environment variable that
// holds its API token.
var envVarByProvider = map[string]string{
	"github": "GITHUB_TOKEN",
	"gitlab": "GITLAB_TOKEN",
}

// MissingTokenError indicates that the environment variable for a
// provider's token is not set or is empty.
type MissingTokenError struct {
	EnvVar string
}

func (e *MissingTokenError) Error() string {
	return fmt.Sprintf("missing required environment variable: %s", e.EnvVar)
}

// ResolveToken returns the API token for the given provider ("github" or
// "gitlab"), read exclusively from GITHUB_TOKEN/GITLAB_TOKEN.
func ResolveToken(provider string) (string, error) {
	envVar, ok := envVarByProvider[provider]
	if !ok {
		return "", fmt.Errorf("unknown provider %q", provider)
	}

	token := strings.TrimSpace(os.Getenv(envVar))
	if token == "" {
		return "", &MissingTokenError{EnvVar: envVar}
	}

	return token, nil
}
