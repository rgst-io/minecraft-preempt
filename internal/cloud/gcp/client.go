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
	"fmt"
	"strings"

	"github.com/jaredallard/minecraft-preempt/internal/cloud"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"cloud.google.com/go/compute/metadata"
)

var (
	// ErrNotStopped is an error that is thrown when an instance is attempted
	// to be stopped but is found to be not running
	ErrNotStopped = errors.New("not stopped")
)

// Client is a gcs client
type Client struct {
	compute  *compute.InstancesClient
	metadata *metadata.Client

	project string
	zone    string
}

// NewClient creates a new client
func NewClient(ctx context.Context, project, zone string) (*Client, error) {
	computeCli, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return nil, err
	}

	return &Client{
		compute:  computeCli,
		metadata: metadata.NewClient(nil),
		project:  project,
		zone:     zone,
	}, nil
}

// Status returns the status of an instance
func (c *Client) Status(ctx context.Context, instanceID string) (cloud.ProviderStatus, error) {
	inst, err := c.compute.Get(ctx, &computepb.GetInstanceRequest{
		Project:  c.project,
		Zone:     c.zone,
		Instance: instanceID,
	})
	if err != nil {
		return "", err
	}

	// HACK: handle invalid statuses
	st := cloud.ProviderStatus(inst.GetStatus())
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
	if inst.GetStatus() == "STAGING" || inst.GetStatus() == "PROVISIONING" {
		st = cloud.StatusStarting
	}

	return st, nil
}

// Start a instance if it's not already running
func (c *Client) Start(ctx context.Context, instanceID string) error {
	inst, err := c.compute.Get(ctx, &computepb.GetInstanceRequest{
		Project:  c.project,
		Zone:     c.zone,
		Instance: instanceID,
	})
	if err != nil {
		return err
	}

	if inst.GetStatus() != "STOPPED" && inst.GetStatus() != "TERMINATED" {
		return ErrNotStopped
	}

	_, err = c.compute.Start(ctx, &computepb.StartInstanceRequest{
		Project:  c.project,
		Zone:     c.zone,
		Instance: instanceID,
	})
	return err
}

// Stop a instance if it's not already stopped
func (c *Client) Stop(ctx context.Context, instanceID string) error {
	_, err := c.compute.Stop(ctx, &computepb.StopInstanceRequest{
		Project:  c.project,
		Zone:     c.zone,
		Instance: instanceID,
	})
	return err
}

// ShouldTerminate checks the current instance's status to see if it's
// being preempted or terminated. If so, it  returns true.
func (c *Client) ShouldTerminate(ctx context.Context) (bool, error) {
	resp, err := c.metadata.Get("instance/preempted")
	if err != nil {
		return false, fmt.Errorf("failed to determine if instance is being preempted")
	}

	if strings.EqualFold(strings.TrimSpace(resp), "TRUE") {
		return true, nil
	}

	return false, nil
}
