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

// Handshake reads the handshake packet and returns the next state
func (c *Client) Handshake() (nextState int32, original *pk.Packet, err error) {
	var p pk.Packet
	if err := c.ReadPacket(&p); err != nil {
		return -1, nil, err
	}
	if p.ID != 0 {
		return -1, nil, fmt.Errorf("packet ID 0x%X is not handshake", p.ID)
	}

	var (
		sid pk.String
		spt pk.Short
	)
	if err := p.Scan(
		(*pk.VarInt)(&c.ProtocolVersion),
		&sid, &spt,
		(*pk.VarInt)(&nextState)); err != nil {
		return -1, nil, err
	}

	return nextState, &p, nil
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
