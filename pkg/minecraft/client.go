package minecraft

import (
	"fmt"

	pk "github.com/Tnze/go-mc/net/packet"
)

// Client is a minecraft protocol aware connection somewhat
type Client struct {
	Conn

	ProtocolVersion int32
}

// getStatus returns a templated status
func getStatus(version string, protoVersion int, statusMessage string) pk.Packet {
	return pk.Packet{
		ID: 0x00,
		Data: pk.String(fmt.Sprintf(`
		{
			"version": {
				"name": "%s",
				"protocol": %d
			},
			"players": {
				"max": 1,
				"online": 0,
				"sample": []
			},	
			"description": {
				"text": "%s"
			}
		}
		`, version, protoVersion, statusMessage)).Encode(),
	}
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

func (c *Client) SendStatus(version string, protoVersion int, statusMessage string) {
	for i := 0; i < 2; i++ {
		p, err := c.ReadPacket()
		if err != nil {
			break
		}

		switch p.ID {
		case 0x00:
			respPack := getStatus(version, protoVersion, statusMessage)
			c.WritePacket(respPack)
		case 0x01:
			c.WritePacket(p)
		}
	}
}
