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

package config

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// This block contains all of the valid cloud providers
var (
	CloudGCP    Cloud = "gcp"
	CloudDocker Cloud = "docker"
)

// Cloud is a cloud provider
type Cloud string

// ProxyConfig is a configuration file for the proxy
type ProxyConfig struct {
	// Servers contains a list of all servers to proxy
	Servers []ServerConfig `yaml:"servers"`
}

// ServerConfig is a configuration block for a server
type ServerConfig struct {
	// Name is a user friendly name for the server
	Name string `yaml:"name"`

	// ListenAddress is the address to listen on
	ListenAddress string `yaml:"listenAddress"`

	// ShutdownAfter is the amount of time to wait before
	// shutting down the server after the last connection
	// is closed
	//
	// Defaults to 15 minutes
	ShutdownAfter time.Duration `yaml:"shutdownAfter"`

	// GCP is the GCP configuration block
	GCP *GCPConfig `yaml:"gcp"`

	// Docker is the Docker configuration block
	Docker *DockerConfig `yaml:"docker"`

	// Minecraft is the Minecraft configuration block
	Minecraft MinecraftServerConfig `yaml:"minecraft"`
}

// Server configuration block
type MinecraftServerConfig struct {
	// Hostname of the remote server
	Hostname string `yaml:"hostname"`

	// Port of the remote server, defaults to 25565
	Port uint `yaml:"port"`
}

// GCPConfig is a configuration block for GCP
// configuration
type GCPConfig struct {
	// InstanceID is the id of the GCP instance
	InstanceID string `yaml:"instanceID"`

	// Project name is the project the instance is in
	Project string `yaml:"project"`

	// Zone is the zone the instance is in
	Zone string `yaml:"zone"`
}

// DockerConfig is a configuration block for Docker
// configuration
type DockerConfig struct {
	// ContainerID is the ID of the container to run
	ContainerID string `yaml:"containerID"`
}

// applyDefaults applies default values to the configuration
func applyDefaults(conf *ProxyConfig) {
	for i := range conf.Servers {
		if conf.Servers[i].ListenAddress == "" {
			conf.Servers[i].ListenAddress = "0.0.0.0:25565"
		}

		if conf.Servers[i].ShutdownAfter == 0 {
			// Default to 15 minutes
			conf.Servers[i].ShutdownAfter = 15 * time.Minute
		}

		if conf.Servers[i].Minecraft.Port == 0 {
			// Default to 25565
			conf.Servers[i].Minecraft.Port = 25565
		}
	}
}

// validateConfig validates the configuration is valid
func validateConfig(conf *ProxyConfig) error {
	if len(conf.Servers) == 0 {
		return fmt.Errorf("no servers defined")
	}

	for i, s := range conf.Servers {
		if s.Name == "" {
			return fmt.Errorf("server %d has no name", i)
		}

		if s.GCP != nil && s.Docker != nil {
			return fmt.Errorf("server %q has both gcp and docker config", s.Name)
		}

		if s.GCP == nil && s.Docker == nil {
			return fmt.Errorf("server %q has no gcp or docker config", s.Name)
		}

		if s.Minecraft.Hostname == "" {
			return fmt.Errorf("server %q has no configured minecraft hostname", s.Name)
		}
	}

	return nil
}

// LoadProxyConfig loads a proxy configuration file
func LoadProxyConfig(path string) (*ProxyConfig, error) {
	var conf ProxyConfig

	// Support loading env from an environment variable

	var reader io.ReadCloser
	if os.Getenv("CONFIG_BASE64") != "" {
		reader = io.NopCloser(base64.NewDecoder(base64.StdEncoding, strings.NewReader(os.Getenv("CONFIG"))))
	} else {
		f, err := os.Open(path)
		if err != nil {
			return nil, errors.Wrap(err, "failed to open config file")
		}
		defer f.Close()

		reader = f
	}

	// decode the config
	if err := yaml.NewDecoder(reader).Decode(&conf); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal config file")
	}

	applyDefaults(&conf)

	if err := validateConfig(&conf); err != nil {
		return nil, errors.Wrap(err, "failed to validate config")
	}

	return &conf, nil
}
