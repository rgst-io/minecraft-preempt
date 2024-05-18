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
	"fmt"
	"io"
	"net"
	"time"

	mcnet "github.com/Tnze/go-mc/net"
	"github.com/charmbracelet/log"
	"github.com/jaredallard/minecraft-preempt/v3/internal/cloud"
	"github.com/jaredallard/minecraft-preempt/v3/internal/minecraft"
	"github.com/pkg/errors"
)

// Proxy is a proxy server
type Proxy struct {
	*mcnet.Listener

	// log is our proxy's logger
	log *log.Logger

	// listenAddress is the address to listen on (the proxy)
	listenAddress string

	// servers is a map of server hostnames to their server information.
	servers map[string]*Server
}

// NewProxy creates a new proxy
func NewProxy(log *log.Logger, listenAddress string, s []*Server) *Proxy {
	servers := make(map[string]*Server)
	for _, server := range s {
		servers[server.config.Hostname] = server
	}

	return &Proxy{
		log:           log,
		listenAddress: listenAddress,
		servers:       servers,
	}
}

// watcher is a status reporter for a proxy and stopper for a server
func (p *Proxy) watcher(ctx context.Context) error {
	for ctx.Err() == nil {
		// Sleep for 15 seconds while respecting context cancellation
		select {
		case <-time.After(15 * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}

		// for each server, check the status. Shutdown if we're empty longer
		// than our configured time.
		for serverAddress, server := range p.servers {
			log := p.log.With("server", serverAddress)
			// if we have connections, don't try to stop the server
			if server.connections.Load() != 0 {
				log.Info("Proxy status", "connections", server.connections.Load())
				continue
			}

			status, err := server.GetStatus(ctx)
			if err != nil {
				log.Error("failed to get server status", "err", err)
				continue
			}
			if status != cloud.StatusRunning {
				continue
			}

			// load the emptySince pointer and check if we've never been empty
			emptySincePtr := server.emptySince.Load()
			if emptySincePtr == nil {
				now := time.Now()
				emptySincePtr = &now
				server.emptySince.Store(emptySincePtr)
			}

			emptySince := *emptySincePtr
			shouldShutdown := time.Since(emptySince) > server.config.ShutdownAfter
			untilShutdown := time.Until(emptySince.Add(server.config.ShutdownAfter))

			log.Info("Proxy status", "connections", server.connections.Load(), "shutdown_in", untilShutdown)
			if shouldShutdown {
				log.Info("No connections in configured time, stopping server")
				if err := server.Stop(ctx); err != nil {
					log.Error("failed to stop server", "err", err)
				}

				// reset the emptySince time
				server.emptySince.Store(nil)
			}
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

	errChan := make(chan error)

	// start the watcher
	go func() {
		if err := p.watcher(ctx); err != nil {
			p.log.Error("proxy server watcher encountered unrecoverable error", "err", err)

			// signal to the main go-routine that the proxy has shut down.
			errChan <- err
		}
	}()

	connChan := make(chan mcnet.Conn)
	go func() {
		for {
			conn, err := p.accept()
			if err != nil {
				p.log.Error("failed to accept connection", "err", err)
			} else {
				connChan <- conn
			}

			if ctx.Err() != nil {
				// We've probably already exited out of the main go-routine by now, but just incase we
				// should communicate back.
				errChan <- ctx.Err()
			}
		}
	}()

	p.log.Info("Proxy started", "address", p.listenAddress)
	for {
		select {
		case err := <-errChan:
			return err
		case conn := <-connChan:
			if err := p.handleConnection(ctx, conn); err != nil {
				p.log.Error("failed to handle connection", "err", err)
			}
		}
	}
}

func (p *Proxy) handleConnection(ctx context.Context, rawConn mcnet.Conn) error {
	minecraftConn := &minecraft.Client{Conn: &rawConn}

	log := p.log.With("client", rawConn.Socket.RemoteAddr())
	h, err := minecraftConn.Handshake()
	if err != nil {
		// don't log EOF
		if !errors.Is(err, io.EOF) {
			return errors.Wrap(err, "failed to handshake")
		}
		return nil
	}

	// Determine the server from the handshake's address.
	server, ok := p.servers[h.ServerAddress]
	if !ok {
		log.Warn("Unknown server", "server", h.ServerAddress)
		return minecraftConn.SendDisconnect(fmt.Sprintf("Unknown server: %s", h.ServerAddress))
	}
	log = log.With("server", server.config.Hostname)

	// tracks if this connection made it to the login state
	// HACK(jaredallard): We should do something better than this.
	var madeItToLogin bool

	// create a new connection
	conn := NewConnection(minecraftConn, log, server, h, &ConnectionHooks{
		OnLogin: func(l *minecraft.LoginStart) {
			log.Info("Login initiated", "username", l.Name)
			// track that we made it to login state for connection
			// tracking
			madeItToLogin = true

			// reset the emptySince time
			server.emptySince.Store(nil)
			server.connections.Add(1)
		},
		OnClose: func() {
			// only decrement if we made it to login state, where we
			// would've incremented the connection count
			if madeItToLogin {
				server.connections.Add(^uint64(0))
			}
		},
	})
	connAddr := rawConn.Socket.RemoteAddr().String()

	// proxy the connection in a goroutine
	go func() {
		p.log.Debug("Handling connection", "addr", connAddr)
		if err := conn.Proxy(ctx); err != nil {
			p.log.Error("failed to proxy connection", "err", err)
		}
		defer conn.Close()

		p.log.Debug("Connection closed", "addr", connAddr)
	}()

	return nil
}

// accept accepts a connection on the proxy listener.
func (p *Proxy) accept() (mcnet.Conn, error) {
	// TODO(george-e-shaw-iv): Someone needs to go and make the underlying library respect context with net.ListenConfig.
	rawConn, err := p.Listener.Accept()
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return mcnet.Conn{}, err // Context cancellation is a real error.
		} else if errors.Is(err, net.ErrClosed) {
			return mcnet.Conn{}, nil // Closed connections are fine.
		}

		// All other errors are unknown and bad.
		return mcnet.Conn{}, fmt.Errorf("failed to accept connection: %w", err)
	}
	return rawConn, nil
}

// Stop stops the server
func (p *Proxy) Stop(ctx context.Context) error {
	if p.Listener != nil {
		return p.Listener.Close()
	}

	// wait for all connections to drain
	for _, server := range p.servers {
		connections := server.connections.Load()
		if connections > 0 {
			p.log.Info("Waiting for connections to drain during shutdown", "server", server.config.Hostname, "connections", connections)

			for {
				// check if we have no connections
				if server.connections.Load() == 0 {
					break // stop watching this server, but check others if present.
				}

				// wait before checking again, but also break if the context is cancelled
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(100 * time.Millisecond):
				}
			}
		}
	}

	return nil
}
