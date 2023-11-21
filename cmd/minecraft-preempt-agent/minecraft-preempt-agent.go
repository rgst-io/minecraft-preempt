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
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	logger "github.com/charmbracelet/log"
	"github.com/egym-playground/go-prefix-writer/prefixer"
	"github.com/jaredallard/minecraft-preempt/internal/cloud"
	"github.com/jaredallard/minecraft-preempt/internal/cloud/docker"
	"github.com/jaredallard/minecraft-preempt/internal/cloud/gcp"
	"github.com/jaredallard/minecraft-preempt/internal/version"
	"github.com/spf13/cobra"
)

// log is the global logger for the agent.
var log = logger.NewWithOptions(os.Stderr, logger.Options{
	ReportCaller:    true,
	ReportTimestamp: true,
	Level:           logger.DebugLevel,
})

// rootCmd is the root command used by cobra
var rootCmd = &cobra.Command{
	Use:     "minecraft-preempt-agent",
	Version: version.Version,

	Short: "minecraft-preempt-agent is a companion to the minecraft-preempt proxy",
	RunE:  entrypoint,
}

// entrypoint is the entrypoint for the root command
func entrypoint(cCmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(cCmd.Context())
	defer cancel()

	dc := cCmd.Flag("docker-compose-file").Value.String()
	cloudProvider := cCmd.Flag("cloud").Value.String()

	_, err := os.Stat(dc)
	if err != nil {
		return fmt.Errorf("failed to find docker-compose file: %w", err)
	}

	log.With("version", version.Version, "cloud", cloudProvider).Info("starting agent")

	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", dc, "up")
	cmd.Stdout = prefixer.New(os.Stdout, func() string { return "[docker-compose] " })
	cmd.Stderr = prefixer.New(os.Stderr, func() string { return "[docker-compose] " })
	// Process group will handle the signal, so we don't need to kill it ourselves.
	cmd.Cancel = func() error { return nil }

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start '%s': %w", cmd.String(), err)
	}

	// Start the watcher.
	if err := watcher(ctx, cancel, cloudProvider); err != nil {
		return fmt.Errorf("failed to start watcher: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		// Only report errors if the context wasn't canceled.
		if ctx.Err() == nil {
			return fmt.Errorf("failed to run '%s': %w", cmd.String(), err)
		}
	}

	log.Info("exited")

	return nil
}

// watcher uses cloud specific APIs to determine when this agent should
// terminate. The provided cancel function will be called when the agent
// should shutdown.
func watcher(ctx context.Context, cancel context.CancelFunc, cloudProvider string) error {
	var c cloud.Provider
	var err error

	switch cloudProvider {
	case "gcp":
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		c, err = gcp.NewClient(ctx, "", "")
	case "docker":
		c, err = docker.NewClient()
	}
	if err != nil {
		return fmt.Errorf("failed to start cloud watcher for cloud %s: %w", cloudProvider, err)
	}

	// Start the watcher.
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()

		for {
			// If we're canceled, exit.
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				shouldStop, err := c.ShouldTerminate(ctx)
				if err != nil {
					log.With("err", err).Warn("failed to determine if instance should terminate")
					continue
				}

				if shouldStop {
					log.Info("instance is being preempted, starting shutdown")
					cancel()
					return
				}
			}
		}
	}()

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
	rootCmd.PersistentFlags().String("cloud", "docker", "cloud provider to use")
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		log.With("err", err).Error("failed to run")
		exitCode = 1
	}
}
