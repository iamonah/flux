package health

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/iamonah/loadbalancer/backend"
	"github.com/rs/zerolog/log"
)

type BackendPool interface {
	GetBackends() []*backend.Backend
	GetHealthCheckPath() *string
}

type HealthCheck struct {
	pools        []BackendPool
	Interval     time.Duration //how often to check the health of the backends
	Client       *http.Client
	maxRetries   int
	initialDelay time.Duration
	maxDelay     time.Duration
}

func NewHealthCheck(pools []BackendPool, interval time.Duration) (*HealthCheck, error) {
	if len(pools) == 0 {
		return nil, fmt.Errorf("no backend pools defined")
	}
	maxRetries := 3
	initialDelay := 100 * time.Millisecond
	maxDelay := 1 * time.Second

	return &HealthCheck{
		pools:        pools,
		Interval:     interval,
		maxRetries:   maxRetries,
		initialDelay: initialDelay,
		maxDelay:     maxDelay,
		Client: &http.Client{
			Timeout: 2 * time.Second,
		},
	}, nil
}

func (hc *HealthCheck) Start() {
	ticker := time.NewTicker(hc.Interval)
	defer ticker.Stop()

	for range ticker.C {
		for _, pool := range hc.pools {
			for _, b := range pool.GetBackends() {
				go hc.checkBackend(pool, b)
			}
		}
	}
}

func (hc *HealthCheck) checkBackend(pool BackendPool, b *backend.Backend) {
	if path := pool.GetHealthCheckPath(); path != nil {
		hc.checkHTTP(b, *path)
		return
	}

	hc.checkTCP(b)
}

func (hc *HealthCheck) checkTCP(b *backend.Backend) {
	urlStr := b.URL.String()

	log.Info().Msgf("Checking backend: %s", urlStr)

	for i := 0; i < hc.maxRetries; i++ {
		conn, err := net.DialTimeout("tcp", b.URL.Host, 2*time.Second)

		if err == nil {
			_ = conn.Close()
			b.IsAlive.Store(true)
			return
		}

		log.Error().
			Err(err).
			Msgf("TCP health check failed: backend %s", urlStr)

		if i == hc.maxRetries-1 {
			break
		}

		hc.backoff(i)
	}

	b.IsAlive.Store(false)
}
func (hc *HealthCheck) checkHTTP(b *backend.Backend, path string) {
	target := b.URL.ResolveReference(&url.URL{
		Path: path,
	})

	urlStr := target.String()

	log.Info().Msgf("Checking backend: %s", urlStr)

	for i := 0; i < hc.maxRetries; i++ {
		resp, err := hc.Client.Get(urlStr)

		if err == nil {
			resp.Body.Close()

			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				b.IsAlive.Store(true)
				return
			}

			log.Error().
				Msgf("HTTP health check failed: backend %s returned status code %d",
					urlStr, resp.StatusCode)
		} else {
			log.Error().
				Err(err).
				Msgf("HTTP health check failed: backend %s", urlStr)
		}

		if i == hc.maxRetries-1 {
			break
		}

		hc.backoff(i)
	}

	b.IsAlive.Store(false)
}

func (hc *HealthCheck) backoff(attempt int) {
	delay := hc.initialDelay * time.Duration(1<<attempt)

	if delay > hc.maxDelay {
		delay = hc.maxDelay
	}

	time.Sleep(delay)
}
