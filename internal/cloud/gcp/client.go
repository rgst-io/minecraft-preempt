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

package gcp

import (
	"context"
	"errors"
	"net/http"

	"github.com/jaredallard/minecraft-preempt/internal/cloud"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
)

var (
	// ErrNotStopped is an error that is thrown when an instance is attempted
	// to be stopped but is found to be not running
	ErrNotStopped = errors.New("not stopped")
)

// Client is a gcs client
type Client struct {
	gclient *http.Client
	compute *compute.Service

	project string
	zone    string
}

// NewClient creates a new client
func NewClient(ctx context.Context, project, zone string) (*Client, error) {
	client, err := google.DefaultClient(ctx, compute.ComputeScope)
	if err != nil {
		return nil, err
	}

	comp, err := compute.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, err
	}

	return &Client{
		gclient: client,
		compute: comp,
		project: project,
		zone:    zone,
	}, nil
}

// Status returns the status of an instance
func (c *Client) Status(ctx context.Context, instanceID string) (cloud.ProviderStatus, error) {
	gr := c.compute.Instances.Get(c.project, c.zone, instanceID)
	i, err := gr.Do()
	if err != nil {
		return "", err
	}

	// HACK: handle invalid statuses
	st := cloud.ProviderStatus(i.Status)
	switch st {
	case cloud.StatusRunning, cloud.StatusStarting, cloud.StatusStopping, cloud.StatusStopped:
	case "TERMINATED":
		// Terminated is a special case, it's not really stopped
		// but can be treated as such
		st = cloud.StatusStopped
	default:
		st = cloud.StatusUnknown
	}

	// convert some of the types to "Starting"
	if i.Status == "STAGING" || i.Status == "PROVISIONING" {
		st = cloud.StatusStarting
	}

	return st, nil
}

// Start a instance if it's not already running
func (c *Client) Start(ctx context.Context, instanceID string) error {
	gr := c.compute.Instances.Get(c.project, c.zone, instanceID)
	i, err := gr.Do()
	if err != nil {
		return err
	}

	if i.Status != "STOPPED" && i.Status != "TERMINATED" {
		return ErrNotStopped
	}

	sr := c.compute.Instances.Start(c.project, c.zone, instanceID)
	_, err = sr.Do()
	return err
}

// Stop a instance if it's not already stopped
func (c *Client) Stop(ctx context.Context, instanceID string) error {
	_, err := c.compute.Instances.Stop(c.project, c.zone, instanceID).Do()
	return err
}
