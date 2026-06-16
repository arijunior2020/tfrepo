package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// LatestVersion queries the GitHub releases API and returns the latest
// version string without the "v" prefix (e.g. "0.2.0").
// baseURL should be "https://api.github.com" in production; tests inject
// an httptest.Server URL.
func LatestVersion(ctx context.Context, client *http.Client, baseURL, repo string) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", baseURL, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}

	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}

	version := strings.TrimPrefix(payload.TagName, "v")
	if version == "" {
		return "", fmt.Errorf("GitHub API returned empty tag_name")
	}
	return version, nil
}

// IsNewer reports whether candidate is strictly newer than current.
// Both strings may optionally have a "v" prefix (e.g. "0.1.0" or "v0.1.0").
func IsNewer(current, candidate string) (bool, error) {
	cMaj, cMin, cPat, err := parseVersion(current)
	if err != nil {
		return false, fmt.Errorf("current %q: %w", current, err)
	}
	nMaj, nMin, nPat, err := parseVersion(candidate)
	if err != nil {
		return false, fmt.Errorf("candidate %q: %w", candidate, err)
	}
	if nMaj != cMaj {
		return nMaj > cMaj, nil
	}
	if nMin != cMin {
		return nMin > cMin, nil
	}
	return nPat > cPat, nil
}

// DownloadURL returns the GitHub release asset URL for the given
// repo/version/goos/goarch combination, matching the archive name template
// in .goreleaser.yaml: <project>_<version>_<os>_<arch>.tar.gz (or .zip on
// Windows).
func DownloadURL(repo, version, goos, goarch string) string {
	project := filepath.Base(repo)
	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}
	filename := fmt.Sprintf("%s_%s_%s_%s.%s", project, version, goos, goarch, ext)
	return fmt.Sprintf("https://github.com/%s/releases/download/v%s/%s", repo, version, filename)
}

// Install downloads the archive at downloadURL, extracts the binary whose
// base name matches filepath.Base(binaryPath), and atomically replaces
// binaryPath.
func Install(ctx context.Context, client *http.Client, downloadURL, binaryPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}

	binaryName := filepath.Base(binaryPath)
	tmpPath := binaryPath + ".new"

	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}

	var found bool
	if strings.HasSuffix(downloadURL, ".tar.gz") {
		found, err = extractTarGZ(resp.Body, binaryName, f)
	} else {
		data, rerr := io.ReadAll(resp.Body)
		if rerr != nil {
			_ = f.Close()
			_ = os.Remove(tmpPath)
			return rerr
		}
		found, err = extractZip(data, binaryName, f)
	}
	_ = f.Close()
	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if !found {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("binary %q not found in archive", binaryName)
	}
	return os.Rename(tmpPath, binaryPath)
}

func extractTarGZ(r io.Reader, name string, dst io.Writer) (bool, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return false, err
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if filepath.Base(hdr.Name) == name {
			if _, err := io.Copy(dst, tr); err != nil {
				return false, err
			}
			return true, nil
		}
	}
}

func extractZip(data []byte, name string, dst io.Writer) (bool, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false, err
	}
	for _, f := range zr.File {
		if filepath.Base(f.Name) == name {
			rc, err := f.Open()
			if err != nil {
				return false, err
			}
			defer func() { _ = rc.Close() }()
			if _, err := io.Copy(dst, rc); err != nil {
				return false, err
			}
			return true, nil
		}
	}
	return false, nil
}

func parseVersion(v string) (major, minor, patch int, err error) {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("expected MAJOR.MINOR.PATCH, got %q", v)
	}
	major, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid major: %w", err)
	}
	minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid minor: %w", err)
	}
	patch, err = strconv.Atoi(parts[2])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid patch: %w", err)
	}
	return
}
