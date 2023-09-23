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
	"sync/atomic"
	"time"

	mcnet "github.com/Tnze/go-mc/net"
	"github.com/charmbracelet/log"
	"github.com/jaredallard/minecraft-preempt/internal/cloud"
	"github.com/jaredallard/minecraft-preempt/internal/cloud/docker"
	"github.com/jaredallard/minecraft-preempt/internal/cloud/gcp"
	"github.com/jaredallard/minecraft-preempt/internal/config"
	"github.com/jaredallard/minecraft-preempt/internal/minecraft"
)

// Server is a proxy server
type Server struct {
	*mcnet.Listener

	// cloud is the cloud provider we're using for this server
	cloud      cloud.Provider
	instanceID string

	// log is our server's logger
	log *log.Logger

	// config is our server's configuration
	config *config.ServerConfig

	// lastMinecraftStatus is the last status we got from the minecraft server
	lastMinecraftStatus atomic.Pointer[minecraft.Status]

	// emptySince is the time we've been empty (had no connections)
	// since
	emptySince atomic.Pointer[time.Time]

	// connections is the number of connections we have
	connections atomic.Uint64
}

// GetCloudProviderForConfig returns a cloud provider for the provided config
func GetCloudProviderForConfig(conf *config.ServerConfig) (cloud.Provider, string, error) {
	var (
		cloudProvider cloud.Provider
		instanceID    string
		err           error
	)

	if conf.GCP != nil && conf.Docker != nil {
		return nil, "", fmt.Errorf("cannot specify both GCP and Docker")
	}

	if conf.GCP != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		cloudProvider, err = gcp.NewClient(ctx, conf.GCP.Project, conf.GCP.Zone)
		instanceID = conf.GCP.InstanceID
	} else if conf.Docker != nil {
		cloudProvider, err = docker.NewClient()
		instanceID = conf.Docker.ContainerID
	} else {
		err = fmt.Errorf("no cloud provider specified")
	}

	return cloudProvider, instanceID, err
}

// NewServer creates a new server
func NewServer(log *log.Logger, conf *config.ServerConfig) (*Server, error) {
	cloudProvider, instanceID, err := GetCloudProviderForConfig(conf)
	if err != nil {
		return nil, err
	}

	return &Server{
		cloud:      cloudProvider,
		instanceID: instanceID,
		log:        log,
		config:     conf,
	}, nil
}

// GetStatus returns the status of the server
func (s *Server) GetStatus(ctx context.Context) (cloud.ProviderStatus, error) {
	return s.cloud.Status(ctx, s.instanceID)
}

// GetMinecraftStatus return the minecraft server's status, this requires
// the server to be running.
func (s *Server) GetMinecraftStatus() (*minecraft.Status, error) {
	return minecraft.GetServerStatus(s.config.Minecraft.Hostname, s.config.Minecraft.Port)
}

// Stop stops the server
func (s *Server) Stop(ctx context.Context) error {
	status, err := s.cloud.Status(ctx, s.instanceID)
	if err != nil {
		return err
	}

	// if the server is already stopped, don't stop it
	if status == cloud.StatusStopped {
		return nil
	}

	return s.cloud.Stop(ctx, s.instanceID)
}

// Start starts the server
func (s *Server) Start(ctx context.Context) error {
	status, err := s.cloud.Status(ctx, s.instanceID)
	if err != nil {
		return err
	}

	// if the server is already running, don't start it
	if status == cloud.StatusRunning {
		return nil
	}

	return s.cloud.Start(ctx, s.instanceID)
}
