package healthcheck

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/LosFurina/tmuxatlas/pkg/common"
	"github.com/LosFurina/tmuxatlas/pkg/socket"
)

type runtimeHealth struct {
	Role       string `json:"role"`
	Deployment string `json:"deployment"`
	Version    string `json:"version"`
	Commit     string `json:"commit"`
	Ready      bool   `json:"ready"`
}

func probe(ctx context.Context, socketPath string) (*runtimeHealth, error) {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
		},
	}
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	defer transport.CloseIdleConnections()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/health", nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	var health runtimeHealth
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&health); err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK || !health.Ready {
		return &health, fmt.Errorf("service is not ready")
	}
	return &health, nil
}

func execute(ctx context.Context, c *cli.Command) error {
	health, err := probe(ctx, c.String("socket"))
	if err != nil {
		return fmt.Errorf("healthcheck failed: %w", err)
	}
	if role := c.String("role"); role != "" && health.Role != role {
		return fmt.Errorf("healthcheck role=%s, want %s", health.Role, role)
	}
	if deployment := c.String("deployment"); deployment != "" && health.Deployment != deployment {
		return fmt.Errorf("healthcheck deployment=%s, want %s", health.Deployment, deployment)
	}
	return nil
}

func init() {
	common.RegisterCommand(&cli.Command{
		Name: "healthcheck", Usage: "check the local TmuxAtlas runtime",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name: "socket", Usage: "private Unix management socket",
				Sources: cli.EnvVars("TMUXATLAS_SOCKET"), Value: socket.DefaultPath(),
			},
			&cli.StringFlag{Name: "role", Usage: "required runtime role"},
			&cli.StringFlag{Name: "deployment", Usage: "required deployment mode"},
		},
		Action: execute,
	})
}
