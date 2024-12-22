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
	b, _, err := bot.PingAndListTimeout(fmt.Sprintf("%s:%d", addr, port), time.Second*30)
	if err != nil {
		return nil, errors.Wrap(err, "failed to ping server")
	}

	var s Status
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, errors.Wrap(err, "failed to parse response")
	}
	return &s, nil
}
