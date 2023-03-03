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

package cloud

import "context"

type ProviderStatus string

var (
	StatusRunning  ProviderStatus = "RUNNING"
	StatusStopped  ProviderStatus = "STOPPED"
	StatusStopping ProviderStatus = "STOPPING"
	StatusStarting ProviderStatus = "STARTING"
	StatusUnknown  ProviderStatus = "UNKNOWN"
)

// Provider is a cloud provider
type Provider interface {
	// Status fetches the status of a remote instance
	Status(ctx context.Context, instanceID string) (ProviderStatus, error)

	// Start starts a remote instance
	Start(ctx context.Context, instanceID string) error

	// Stop stops a remote instance
	Stop(ctx context.Context, instanceID string) error
}
