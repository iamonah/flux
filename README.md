# Flux — A Go Load Balancer

Flux is a multi-tier load balancer built in Go, with a Layer 4 proxy for connection-level load balancing and an optional Layer 7 HTTP proxy layer for application-aware routing and service-level load balancing. Each Layer 7 instance routes requests to independently managed service backends.

Layer 4 supports TCP and UDP traffic. It can distribute traffic for a single service across multiple backend instances and can support multiple services when they have distinct Layer 4 endpoints, such as different ports.

Layer 7 handles application-level routing using HTTP request information such as URL paths. Backend instances for both layers are discovered dynamically through Consul.

## Architecture

The architecture separates connection-level routing from application-level routing. Layer 7 is responsible for identifying the application-level service, while Layer 4 is responsible for connection-level forwarding and backend selection.

Layer 4 can operate independently when application-level routing is not required.

```text
                                       Clients
                                          │
                                          ▼
                               ┌──────────────────────┐
                               │       Flux L7        │
                               │     HTTP Proxy       │
                               │ Application Routing  │
                               └──────────┬───────────┘
                                          │
                                     Path matching
                                          │
                           ┌──────────────┼──────────────┐
                           ▼              ▼              ▼
                        Payments         Users         Orders
                           │              │              │
                           ▼              ▼              ▼
                        Flux L4         Flux L4         Flux L4
                           │              │              │
                     ┌─────┼─────┐   ┌────┼────┐   ┌────┼────┐
                     ▼     ▼     ▼   ▼    ▼    ▼   ▼    ▼    ▼
                    P1    P2    P3  U1   U2   U3  O1   O2   O3
```

At Layer 7, each configured service owns a `BackendPool`. The pool represents one service and its discovered instances; it is independent from every other service pool. Incoming requests are matched against their URL path, the selected pool chooses a healthy backend using its configured strategy, and an `httputil.ReverseProxy` forwards the request upstream.

At Layer 4, the configured protocol and port identify the service endpoint. Flux then selects a healthy backend from that service's `BackendPool`.

For example:

```text
                         Flux L4
                            │
              ┌─────────────┼─────────────┐
              │             │             │
           TCP :8081     TCP :8082     UDP :5353
              │             │             │
         payments-v1    payments-v2       dns
              │             │             │
           ┌──┼──┐      ┌──┼──┐        ┌──┼──┐
           ▼  ▼  ▼      ▼  ▼  ▼        ▼  ▼  ▼
           P1 P2 P3     P1 P2 P3       D1 D2 D3
```

Pure Layer 4 routing cannot distinguish multiple services that share the same destination IP, port, and protocol because that information does not identify the application-level service. In that case, Layer 7 is required to inspect application-level information such as the HTTP path.

### Multi-Service Layer 4

A single Flux L4 instance can handle multiple services when each service has a distinct Layer 4 endpoint, such as a different port.

This model is primarily intended for internal infrastructure, where services can be assigned separate internal endpoints:

```text
Single Flux L4

TCP :8081 → payments-v1
TCP :8082 → payments-v2
UDP :5353 → dns
```

The destination port identifies the service, so Flux does not need to inspect application-level data.

If multiple public services share the same public IP, port, and protocol, Layer 4 alone cannot distinguish them. Application-level routing through Layer 7 is required in that case.

The current demo uses a single Flux L4 instance to handle multiple services through their separate configured ports.

Backend instances are discovered dynamically through Consul rather than being defined as static replicas in the Flux configuration.

The current Layer 7 path matcher supports exact and prefix matches.

## Features

* **Layer 4 TCP proxy** — connection-level TCP load balancing
* **Layer 4 UDP proxy** — datagram-level UDP load balancing
* **Five-tuple-based backend selection** — uses source IP, source port, destination IP, destination port, and protocol for flow-based selection
* **Load-balancing strategies** — `round-robin`, `weighted-round-robin`
* **HTTP reverse proxy** — forwards requests to discovered backend services
* **Path-based routing** — supports exact and prefix path matching
* **Per-service backend pools** — each service maintains its own independent pool of backend instances
* **Service discovery** — dynamically discovers service instances through Consul
* **Health checks** — supports HTTP, TCP, and UDP health checks

  * When `health_check.path` is configured, Flux performs an HTTP health check against the specified endpoint.
  * If no health-check path is configured for a TCP service, Flux falls back to a TCP connection check against the backend host and port.
  * If no health-check path is configured for a UDP service, Flux performs a UDP probe against the backend.
