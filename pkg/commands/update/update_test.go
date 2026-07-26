package update

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestChecksumForAndVerify(t *testing.T) {
	dir := t.TempDir()
	archiveName := "tmuxatlas-v1.2.3-linux-amd64.tar.gz"
	archivePath := filepath.Join(dir, archiveName)
	content := []byte("release archive")
	if err := os.WriteFile(archivePath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	expected := hex.EncodeToString(sum[:])
	checksumsPath := filepath.Join(dir, "checksums.txt")
	if err := os.WriteFile(checksumsPath, []byte(expected+"  "+archiveName+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := checksumFor(checksumsPath, archiveName)
	if err != nil {
		t.Fatal(err)
	}
	if got != expected {
		t.Fatalf("checksum = %q, want %q", got, expected)
	}
	if err := verifyChecksum(archivePath, got); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archivePath, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyChecksum(archivePath, got); err == nil {
		t.Fatal("expected checksum mismatch")
	}
}

func TestExtractBinary(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "release.tar.gz")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	content := []byte("new tmuxatlas binary")
	if err := tarWriter.WriteHeader(&tar.Header{
		Name: "tmuxatlas", Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	destination := filepath.Join(dir, "tmuxatlas")
	if err := extractBinary(archivePath, destination); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Fatalf("content = %q", got)
	}
}

func TestLatestReleaseUsesGitHubAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/releases/latest" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("missing token header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3","assets":[{"name":"checksums.txt","browser_download_url":"https://example.com/checksums.txt"}]}`))
	}))
	defer server.Close()

	u := &updater{
		client: server.Client(), apiBase: server.URL,
		repository: "owner/repo", token: "test-token",
	}
	result, err := u.latest(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if result.Tag != "v1.2.3" {
		t.Fatalf("tag = %q", result.Tag)
	}
	if _, err := assetURL(result, "checksums.txt"); err != nil {
		t.Fatal(err)
	}
}
