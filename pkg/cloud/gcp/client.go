package instance

import (
	"context"
	"errors"
	"net/http"

	"github.com/jaredallard/minecraft-preempt/pkg/cloud"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"
)

var (
	// ErrNotStopped is an error that is thrown when an instance is attempted
	// to be stopped but is found to be not running
	ErrNotStopped = errors.New("Not stopped")
)

// Client is a gcs client
type Client struct {
	context context.Context
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

	comp, err := compute.New(client)
	if err != nil {
		return nil, err
	}

	return &Client{
		context: context.Background(),
		gclient: client,
		compute: comp,
		project: project,
		zone:    zone,
	}, nil
}

// Status returns the status of an instance
func (c *Client) Status(instanceID string) (cloud.ProviderStatus, error) {
	gr := c.compute.Instances.Get(c.project, c.zone, instanceID)
	i, err := gr.Do()
	if err != nil {
		return "", err
	}

	// HACK: handle invalid statuses
	st := cloud.ProviderStatus(i.Status)
	switch st {
	case cloud.StatusRunning, cloud.StatusStarting, cloud.StatusStopping, cloud.StatusStopped:
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
func (c *Client) Start(instanceID string) error {
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
