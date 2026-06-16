package credentials

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type ProviderCredential struct {
	Token string `yaml:"token"`
}

type Credentials struct {
	Providers map[string]ProviderCredential `yaml:"providers"`
}

type KnownProvider struct {
	ID    string
	Label string
}

var KnownProviders = []KnownProvider{
	{ID: "github", Label: "GitHub"},
	{ID: "gitlab", Label: "GitLab"},
}

func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".tfrepo", "credentials")
	}
	return filepath.Join(home, ".tfrepo", "credentials")
}

func Load(path string) (*Credentials, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Credentials{Providers: make(map[string]ProviderCredential)}, nil
	}
	if err != nil {
		return nil, err
	}
	var c Credentials
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, err
	}
	if c.Providers == nil {
		c.Providers = make(map[string]ProviderCredential)
	}
	return &c, nil
}

func Save(path string, c *Credentials) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func (c *Credentials) Token(provider string) string {
	if c == nil || c.Providers == nil {
		return ""
	}
	return strings.TrimSpace(c.Providers[provider].Token)
}
