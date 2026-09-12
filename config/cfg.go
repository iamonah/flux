package config

import (
	"fmt"
	"io"

	"go.yaml.in/yaml/v4"
)

type TLSConfig struct {
	Enable   bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file,omitempty"`
	KeyFile  string `yaml:"key_file,omitempty"`
}

type HealthCheckConfig struct {
	Path string `yaml:"path"`
}

type Metadata struct {
	Weight     *uint32 `yaml:"weight"`
	MaxRetries *uint32 `yaml:"max_retries"` //default: 3
}

type Replica struct {
	URL      string   `yaml:"url"`
	Metadata Metadata `yaml:"metadata"`
}

type Service struct {
	Name        string             `yaml:"name"`
	Matcher     string             `yaml:"matcher"`
	Replicas    []Replica          `yaml:"replicas"`
	Strategy    *string            `yaml:"strategy"`
	HealthCheck *HealthCheckConfig `yaml:"health_check,omitempty"`
}

type Config struct {
	Services []*Service `yaml:"services"`
	Mode     *string    `yaml:"mode,omitempty"`
	TLS      TLSConfig  `yaml:"tls,omitempty"`
}

func LoadConfig(reader io.Reader) (*Config, error) {
	var config Config
	err := yaml.NewDecoder(reader).Decode(&config)
	if err != nil {
		return nil, fmt.Errorf("loadconfig: %w", err)
	}
	return &config, nil
}
