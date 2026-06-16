package security

import (
	"fmt"
	"os"
	"strings"

	"github.com/arijunior2020/tfrepo/internal/credentials"
)

var envVarByProvider = map[string]string{
	"github": "GITHUB_TOKEN",
	"gitlab": "GITLAB_TOKEN",
}

var credentialsPathFn = credentials.DefaultPath

type MissingTokenError struct {
	Provider string
	EnvVar   string
}

func (e *MissingTokenError) Error() string {
	return fmt.Sprintf(
		"token para %q não encontrado (verificado %s e ~/.tfrepo/credentials) — execute: tfrepo configure",
		e.Provider, e.EnvVar,
	)
}

func EnvVarForProvider(provider string) string {
	return envVarByProvider[provider]
}

func ResolveToken(provider string) (string, error) {
	envVar, ok := envVarByProvider[provider]
	if !ok {
		return "", fmt.Errorf("unknown provider %q", provider)
	}

	if token := strings.TrimSpace(os.Getenv(envVar)); token != "" {
		return token, nil
	}

	creds, err := credentials.Load(credentialsPathFn())
	if err != nil {
		return "", err
	}
	if token := creds.Token(provider); token != "" {
		return token, nil
	}

	return "", &MissingTokenError{Provider: provider, EnvVar: envVar}
}
