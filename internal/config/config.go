package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// ProviderConfig describes one side (source or target) of a migration.
type ProviderConfig struct {
	Provider  string `yaml:"provider"`          // "github" | "gitlab"
	BaseURL   string `yaml:"baseUrl,omitempty"` // GitHub Enterprise / GitLab self-managed
	Namespace string `yaml:"namespace"`
}

// Filters controls which repositories are included when building a
// migration plan.
type Filters struct {
	Include []string `yaml:"include"` // default: []string{"*"}
	Exclude []string `yaml:"exclude"` // default: []string{}
}

// Config is the parsed and validated contents of transferepo.config.yaml.
type Config struct {
	Source  ProviderConfig    `yaml:"source"`
	Target  ProviderConfig    `yaml:"target"`
	Filters Filters           `yaml:"filters"`
	Mapping map[string]string `yaml:"mapping"`
}

// ConfigError is returned by Load when the configuration file is missing,
// contains invalid YAML, or fails validation. It mirrors the ConfigError
// class in packages/core/src/config.ts.
type ConfigError struct {
	Message string
}

func (e *ConfigError) Error() string {
	return e.Message
}

// Load reads, parses and validates the configuration file at path,
// replicating the behavior of loadConfig in packages/core/src/config.ts.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, &ConfigError{Message: fmt.Sprintf("Could not read configuration file at %s: %s", path, err)}
	}

	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, &ConfigError{Message: fmt.Sprintf("Invalid YAML in %s: %s", path, err)}
	}

	cfg.applyDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, &ConfigError{Message: fmt.Sprintf("Invalid configuration in %s: %s", path, err)}
	}

	return &cfg, nil
}

// applyDefaults fills in Filters and Mapping when absent from the YAML,
// matching FiltersSchema.default(...) and the mapping record's
// .default({}) in packages/core/src/config.ts. A nil slice/map means the
// corresponding YAML key was entirely absent.
func (c *Config) applyDefaults() {
	if c.Filters.Include == nil {
		c.Filters.Include = []string{"*"}
	}
	if c.Filters.Exclude == nil {
		c.Filters.Exclude = []string{}
	}
	if c.Mapping == nil {
		c.Mapping = map[string]string{}
	}
}

// Validate checks the configuration against the rules in
// TransferepoConfigSchema: source and target must each have a known
// provider and a non-empty namespace, and baseUrl, if set, must be a valid
// absolute URL. Errors are formatted as "path.to.field: message" joined by
// "; ", matching the issue formatting in loadConfig.
func (c *Config) Validate() error {
	var issues []string
	issues = append(issues, validateProviderConfig("source", c.Source)...)
	issues = append(issues, validateProviderConfig("target", c.Target)...)

	if len(issues) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(issues, "; "))
}

func validateProviderConfig(field string, pc ProviderConfig) []string {
	var issues []string

	if pc.Provider != "github" && pc.Provider != "gitlab" {
		issues = append(issues, fmt.Sprintf(`%s.provider: must be "github" or "gitlab"`, field))
	}

	if pc.Namespace == "" {
		issues = append(issues, fmt.Sprintf("%s.namespace: is required", field))
	}

	if pc.BaseURL != "" {
		u, err := url.Parse(pc.BaseURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			issues = append(issues, fmt.Sprintf("%s.baseUrl: must be a valid URL", field))
		}
	}

	return issues
}
