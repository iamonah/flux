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
	"github.com/iamonah/loadbalancer/util/consul"
	"github.com/iamonah/loadbalancer/util/health"
	"github.com/rs/zerolog/log"
)

type fluxl7 struct {
	// Finds the server pool by matcher.
	//
	// Note(self): The matcher could be more sophisticated, such as regex
	// or subdomain-based matching, but for now we use simple string matching.
	servicePool map[string]*BackendPool

	// This could later come from an external config file or service discovery mechanism.
	config    *config.Config
	discovery consul.Discovery
	proxy     httputil.ReverseProxy
}

type selectedBackendKey struct{}

var backendContextKey selectedBackendKey

func Newfluxl7(
	cfg *config.Config,
	serviceDiscovery consul.Discovery,
) (*fluxl7, error) {
	svcPools := make(map[string]*BackendPool)
	pools := make([]health.BackendPool, 0, len(cfg.Services))

	for _, service := range cfg.Services {
		pool, err := NewBackendPool(service)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to create server pool for service %s: %w",
				service.Name,
				err,
			)
		}

		svcPools[service.Matcher] = pool
		pools = append(pools, pool)
	}

	lb := &fluxl7{
		config:      cfg,
		servicePool: svcPools,
		discovery:   serviceDiscovery,
	}

	discoveryPools := make([]*BackendPool, 0, len(svcPools))

	for _, pool := range svcPools {
		discoveryPools = append(discoveryPools, pool)
	}

	sd := NewServiceDiscovery(
		lb.discovery,
		discoveryPools,
		10*time.Second,
	)

	// Perform the initial discovery synchronously so the backend
	// pools are populated before the load balancer starts serving requests.
	sd.discover(context.Background())

	hc, err := health.NewHealthCheck(pools, 1*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to create health checker: %w", err)
	}

	go hc.Start()

	go sd.Start(context.Background())

	lb.proxy = httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			selected, ok := pr.In.Context().
				Value(backendContextKey).
				(*backend.Backend)

			if !ok || selected == nil {
				log.Error().Msg("selected backend missing from request context")
				return
			}

			pr.SetURL(selected.URL)
			pr.Out.Header.Del("X-Internal-Secret")
			pr.SetXForwarded()
		},

		ErrorHandler: func(
			w http.ResponseWriter,
			r *http.Request,
			err error,
		) {
			log.Error().
				Err(err).
				Msgf(
					"Error proxying request for %s",
					r.URL.String(),
				)

			w.WriteHeader(http.StatusBadGateway)
			fmt.Fprint(w, "bad gateway")
		},
	}

	return lb, nil
}

// ServeHTTP implements http.Handler.
func (lb *fluxl7) ServeHTTP(
	w http.ResponseWriter,
	r *http.Request,
) {
	log.Info().Msgf(
		"Received new request for %s",
		r.URL.String(),
	)

	log.Info().Msgf(
		"L7 Proxy routing request: path: %s host: %s scheme: %s",
		r.URL.Path,
		r.Host,
		r.URL.Scheme,
	)

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

	ctx := context.WithValue(
		r.Context(),
		backendContextKey,
		selected,
	)

	lb.proxy.ServeHTTP(
		w,
		r.WithContext(ctx),
	)
}

// TODO: implement a more sophisticated matcher, such as regex or
// subdomain-based matching.
//
// Prefix matching is currently O(N) because we iterate over configured
// matchers. This is fine for now, but I will consider using a trie/radix
// tree if the number of routes grows significantly.
func (lb *fluxl7) findPool(
	reqPath string,
) (*BackendPool, bool) {
	log.Info().Msgf(
		"Finding pool for request path: %s",
		reqPath,
	)

	for matcher, pool := range lb.servicePool {
		if reqPath == matcher ||
			strings.HasPrefix(reqPath, matcher+"/") {

			log.Info().Msgf(
				"Matched request path %s to service %s",
				reqPath,
				pool.serviceName,
			)

			return pool, true
		}
	}

	return nil, false
}

func (lb *fluxl7) selectBackend(
	pool *BackendPool,
) *backend.Backend {
	healthyBackends := pool.GetHealthyBackends()

	if len(healthyBackends) == 0 {
		log.Error().Msgf(
			"No healthy backends available for service %s",
			pool.serviceName,
		)

		return nil
	}

	return pool.getNextBackend(healthyBackends)
}