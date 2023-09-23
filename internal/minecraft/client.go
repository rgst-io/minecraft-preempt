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

package minecraft

import (
	"encoding/json"
	"fmt"

	mcnet "github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/pkg/errors"
)

// Client is a minecraft protocol aware connection somewhat
type Client struct {
	*mcnet.Conn

	ProtocolVersion int32
}

// ClientState is the state of the client during
// the handshake process
type ClientState int

// Contains definitions for the ClientState type
const (
	// ClientStateCheck is when a client is attempting to check the status
	// of the server
	ClientStateCheck ClientState = iota + 1

	// ClientStatePlayerLogin is the state of the client when trying to login to
	// the server
	ClientStatePlayerLogin
)

// Handshake is the first packet sent to a Minecraft server.
//
// See: https://wiki.vg/Protocol#Handshaking
type Handshake struct {
	// Packet is the original packet that was sent, and consumed, by the
	// proxy.
	Packet *pk.Packet

	// ProtocolVersion is the version of the Minecraft protocol the client
	// is using.
	ProtocolVersion int32

	// ServerAddress is the address of the server the client is trying to
	// connect to.
	ServerAddress string

	// ServerPort is the port of the server the client is trying to
	// connect to.
	ServerPort uint16

	// NextState is the next state the client is trying to transition to.
	NextState int32
}

// Handshake reads the handshake packet and returns the next state
func (c *Client) Handshake() (*Handshake, error) {
	var p pk.Packet
	if err := c.ReadPacket(&p); err != nil {
		return nil, fmt.Errorf("failed to read packet: %w", err)
	}
	if p.ID != 0 {
		return nil, fmt.Errorf("packet ID 0x%X is not handshake", p.ID)
	}

	h := &Handshake{Packet: &p}
	if err := p.Scan(
		(*pk.VarInt)(&h.ProtocolVersion),
		(*pk.String)(&h.ServerAddress),
		(*pk.UnsignedShort)(&h.ServerPort),
		(*pk.VarInt)(&h.NextState)); err != nil {
		return nil, fmt.Errorf("failed to scan packet: %w", err)
	}

	// set the protocol version on the client.
	c.ProtocolVersion = h.ProtocolVersion

	return h, nil
}

// LoginStart is the packet sent by the client when they're trying to login
// to the server.
//
// See https://wiki.vg/Protocol#Login_Start.
type LoginStart struct {
	Name string `json:"name"`
}

// ReadLoginStart reads the login start packet from the client. It returns
// the parsed packet, the original packet, and an error if one occurred.
//
// This should not be trusted as the client can send any string they want
// and hasn't been authenticated until later in the login process.
func (c *Client) ReadLoginStart() (*LoginStart, *pk.Packet, error) {
	var p pk.Packet
	if err := c.ReadPacket(&p); err != nil {
		return nil, nil, err
	}
	if p.ID != 0 {
		return nil, nil, fmt.Errorf("packet ID 0x%X is not login start", p.ID)
	}

	var name pk.String
	if err := p.Scan(&name); err != nil {
		return nil, nil, err
	}

	return &LoginStart{Name: string(name)}, &p, nil
}

// SendDisconnect sends a disconnect packet to the client with the provided reason
func (c *Client) SendDisconnect(reason string) error {
	disconnect := map[string]interface{}{
		"translate": "chat.type.text",
		"with": []interface{}{
			map[string]interface{}{
				"text": reason,
			},
		},
	}

	b, err := json.Marshal(disconnect)
	if err != nil {
		return err
	}

	return c.WritePacket(pk.Marshal(0x00, pk.String(string(b))))
}

// SendStatus sends status request and ping packets to the client
func (c *Client) SendStatus(status *Status) error {
	for i := 0; i < 2; i++ {
		var p pk.Packet
		if err := c.ReadPacket(&p); err != nil {
			break
		}

		var err error
		switch p.ID {
		case 0x00: // Status Request
			b, jerr := json.Marshal(status)
			if jerr == nil {
				err = c.WritePacket(pk.Marshal(p.ID, pk.String(b)))
			}
		case 0x01: // Ping
			err = c.WritePacket(p)
		}
		if err != nil {
			return errors.Wrapf(err, "failed to send %d packet", p.ID)
		}
	}
	return nil
}
