package instance

import (
	"context"
	"errors"
	"net/http"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"
)

var (
	// ErrNotStopped is an error that is thrown when an instance is attempted
	// to be started but is found to be running
	ErrNotStopped = errors.New("Not stopped")

	// ErrNotRunning is an error that is thrown when an instance is attempted
	// to be stopped but is found to be not running
	ErrNotRunning = errors.New("Not running")
)

// Client is a gcs client
type Client struct {
	context context.Context
	gclient *http.Client
	compute *compute.Service
}

// NewClient creates a new client
func NewClient() *Client {
	return &Client{
		context: context.Background(),
	}
}

func (c *Client) getClient() (*http.Client, *compute.Service, error) {
	if c.gclient == nil {
		client, err := google.DefaultClient(c.context, compute.ComputeScope)
		if err != nil {
			return nil, nil, err
		}

		comp, err := compute.New(client)
		if err != nil {
			return nil, nil, err
		}

		c.gclient = client
		c.compute = comp
	}

	return c.gclient, c.compute, nil
}

// Status returns the status of an instance
func (c *Client) Status(project, zone, instanceID string) (string, error) {
	_, comp, err := c.getClient()
	if err != nil {
		return "", err
	}

	gr := comp.Instances.Get(project, zone, instanceID)
	i, err := gr.Do()
	if err != nil {
		return "", err
	}

	return i.Status, nil
}

// Start an instance if it's not already running
func (c *Client) Start(project, zone, instanceID string) error {
	_, comp, err := c.getClient()
	if err != nil {
		return err
	}

	gr := comp.Instances.Get(project, zone, instanceID)
	i, err := gr.Do()
	if err != nil {
		return err
	}

	if i.Status != "STOPPED" && i.Status != "TERMINATED" {
		return ErrNotStopped
	}

	sr := comp.Instances.Start(project, zone, instanceID)
	_, err = sr.Do()
	return err
}

// Stop an instance
func (c *Client) Stop(project, zone, instanceID string) error {
	_, comp, err := c.getClient()
	if err != nil {
		return err
	}

	gr := comp.Instances.Get(project, zone, instanceID)
	i, err := gr.Do()
	if err != nil {
		return err
	}

	if i.Status != "RUNNING" {
		return ErrNotRunning
	}

	sr := comp.Instances.Stop(project, zone, instanceID)
	_, err = sr.Do()
	return err
}
