package l7

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"github.com/iamonah/loadbalancer/backend"
	"github.com/iamonah/loadbalancer/config"
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
	config *config.Config

	proxy httputil.ReverseProxy
}

func Newfluxl7(cfg *config.Config) (*fluxl7, error) {
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

	hc, err := health.NewHealthCheck(pools, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to create health checker: %w", err)
	}

	go hc.Start()

	lb := &fluxl7{
		config:      cfg,
		servicePool: svcPools,
	}

	lb.proxy = httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			backend := lb.selectBackend(pr.In)
			if backend == nil {
				return
			}

			pr.SetURL(backend.URL)

			req := pr.Out

			req.Header.Del("X-Forwarded-For")
			req.Header.Del("X-Forwarded-Host")
			req.Header.Del("X-Forwarded-Proto")
			req.Header.Del("X-Internal-Secret")
			req.Header.Del("Server")

			clientIP, _, err := net.SplitHostPort(pr.In.RemoteAddr)
			if err == nil {
				req.Header.Set("X-Forwarded-For", clientIP)
			} else {
				req.Header.Set("X-Forwarded-For", pr.In.RemoteAddr)
			}

			req.Header.Set("X-Forwarded-Host", pr.In.Host)

			if pr.In.TLS != nil {
				req.Header.Set("X-Forwarded-Proto", "https")
			} else {
				req.Header.Set("X-Forwarded-Proto", "http")
			}
		},
	}

	return lb, nil
}

// ServeHTTP implements http.Handler.
func (lb *fluxl7) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Info().Msgf("Received new request for %s", r.URL.String())

	log.Info().Msgf(
		"L7 Proxy routing request: path: %s host: %s scheme: %s",
		r.URL.Path,
		r.URL.Host,
		r.URL.Scheme,
	)

	lb.proxy.ServeHTTP(w, r)
}

// Prefix matching is currently O(N) because we iterate over configured
// matchers. This is fine for now, but I will consider using a trie/radix
// tree if the number of routes grows significantly.
func (lb *fluxl7) findPool(reqPath string) (*BackendPool, bool) {
	log.Info().Msgf("Finding pool for request path: %s", reqPath)

	for matcher, pool := range lb.servicePool {
		if reqPath == matcher || strings.HasPrefix(reqPath, matcher+"/") {
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

func (lb *fluxl7) selectBackend(r *http.Request) *backend.Backend {
	pool, ok := lb.findPool(r.URL.Path)
	if !ok {
		log.Warn().Msgf(
			"No matching service pool found for request path: %s",
			r.URL.Path,
		)

		return nil
	}

	server := pool.getNextBackend()
	if server == nil {
		log.Error().Msgf(
			"No available backends for service %s",
			pool.serviceName,
		)

		return nil
	}

	return server
}
