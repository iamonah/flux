package config

import (
	"strings"
	"testing"
)

func TestConfig(t *testing.T) {
	data := strings.NewReader(`
mode: l7
max_retries: 3

tls:
  enabled: true
  cert_file: ./certs/server.crt
  key_file: ./certs/server.key

consul:
  address: localhost:8500

services:
  - name: service1
    matcher: /api/v1/payments
    strategy: round-robin
    health_check:
      path: /health

  - name: service2
    matcher: /service2
    strategy: weighted-round-robin
    health_check:
      path: /health
`)

	config, err := LoadConfig(data)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if config.Mode == nil {
		t.Fatal("Expected mode to be set")
	}

	if *config.Mode != "l7" {
		t.Errorf("Expected mode 'l7', got '%s'", *config.Mode)
	}

	if config.MaxRetries != 3 {
		t.Errorf("Expected max retries 3, got %d", config.MaxRetries)
	}

	if !config.TLS.Enabled {
		t.Error("Expected TLS to be enabled")
	}

	if config.TLS.CertFile != "./certs/server.crt" {
		t.Errorf(
			"Expected certificate file './certs/server.crt', got '%s'",
			config.TLS.CertFile,
		)
	}

	if config.TLS.KeyFile != "./certs/server.key" {
		t.Errorf(
			"Expected key file './certs/server.key', got '%s'",
			config.TLS.KeyFile,
		)
	}

	if config.Consul.Address != "localhost:8500" {
		t.Errorf(
			"Expected Consul address 'localhost:8500', got '%s'",
			config.Consul.Address,
		)
	}

	if len(config.Services) != 2 {
		t.Fatalf(
			"Expected 2 services, got %d",
			len(config.Services),
		)
	}

	if config.Services[0].Name != "service1" {
		t.Errorf(
			"Expected first service name 'service1', got '%s'",
			config.Services[0].Name,
		)
	}

	if config.Services[0].Matcher != "/api/v1/payments" {
		t.Errorf(
			"Expected matcher '/api/v1/payments', got '%s'",
			config.Services[0].Matcher,
		)
	}

	if config.Services[0].StrategyType == "" {
		t.Fatal("Expected strategy for service1 to be set")
	}

	if config.Services[0].StrategyType != "round-robin" {
		t.Errorf(
			"Expected strategy 'round-robin', got '%s'",
			config.Services[0].StrategyType,
		)
	}

	if config.Services[0].HealthCheck == nil {
		t.Fatal("Expected health check for service1 to be set")
	}

	if config.Services[0].HealthCheck.Path != "/health" {
		t.Errorf(
			"Expected health check path '/health', got '%s'",
			config.Services[0].HealthCheck.Path,
		)
	}

	if config.Services[1].Name != "service2" {
		t.Errorf(
			"Expected second service name 'service2', got '%s'",
			config.Services[1].Name,
		)
	}


	if config.Services[1].StrategyType == "" {
		t.Fatal("Expected strategy for service2 to be set")
	}

	if config.Services[1].StrategyType != "weighted-round-robin" {
		t.Errorf(
			"Expected strategy 'weighted-round-robin', got '%s'",
			config.Services[1].StrategyType,
		)
	}

	if config.Services[1].HealthCheck == nil {
		t.Fatal("Expected health check for service2 to be set")
	}

	if config.Services[1].HealthCheck.Path != "/health" {
		t.Errorf(
			"Expected health check path '/health', got '%s'",
			config.Services[1].HealthCheck.Path,
		)
	}
}
