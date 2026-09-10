# Flux — A Go Load Balancer

Flux is a multi-tier load balancer built in Go, with a Layer 4 TCP edge for connection-level load balancing and an optional Layer 7 HTTP proxy layer for application-aware routing and service-level load balancing. Each Layer 7 instance routes requests to independently managed service backends.

The current implementation is the Layer 7 data plane. Layer 4 proxying, service discovery, and additional balancing policies are being developed next.

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
```

At Layer 7, each configured route owns a `BackendPool`. The pool represents one service and its replicas; it is independent from every other service pool. Incoming requests are matched against their URL path, the selected pool chooses a backend, and an `httputil.ReverseProxy` forwards the request upstream.

The current path matcher supports exact and prefix matches.

## Features

* **Modes** — `l7`, with `l4` planned
* **Load-balancing strategies** — `round-robin`, `weighted-round-robin`
* **HTTP reverse proxy** — forwards requests to configured backend services
* **Path-based routing** — supports exact and prefix path matching
* **Per-service backend pools** — each service maintains its own independent pool of replicas
* **Health checks** — HTTP health endpoints or TCP connection checks with exponential-backoff retries
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

  * Dynamic backend discovery
  * Consul integration
  * Kubernetes service discovery

* **Pool coordination**

  * Distributed backend-pool coordination
  * Left-Right wait-free synchronization for read-heavy workloads

* **Reliability**

  * Passive health checks
  * Advanced failure handling and retry policies
  * Backend state tracking and recovery

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

                load-balancing strategy

                    |

                    v

                Backend reverse proxy

                    |

                    v

                upstream replica
```

Each pool delegates backend selection to its configured strategy. Round robin distributes requests across replicas using an atomic counter, while weighted round robin distributes requests according to each replica's configured weight.

The reverse proxy also replaces client-controlled forwarding headers with verified `X-Forwarded-For`, `X-Forwarded-Host`, and `X-Forwarded-Proto` values.

## Pool Coordination

Each `BackendPool` owns the strategy selected for its service. This keeps backend selection independent: one service can use round robin while another uses weighted round robin.

The current round-robin implementation uses atomic state so concurrent requests can advance its selection index safely. Backend-pool access uses an `RWMutex`, providing a safe foundation for concurrent backend access, health checking, and future dynamic backend management.

## Configuration

Flux is configured through `config.yaml`.

```yaml
mode: l7

services:
  - name: payments-v1
    matcher: /api/v1/payments

    health_check:
      path: /health

    replicas:
      - url: http://localhost:8081
      - url: http://localhost:8082
      - url: http://localhost:8083

  - name: payments-v2
    matcher: /api/v2/payments
    strategy: weighted-round-robin

    health_check:
      path: /health

    replicas:
      - url: http://localhost:9081
        metadata:
          weight: 1

      - url: http://localhost:9082
        metadata:
          weight: 2

      - url: http://localhost:9083
        metadata:
          weight: 1
```

The `strategy` field is optional. When omitted, Flux uses its default load-balancing strategy.

Each service can configure its own strategy independently.

When `health_check.path` is configured, Flux performs an HTTP health check against the specified endpoint. If no health-check path is configured, Flux falls back to a TCP connection check against the backend host and port.

## Run Locally

```bash
go run ./cmd/loadb -port 8080 -config-path config.yaml
```

## Verify

```bash
go test ./...
```
