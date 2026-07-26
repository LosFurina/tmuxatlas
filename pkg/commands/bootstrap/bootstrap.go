package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/LosFurina/tmuxatlas/pkg/common"
	"github.com/LosFurina/tmuxatlas/pkg/socket"
)

func execute(_ context.Context, command *cli.Command) error {
	path := command.String("socket")
	if path == "" {
		path = socket.DefaultPath()
	}
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{DialContext: func(context.Context, string, string) (net.Conn, error) {
			return net.Dial("unix", path)
		}},
	}
	response, err := client.Post("http://localhost/api/auth/bootstrap/rotate", "application/json", nil)
	if err != nil {
		return fmt.Errorf("rotate bootstrap token through %s: %w", path, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("bootstrap rotation failed with HTTP %d", response.StatusCode)
	}
	var result struct {
		Token string `json:"setup_token"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return err
	}
	fmt.Println(result.Token)
	return nil
}

func init() {
	common.RegisterCommand(&cli.Command{
		Name:  "bootstrap-token",
		Usage: "rotate and print the initial Passkey setup token through the local Unix socket",
		Flags: []cli.Flag{&cli.StringFlag{
			Name: "socket", Sources: cli.EnvVars("TMUXATLAS_SOCKET", "GUPPI_SOCKET"),
		}},
		Action: execute,
	})
}
