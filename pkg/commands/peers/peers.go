package peers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/LosFurina/tmuxatlas/pkg/common"
	"github.com/LosFurina/tmuxatlas/pkg/identity"
	"github.com/LosFurina/tmuxatlas/pkg/socket"
)

func init() {
	cmd := &cli.Command{
		Name:  "peers",
		Usage: "manage paired peers",
		Commands: []*cli.Command{
			{
				Name:  "list",
				Usage: "list all paired peers",
				Action: func(ctx context.Context, c *cli.Command) error {
					store, err := identity.NewPeerStore()
					if err != nil {
						return err
					}

					peers := store.List()
					if len(peers) == 0 {
						fmt.Println("No paired peers.")
						fmt.Println("Use 'tmuxatlas pair' to pair with another machine.")
						return nil
					}

					fmt.Printf("%-20s %-12s %s\n", "NAME", "FINGERPRINT", "PAIRED AT")
					for _, p := range peers {
						fmt.Printf("%-20s %-12s %s\n", p.Name, p.Fingerprint(), p.PairedAt.Format("2006-01-02 15:04"))
					}
					return nil
				},
			},
			{
				Name:      "remove",
				Usage:     "remove a paired peer",
				ArgsUsage: "<peer-name>",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "socket", Usage: "Hub Unix socket path"},
				},
				Action: func(ctx context.Context, c *cli.Command) error {
					if c.NArg() == 0 {
						return fmt.Errorf("peer name is required")
					}
					name := c.Args().First()

					socketPath := c.String("socket")
					if socketPath == "" {
						socketPath = socket.DefaultPath()
					}
					online, err := revokeViaHub(ctx, socketPath, name)
					if err != nil {
						return err
					}
					if !online {
						store, err := identity.NewPeerStore()
						if err != nil {
							return err
						}
						if err := store.Remove(name); err != nil {
							return err
						}
					}

					fmt.Printf("Removed peer %q\n", name)
					return nil
				},
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			// Default action: list peers
			store, err := identity.NewPeerStore()
			if err != nil {
				return err
			}

			id, err := identity.Load()
			if err == nil {
				fmt.Printf("This node: %s (%s)\n\n", id.Name, id.Fingerprint())
			}

			peers := store.List()
			if len(peers) == 0 {
				fmt.Println("No paired peers.")
				fmt.Println("Use 'tmuxatlas pair' to pair with another machine.")
				return nil
			}

			fmt.Printf("%-20s %-12s %s\n", "NAME", "FINGERPRINT", "PAIRED AT")
			for _, p := range peers {
				fmt.Printf("%-20s %-12s %s\n", p.Name, p.Fingerprint(), p.PairedAt.Format("2006-01-02 15:04"))
			}
			return nil
		},
	}

	common.RegisterCommand(cmd)
}

func revokeViaHub(ctx context.Context, socketPath, name string) (bool, error) {
	probe, err := net.DialTimeout("unix", socketPath, 100*time.Millisecond)
	if err != nil {
		return false, nil
	}
	_ = probe.Close()
	body, _ := json.Marshal(map[string]string{"name": name})
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
		},
	}
	defer transport.CloseIdleConnections()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"http://tmuxatlas/api/peers/revoke", bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Transport: transport}).Do(request)
	if err != nil {
		return true, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return true, fmt.Errorf("running hub rejected peer revocation: %s", response.Status)
	}
	return true, nil
}
