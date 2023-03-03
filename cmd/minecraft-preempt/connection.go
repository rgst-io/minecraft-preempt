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
	"strconv"
	"strings"

	mcnet "github.com/Tnze/go-mc/net"
	"github.com/charmbracelet/log"
	"github.com/function61/gokit/io/bidipipe"
	"github.com/jaredallard/minecraft-preempt/internal/cloud"
	"github.com/jaredallard/minecraft-preempt/internal/minecraft"
	"github.com/pkg/errors"
)

// Connection is a connection to our proxy instance.
type Connection struct {
	*minecraft.Client

	// log is our connection's logger
	log log.Logger

	// s is the server we're proxying to
	s *Server
}

// NewConnection creates a new connection to the provided server
func NewConnection(conn *mcnet.Conn, log log.Logger, s *Server) *Connection {
	return &Connection{&minecraft.Client{Conn: conn}, log, s}
}

// Close closes the connection
func (c *Connection) Close() error {
	return c.Conn.Close()
}

// checkState checks the state of the connection to see if we should send
// a status response, or if we should start a server.
func (c *Connection) checkState(ctx context.Context, clientState minecraft.ClientState) (shouldContinue bool, err error) {
	status, err := c.s.GetStatus(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to get server status")
	}

	switch clientState {
	case minecraft.ClientStateCheck:
		c.log.Debug("Client is requesting status, sending status response")

		var mcStatus *minecraft.Status

		// attempt to get the status of the server from the server
		if status == cloud.StatusRunning {
			var err error
			mcStatus, err = c.s.GetMinecraftStatus()
			if err != nil {
				c.log.Warn("Failed to get server status", "err", err)
				status = cloud.StatusUnknown
			} else if mcStatus.Version != nil {
				c.log.Debug("Remote server information",
					"version.name", mcStatus.Version.Name,
					"version.protocol", mcStatus.Version.Protocol,
				)
				c.s.lastMinecraftStatus.Store(mcStatus)
			}
		}

		// Server isn't running, or we failed to get the status
		if mcStatus == nil {
			// Not running, or something else, build a status
			// response with the server offline.
			v := &minecraft.StatusVersion{
				Name: "unknown",
				// TODO(jaredallard): How do we handle this? 754 works
				// for 1.16.5+ but not below.
				Protocol: 754,
			}

			// attempt to read the version information out of the last status
			// we received from the server.
			if c.s.lastMinecraftStatus.Load() != nil {
				lastMcStatus := c.s.lastMinecraftStatus.Load()
				v = lastMcStatus.Version
			}

			mcStatus = &minecraft.Status{
				Version: v,
				Players: &minecraft.StatusPlayers{
					Max:    0,
					Online: 0,
				},
				Description: &minecraft.StatusDescription{
					Text: fmt.Sprintf("Server status: %s", status),
				},
			}
		}

		if err := c.SendStatus(mcStatus); err != nil {
			return false, errors.Wrap(err, "failed to send status response")
		}
		return false, nil
	case minecraft.ClientStatePlayerLogin:
		c.log.Debug("Client is requesting login, checking server status ")
		if status != cloud.StatusRunning {
			c.log.Info("Server is not running, starting server")
			if err := c.s.Start(ctx); err != nil {
				return false, errors.Wrap(err, "failed to start server")
			}

			// send disconnect message
			if err := c.SendDisconnect("Server is being started, please try again later"); err != nil {
				return false, errors.Wrap(err, "failed to send disconnect message")
			}

			return false, nil
		}

		// server is running, continue
		return true, nil
	default:
		return false, errors.Errorf("unknown client state: %d", clientState)
	}
}

// Proxy proxies the connection to the server
func (c *Connection) Proxy(ctx context.Context) error {
	c.log.Info("Proxying connection", "client", c.Conn.Socket.RemoteAddr())
	nextState, originalHandshake, err := c.Handshake()
	if err != nil {
		// don't log EOF
		if !errors.Is(err, io.EOF) {
			return errors.Wrap(err, "failed to handshake")
		}
		return nil
	}

	shouldContinue, err := c.checkState(ctx, minecraft.ClientState(nextState))
	if err != nil {
		return errors.Wrap(err, "failed to check server status")
	}
	if !shouldContinue {
		return nil
	}

	sconf := c.s.config.Minecraft
	c.log.Info("Creating connection to remote server", "host", sconf.Hostname, "port", sconf.Port)
	rconn, err := mcnet.DialMC(sconf.Hostname + ":" + strconv.Itoa(int(sconf.Port)))
	if err != nil {
		return errors.Wrap(err, "failed to connect to remote")
	}
	defer rconn.Close()

	if err := rconn.WritePacket(*originalHandshake); err != nil {
		return errors.Wrap(err, "failed to write handshake")
	}

	if err := bidipipe.Pipe(
		bidipipe.WithName("client", c.Conn.Socket),
		bidipipe.WithName("remote", rconn),
	); err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
		// TODO(jaredallard): Figure out why this errors on Status calls.
		return errors.Wrap(err, "failed to proxy")
	}

	return nil
}
