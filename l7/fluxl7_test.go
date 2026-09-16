package l7

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/iamonah/loadbalancer/config"
	"github.com/iamonah/loadbalancer/util/consul"
)

type mockDiscovery struct {
	instances []consul.Instance
}

func (m *mockDiscovery) Discover(ctx context.Context, serviceName string) ([]consul.Instance, error) {
	return m.instances, nil
}

func instanceFromServer(id string, serviceName string, serverURL string) consul.Instance {
	parsedURL, _ := url.Parse(serverURL)

	host := parsedURL.Hostname()
	port, _ := strconv.Atoi(parsedURL.Port())

	return consul.Instance{
		ID:      id,
		SvcName: serviceName,
		Address: host,
		Port:    port,
	}
}

func TestBackendServers(t *testing.T) {
	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello from demo server 1"))
	},
	))
	defer backend1.Close()

	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello from demo server 2"))
	},
	))
	defer backend2.Close()

	backend3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello from demo server 3"))
	},
	))
	defer backend3.Close()

	cfg, err := config.LoadConfig(strings.NewReader(`
mode: l7

services:
  - name: payments-v1
    matcher: /api/v1/payments
    strategy: round-robin
    health_check:
      path: /
`))
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	instances := []consul.Instance{
		instanceFromServer("backend-1", "payments-v1", backend1.URL),
		instanceFromServer("backend-2", "payments-v1", backend2.URL),
		instanceFromServer("backend-3", "payments-v1", backend3.URL),
	}

	discovery := &mockDiscovery{
		instances: instances,
	}

	lb, err := Newfluxl7(cfg, discovery)
	if err != nil {
		t.Fatalf("failed to create load balancer: %v", err)
	}

	server := httptest.NewServer(lb)
	defer server.Close()

	for i := 0; i < 4; i++ {
		response, err := http.Get(
			server.URL + "/api/v1/payments",
		)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}

		body, err := io.ReadAll(response.Body)
		response.Body.Close()

		if err != nil {
			t.Fatalf("failed to read response body: %v", err)
		}

		if response.StatusCode != http.StatusOK {
			t.Fatalf("expected status code 200, got %d", response.StatusCode)
		}

		t.Logf("Response body: %s", body)

		if !strings.Contains(string(body), "Hello from demo server") {
			t.Fatalf("expected response containing "+"'Hello from demo server', got %s", body)
		}
	}
}
