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

// Package cloud contains the interfaces and shared types between cloud
// providers.
package cloud

import (
	"context"
)

// ProviderStatus is the status of an instance
type ProviderStatus string

// Contains definitions for the ProviderStatus type
var (
	// StatusRunning denotes an instances is currently running and is able
	// to accept connections.
	StatusRunning ProviderStatus = "RUNNING"

	// StatusStopped denotes an instance is stopped and is not able to accept
	// connections, but can be started.
	StatusStopped ProviderStatus = "STOPPED"

	// StatusStopping denotes an instance is currently stopping and is not able
	// to accept connections or be started.
	StatusStopping ProviderStatus = "STOPPING"

	// StatusStarting denotes an instance is currently starting and is not able
	// to accept connections or be started.
	StatusStarting ProviderStatus = "STARTING"

	// StatusUnknown denotes an instance is in an unknown state and cannot be
	// determined. This is usually an error state.
	StatusUnknown ProviderStatus = "UNKNOWN"
)

// Provider is a cloud provider
type Provider interface {
	// Status fetches the status of a remote instance
	Status(ctx context.Context, instanceID string) (ProviderStatus, error)

	// Start starts a remote instance
	Start(ctx context.Context, instanceID string) error

	// Stop stops a remote instance
	Stop(ctx context.Context, instanceID string) error

	// ShouldTerminate returns true if the instance should be terminated.
	ShouldTerminate(ctx context.Context) (bool, error)
}
