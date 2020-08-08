package config

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v2"
)

// ProxyConfig is a configuration file for the proxy
type ProxyConfig struct {
	// Version of the configuration file
	Version string `yaml:"version"`

	// Server configuration block
	Server struct {
		// Hostname of the remote server
		Hostname string `yaml:"hostname"`

		// Port of the remote server
		Port int `yaml:"port"`

		// ProtocolVersion of the server
		ProtocolVersion int `yaml:"protocolVersion"`

		// Version of the server
		Version string `yaml:"textVersion"`

		// ShutDownAfter this amount of time has passed since the last user left
		ShutDownAfter time.Duration `yaml:"shutDownAfter"`
	} `yaml:"server"`

	// Instance is a GCP instance
	Instance struct {
		// ID of the instance
		ID string `yaml:"id"`

		// Project name
		Project string `yaml:"project"`

		// Zone of the instance
		Zone string `yaml:"zone"`
	} `yaml:"instance"`
}

// LoadProxyConfig loads a proxy configuration fiole
func LoadProxyConfig() (*ProxyConfig, error) {
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		return nil, err
	}

	b, err := ioutil.ReadFile(filepath.Join(dir, "../config/config.yaml"))
	if err != nil {
		return nil, err
	}

	var conf *ProxyConfig
	err = yaml.Unmarshal(b, &conf)
	return conf, err
}