* **Forwarded headers** — manages `X-Forwarded-For`, `X-Forwarded-Host`, and `X-Forwarded-Proto`

## Roadmap

### Load-balancing strategies

* Least-connections
* Additional balancing policies

### Service discovery

* Kubernetes service discovery

### Reliability

* Passive health checks
* Advanced failure handling and retry policies
* Backend state tracking and recovery

### Security

* Consul ACLs and authentication
* TLS/mTLS between Flux and Consul
* Least-privilege permissions for service discovery
* HTTPS connections to backend services
* Rate limiting
* Request and connection limits
* Stronger API and edge security

## Request Path

### Layer 7

```text
                 HTTP request
                      │
                      ▼
                 Path matcher
                      │
                      ▼
                   BackendPool
                      │
                      ▼
                 Healthy backends
                      │
                      ▼
               Load-balancing strategy
                      │
                      ▼
                Backend reverse proxy
                      │
                      ▼
                 Upstream instance
```

Each Layer 7 pool delegates backend selection to its configured strategy. Round robin distributes requests across backend instances using an atomic counter, while weighted round robin distributes requests according to each instance's configured weight.

Backend instances are discovered from Consul and maintained locally in each service's `BackendPool`. Flux performs backend selection locally rather than querying Consul for every request.

The reverse proxy also replaces client-controlled forwarding headers with verified `X-Forwarded-For`, `X-Forwarded-Host`, and `X-Forwarded-Proto` values.

### Layer 4 TCP

```text
             TCP connection
                    │
                    ▼
                L4 listener
                    │
                    ▼
                 BackendPool
                    │
                    ▼
               Healthy backends
                    │
                    ▼
              Backend selection
                    │
                    ▼
                TCP backend
                    │
                    ▼
             Bidirectional forwarding
```

Flux accepts the TCP connection, identifies the corresponding service from the listener endpoint, selects a healthy backend, establishes a connection to that backend, and forwards data bidirectionally.

For hash-based selection, Flux uses the connection's five-tuple to select a backend consistently for the flow.

### Layer 4 UDP

UDP is connectionless and does not have a TCP-style `Accept()` operation. A single UDP socket can receive datagrams from multiple clients.

```text
  Client A ───────┐
                  │
  Client B ───────┼──────► Flux UDP socket
                  │
  Client C ───────┘
                          │
                          ▼
                       Five-tuple
                          │
                          ▼
                    Backend selection
                          │
                          ▼
                       UDP backend
```

Flux receives each datagram together with the client's address, derives the flow's five-tuple, and uses the corresponding service `BackendPool` to select a healthy backend.

UDP flow state allows subsequent datagrams belonging to the same flow to continue using the same backend.

Backend responses are forwarded back to the original client address.

```text
                 Client
                   │
                   │ UDP datagram
                   ▼
              Flux UDP socket
                   │
                   │ select backend
                   ▼
               UDP backend
                   │
                   │ response
                   ▼
              Flux UDP socket
                   │
                   ▼
              Original client
```

Flux also performs UDP health checks when no HTTP health-check path is configured. The health check sends a UDP probe to the backend and verifies that a response is received.

## Backend Pool Synchronization

Each `BackendPool` owns the strategy selected for its service. This keeps backend selection independent: one service can use round robin while another uses weighted round robin.

Backend instances are updated dynamically through service discovery and health checking. The pool uses Left-Right synchronization to allow concurrent readers to access backend snapshots without contending on a reader-writer lock.

Writers update the inactive backend snapshot, publish it by switching the active index, and wait for readers using the previous snapshot to finish before reusing it.

This is designed for the read-heavy workload of backend selection, where requests frequently read the current backend set while discovery and health checks update it less frequently.

## Configuration

Flux is configured through `config.yaml`.

### Layer 7

