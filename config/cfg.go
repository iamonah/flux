package config

import (
	"fmt"
	"io"

	"go.yaml.in/yaml/v4"
)

type TLSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file,omitempty"`
	KeyFile  string `yaml:"key_file,omitempty"`
}

type HealthCheckConfig struct {
	Path string `yaml:"path"`
}

type Service struct {
	Name        string             `yaml:"name"`
	Matcher     string             `yaml:"matcher"`
	Strategy    *string            `yaml:"strategy,omitempty"`
	HealthCheck *HealthCheckConfig `yaml:"health_check,omitempty"`
}

type ConsulConfig struct {
	Address string `yaml:"address"`
}

type Config struct {
	FluxPort   int          `yaml:"flux_port"`
	Mode       *string      `yaml:"mode,omitempty"`
	MaxRetries uint32       `yaml:"max_retries"`
	TLS        TLSConfig    `yaml:"tls,omitempty"`
	Consul     ConsulConfig `yaml:"consul"`
	Services   []*Service   `yaml:"services"`
}

func LoadConfig(reader io.Reader) (*Config, error) {
	var config Config

	err := yaml.NewDecoder(reader).Decode(&config)
	if err != nil {
		return nil, fmt.Errorf("loadconfig: %w", err)
	}

	return &config, nil
}
