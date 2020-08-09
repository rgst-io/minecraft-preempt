package config

import (
	"io/ioutil"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

var (
	CloudGCP    Cloud = "gcp"
	CloudDocker Cloud = "docker"
)

type Cloud string

// ProxyConfig is a configuration file for the proxy
type ProxyConfig struct {
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
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read config file")
	}

	var conf *ProxyConfig
	return conf, errors.Wrap(yaml.Unmarshal(b, &conf), "failed to parse config as yaml")
}
