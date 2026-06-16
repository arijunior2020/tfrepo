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

func TestNewRootCommandRegistersMigrateWithFlags(t *testing.T) {
	exitCode := 0
	root := newRootCommand(&exitCode)

	migrateCmd, _, err := root.Find([]string{"migrate"})
	if err != nil {
		t.Fatalf("Find(migrate): %v", err)
	}
	// cobra.Find retorna o root silenciosamente quando o subcomando não existe;
	// verificar Use é obrigatório para detectar esse caso.
	if migrateCmd.Use != "migrate" {
		t.Fatalf("Find(migrate).Use = %q, want %q", migrateCmd.Use, "migrate")
	}

	dryRunFlag := migrateCmd.Flags().Lookup(dryRunFlagName)
	if dryRunFlag == nil {
		t.Fatal("migrate command missing --dry-run flag")
	}
	if dryRunFlag.DefValue != "false" {
		t.Errorf("--dry-run default = %q, want %q", dryRunFlag.DefValue, "false")
	}

	concurrencyFlag := migrateCmd.Flags().Lookup(concurrencyFlagName)
	if concurrencyFlag == nil {
		t.Fatal("migrate command missing --concurrency flag")
	}
	if concurrencyFlag.DefValue != strconv.Itoa(defaultScanConcurrency) {
		t.Errorf("--concurrency default = %q, want %q", concurrencyFlag.DefValue, strconv.Itoa(defaultScanConcurrency))
	}
}

func TestNewRootCommandRegistersValidate(t *testing.T) {
	exitCode := 0
	root := newRootCommand(&exitCode)

	validateCmd, _, err := root.Find([]string{"validate"})
	if err != nil {
		t.Fatalf("Find(validate): %v", err)
	}
	if validateCmd.Use != "validate" {
		t.Fatalf("Find(validate).Use = %q, want %q", validateCmd.Use, "validate")
	}
}

func TestExecuteMigrateReturnsExitCode1WhenPlanMissing(t *testing.T) {
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
	root.SetArgs([]string{"migrate", "--config", configPath})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v (stderr: %s)", err, stderr.String())
	}
	if exitCode != 1 {
		t.Fatalf("exitCode = %d, want 1", exitCode)
	}
}

func TestExecuteValidateReturnsExitCode1WhenPlanMissing(t *testing.T) {
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
	root.SetArgs([]string{"validate", "--config", configPath})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v (stderr: %s)", err, stderr.String())
	}
	if exitCode != 1 {
		t.Fatalf("exitCode = %d, want 1", exitCode)
	}
}

func TestNewRootCommandRegistersSetup(t *testing.T) {
	exitCode := 0
	root := newRootCommand(&exitCode)

	setupCmd, _, err := root.Find([]string{"setup"})
	if err != nil {
		t.Fatalf("Find(setup): %v", err)
	}
	if setupCmd.Use != "setup" {
		t.Fatalf("Find(setup).Use = %q, want %q", setupCmd.Use, "setup")
	}
}

func TestNewRootCommandRegistersUpdate(t *testing.T) {
	exitCode := 0
	root := newRootCommand(&exitCode)

	updateCmd, _, err := root.Find([]string{"update"})
	if err != nil {
		t.Fatalf("Find(update): %v", err)
	}
	if updateCmd.Use != "update" {
		t.Fatalf("Find(update).Use = %q, want %q", updateCmd.Use, "update")
	}
}

func TestNewRootCommandRegistersDestroy(t *testing.T) {
	exitCode := 0
	root := newRootCommand(&exitCode)

	destroyCmd, _, err := root.Find([]string{"destroy"})
	if err != nil {
		t.Fatalf("Find(destroy): %v", err)
	}
	if destroyCmd.Use != "destroy" {
		t.Fatalf("Find(destroy).Use = %q, want %q", destroyCmd.Use, "destroy")
	}

	includeConfigFlag := destroyCmd.Flags().Lookup(includeConfigFlagName)
	if includeConfigFlag == nil {
		t.Fatal("destroy command missing --include-config flag")
	}
	if includeConfigFlag.DefValue != "false" {
		t.Errorf("--include-config default = %q, want %q", includeConfigFlag.DefValue, "false")
	}
}
