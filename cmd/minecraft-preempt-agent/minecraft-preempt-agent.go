// Copyright (C) 2023 Jared Allard <jared@rgst.io>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

// Package main implements a lightweight agent that runs a minecraft
// server (via docker-compose) and handles shutting down the server when
// the proxy informs it to.
//
// Currently, this doesn't do much but tell a prebuilt docker-compose
// stack to stop and start. Shutdown signals are handled by shutting
// down the VM, which this agent in turn listens to (either via Docker
// shutting itself down, or preempting the VM).
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/jaredallard/minecraft-preempt/internal/version"
	"github.com/spf13/cobra"
)

// rootCmd is the root command used by cobra
var rootCmd = &cobra.Command{
	Use:     "minecraft-preempt-agent",
	Version: version.Version,

	Short: "minecraft-preempt-agent is a companion to the minecraft-preempt proxy",
	RunE:  entrypoint,
}

// entrypoint is the entrypoint for the root command
func entrypoint(cCmd *cobra.Command, args []string) error {
	ctx := cCmd.Context()
	dc := cCmd.Flag("docker-compose-file").Value.String()

	_, err := os.Stat(dc)
	if err != nil {
		return fmt.Errorf("failed to find docker-compose file: %w", err)
	}

	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", dc, "up")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Process group will handle the signal, so we don't need to kill it ourselves.
	cmd.Cancel = func() error { return nil }
	if err := cmd.Run(); err != nil {
		// if we're context canceled, don't error
		if errors.Is(err, context.Canceled) {
			return nil
		}

		return fmt.Errorf("failed to run '%s': %w", cmd.String(), err)
	}

	fmt.Println("minecraft-preempt-agent: exiting")

	return nil
}

// main is the entrypoint for the proxy
func main() {
	exitCode := 0
	defer func() {
		os.Exit(exitCode)
	}()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	rootCmd.PersistentFlags().String("docker-compose-file", "docker-compose.yml", "path to docker-compose.yml")
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		exitCode = 1
	}
}
