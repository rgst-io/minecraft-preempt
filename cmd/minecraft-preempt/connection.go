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
	pk "github.com/Tnze/go-mc/net/packet"
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
	log *log.Logger

	// s is the server we're proxying to
	s *Server

	// hooks contains hooks that are called when certain events happen
	// on the connection.
	hooks *ConnectionHooks
}

// ConnectionHooks are hooks that are called when certain events happen
// on the connection.
type ConnectionHooks struct {
	// OnClose is called when the connection is closed
	OnClose func()

	// OnConnect is called when the connection is established
	OnConnect func()

	// OnLogin is called when the client sends a login packet
	OnLogin func(*minecraft.LoginStart)

	// OnStatus is called when the client sends a status packet
	OnStatus func()
}

// NewConnection creates a new connection to the provided server
func NewConnection(conn *mcnet.Conn, log *log.Logger, s *Server, h *ConnectionHooks) *Connection {
	return &Connection{&minecraft.Client{Conn: conn}, log, s, h}
}

// Close closes the connection
func (c *Connection) Close() error {
	if c.hooks.OnClose != nil {
		c.hooks.OnClose()
	}
	return c.Conn.Close()
}

// status implements the Status packet. Checks to see if the server
// is running or not. If the server is running, it proxies the status
// packet to the server and returns the response to the client.
//
// If the server is not running, it returns a status response with
// the server's status.
func (c *Connection) status(ctx context.Context, status cloud.ProviderStatus) error {
	if c.hooks.OnStatus != nil {
		c.hooks.OnStatus()
	}

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

	// send the status back to the client
	return errors.Wrap(c.SendStatus(mcStatus), "failed to send status response")
}

// isWhitelisted checks to see if the player is whitelisted on the server.
func (c *Connection) isWhitelisted(playerName string) bool {
	for _, name := range c.s.config.Whitelist {
		if name == playerName {
			return true
		}
	}

	return false
}

// checkState checks the state of the connection to see if we should send
// a status response, or if we should start a server.
func (c *Connection) checkState(ctx context.Context, state minecraft.ClientState) (replay []*pk.Packet, err error) {
	status, err := c.s.GetStatus(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get server status")
	}

	switch state {
	case minecraft.ClientStateCheck: // Status request
		c.status(ctx, status)
		return nil, nil
	case minecraft.ClientStatePlayerLogin: // Login request
		// read the next packet to get the login information
		login, originalLogin, err := c.ReadLoginStart()
		if err != nil {
			return nil, errors.Wrap(err, "failed to read login packet")
		}

		if c.hooks.OnLogin != nil {
			c.hooks.OnLogin(login)
		}

		// HACK: We'll want a better framework for "plugins" like this than
		// checkState.
		if len(c.s.config.Whitelist) > 0 {
			if !c.isWhitelisted(login.Name) {
				c.log.Info("Player is not whitelisted, disconnecting")
				if err := c.SendDisconnect("You are not whitelisted on this server"); err != nil {
					return nil, errors.Wrap(err, "failed to send disconnect message")
				}
			}
		}

		c.log.Debug("Client is requesting login, checking server status")
		if status != cloud.StatusRunning {
			c.log.Info("Server is not running, starting server")
			if err := c.s.Start(ctx); err != nil {
				return nil, errors.Wrap(err, "failed to start server")
			}

			// send disconnect message
			if err := c.SendDisconnect("Server is being started, please try again later"); err != nil {
				return nil, errors.Wrap(err, "failed to send disconnect message")
			}

			return nil, nil
		}

		// server is running, continue
		return []*pk.Packet{originalLogin}, nil
	default:
		return nil, errors.Errorf("unknown client state: %d", state)
	}
}

// Proxy proxies the connection to the server
func (c *Connection) Proxy(ctx context.Context) error {
	if c.hooks.OnConnect != nil {
		c.hooks.OnConnect()
	}

	c.log.Info("Proxying connection", "client", c.Conn.Socket.RemoteAddr())
	nextState, originalHandshake, err := c.Handshake()
	if err != nil {
		// don't log EOF
		if !errors.Is(err, io.EOF) {
			return errors.Wrap(err, "failed to handshake")
		}
		return nil
	}

	replayPackets, err := c.checkState(ctx, minecraft.ClientState(nextState))
	if err != nil {
		return errors.Wrap(err, "failed to check server status")
	}
	if len(replayPackets) == 0 {
		return nil
	}

	sconf := c.s.config.Minecraft
	c.log.Info("Creating connection to remote server", "host", sconf.Hostname, "port", sconf.Port)
	rconn, err := mcnet.DialMC(sconf.Hostname + ":" + strconv.Itoa(int(sconf.Port)))
	if err != nil {
		return errors.Wrap(err, "failed to connect to remote")
	}
	defer rconn.Close()

	// Replay the original handshake to the remote server
	for _, p := range append([]*pk.Packet{originalHandshake}, replayPackets...) {
		log.Debug("Replaying packet", "id", p.ID, "data", string(p.Data))
		if err := rconn.WritePacket(*p); err != nil {
			return errors.Wrap(err, "failed to write handshake")
		}
	}

	// Proxy the connection to the remote server
	if err := bidipipe.Pipe(
		bidipipe.WithName("client", c.Conn.Socket),
		bidipipe.WithName("remote", rconn),
	); err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
		return errors.Wrap(err, "failed to proxy")
	}

	return nil
}
