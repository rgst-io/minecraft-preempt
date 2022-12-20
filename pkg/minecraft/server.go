package minecraft

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/Tnze/go-mc/bot"
	mcnet "github.com/Tnze/go-mc/net"
	"github.com/pkg/errors"
)

type Status struct {
	Version     *StatusVersion     `json:"version"`
	Players     *StatusPlayers     `json:"players"`
	Description *StatusDescription `json:"description"`
	Favicon     string             `json:"favicon"`
}

type StatusVersion struct {
	Name     string `json:"name"`
	Protocol int    `json:"protocol"`
}

type StatusPlayers struct {
	Max    int           `json:"max"`
	Online int           `json:"online"`
	Sample []interface{} `json:"sample"`
}

type StatusDescription struct {
	Text string `json:"text"`
}

// ListenMC listens on a minecraft port
func ListenMC(addr string) (*mcnet.Listener, error) {
	return mcnet.ListenMC(addr)
}

// GetServerStatus returns a server's status
func GetServerStatus(addr string, port uint) (*Status, error) {
	b, _, err := bot.PingAndListTimeout(addr+":"+strconv.Itoa(int(port)), time.Second*30)
	if err != nil {
		return nil, errors.Wrap(err, "failed to ping server")
	}

	var s Status
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, errors.Wrap(err, "failed to parse response")
	}
	return &s, nil
}
