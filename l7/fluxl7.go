package l7

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"github.com/iamonah/loadbalancer/backend"
	"github.com/iamonah/loadbalancer/config"
	"github.com/iamonah/loadbalancer/strategy"
	"github.com/iamonah/loadbalancer/util"
	"github.com/iamonah/loadbalancer/util/consul"
	"github.com/iamonah/loadbalancer/util/health"
	"github.com/rs/zerolog/log"
)

type fluxl7 struct {
	servicePool map[string]*backend.BackendPool
	config      *config.Config
	proxy       httputil.ReverseProxy
}

type selectedBackendKey struct{}

var backendContextKey selectedBackendKey

func Newfluxl7(cfg *config.Config, serviceDiscovery consul.Discovery) (*fluxl7, error) {
	svcPools := make(map[string]*backend.BackendPool)
	pools := make([]health.BackendPool, 0, len(cfg.Services))

	for _, service := range cfg.Services {
		strategy, err := strategy.NewStrategy(service.StrategyType)
		if err != nil {
			return nil, fmt.Errorf("failed to create strategy for service %s: %w", service.Name, err)
		}

		pool, err := backend.NewBackendPool(service, strategy)
		if err != nil {
			return nil, fmt.Errorf("failed to create server pool for service %s: %w", service.Name, err)
		}

		svcPools[service.Matcher] = pool
		pools = append(pools, pool)
	}

	lb := &fluxl7{config: cfg, servicePool: svcPools}

	discoveryPools := make([]*backend.BackendPool, 0, len(svcPools))
	for _, pool := range svcPools {
		discoveryPools = append(discoveryPools, pool)
	}

	sd := util.NewServiceDiscovery(serviceDiscovery, discoveryPools, 10*time.Second)

	// Perform initial discovery synchronously so the backend
	// pools are populated before the load balancer starts serving requests.
	sd.Discover(context.Background(), true)

	hc, err := health.NewHealthCheck(pools, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to create health checker: %w", err)
	}

	// Perform the initial health check synchronously ensuring backend health is established before serving requests.
	hc.SyncAllHealthyBackends()

	// Continue health checks in the background.
	go hc.Start()

	// Continue service discovery in the background.
	go sd.Start(context.Background())

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 300
	transport.MaxIdleConnsPerHost = 100

	lb.proxy = httputil.ReverseProxy{
		Transport: transport,

		Rewrite: func(pr *httputil.ProxyRequest) {
			selected, ok := pr.In.Context().Value(backendContextKey).(*backend.Backend)
			if !ok || selected == nil {
				log.Error().Msg("selected backend missing from request context")
				return
			}

			pr.SetURL(selected.URL)
			pr.Out.Header.Del("X-Internal-Secret")
			pr.SetXForwarded()
		},

		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Error().Err(err).Msgf("Error proxying request for %s", r.URL.String())

			w.WriteHeader(http.StatusBadGateway)
			fmt.Fprint(w, "bad gateway")
		},
	}

	return lb, nil
}

// ServeHTTP implements http.Handler.
func (lb *fluxl7) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pool, ok := lb.findPool(r.URL.Path)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "service not found")
		return
	}

	selected := lb.selectBackend(pool)
	if selected == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, "service unavailable")
		return
	}

	ctx := context.WithValue(r.Context(), backendContextKey, selected)
	lb.proxy.ServeHTTP(w, r.WithContext(ctx))
}

// TODO: implement a more sophisticated matcher, such as regex or
// subdomain-based matching.
//
// Prefix matching is currently O(N) because we iterate over configured
// matchers. This is fine for now, but I will consider using a trie/radix
// tree if the number of routes grows significantly.
func (lb *fluxl7) findPool(reqPath string) (*backend.BackendPool, bool) {
	for matcher, pool := range lb.servicePool {
		if reqPath == matcher || strings.HasPrefix(reqPath, matcher+"/") {
			return pool, true
		}
	}
	return nil, false
}

func (lb *fluxl7) selectBackend(pool *backend.BackendPool) *backend.Backend {
	healthyBackends := pool.GetHealthyBackends()
	if len(healthyBackends) == 0 {
		log.Error().Msgf("No healthy backends available for service %s", pool.ServiceName)
		return nil
	}
	return pool.GetNextBackend(healthyBackends)
}
