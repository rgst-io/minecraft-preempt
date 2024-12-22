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

// Package main implements a minecraft server proxy that
// proxies connections to relevant servers, stopping and starting
// them as needed.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	"github.com/jaredallard/minecraft-preempt/v3/internal/config"
	"github.com/jaredallard/minecraft-preempt/v3/internal/version"
)

// rootCmd is the root command used by cobra
var rootCmd = &cobra.Command{
	Use:     "minecraft-preempt",
	Version: version.Version,

	Short: "minecraft-preempt is a proxy for minecraft servers that can start and stop them",
	Long: `minecraft-preempt is a proxy for minecraft servers that can start and stop them based on ` +
		`the number of connections to them.` + "\n" + `This is useful for running a large number of servers ` +
		`on a single machine, and only having them running when needed.`,
	Run: entrypoint,
}

// entrypoint is the entrypoint for the root command
func entrypoint(cmd *cobra.Command, _ []string) {
	ctx := cmd.Context()

	//nolint:gocritic // Why: OK shadowing log.
	log := log.NewWithOptions(os.Stderr, log.Options{
		ReportCaller:    true,
		ReportTimestamp: true,
		Level:           log.DebugLevel,
	})

	confPath, err := cmd.Flags().GetString("config")
	if err != nil {
		log.Error("failed to get config path", "err", err)
		return
	}

	conf, err := config.LoadProxyConfig(confPath)
	if err != nil {
		log.Error("failed to load config", "err", err)
		return
	}

	servers := make([]*Server, len(conf.Servers))
	for i := range conf.Servers {
		sconf := &conf.Servers[i]
		logger := log.With("server", sconf.Hostname)

		logger.Info("Creating Server")
		s, err := NewServer(logger, sconf)
		if err != nil {
			log.Error("failed to create server", "err", err)
			return
		}
		servers[i] = s
	}

	finisedChan := make(chan struct{})
	p := NewProxy(log, conf.ListenAddress, servers)

	// start the proxy in a goroutine so we can wait for it to exit later.
	go func() {
		if err := p.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Error("proxy exited", "err", err)
		}
		log.Info("Proxy exited")

		close(finisedChan)
	}()

	// wait for the context to be cancelled
	<-ctx.Done()
	log.Info("Shutting down")

	// create a new context with a 15 second timeout
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// stop the proxy
	if err := p.Stop(ctx); err != nil {
		log.Warn("failed to stop proxy", "err", err)
	}
	<-finisedChan

	log.Info("Shutdown complete")
}

// main is the entrypoint for the proxy
func main() {
	exitCode := 0
	defer func() {
		os.Exit(exitCode)
	}()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	rootCmd.PersistentFlags().String("config", "./config/config.yaml", "config file")
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		exitCode = 1
	}
}
