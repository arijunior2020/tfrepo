package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestExecuteInitCreatesConfigFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "transferepo.config.yaml")

	exitCode := 0
	root := newRootCommand(&exitCode)

	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"init", "--config", configPath})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v (stderr: %s)", err, stderr.String())
	}
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0 (stdout: %s, stderr: %s)", exitCode, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Errorf("config file not created: %v", err)
	}
}

func TestExecuteInitReturnsExitCode1WhenConfigExists(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "transferepo.config.yaml")
	if err := os.WriteFile(configPath, []byte("existing"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	exitCode := 0
	root := newRootCommand(&exitCode)

	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"init", "--config", configPath})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v (stderr: %s)", err, stderr.String())
	}
	if exitCode != 1 {
		t.Fatalf("exitCode = %d, want 1", exitCode)
	}
}

func TestExecuteNoArgsPrintsBanner(t *testing.T) {
	exitCode := 0
	root := newRootCommand(&exitCode)

	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(stdout.String(), bannerSubtitle) {
		t.Errorf("stdout = %q, want it to contain the banner subtitle", stdout.String())
	}
}

func TestNewRootCommandRegistersScanWithConcurrencyFlag(t *testing.T) {
	exitCode := 0
	root := newRootCommand(&exitCode)

	scanCmd, _, err := root.Find([]string{"scan"})
	if err != nil {
		t.Fatalf("Find(scan): %v", err)
	}

	flag := scanCmd.Flags().Lookup(concurrencyFlagName)
	if flag == nil {
		t.Fatal("scan command missing --concurrency flag")
	}
	if flag.DefValue != strconv.Itoa(defaultScanConcurrency) {
		t.Errorf("--concurrency default = %q, want %q", flag.DefValue, strconv.Itoa(defaultScanConcurrency))
	}
}

func TestNewRootCommandRegistersPlan(t *testing.T) {
	exitCode := 0
	root := newRootCommand(&exitCode)

	planCmd, _, err := root.Find([]string{"plan"})
	if err != nil {
		t.Fatalf("Find(plan): %v", err)
	}
	if planCmd.Use != "plan" {
		t.Fatalf("Find(plan).Use = %q, want %q", planCmd.Use, "plan")
	}
}

func TestExecuteScanReturnsExitCode1WhenConfigMissing(t *testing.T) {
	dir := t.TempDir()

	exitCode := 0
	root := newRootCommand(&exitCode)

	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"scan", "--config", filepath.Join(dir, "does-not-exist.yaml")})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v (stderr: %s)", err, stderr.String())
	}
	if exitCode != 1 {
		t.Fatalf("exitCode = %d, want 1", exitCode)
	}
}

func TestExecutePlanReturnsExitCode1WhenInventoryMissing(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	configPath := filepath.Join(dir, "transferepo.config.yaml")
	configYAML := "source:\n  provider: github\n  namespace: my-org\ntarget:\n  provider: gitlab\n  namespace: my-group\n"
	if err := os.WriteFile(configPath, []byte(configYAML), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	exitCode := 0
	root := newRootCommand(&exitCode)

	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"plan", "--config", configPath})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v (stderr: %s)", err, stderr.String())
	}
	if exitCode != 1 {
		t.Fatalf("exitCode = %d, want 1", exitCode)
	}
}
