package config

import (
	"strings"
	"testing"
)

func TestConfig(t *testing.T) {
	data := strings.NewReader(
		`services:
  - name: service1
    matcher: /api/v1/payments
    replicas:
      - url: http://localhost:8081
      - url: http://localhost:8082
    strategy: round-robin
  - name: service2
    matcher: /service2
    replicas:
      - url: http://localhost:9091
      - url: http://localhost:9092
    strategy: round-robin`,
	)

	config, err := LoadConfig(data)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if config.Services[0].Matcher != "/api/v1/payments" {
		t.Errorf("Expected matcher '/api/v1/payments', got '%s'", config.Services[0].Matcher)
	}

	if *config.Services[0].Strategy != "round-robin" {
		t.Errorf("Expected strategy 'round-robin', got '%s'", *config.Services[0].Strategy)
	}

	if *config.Services[1].Strategy != "weighted-round-robin" {
		t.Errorf("Expected strategy 'weighted-round-robin', got '%s'", *config.Services[1].Strategy)
	}

	if len(config.Services[0].Replicas) != 2 {
		t.Errorf("Expected 2 replicas for service1, got %d", len(config.Services[0].Replicas))
	}

	if len(config.Services[1].Replicas) != 2 {
		t.Errorf("Expected 2 replicas for service2, got %d", len(config.Services[1].Replicas))
	}

	if config.Services[0].Replicas[0].URL != "http://localhost:8081" {
		t.Errorf("Expected first replica of service1 to be 'http://localhost:8081', got '%s'", config.Services[0].Replicas[0].URL)
	}

	if config.Services[1].Replicas[1].URL != "http://localhost:9092" {
		t.Errorf("Expected second replica of service2 to be 'http://localhost:9092', got '%s'", config.Services[1].Replicas[1].URL)
	}
}


func TestWeightedRoundRobinConfig(t *testing.T) {
	data := strings.NewReader(`
services:
  - name: service1
    matcher: /api/v1/payments
    replicas:
      - url: http://localhost:8081
        metadata:
          weight: 10
      - url: http://localhost:8082
        metadata:
          weight: 5
    strategy: weighted-round-robin

  - name: service2
    matcher: /service2
    replicas:
      - url: http://localhost:9091
        metadata:
          weight: 1
      - url: http://localhost:9092
        metadata:
          weight: 2
    strategy: weighted-round-robin
`)

	config, err := LoadConfig(data)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if config.Services[0].Matcher != "/api/v1/payments" {
		t.Errorf("Expected matcher '/api/v1/payments', got '%s'", config.Services[0].Matcher)
	}

	if *config.Services[0].Strategy != "weighted-round-robin" {
		t.Errorf("Expected strategy 'round-robin', got '%s'", *config.Services[0].Strategy)
	}
	if *config.Services[0].Replicas[0].Metadata.Weight != 10 {
		t.Errorf("Expected weight 10 for first replica of service1, got %d", *config.Services[0].Replicas[0].Metadata.Weight)
	}

	if *config.Services[0].Replicas[1].Metadata.Weight != 5 {
		t.Errorf("Expected weight 5 for second replica of service1, got %d", *config.Services[0].Replicas[1].Metadata.Weight)
	}

	if *config.Services[1].Replicas[0].Metadata.Weight != 1 {
		t.Errorf("Expected weight 1 for first replica of service2, got %d", *config.Services[1].Replicas[0].Metadata.Weight)
	}

	if *config.Services[1].Replicas[1].Metadata.Weight != 2 {
		t.Errorf("Expected weight 2 for second replica of service2, got %d", *config.Services[1].Replicas[1].Metadata.Weight)
	}

	if *config.Services[1].Strategy != "weighted-round-robin" {
		t.Errorf("Expected strategy 'weighted-round-robin', got '%s'", *config.Services[1].Strategy)
	}

	if len(config.Services[0].Replicas) != 2 {
		t.Errorf("Expected 2 replicas for service1, got %d", len(config.Services[0].Replicas))
	}

	if len(config.Services[1].Replicas) != 2 {
		t.Errorf("Expected 2 replicas for service2, got %d", len(config.Services[1].Replicas))
	}

	if config.Services[0].Replicas[0].URL != "http://localhost:8081" {
		t.Errorf("Expected first replica of service1 to be 'http://localhost:8081', got '%s'", config.Services[0].Replicas[0].URL)
	}

	if config.Services[1].Replicas[1].URL != "http://localhost:9092" {
		t.Errorf("Expected second replica of service2 to be 'http://localhost:9092', got '%s'", config.Services[1].Replicas[1].URL)
	}
}