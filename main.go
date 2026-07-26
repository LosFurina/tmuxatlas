package main

import (
	"context"
	"os"

	"github.com/rancher/wrangler/pkg/signals"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"

	"github.com/LosFurina/tmuxatlas/pkg/common"

	_ "github.com/LosFurina/tmuxatlas/pkg/commands/agent-setup"
	_ "github.com/LosFurina/tmuxatlas/pkg/commands/install"
	_ "github.com/LosFurina/tmuxatlas/pkg/commands/notify"
	_ "github.com/LosFurina/tmuxatlas/pkg/commands/pair"
	_ "github.com/LosFurina/tmuxatlas/pkg/commands/peers"
	_ "github.com/LosFurina/tmuxatlas/pkg/commands/server"
)

func main() {
	var exitCode int

	func() {
		defer func() {
			if r := recover(); r != nil {
				logrus.WithField("panic", r).Error("panic recovered")
				exitCode = 1
			}
		}()

		app := &cli.Command{
			Name:    common.AppVersion.Name,
			Usage:   "web dashboard for monitoring and interacting with tmux sessions",
			Version: common.AppVersion.Summary,
			Authors: []any{
				"LosFurina",
			},
			Commands: common.GetCommands(),
			CommandNotFound: func(ctx context.Context, command *cli.Command, s string) {
				logrus.WithField("command", s).Error("command not found")
			},
			EnableShellCompletion: true,
			Before:                common.Before,
			Flags:                 common.Flags(),
		}

		ctx := signals.SetupSignalContext()
		if err := app.Run(ctx, os.Args); err != nil {
			logrus.WithError(err).Error("fatal error")
			exitCode = 1
		}
	}()

	if exitCode != 0 {
		os.Exit(exitCode)
	}
}
