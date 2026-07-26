package pair

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/LosFurina/tmuxatlas/pkg/common"
	"github.com/LosFurina/tmuxatlas/pkg/identity"
	"github.com/LosFurina/tmuxatlas/pkg/paths"
	"github.com/LosFurina/tmuxatlas/pkg/socket"
)

func init() {
	cmd := &cli.Command{
		Name:  "pair",
		Usage: "pair with a hub or generate a pairing code",
		Description: `On the hub: run 'tmuxatlas pair' to generate a pairing code.
On the peer: run 'tmuxatlas pair --hub <address> --code <code>' to complete pairing.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "hub",
				Usage:   "Hub address to pair with (e.g. desktop.ts.net:7654)",
				Sources: cli.EnvVars("TMUXATLAS_HUB", "GUPPI_HUB"),
			},
			&cli.StringFlag{
				Name:  "code",
				Usage: "Pairing code from the hub",
			},
			&cli.StringFlag{
				Name:    "socket",
				Usage:   "Path to tmuxatlas server socket (for generating codes)",
				Sources: cli.EnvVars("TMUXATLAS_SOCKET", "GUPPI_SOCKET"),
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			hubAddr := c.String("hub")
			code := c.String("code")

			if hubAddr != "" && code != "" {
				for _, name := range []string{"TMUXATLAS_INSECURE", "GUPPI_INSECURE"} {
					if _, ok := os.LookupEnv(name); ok {
						return fmt.Errorf("%s is no longer supported; the hub must use a system-trusted HTTPS certificate", name)
					}
				}
				return pairWithHub(hubAddr, code)
			}

			if hubAddr == "" && code == "" {
				return generatePairingCode(c.String("socket"))
			}

			return fmt.Errorf("provide both --hub and --code, or neither (to generate a code)")
		},
	}

	common.RegisterCommand(cmd)
}

// normalizeURL takes a bare host:port or full HTTP(S) URL and returns a
// validated hub base URL. Bare addresses default to HTTPS.
func normalizeURL(addr string) (*url.URL, error) {
	if !strings.Contains(addr, "://") {
		addr = "https://" + addr
	}
	u, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}
	if u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("hub URL must be an absolute http or https URL")
	}
	if u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("hub URL must not contain credentials, a path, query, or fragment")
	}
	u.Path = ""
	return u, nil
}

func newPairHTTPClient() *http.Client {
	return &http.Client{Timeout: 15 * time.Second}
}

// pairWithHub completes the pairing handshake with a remote hub via HTTP POST
func pairWithHub(hubAddr, code string) error {
	hostname, _ := os.Hostname()
	id, err := identity.LoadOrCreate(hostname)
	if err != nil {
		return fmt.Errorf("load identity: %w", err)
	}

	peerStore, err := identity.NewPeerStore()
	if err != nil {
		return fmt.Errorf("load peer store: %w", err)
	}

	u, err := normalizeURL(hubAddr)
	if err != nil {
		return fmt.Errorf("invalid hub address %q: %w", hubAddr, err)
	}
	hubBaseURL := u.String()
	u.Path = "/api/pair/complete"

	fmt.Printf("Connecting to %s...\n", u.Host)

	reqBody, _ := json.Marshal(map[string]string{
		"code":       code,
		"name":       id.Name,
		"public_key": id.PublicKey,
	})
	signature, err := id.Sign(identity.PairingTranscript(hubBaseURL, code, id.Name, id.PublicKey))
	if err != nil {
		return fmt.Errorf("sign pairing proof: %w", err)
	}
	var request map[string]any
	_ = json.Unmarshal(reqBody, &request)
	request["version"] = identity.PairingVersion
	request["signature"] = base64.StdEncoding.EncodeToString(signature)
	reqBody, _ = json.Marshal(request)

	client := newPairHTTPClient()

	resp, err := client.Post(u.String(), "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("connect to hub: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("pairing failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result struct {
		Status    string `json:"status"`
		Name      string `json:"name"`
		PublicKey string `json:"public_key"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("invalid response: %w", err)
	}

	if result.Status != "paired" {
		return fmt.Errorf("pairing failed: %s", result.Status)
	}

	// Store the hub as a peer
	peer := identity.Peer{
		Name:      result.Name,
		PublicKey: result.PublicKey,
		PairedAt:  time.Now(),
	}
	if err := peerStore.Add(peer); err != nil {
		return fmt.Errorf("store hub peer: %w", err)
	}
	if err := paths.SaveEnvValue("TMUXATLAS_HUB", hubBaseURL); err != nil {
		return fmt.Errorf("save hub URL: %w", err)
	}

	fmt.Printf("Paired with \"%s\" successfully!\n", result.Name)
	fmt.Printf("Saved Hub URL in ~/.config/tmuxatlas/.env\n")
	fmt.Printf("\nTo connect now:\n  tmuxatlas agent\n")
	fmt.Printf("\nTo install the background service:\n  tmuxatlas install --mode agent\n")

	return nil
}

// generatePairingCode calls the running tmuxatlas server via unix socket to generate a pairing code
func generatePairingCode(socketPath string) error {
	if socketPath == "" {
		socketPath = socket.DefaultPath()
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
	}

	resp, err := client.Post("http://localhost/api/pair", "application/json", nil)
	if err != nil {
		return fmt.Errorf("failed to reach tmuxatlas server via socket %s: %w\nMake sure 'tmuxatlas server' is running", socketPath, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server error (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result struct {
		Code      string    `json:"code"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("invalid response: %w", err)
	}

	fmt.Printf("Pairing code: %s\n", result.Code)
	fmt.Printf("Expires in %s\n", time.Until(result.ExpiresAt).Round(time.Second))
	fmt.Println("\nOn the remote machine, run:")
	fmt.Printf("  tmuxatlas pair --hub <this-machine-address> --code %s\n", result.Code)

	return nil
}
