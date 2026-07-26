package agent

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"

	servercommand "github.com/LosFurina/tmuxatlas/pkg/commands/server"
	"github.com/LosFurina/tmuxatlas/pkg/common"
	"github.com/LosFurina/tmuxatlas/pkg/tmux"
)

func init() {
	common.RegisterCommand(&cli.Command{
		Name:        "agent",
		Usage:       "run the outbound-only tmux agent",
		Description: "Connect local tmux sessions to a trusted Hub without opening a TCP listener or initializing WebAuthn.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name: "hub", Usage: "Trusted Hub URL",
				Sources: cli.EnvVars("TMUXATLAS_HUB"), Required: true,
			},
			&cli.IntFlag{
				Name: "discovery-interval", Usage: "Session discovery interval in seconds",
				Sources: cli.EnvVars("TMUXATLAS_DISCOVERY_INTERVAL"), Value: 2,
			},
			&cli.BoolFlag{
				Name: "no-control-mode", Usage: "Disable tmux control mode",
				Sources: cli.EnvVars("TMUXATLAS_NO_CONTROL_MODE"),
			},
			&cli.StringFlag{
				Name: "socket", Usage: "Local Unix socket path",
				Sources: cli.EnvVars("TMUXATLAS_SOCKET"),
			},
		},
		Before: func(ctx context.Context, _ *cli.Command) (context.Context, error) {
			logrus.Info("checking for tmux...")
			if _, err := tmux.NewClient(); err != nil {
				return ctx, fmt.Errorf("tmux unavailable: %w", err)
			}
			logrus.Info("tmux found")
			return ctx, nil
		},
		Action: servercommand.ExecuteAgent,
	})
}
