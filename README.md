# **Go Load Balancer**

The project is designed around a multi-tier architecture: a Layer 4 TCP edge can distribute connections across Layer 7 HTTP reverse proxies, while each Layer 7 instance routes requests to independently managed service backends.

The current implementation is the Layer 7 data plane. Layer 4 proxying, health checks, service discovery, and advanced balancing policies are intentionally being developed next rather than presented as completed features.

## Architecture

The target architecture separates connection-level routing from application-level routing. This lets the TCP edge scale independently from HTTP processing, which is responsible for path matching, request forwarding, and backend selection.

```text
                     Clients

                        |

                        v

                 +--------------+

                 | Layer 4 TCP  |

                 +--------------+

                    /        \

                   v          v

              +--------+  +--------+

              | L7 #1  |  | L7 #2  |

              +--------+  +--------+

                   \        /

                    \      /

                     \    /

                      \  /

                       v

                +----------------+

                | L7 Route Table |

                +----------------+

                  /      |      \

                 v       v       v

             +-------+ +----------+ +--------+

             | Users | | Payments | | Orders |

             +-------+ +----------+ +--------+

                 |          |          |

                 v          v          v

           +---------+ +----------+ +-------------+

           | Round   | | Least    | | Weighted    |

           | Robin   | | Conn.    | | Round Robin |

           +---------+ +----------+ +-------------+

                 |          |          |

                 v          v          v

             +-------+  +-------+  +-------+

             | Pool  |  | Pool  |  | Pool  |

             +-------+  +-------+  +-------+

              /  |  \    /  |  \    /  |  \

             U1 U2 U3   P1 P2 P3   O1 O2 O3
```

At Layer 7, each configured route owns a `BackendPool`. The pool represents one service and its replicas; it is independent from every other service pool. Incoming requests are matched against their URL path, the selected pool chooses a backend, and an `httputil.ReverseProxy` forwards the request upstream.

The current path matcher supports exact and prefix matches.

## Feature Scope

### Layer 4 — planned

* TCP proxying for connection-level traffic forwarding
* 5-tuple-based backend selection

### Layer 7

* HTTP reverse proxying to configured backend services
* Application-aware routing through request path matchers
* Per-service backend pools and pluggable balancing strategies
* Round-robin request distribution
* Forwarded-header management for proxied requests

### Reliability and Operations — planned

* Active backend health checks and unhealthy-backend removal
* Failure handling and request retries where appropriate
* Dynamic backend management and service discovery
* Additional strategies, including least-connections and weighted round robin
* Pool coordination using a Left-Right wait-free pattern for read-heavy traffic

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

Each pool delegates backend selection to a strategy. The implemented strategy is round robin: requests are distributed across a service's replicas using an atomic counter. The reverse proxy also replaces client-controlled forwarding headers with verified `X-Forwarded-For`, `X-Forwarded-Host`, and `X-Forwarded-Proto` values.

## Pool Coordination

Each `BackendPool` owns the strategy selected for its service. This keeps backend selection independent: one service can use round robin while another uses a different strategy as those implementations are added.

The current round-robin implementation uses atomic state so concurrent requests can advance its selection index safely. Backend-pool access uses an `RWMutex`, providing a safe foundation for dynamic backend management and health checking. Weighted round robin and least-connections remain planned strategy implementations.

## Configuration

Services are declared in `config.yaml`. Each service defines a name, route matcher, load balancing strategy, and one or more replicas.

### Single Service

A configuration with a single service can be defined as:

```yaml
services:
  - name: payments
    matcher: /payments
    strategy: round-robin
    replicas:
      - http://localhost:8081
      - http://localhost:8082
```

### Multiple Services

Multiple services can be configured independently. Each service maintains its own replica pool and load balancing strategy.

```yaml
services:
  - name: payments
    matcher: /api/payments
    strategy: round-robin
    replicas:
      - http://localhost:8081
      - http://localhost:8082

  - name: users
    matcher: /api/users
    strategy: round-robin
    replicas:
      - http://localhost:8083
      - http://localhost:8084
```

Requests matching `/api/payments` are distributed only across the `payments` replicas, while requests matching `/api/users` are distributed only across the `users` replicas.

### Round Robin

A service using round robin lists its replicas directly:

```yaml
services:
  - name: payments
    matcher: /api/payments
    strategy: round-robin
    replicas:
      - http://localhost:8081
      - http://localhost:8082
      - http://localhost:8083
```

Each replica participates equally in the rotation.

### Weighted Round Robin

A service using weighted round robin assigns a weight to each replica:

```yaml
services:
  - name: payments
    matcher: /api/payments
    strategy: weighted-round-robin
    replicas:
      - address: http://localhost:8081
        weight: 3
      - address: http://localhost:8082
        weight: 2
      - address: http://localhost:8083
        weight: 1
```

The weight determines the replica's relative share of requests.

For the configuration above, the relative distribution is:

```text
localhost:8081 → 3
localhost:8082 → 2
localhost:8083 → 1
```

The weights are relative values rather than percentages. Therefore, `3:2:1` and `30:20:10` represent the same distribution.

Each service can define its own strategy independently:

```yaml
services:
  - name: payments
    matcher: /api/payments
    strategy: round-robin
    replicas:
      - http://localhost:8081
      - http://localhost:8082

  - name: users
    matcher: /api/users
    strategy: weighted-round-robin
    replicas:
      - address: http://localhost:8083
        weight: 2
      - address: http://localhost:8084
        weight: 1
```

This allows balancing to be configured at the service level rather than globally across all replicas.


## Run Locally

```bash
go run ./cmd/loadb -port 8080 -config-path config.yaml
```

## Verify

```bash
go test ./...
```
