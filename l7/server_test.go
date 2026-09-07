package l7

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iamonah/loadbalancer/config"
)

func TestBackendServers(t *testing.T) {
	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello from demo server 1"))
	}))
	defer backend1.Close()

	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello from demo server 2"))
	}))
	defer backend2.Close()

	backend3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello from demo server 3"))
	}))
	defer backend3.Close()

	cfg, err := config.LoadConfig(strings.NewReader(`
services:
  - name: payments-v1
    matcher: /api/v1/payments
    strategy: round-robin
`))

	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// httptest servers use dynamic ports, so inject their URLs into the test config.
	cfg.Services[0].Replicas = []config.Replica{
		{URL: backend1.URL},
		{URL: backend2.URL},
		{URL: backend3.URL},
	}

	lb, err := NewLoadBalancer(cfg)
	if err != nil {
		t.Fatalf("Failed to create load balancer: %v", err)
	}

	server := httptest.NewServer(lb)
	defer server.Close()

	type responseResult struct {
		status int
		body   string
	}

	responses := make([]responseResult, 0)

	for i := 0; i < 4; i++ {
		response, err := http.Get(server.URL + "/api/v1/payments")
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		body, err := io.ReadAll(response.Body)
		response.Body.Close()

		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}

		responses = append(responses, responseResult{
			status: response.StatusCode,
			body:   string(body),
		})
	}

	for _, response := range responses {
		if response.status != http.StatusOK {
			t.Fatalf("Expected status code 200, got %d", response.status)
		}

		t.Logf("Response body: %s", response.body)

		if !strings.Contains(response.body, "Hello from demo server") {
			t.Fatalf(
				"Expected response containing 'Hello from demo server', got %s",
				response.body,
			)
		}
	}
}
