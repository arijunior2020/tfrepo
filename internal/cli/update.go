package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
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

	if err := updater.Install(ctx, client, url, binaryPath); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	fmt.Fprintf(stdout, "Atualizado para v%s com sucesso!\n", latest)
	return 0
}
