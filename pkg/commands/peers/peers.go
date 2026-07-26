package peers

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/LosFurina/tmuxatlas/pkg/common"
	"github.com/LosFurina/tmuxatlas/pkg/identity"
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
				Action: func(ctx context.Context, c *cli.Command) error {
					if c.NArg() == 0 {
						return fmt.Errorf("peer name is required")
					}
					name := c.Args().First()

					store, err := identity.NewPeerStore()
					if err != nil {
						return err
					}

					if err := store.Remove(name); err != nil {
						return err
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
