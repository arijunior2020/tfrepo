package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/arijunior2020/tfrepo/internal/updater"
	"github.com/spf13/cobra"
)

const (
	githubRepo    = "arijunior2020/tfrepo"
	githubAPIBase = "https://api.github.com"
)

func newUpdateCommand(exitCode *int) *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Atualiza o tfrepo para a versão mais recente",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			*exitCode = runUpdate(ctx, cmd.OutOrStdout(), cmd.ErrOrStderr())
			return nil
		},
	}
}

func runUpdate(ctx context.Context, stdout, stderr io.Writer) int {
	client := &http.Client{Timeout: 60 * time.Second}

	fmt.Fprintln(stdout, "Buscando última versão...")
	latest, err := updater.LatestVersion(ctx, client, githubAPIBase, githubRepo)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	newer, err := updater.IsNewer(programVersion, latest)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if !newer {
		fmt.Fprintf(stdout, "Já está na versão mais recente (v%s).\n", programVersion)
		return 0
	}

	fmt.Fprintf(stdout, "Nova versão disponível: v%s → v%s\n", programVersion, latest)

	binaryPath, err := os.Executable()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	binaryPath, err = filepath.EvalSymlinks(binaryPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	url := updater.DownloadURL(githubRepo, latest, runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(stdout, "Baixando %s...\n", url)

	if err := installUpdate(ctx, client, url, binaryPath, stdout, stderr); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	fmt.Fprintf(stdout, "Atualizado para v%s com sucesso!\n", latest)
	return 0
}

func installUpdate(ctx context.Context, client *http.Client, url, binaryPath string, stdout, stderr io.Writer) error {
	if runtime.GOOS != "windows" && !canWriteInDir(filepath.Dir(binaryPath)) {
		return installUpdateWithSudo(ctx, client, url, binaryPath, stdout, stderr)
	}

	err := updater.Install(ctx, client, url, binaryPath)
	if err != nil && runtime.GOOS != "windows" && os.IsPermission(err) {
		return installUpdateWithSudo(ctx, client, url, binaryPath, stdout, stderr)
	}
	return err
}

func canWriteInDir(dir string) bool {
	f, err := os.CreateTemp(dir, ".tfrepo-update-check-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

func installUpdateWithSudo(ctx context.Context, client *http.Client, url, binaryPath string, stdout, stderr io.Writer) error {
	if _, err := exec.LookPath("sudo"); err != nil {
		return fmt.Errorf("sem permissão para atualizar %s e sudo não foi encontrado", binaryPath)
	}

	tmpDir, err := os.MkdirTemp("", "tfrepo-update-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	tmpBinaryPath, err := updater.DownloadBinary(ctx, client, url, filepath.Base(binaryPath), tmpDir)
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "Permissão necessária para atualizar %s. Solicitando sudo...\n", binaryPath)

	tmpTargetPath := fmt.Sprintf("%s.new.%d", binaryPath, os.Getpid())
	script := `set -e
trap 'rm -f "$2"' EXIT
cp "$1" "$2"
chmod 0755 "$2"
mv -f "$2" "$3"
trap - EXIT`
	cmd := exec.CommandContext(ctx, "sudo", "sh", "-c", script, "sh", tmpBinaryPath, tmpTargetPath, binaryPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sudo install failed: %w", err)
	}
	return nil
}
