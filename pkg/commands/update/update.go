package update

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/LosFurina/tmuxatlas/pkg/common"
)

const (
	defaultRepository = "LosFurina/tmuxatlas"
	maxDownloadSize   = 200 << 20
)

type releaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type release struct {
	Tag    string         `json:"tag_name"`
	Assets []releaseAsset `json:"assets"`
}

type updater struct {
	client     *http.Client
	apiBase    string
	repository string
	token      string
}

func newUpdater(repository string) *updater {
	return &updater{
		client:     &http.Client{Timeout: 60 * time.Second},
		apiBase:    "https://api.github.com",
		repository: repository,
		token:      firstNonEmpty(os.Getenv("GITHUB_TOKEN"), os.Getenv("GH_TOKEN")),
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func (u *updater) request(ctx context.Context, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "tmuxatlas/"+common.SUMMARY)
	if u.token != "" {
		req.Header.Set("Authorization", "Bearer "+u.token)
	}
	resp, err := u.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("GitHub returned %s: %s", resp.Status, strings.TrimSpace(string(message)))
	}
	return resp, nil
}

func (u *updater) latest(ctx context.Context) (*release, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/releases/latest", strings.TrimSuffix(u.apiBase, "/"), u.repository)
	resp, err := u.request(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode latest release: %w", err)
	}
	if result.Tag == "" {
		return nil, errors.New("latest release has no tag")
	}
	return &result, nil
}

func assetURL(result *release, name string) (string, error) {
	for _, asset := range result.Assets {
		if asset.Name == name && asset.URL != "" {
			return asset.URL, nil
		}
	}
	return "", fmt.Errorf("release %s does not contain %s", result.Tag, name)
}

