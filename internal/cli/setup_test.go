package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arijunior2020/tfrepo/internal/config"
)

func TestBuildConfigYAMLAllRepos(t *testing.T) {
	input := WizardInput{
		SourceProvider:  "github",
		SourceNamespace: "my-org",
		TargetProvider:  "gitlab",
		TargetNamespace: "my-group",
	}
	yml, err := buildConfigYAML(input)
	if err != nil {
		t.Fatalf("buildConfigYAML: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "transferepo.config.yaml")
	if err := os.WriteFile(path, yml, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg.Source.Provider != "github" {
		t.Errorf("source.provider = %q, want %q", cfg.Source.Provider, "github")
	}
	if cfg.Source.Namespace != "my-org" {
		t.Errorf("source.namespace = %q, want %q", cfg.Source.Namespace, "my-org")
	}
	if cfg.Target.Provider != "gitlab" {
		t.Errorf("target.provider = %q, want %q", cfg.Target.Provider, "gitlab")
	}
	if cfg.Target.Namespace != "my-group" {
		t.Errorf("target.namespace = %q, want %q", cfg.Target.Namespace, "my-group")
	}
	if len(cfg.Filters.Include) != 1 || cfg.Filters.Include[0] != "*" {
		t.Errorf("filters.include = %v, want [*]", cfg.Filters.Include)
	}
}

func TestBuildConfigYAMLSelectedRepos(t *testing.T) {
	input := WizardInput{
		SourceProvider:  "github",
		SourceNamespace: "my-org",
		TargetProvider:  "gitlab",
		TargetNamespace: "my-group",
		SelectedRepos:   []string{"repo-b", "repo-a"},
	}
	yml, err := buildConfigYAML(input)
	if err != nil {
		t.Fatalf("buildConfigYAML: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "transferepo.config.yaml")
	if err := os.WriteFile(path, yml, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	want := []string{"repo-a", "repo-b"}
	if len(cfg.Filters.Include) != 2 ||
		cfg.Filters.Include[0] != want[0] ||
		cfg.Filters.Include[1] != want[1] {
		t.Errorf("filters.include = %v, want %v", cfg.Filters.Include, want)
	}
}

func TestBuildConfigYAMLWithBaseURL(t *testing.T) {
	input := WizardInput{
		SourceProvider:  "gitlab",
		SourceNamespace: "my-group",
		SourceBaseURL:   "https://gitlab.example.com",
		TargetProvider:  "github",
		TargetNamespace: "my-org",
	}
	yml, err := buildConfigYAML(input)
	if err != nil {
		t.Fatalf("buildConfigYAML: %v", err)
	}
	if !strings.Contains(string(yml), "gitlab.example.com") {
		t.Errorf("YAML não contém baseUrl: %s", yml)
	}
}

func TestValidateSpecificRepoSelectionRequiresAtLeastOneRepo(t *testing.T) {
	if err := validateSpecificRepoSelection(nil); err == nil {
		t.Fatal("validateSpecificRepoSelection(nil) error = nil, want error")
	}
	if err := validateSpecificRepoSelection([]string{}); err == nil {
		t.Fatal("validateSpecificRepoSelection(empty) error = nil, want error")
	}
	if err := validateSpecificRepoSelection([]string{"repo-a"}); err != nil {
		t.Fatalf("validateSpecificRepoSelection(repo-a): %v", err)
	}
}

func TestSetupAndWriteCreatesConfigFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "transferepo.config.yaml")

	input := WizardInput{
		SourceProvider:  "github",
		SourceNamespace: "my-org",
		TargetProvider:  "gitlab",
		TargetNamespace: "my-group",
	}
	var stdout, stderr bytes.Buffer
	code := setupAndWrite(input, configPath, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("setupAndWrite = %d, want 0 (stderr: %s)", code, stderr.String())
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("config file not created: %v", err)
	}
	if !strings.Contains(stdout.String(), configPath) {
		t.Errorf("stdout = %q, want to contain %q", stdout.String(), configPath)
	}
}

func TestSetupAndWriteOverwritesExistingFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "transferepo.config.yaml")
	if err := os.WriteFile(configPath, []byte("old content"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	input := WizardInput{
		SourceProvider:  "github",
		SourceNamespace: "my-org",
		TargetProvider:  "gitlab",
		TargetNamespace: "my-group",
	}
	var stdout, stderr bytes.Buffer
	code := setupAndWrite(input, configPath, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("setupAndWrite = %d, want 0", code)
	}
	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(got), "old content") {
		t.Error("config file was not overwritten")
	}
}
