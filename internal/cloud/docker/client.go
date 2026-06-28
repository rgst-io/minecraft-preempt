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

// Package docker contains an implementation of the cloud package's
// interface that uses Docker as the backing implementation.
package docker

import (
	"context"
	"os"

	dockerclient "github.com/moby/moby/client"
	"github.com/pkg/errors"
	"go.rgst.io/idlerealm/minecraft-preempt/v4/internal/cloud"
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
	c, err := dockerclient.New(dockerclient.FromEnv)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create docker client")
	}

	return &Client{c}, nil
}

// Start starts a container
func (c *Client) Start(ctx context.Context, containerID string) error {
	resp, err := c.d.ContainerInspect(ctx, containerID, dockerclient.ContainerInspectOptions{})
	if err != nil {
		return err
	}

	if resp.Container.State.Status != "exited" {
		return ErrNotStopped
	}

	_, err = c.d.ContainerStart(ctx, resp.Container.ID, dockerclient.ContainerStartOptions{})
	return err
}

// Status returns the status of a container
func (c *Client) Status(ctx context.Context, containerID string) (cloud.ProviderStatus, error) {
	resp, err := c.d.ContainerInspect(ctx, containerID, dockerclient.ContainerInspectOptions{})
	if err != nil {
		return "", err
	}

	switch resp.Container.State.Status {
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
	resp, err := c.d.ContainerInspect(ctx, containerID, dockerclient.ContainerInspectOptions{})
	if err != nil {
		return err
	}

	if resp.Container.State.Status == "exited" {
		return ErrNotStopped
	}

	_, err = c.d.ContainerStop(ctx, resp.Container.ID, dockerclient.ContainerStopOptions{})
	return err
}

// ShouldTerminate returns true if the instance should be terminated.
func (c *Client) ShouldTerminate(_ context.Context) (bool, error) {
	// Here for tests, if `shutdown.txt` exists, shutdown.
	if _, err := os.Stat("shutdown.txt"); err == nil {
		return true, nil
	}

	// There is currently no dynamic system in place to restart in the
	// case of using Docker.
	return false, nil
}