```yaml
mode: l7
max_retries: 3

tls:
  enabled: true
  cert_file: ./certs/server.crt
  key_file: ./certs/server.key

consul:
  address: localhost:8500

services:
  - name: payments-v1
    matcher: /api/v1/payments
    strategy: round-robin
    health_check:
      path: /health

  - name: payments-v2
    matcher: /api/v2/users
    strategy: weighted-round-robin
    health_check:
      path: /health
```

### Layer 4

```yaml
mode: l4
max_retries: 3

consul:
  address: localhost:8500

services:
  - name: payments-v1
    protocol: tcp
    port: 8081
    strategy: round-robin

  - name: payments-v2
    protocol: tcp
    port: 8082
    strategy: hash

  - name: dns
    protocol: udp
    port: 5353
    strategy: hash
```

For Layer 7, the frontend endpoint is shared by the HTTP proxy and services are identified using application-level information such as the request path.

For Layer 4, the configured protocol and port identify the service endpoint on which Flux accepts traffic.

Multiple services can therefore be exposed through different Layer 4 endpoints:

```text
        TCP :8081 → payments-v1
        TCP :8082 → payments-v2
        UDP :5353 → dns
```

The backend port does not have to match the Flux listening port. The actual backend address and port are obtained through Consul service discovery.

The `strategy` field is optional. When omitted, Flux uses its default load-balancing strategy.

Each service can configure its own strategy independently.

Backend instances are not configured directly in `config.yaml`. Services register their instances with Consul, and Flux periodically discovers the available instances for each configured service.

When `health_check.path` is configured, Flux performs an HTTP health check against the specified endpoint.

If no health-check path is configured:

* TCP services use TCP connection checks.
* UDP services use UDP probe checks.

## Layer 7 → Layer 4

Layer 7 and Layer 4 can be composed internally.

Layer 7 first determines which service the request belongs to:

```text
                 Client
                    │
                    │ HTTPS /payments
                    ▼
                 Flux L7
                    │
                    │ service identified
                    ▼
                 Flux L4
                    │
                    │ connection-level load balancing
                    ▼
              Payment instances
```

For example:

```text
                       Flux L7
                          :443
                            │
                            │ /payments
                            ▼
                       Flux L4
                          :8080
                            │
                    ┌───────┼───────┐
                    ▼       ▼       ▼
                 payment-1 payment-2 payment-3
```

In this architecture, Layer 7 performs service identification while Layer 4 performs instance selection. Layer 4 does not need to understand the original HTTP path.

A single Flux L4 instance can expose multiple service endpoints on different ports. These endpoints can be registered with Consul as separate service registrations, even when they share the same host address.

For example:

```text
payments-l4 → 10.0.0.20:8081
users-l4    → 10.0.0.20:8082
orders-l4   → 10.0.0.20:8083
```

These registrations may refer to different ports on the same Flux L4 instance:

```text
                         Single Flux L4
                           10.0.0.20
                                │
                  ┌─────────────┼─────────────┐
                  │             │             │
               :8081         :8082         :8083
                  │             │             │
              payments         users         orders
```

Layer 7 can discover the appropriate L4 service through Consul and use the discovered address and port as its upstream endpoint. This avoids requiring Layer 7 to hard-code the location of the Layer 4 endpoint.

This also allows the same Layer 4 process to serve multiple logical services while keeping service discovery independent from the physical process or host.

The resulting request path is:

```text
            HTTP request
                │
                ▼
            Flux L7
                │
                │ application-level routing
                ▼
            Consul-discovered L4 endpoint
                │
                │ connection-level routing
                ▼
            Flux L4
                │
                │ backend selection
                ▼
            Backend instance
```

Consul therefore provides service-level indirection between the layers: Layer 7 discovers the L4 endpoint for a logical service, while Layer 4 independently discovers and selects instances of the backend service.

## Run Locally

```bash
go run ./cmd/flux -config-path config.yaml
```

## Verify

The test suite expects:

* Consul to be running
* The demo backend servers to be running

Start the demo backend servers, then run:

```bash
go run ./cmd/demo-server
```

Run the test suite:

```bash
go test ./...
```
