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

// Package main implements a minecraft server proxy that
// proxies connections to relevant servers, stopping and starting
// them as needed.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/charmbracelet/log"

	"github.com/function61/gokit/io/bidipipe"
	"github.com/getoutreach/gobox/pkg/async"
	"github.com/golang/glog"

	mcnet "github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"

	"github.com/jaredallard/minecraft-preempt/internal/cloud"
	"github.com/jaredallard/minecraft-preempt/internal/cloud/docker"
	"github.com/jaredallard/minecraft-preempt/internal/cloud/gcp"
	"github.com/jaredallard/minecraft-preempt/internal/config"
	"github.com/jaredallard/minecraft-preempt/internal/minecraft"
)

var (
	configPath = flag.String("configPath", "config/config.yaml", "Configuration File")
)

// Cached last status of the server
var (
	cachedStatus = cloud.StatusUnknown
)

const (
	CheckState = iota + 1
	PlayerLogin
)

func sendStatus(sconf *config.ServerConfig, mc *minecraft.Client) error {
	glog.Info("Handling status request")
	status := &minecraft.Status{
		Version: &minecraft.StatusVersion{
			Name:     sconf.Version,
			Protocol: int(sconf.ProtocolVersion),
		},
		Players: &minecraft.StatusPlayers{
			Max:    0,
			Online: 0,
		},
		Description: &minecraft.StatusDescription{
			Text: "",
		},
	}

	switch cachedStatus {
	case cloud.StatusRunning:
		newStatus, err := minecraft.GetServerStatus(sconf.Hostname, sconf.Port)
		if err != nil {
			status.Description.Text = "Server is online, but failed to proxy status"
		} else {
			status = newStatus
		}
	case cloud.StatusStarting:
		status.Description.Text = "Server is starting, please wait!"
	case cloud.StatusStopping:
		status.Description.Text = "Server is stopping, please wait to start it!"
	default:
		status.Description.Text = "Server is hibernated. Join to start it."
	}

	return mc.SendStatus(status)
}

// sendDisconnect sends a disconnect packet to the client
func sendDisconnect(mc *minecraft.Client, reason string) error {
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

	return mc.WritePacket(pk.Marshal(0x00, pk.String(string(b))))
}

// handle handles minecraft connections
func handle(ctx context.Context, conn mcnet.Conn, s *config.ServerConfig, instanceID string, cld cloud.Provider) {
	c := &minecraft.Client{Conn: &conn}
	defer c.Close()

	nextState, originalHandshake, err := c.Handshake()
	if err != nil {
		// don't log EOF
		if !errors.Is(err, io.EOF) {
			glog.Errorf("handshake failed: %v", err)
		}
		return
	}

	switch nextState {
	default:
		glog.Errorf("unknown next state: %d", nextState)
		return
	case CheckState:
		if err := sendStatus(s, c); err != nil {
			glog.Errorf("failed to send status: %v", err)
		}
		return
	case PlayerLogin:
		glog.Infof("Starting proxy session with %q", conn.Socket.RemoteAddr())

		// start the instance, if needed
		switch cachedStatus {
		case cloud.StatusRunning:
			// do nothing, we'll just proxy the connection
		case cloud.StatusStopped:
			glog.Infof("starting server ...")
			if err := sendDisconnect(c, "Server is starting"); err != nil {
				glog.Warningf("failed to send starting packet: %v", err)
			}

			if err := cld.Start(ctx, instanceID); err != nil {
				glog.Errorf("failed to start instance: %v", err)
				return
			}

			// update the status
			cachedStatus = cloud.StatusStarting
			return
		default: // not running or stopped, so we're starting or stopping
			if err := sendDisconnect(c, fmt.Sprintf("Waiting for server to start (Status: %q)", cachedStatus)); err != nil {
				glog.Warningf("failed to send starting packet: %v", err)
			}
			return
		}

		glog.Infof("Creating connection to '%s:%d'", s.Hostname, s.Port)
		rconn, err := mcnet.DialMC(s.Hostname + ":" + strconv.Itoa(int(s.Port)))
		if err != nil {
			glog.Errorf("failed to open connection to remote: %v", err)
			return
		}
		defer rconn.Close()

		// send the original handshake packet then pipe the rest
		glog.Info("Replaying original handshake packet")
		if err := rconn.WritePacket(*originalHandshake); err != nil {
			glog.Errorf("failed to write handshake to remote: %v", err)
		}

		glog.Info("Piping client <-> remote")
		if err := bidipipe.Pipe(
			bidipipe.WithName("client", conn.Socket),
			bidipipe.WithName("remote", rconn),
		); err != nil {
			glog.Errorf("failed to write to remote from client: %v", err)
			return
		}
	}
}

func main() {
	exitCode := 0
	defer func() {
		os.Exit(exitCode)
	}()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	log := log.New()
	log.SetReportCaller(true)

	conf, err := config.LoadProxyConfig(*configPath)
	if err != nil {
		log.Error("failed to load config", "err", err)
		return
	}

	var cloudProvider cloud.Provider
	var instanceID string

	switch conf.Cloud {
	case config.CloudGCP:
		instanceID = conf.CloudConfig.GCP.InstanceID
		cloudProvider, err = gcp.NewClient(ctx, conf.CloudConfig.GCP.Project, conf.CloudConfig.GCP.Zone)
	case config.CloudDocker:
		instanceID = conf.CloudConfig.Docker.ContainerID
		cloudProvider, err = docker.NewClient()
	default:
		err = fmt.Errorf("unknown cloud provider")
	}
	if err != nil {
		log.Error("failed to create cloud provider", "err", err, "cloud", conf.Cloud)
		return
	}

	if instanceID == "" {
		log.Error("instance ID is required")
		return
	}

	// Listen for incoming connections.
	glog.Infof("Creating proxy on '%s'", conf.ListenAddress)
	l, err := minecraft.ListenMC(conf.ListenAddress)
	if err != nil {
		glog.Errorf("failed to start proxy", "err", err)
		return
	}
	defer l.Close()

	// update the cached status every 5 minutes
	go func() {
		for ctx.Err() != nil {
			log.Info("Checking server status")
			status, err := cloudProvider.Status(ctx, instanceID)
			if err != nil {
				log.Warn("failed to get parent instance status", "err", err)
				return
			}
			log.Info("Server status report", "status", status)

			if cachedStatus != status {
				log.Info("Server status changed", "status.old", cachedStatus, "status.new", status)
				cachedStatus = status
			}

			async.Sleep(ctx, 5*time.Minute)
		}
	}()

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Error("failed to accept connection", "err", err)
			continue
		}
		go handle(ctx, conn, conf.Server, instanceID, cloudProvider)
	}
}
