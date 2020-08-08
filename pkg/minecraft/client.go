package minecraft

import (
	"encoding/json"
	"fmt"

	pk "github.com/Tnze/go-mc/net/packet"
)

// Client is a minecraft protocol aware connection somewhat
type Client struct {
	Conn

	ProtocolVersion int32
}

func (c *Client) Handshake() (nextState int32, err error) {
	p, err := c.ReadPacket()
	if err != nil {
		return -1, err
	}
	if p.ID != 0 {
		return -1, fmt.Errorf("packet ID 0x%X is not handshake", p.ID)
	}

	var (
		sid pk.String
		spt pk.Short
	)
	if err := p.Scan(
		(*pk.VarInt)(&c.ProtocolVersion),
		&sid, &spt,
		(*pk.VarInt)(&nextState)); err != nil {
		return -1, err
	}

	return nextState, nil
}

func (c *Client) SendStatus(status *Status) {
	for i := 0; i < 2; i++ {
		p, err := c.ReadPacket()
		if err != nil {
			break
		}

		switch p.ID {
		case 0x00:
			b, err := json.Marshal(status)
			if err == nil {
				c.WritePacket(pk.Packet{
					ID:   0x00,
					Data: pk.String(b).Encode(),
				})
			}
		case 0x01:
			c.WritePacket(p)
		}
	}
}
