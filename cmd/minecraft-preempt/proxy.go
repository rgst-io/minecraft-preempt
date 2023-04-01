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

package main

import (
	"context"
	"net"
	"sync/atomic"
	"time"

	mcnet "github.com/Tnze/go-mc/net"
	"github.com/charmbracelet/log"
	"github.com/getoutreach/gobox/pkg/async"
	"github.com/jaredallard/minecraft-preempt/internal/cloud"
	"github.com/jaredallard/minecraft-preempt/internal/minecraft"
	"github.com/pkg/errors"
)

// Proxy is a proxy server
type Proxy struct {
	*mcnet.Listener

	// log is our proxy's logger
	log log.Logger

	// listenAddress is the address to listen on (the proxy)
	listenAddress string

	// server is the underlying server that this
	// proxy is proxying to
	server *Server

	// connections is the number of connections we have
	connections atomic.Uint64

	// emptySince is the time we've been empty (had no connections)
	// since
	emptySince atomic.Pointer[time.Time]
}

// NewProxy creates a new proxy
func NewProxy(log log.Logger, listenAddress string, s *Server) *Proxy {
	return &Proxy{
		log:           log,
		listenAddress: listenAddress,
		server:        s,
	}
}

// watcher is a status reporter for a proxy and stopper for a server
func (p *Proxy) watcher(ctx context.Context) error {
	for ctx.Err() == nil {
		if async.Sleep(ctx, 15*time.Second); ctx.Err() != nil {
			return nil
		}

		// if we have connections, don't try to stop the server
		if p.connections.Load() != 0 {
			p.log.Info("Proxy status", "connections", p.connections.Load())
			continue
		}

		status, err := p.server.GetStatus(ctx)
		if err != nil {
			p.log.Error("failed to get server status", "err", err)
			continue
		}
		if status != cloud.StatusRunning {
			p.log.Debug("Server is not running, skipping shutdown check")
			continue
		}

		// load the emptySince pointer and check if we've never been empty
		emptySincePtr := p.emptySince.Load()
		if emptySincePtr == nil {
			now := time.Now()
			emptySincePtr = &now
			p.emptySince.Store(emptySincePtr)
		}

		emptySince := *emptySincePtr
		shouldShutdown := time.Since(emptySince) > p.server.config.ShutdownAfter
		untilShutdown := time.Until(emptySince.Add(p.server.config.ShutdownAfter))

		p.log.Info("Proxy status", "connections", p.connections.Load(), "shutdown_in", untilShutdown)

		// if we've been empty for X time, stop the server
		// TODO(jaredallard): make this configurable
		if shouldShutdown {
			p.log.Info("No connections in configured time, stopping server")
			if err := p.server.Stop(ctx); err != nil {
				p.log.Error("failed to stop server", "err", err)
			}

			// reset the emptySince time
			p.emptySince.Store(nil)
		}
	}
	return ctx.Err()
}

// Start starts the proxy to the server, this is a blocking call
func (p *Proxy) Start(ctx context.Context) error {
	l, err := minecraft.ListenMC(p.listenAddress)
	if err != nil {
		return errors.Wrap(err, "failed to listen on address")
	}
	p.Listener = l

	// attempt to populate the server's status
	mcStatus, err := minecraft.GetServerStatus(p.server.config.Minecraft.Hostname, p.server.config.Minecraft.Port)
	if err != nil {
		p.log.Warn("Failed to get initial server status for protocol detection, will fetch on first connection", "err", err)
	} else {
		p.server.lastMinecraftStatus.Store(mcStatus)
		p.log.Info("Detected server version", "version", mcStatus.Version.Name, "protocol", mcStatus.Version.Protocol)
	}

	// start the watcher
	go func() {
		if err := p.watcher(ctx); err != nil {
			p.log.Error("failed to start watcher", "err", err)
			// TODO(jaredallard): handle this better?
		}
	}()

	p.log.Info("Proxy started", "address", p.listenAddress)

	for {
		rawConn, err := l.Accept()
		if err != nil {
			// handle context cancel or net closed
			if errors.Is(err, context.Canceled) || errors.Is(err, net.ErrClosed) {
				// context cancel should propagate the error
				if errors.Is(err, context.Canceled) {
					return err
				}

				// successful exit if we're closed
				return nil
			}

			// otherwise, log the error and continue
			log.Error("failed to accept connection", "err", err)
			continue
		}

		// create a new connection
		conn := NewConnection(&rawConn, p.log, p.server, &ConnectionHooks{
			OnLogin: func() {
				// reset the emptySince time
				p.emptySince.Store(nil)
				p.connections.Add(1)
			},
			OnClose: func() {
				p.connections.Add(^uint64(0))
			},
		})
		connAddr := rawConn.Socket.RemoteAddr().String()

		// proxy the connection in a goroutine
		p.log.Debug("Handling connection", "addr", connAddr)
		go func() {
			if err := conn.Proxy(ctx); err != nil {
				p.log.Error("failed to proxy connection", "err", err)
			}
			defer conn.Close()

			p.log.Info("Connection closed", "addr", connAddr)
		}()
	}
}

// Stop stops the server
func (p *Proxy) Stop(ctx context.Context) error {
	if p.Listener != nil {
		return p.Listener.Close()
	}

	// wait for all connections to drain
	if p.connections.Load() > 0 {
		p.log.Info("Waiting for connections to drain during shutdown")

		for {
			// check if we have no connections
			if p.connections.Load() == 0 {
				break
			}

			// sleep for 100ms
			time.Sleep(100 * time.Millisecond)

			// handle context cancellation
			if err := ctx.Err(); err != nil {
				return err
			}
		}
	}

	return nil
}
