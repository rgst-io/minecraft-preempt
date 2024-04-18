// Copyright (C) 2022 Jared Allard <jared@rgst.io>
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

package docker

import (
	"context"
	"os"

	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
	"github.com/jaredallard/minecraft-preempt/v3/internal/cloud"
	"github.com/pkg/errors"
)

// Contains all of the error types for this package
var (
	// ErrNotStopped is an error that is thrown when an instance is attempted to be
	// started but is found to be not stopped
	ErrNotStopped = errors.New("not stopped")
)

// Client is a docker client
type Client struct {
	d dockerclient.APIClient
}

// NewClient creates a new client
func NewClient() (*Client, error) {
	c, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return nil, errors.Wrap(err, "failed to create docker client")
	}

	return &Client{c}, nil
}

// Start starts a container
func (c *Client) Start(ctx context.Context, containerID string) error {
	cont, err := c.d.ContainerInspect(ctx, containerID)
	if err != nil {
		return err
	}

	if cont.State.Status != "exited" {
		return ErrNotStopped
	}

	return c.d.ContainerStart(ctx, cont.ID, container.StartOptions{})
}

// Status returns the status of a container
func (c *Client) Status(ctx context.Context, containerID string) (cloud.ProviderStatus, error) {
	cont, err := c.d.ContainerInspect(ctx, containerID)
	if err != nil {
		return "", err
	}

	switch cont.State.Status {
	case "exited", "dead":
		return cloud.StatusStopped, nil
	case "removing":
		return cloud.StatusStopping, nil
	case "running":
		return cloud.StatusRunning, nil
	case "created":
		return cloud.StatusStarting, nil
	}

	return cloud.StatusUnknown, nil
}

// Stop stops a container
func (c *Client) Stop(ctx context.Context, containerID string) error {
	cont, err := c.d.ContainerInspect(ctx, containerID)
	if err != nil {
		return err
	}

	if cont.State.Status == "exited" {
		return ErrNotStopped
	}

	return c.d.ContainerStop(ctx, cont.ID, container.StopOptions{})
}

// ShouldTerminate returns true if the instance should be terminated.
func (c *Client) ShouldTerminate(ctx context.Context) (bool, error) {
	// Here for tests, if `shutdown.txt` exists, shutdown.
	if _, err := os.Stat("shutdown.txt"); err == nil {
		return true, nil
	}

	// There is currently no dynamic system in place to restart in the
	// case of using Docker.
	return false, nil
}
