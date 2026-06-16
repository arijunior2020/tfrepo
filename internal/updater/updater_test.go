package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func makeTarGZ(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	hdr := &tar.Header{Name: name, Mode: 0755, Size: int64(len(content))}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("WriteHeader: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("tw.Write: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tw.Close: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("gw.Close: %v", err)
	}
	return buf.Bytes()
}

func makeZip(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create(name)
	if err != nil {
		t.Fatalf("zip.Create: %v", err)
	}
	if _, err := f.Write(content); err != nil {
		t.Fatalf("zip.Write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip.Close: %v", err)
	}
	return buf.Bytes()
}

func TestLatestVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/releases/latest" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"tag_name": "v1.2.3"}`)
	}))
	defer srv.Close()

	got, err := LatestVersion(context.Background(), http.DefaultClient, srv.URL, "owner/repo")
	if err != nil {
		t.Fatalf("LatestVersion: %v", err)
	}
	if got != "1.2.3" {
		t.Errorf("LatestVersion = %q, want %q", got, "1.2.3")
	}
}

func TestLatestVersionHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := LatestVersion(context.Background(), http.DefaultClient, srv.URL, "owner/repo")
	if err == nil {
		t.Fatal("LatestVersion() error = nil, want error")
	}
}

func TestIsNewer(t *testing.T) {
	tests := []struct {
		current   string
		candidate string
		want      bool
	}{
		{"0.1.0", "0.2.0", true},
		{"0.1.0", "1.0.0", true},
		{"0.1.0", "0.1.1", true},
		{"0.2.0", "0.1.0", false},
		{"0.1.0", "0.1.0", false},
		{"1.0.0", "0.9.9", false},
		{"v0.1.0", "v0.2.0", true},
		{"v0.1.0", "v0.1.0", false},
	}
	for _, tt := range tests {
		got, err := IsNewer(tt.current, tt.candidate)
		if err != nil {
			t.Errorf("IsNewer(%q, %q): %v", tt.current, tt.candidate, err)
			continue
		}
		if got != tt.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", tt.current, tt.candidate, got, tt.want)
		}
	}
}

func TestIsNewerInvalidVersion(t *testing.T) {
	_, err := IsNewer("not-a-version", "0.1.0")
	if err == nil {
		t.Fatal("IsNewer() error = nil, want error")
	}
}

func TestDownloadURL(t *testing.T) {
	got := DownloadURL("owner/repo", "0.2.0", "linux", "amd64")
	want := "https://github.com/owner/repo/releases/download/v0.2.0/repo_0.2.0_linux_amd64.tar.gz"
	if got != want {
		t.Errorf("DownloadURL = %q, want %q", got, want)
	}
}

func TestDownloadURLWindows(t *testing.T) {
	got := DownloadURL("owner/repo", "0.2.0", "windows", "amd64")
	want := "https://github.com/owner/repo/releases/download/v0.2.0/repo_0.2.0_windows_amd64.zip"
	if got != want {
		t.Errorf("DownloadURL = %q, want %q", got, want)
	}
}

func TestInstallTarGZ(t *testing.T) {
	content := []byte("#!/bin/sh\necho hello")
	archive := makeTarGZ(t, "tfrepo", content)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(archive)
	}))
	defer srv.Close()

	dir := t.TempDir()
	binaryPath := filepath.Join(dir, "tfrepo")

	err := Install(context.Background(), http.DefaultClient, srv.URL+"/tfrepo_0.2.0_linux_amd64.tar.gz", binaryPath)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	got, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("content = %q, want %q", got, content)
	}

	info, err := os.Stat(binaryPath)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode()&0111 == 0 {
		t.Error("binary is not executable")
	}
}

func TestInstallZip(t *testing.T) {
	content := []byte("fake windows binary")
	archive := makeZip(t, "tfrepo.exe", content)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(archive)
	}))
	defer srv.Close()

	dir := t.TempDir()
	binaryPath := filepath.Join(dir, "tfrepo.exe")

	err := Install(context.Background(), http.DefaultClient, srv.URL+"/tfrepo_0.2.0_windows_amd64.zip", binaryPath)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	got, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("content = %q, want %q", got, content)
	}
}

func TestDownloadBinaryExtractsToTempDir(t *testing.T) {
	content := []byte("#!/bin/sh\necho updated")
	archive := makeTarGZ(t, "tfrepo", content)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(archive)
	}))
	defer srv.Close()

	dir := t.TempDir()
	tmpPath, err := DownloadBinary(context.Background(), http.DefaultClient, srv.URL+"/tfrepo_0.2.0_linux_amd64.tar.gz", "tfrepo", dir)
	if err != nil {
		t.Fatalf("DownloadBinary: %v", err)
	}
	defer func() { _ = os.Remove(tmpPath) }()

	if filepath.Dir(tmpPath) != dir {
		t.Errorf("DownloadBinary path dir = %q, want %q", filepath.Dir(tmpPath), dir)
	}

	got, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("content = %q, want %q", got, content)
	}

	info, err := os.Stat(tmpPath)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode()&0111 == 0 {
		t.Error("binary is not executable")
	}
}

func TestInstallBinaryNotInArchive(t *testing.T) {
	archive := makeTarGZ(t, "other-binary", []byte("content"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer srv.Close()

	dir := t.TempDir()
	binaryPath := filepath.Join(dir, "tfrepo")

	err := Install(context.Background(), http.DefaultClient, srv.URL+"/tfrepo_0.2.0_linux_amd64.tar.gz", binaryPath)
	if err == nil {
		t.Fatal("Install() error = nil, want error")
	}
}
