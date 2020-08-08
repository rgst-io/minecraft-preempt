package minecraft

import (
	"encoding/json"
	"time"

	"github.com/Tnze/go-mc/bot"
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

// GetServerStatus returns a server's status
func GetServerStatus(addr string, port uint) (*Status, error) {
	b, _, err := bot.PingAndListTimeout(addr, int(port), time.Second*30)
	if err != nil {
		return nil, errors.Wrap(err, "failed to ping server")
	}

	var s Status
	return &s, errors.Wrap(json.Unmarshal(b, &s), "failed to parse response")
}
