package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, dir, contents string) string {
	t.Helper()
	path := filepath.Join(dir, "transferepo.config.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func requireConfigError(t *testing.T, err error) *ConfigError {
	t.Helper()
	var configErr *ConfigError
	if !errors.As(err, &configErr) {
		t.Fatalf("error type = %T, want *ConfigError", err)
	}
	return configErr
}

func TestLoadFullConfig(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, `
source:
  provider: github
  namespace: my-org

target:
  provider: gitlab
  baseUrl: https://gitlab.example.com
  namespace: my-group

filters:
  include: ["repo-*"]
  exclude: ["repo-legacy"]

mapping:
  old-repo: new-repo
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	want := &Config{
		Source: ProviderConfig{Provider: "github", Namespace: "my-org"},
		Target: ProviderConfig{Provider: "gitlab", BaseURL: "https://gitlab.example.com", Namespace: "my-group"},
		Filters: Filters{
			Include: []string{"repo-*"},
			Exclude: []string{"repo-legacy"},
		},
		Mapping: map[string]string{"old-repo": "new-repo"},
	}

	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("Load() = %+v, want %+v", cfg, want)
	}
}

func TestLoadAppliesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, `
source:
  provider: github
  namespace: my-org

target:
  provider: gitlab
  namespace: my-group
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	wantFilters := Filters{Include: []string{"*"}, Exclude: []string{}}
	if !reflect.DeepEqual(cfg.Filters, wantFilters) {
		t.Errorf("Filters = %+v, want %+v", cfg.Filters, wantFilters)
	}

	if len(cfg.Mapping) != 0 {
		t.Errorf("Mapping = %+v, want empty map", cfg.Mapping)
	}
}

func TestLoadInvalidProvider(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, `
source:
  provider: bitbucket
  namespace: my-org

target:
  provider: gitlab
  namespace: my-group
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}

	configErr := requireConfigError(t, err)
	if !strings.Contains(configErr.Message, "source.provider") {
		t.Errorf("Load() error = %q, want to contain %q", configErr.Message, "source.provider")
	}
}

func TestLoadMissingNamespace(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, `
source:
  provider: github

target:
  provider: gitlab
  namespace: my-group
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}

	configErr := requireConfigError(t, err)
	if !strings.Contains(configErr.Message, "source.namespace") {
		t.Errorf("Load() error = %q, want to contain %q", configErr.Message, "source.namespace")
	}
}

func TestLoadInvalidBaseURL(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, `
source:
  provider: github
  namespace: my-org

target:
  provider: gitlab
  baseUrl: "not a url"
  namespace: my-group
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}

	configErr := requireConfigError(t, err)
	if !strings.Contains(configErr.Message, "target.baseUrl") {
		t.Errorf("Load() error = %q, want to contain %q", configErr.Message, "target.baseUrl")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, "source: [unterminated")

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}

	_ = requireConfigError(t, err)
}

func TestLoadMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.yaml")

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}

	_ = requireConfigError(t, err)
}
