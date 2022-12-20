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
	"os"

	"github.com/kelseyhightower/envconfig"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

var (
	CloudGCP    Cloud = "gcp"
	CloudDocker Cloud = "docker"
)

type Cloud string

// ProxyConfig is a configuration file for the proxy
type ProxyConfig struct {
	// ListenAddress is the address to listen on (the proxy)
	ListenAddress string `yaml:"listenAddress"`

	// The Cloud this instance is in
	Cloud       Cloud         `yaml:"cloud"`
	Server      *ServerConfig `yaml:"server"`
	CloudConfig struct {
		GCP    *GCPConfig    `yaml:"gcp"`
		Docker *DockerConfig `yaml:"docker"`
	} `yaml:"cloudConfig"`
}

// Server configuration block
type ServerConfig struct {
	// Hostname of the remote server
	Hostname string `yaml:"hostname"`

	// Port of the remote server
	Port uint `yaml:"port"`

	// ProtocolVersion of the server
	ProtocolVersion uint `yaml:"protocolVersion"`

	// Version of the server
	Version string `yaml:"textVersion"`
}

type GCPConfig struct {
	// InstanceID is the id of the GCP instance
	InstanceID string `yaml:"instanceID"`

	// Project name is the project the instance is in
	Project string `yaml:"project"`

	// Zone is the zone the instance is in
	Zone string `yaml:"zone"`
}

type DockerConfig struct {
	// ContainerID is the ID of the container to run
	ContainerID string `yaml:"containerID"`
}

// LoadProxyConfig loads a proxy configuration file
func LoadProxyConfig(path string) (*ProxyConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open config file")
	}

	var conf ProxyConfig
	if err := yaml.NewDecoder(f).Decode(&conf); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal config file")
	}
	if err := envconfig.Process("minecraft_preempt", &conf); err != nil {
		return nil, errors.Wrap(err, "failed to load config from env")
	}
	return &conf, nil
}
