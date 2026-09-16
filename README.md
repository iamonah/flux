# Flux — A Go Load Balancer

Flux is a multi-tier load balancer built in Go, with a Layer 4 TCP edge for connection-level load balancing and an optional Layer 7 HTTP proxy layer for application-aware routing and service-level load balancing. Each Layer 7 instance routes requests to independently managed service backends.

The current implementation is the Layer 7 data plane. Layer 4 proxying and additional balancing policies are being developed next.

## Architecture

The target architecture separates connection-level routing from application-level routing. This lets the TCP edge scale independently from HTTP processing, which is responsible for path matching, request forwarding, and backend selection. Each service maintains its own backend pool, with the load-balancing strategy configured independently per pool.

```text
                                      Clients

                                          ▼

                              ┌──────────────────────┐
                              │       Flux L4        │
                              │      TCP Edge        │
                              │   Connection-level   │
                              │   load balancing     │
                              └──────────┬───────────┘

                          ┌──────────────┴─────────────────────┐
                          ▼                                    ▼

                ┌─────────────────┐                   ┌─────────────────┐
                │    Flux L7 #1   │                   │    Flux L7 #2   │
                │    HTTP Proxy   │                   │    HTTP Proxy   │
                └────────┬────────┘                   └────────┬────────┘

                  HTTP routing                           HTTP routing

            ┌────────────┼────────────┐           ┌────────────┼────────────┐
            ▼            ▼            ▼           ▼            ▼            ▼

          Users       Payments      Orders       Users       Payments      Orders
          Service      Service      Service     Service      Service      Service

            ▼            ▼            ▼          ▼             ▼             ▼

        ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐
        │ Users   │  │Payments │  │ Orders  │  │ Users   │  │Payments │  │ Orders  │
        │  Pool   │  │  Pool   │  │  Pool   │  │  Pool   │  │  Pool   │  │  Pool   │
        │   RR    │  │   WRR   │  │   RR    │  │   RR    │  │   WRR   │  │   RR    │
        └────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘

          ┌──┼──┐      ┌──┼──┐      ┌──┼──┐      ┌──┼──┐      ┌──┼──┐      ┌──┼──┐
          ▼  ▼  ▼      ▼  ▼  ▼      ▼  ▼  ▼      ▼  ▼  ▼      ▼  ▼  ▼      ▼  ▼  ▼

          R1 R2 R3     R1 R2 R3     R1 R2 R3     R1 R2 R3     R1 R2 R3     R1 R2 R3
````

At Layer 7, each configured service owns a `BackendPool`. The pool represents one service and its discovered instances; it is independent from every other service pool. Incoming requests are matched against their URL path, the selected pool chooses a healthy backend using its configured strategy, and an `httputil.ReverseProxy` forwards the request upstream.

Backend instances are discovered dynamically through Consul rather than being defined as static replicas in the Flux configuration.

The current path matcher supports exact and prefix matches.

## Features

* **Modes** — `l7`, with `l4` planned

* **Load-balancing strategies** — `round-robin`, `weighted-round-robin`

* **HTTP reverse proxy** — forwards requests to discovered backend services

* **Path-based routing** — supports exact and prefix path matching

* **Per-service backend pools** — each service maintains its own independent pool of backend instances

* **Service discovery** — dynamically discovers healthy service instances through Consul

* **Health checks** — HTTP health endpoints or TCP connection checks with exponential-backoff retries
  * When `health_check.path` is configured, Flux performs an HTTP health check against the specified endpoint. If no health-check path is configured, Flux falls back to a TCP connection check against the backend host and port.

* **Forwarded headers** — manages `X-Forwarded-For`, `X-Forwarded-Host`, and `X-Forwarded-Proto`

<!-- * **Pluggable strategies** — balancing strategies are selected per service through the strategy registry -->

## Roadmap

* **Layer 4 TCP proxy**
  * TCP connection-level load balancing
  * 5-tuple-based backend selection

* **Load-balancing strategies**
  * Least-connections
  * Additional balancing policies

* **Service discovery**
  * Kubernetes service discovery

* **Reliability**
  * Passive health checks
  * Advanced failure handling and retry policies
  * Backend state tracking and recovery

* **Security**
  * Consul ACLs and authentication
  * TLS/mTLS between Flux and Consul
  * Least-privilege permissions for service discovery
  * HTTPS connections to backend services
  * Rate limiting
  * Request and connection limits
  * Stronger API and edge security

## Request Path

```text
                HTTP request
                    |
                    v
                path matcher
                    |
                    v
                BackendPool
                    |
                    v
                healthy backends
                    |
                    v
                load-balancing strategy
                    |
                    v
                Backend reverse proxy
                    |
                    v
                upstream instance
```

Each pool delegates backend selection to its configured strategy. Round robin distributes requests across backend instances using an atomic counter, while weighted round robin distributes requests according to each instance's configured weight.

Backend instances are discovered from Consul and maintained locally in each service's `BackendPool`. Flux performs backend selection locally rather than querying Consul for every request.

The reverse proxy also replaces client-controlled forwarding headers with verified `X-Forwarded-For`, `X-Forwarded-Host`, and `X-Forwarded-Proto` values.

## Backend Pool Synchronization

Each `BackendPool` owns the strategy selected for its service. This keeps backend selection independent: one service can use round robin while another uses weighted round robin.

Backend instances are updated dynamically through service discovery and health checking. The pool uses Left-Right synchronization to allow concurrent readers to access backend snapshots without contending on a reader-writer lock.

Writers update the inactive backend snapshot, publish it by switching the active index, and wait for readers using the previous snapshot to finish before reusing it.

This is designed for the read-heavy workload of backend selection, where requests frequently read the current backend set while discovery and health checks update it less frequently.

## Configuration

Flux is configured through `config.yaml`.

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

The `strategy` field is optional. When omitted, Flux uses its default load-balancing strategy.

Each service can configure its own strategy independently.

Backend instances are not configured directly in `config.yaml`. Services register their instances with Consul, and Flux periodically discovers the available instances for each configured service.

When `health_check.path` is configured, Flux performs an HTTP health check against the specified endpoint. If no health-check path is configured, Flux falls back to a TCP connection check against the backend host and port.

## Run Locally

```bash
go run ./cmd/flux -config-path config.yaml
```

## Verify

The test suite expects:

* Consul to be running 
* The initial demo backend servers to be running

Start the demo backend servers, then run:

```bash
go run ./cmd/demo-server
```

```bash
go test ./...
```