func (u *updater) download(ctx context.Context, rawURL, destination string) error {
	resp, err := u.request(ctx, rawURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	written, err := io.Copy(file, io.LimitReader(resp.Body, maxDownloadSize+1))
	if err != nil {
		return err
	}
	if written > maxDownloadSize {
		return errors.New("download exceeds 200 MiB limit")
	}
	return file.Sync()
}

func checksumFor(checksumsPath, archiveName string) (string, error) {
	file, err := os.Open(checksumsPath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == archiveName {
			if len(fields[0]) != sha256.Size*2 {
				return "", errors.New("release checksum is not SHA-256")
			}
			return strings.ToLower(fields[0]), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("checksums.txt does not contain %s", archiveName)
}

func verifyChecksum(path, expected string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if actual != strings.ToLower(expected) {
		return fmt.Errorf("checksum mismatch: got %s, want %s", actual, expected)
	}
	return nil
}

func extractBinary(archivePath, destination string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if header.Typeflag != tar.TypeReg || filepath.Base(header.Name) != "tmuxatlas" {
			continue
		}
		output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
		if err != nil {
			return err
		}
		written, copyErr := io.Copy(output, io.LimitReader(reader, maxDownloadSize+1))
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if written > maxDownloadSize {
			return errors.New("extracted binary exceeds 200 MiB limit")
		}
		return nil
	}
	return errors.New("release archive does not contain tmuxatlas")
}

func replaceExecutable(source, executable string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	temp, err := os.CreateTemp(filepath.Dir(executable), ".tmuxatlas-update-*")
	if err != nil {
		return fmt.Errorf("cannot write beside %s: %w", executable, err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o755); err != nil {
		temp.Close()
		return err
	}
	if _, err := io.Copy(temp, input); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, executable); err != nil {
		return fmt.Errorf("replace %s: %w", executable, err)
	}
	return nil
}

func normalizeVersion(value string) string {
	return strings.TrimPrefix(strings.TrimSpace(value), "v")
}

func execute(ctx context.Context, c *cli.Command) error {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return fmt.Errorf("self-update is not supported on %s", runtime.GOOS)
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		return fmt.Errorf("self-update is not supported on %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	u := newUpdater(c.String("repository"))
	result, err := u.latest(ctx)
	if err != nil {
		return fmt.Errorf("check latest release: %w", err)
	}
	fmt.Printf("Current: %s\nLatest:  %s\n", common.SUMMARY, result.Tag)
	if normalizeVersion(common.SUMMARY) == normalizeVersion(result.Tag) && !c.Bool("force") {
		fmt.Println("TmuxAtlas is already up to date.")
		return nil
	}
	if c.Bool("check") {
		fmt.Println("An update is available.")
		return nil
	}

	executable, err := currentExecutable()
	if err != nil {
		return fmt.Errorf("resolve current executable: %w", err)
	}
	service, err := discoverService()
	if err != nil {
		return fmt.Errorf("inspect user service: %w", err)
	}
	if err := validateServiceExecutable(service, executable); err != nil {
		return err
	}
	if service != nil {
		state := "installed but not running"
		if service.active {
			state = "running"
		}
		fmt.Printf("Detected %s service %s (%s).\n", service.kind, service.name, state)
	}

	version := normalizeVersion(result.Tag)
	archiveName := fmt.Sprintf("tmuxatlas-v%s-%s-%s.tar.gz", version, runtime.GOOS, runtime.GOARCH)
	archiveURL, err := assetURL(result, archiveName)
	if err != nil {
		return err
	}
	checksumsURL, err := assetURL(result, "checksums.txt")
	if err != nil {
		return err
	}
	tempDir, err := os.MkdirTemp("", "tmuxatlas-update-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)
	archivePath := filepath.Join(tempDir, archiveName)
	checksumsPath := filepath.Join(tempDir, "checksums.txt")
	fmt.Printf("Downloading %s...\n", archiveName)
	if err := u.download(ctx, archiveURL, archivePath); err != nil {
		return fmt.Errorf("download release: %w", err)
	}
	if err := u.download(ctx, checksumsURL, checksumsPath); err != nil {
		return fmt.Errorf("download checksums: %w", err)
	}
	expected, err := checksumFor(checksumsPath, archiveName)
	if err != nil {
		return err
	}
	if err := verifyChecksum(archivePath, expected); err != nil {
		return err
	}
	binaryPath := filepath.Join(tempDir, "tmuxatlas")
	if err := extractBinary(archivePath, binaryPath); err != nil {
		return fmt.Errorf("extract release: %w", err)
	}
	if err := replaceExecutable(binaryPath, executable); err != nil {
		return err
	}
	fmt.Printf("Updated %s to %s\n", executable, result.Tag)
	switch {
	case service == nil:
		fmt.Println("No TmuxAtlas user service was detected.")
	case !service.active:
		fmt.Printf("%s is not running, so it was not started.\n", service.name)
	case c.Bool("no-restart"):
		fmt.Printf("%s is still running the previous version; restart it when ready.\n", service.name)
	default:
		fmt.Printf("Restarting %s...\n", service.name)
		if err := service.restart(ctx); err != nil {
			return fmt.Errorf("binary updated, but service restart failed: %w", err)
		}
		fmt.Printf("Restarted %s successfully.\n", service.name)
		if !strings.Contains(service.name, "agent") {
			fmt.Println("Existing in-memory browser sessions were cleared; sign in with your Passkey again.")
		}
	}
	return nil
}

func init() {
	common.RegisterCommand(&cli.Command{
		Name:  "update",
		Usage: "update TmuxAtlas to the latest GitHub release",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "check", Usage: "check for an update without installing it"},
			&cli.BoolFlag{Name: "force", Usage: "reinstall even when the version is current"},
			&cli.BoolFlag{Name: "no-restart", Usage: "do not restart a running systemd/launchd service"},
			&cli.StringFlag{
				Name:    "repository",
				Usage:   "GitHub owner/repository",
				Value:   defaultRepository,
				Sources: cli.EnvVars("TMUXATLAS_REPOSITORY"),
			},
		},
		Action: execute,
	})
}
