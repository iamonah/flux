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
	pools    []BackendPool
	Interval time.Duration //how often to check the health of the backends
	Client   *http.Client
}

func NewHealthCheck(pools []BackendPool, interval time.Duration) (*HealthCheck, error) {
	if len(pools) == 0 {
		return nil, fmt.Errorf("no backend pools defined")
	}

	return &HealthCheck{
		pools:    pools,
		Interval: interval,
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
	fmt.Println("Checking backend:", b.URL.String())

	conn, err := net.DialTimeout("tcp", b.URL.Host, 2*time.Second)
	if err != nil {
		urlStr := b.URL.String()
		log.Error().Err(err).Msgf("TCP health check failed: backend %s", urlStr)
		b.IsAlive.Store(false)
		return
	}

	_ = conn.Close()
	b.IsAlive.Store(true)
}

func (hc *HealthCheck) checkHTTP(b *backend.Backend, path string) {
	target := b.URL.ResolveReference(&url.URL{
		Path: path,
	})

	urlStr := target.String()
	log.Info().Msgf("Checking backend: %s", urlStr)

	resp, err := hc.Client.Get(target.String())
	if err != nil {
		log.Error().Err(err).Msg("HTTP health check failed")
		b.IsAlive.Store(false)
		return
	}

	defer resp.Body.Close()

	// Consider any 2xx status code as healthy
	alive := resp.StatusCode >= 200 && resp.StatusCode < 300
	b.IsAlive.Store(alive)
	if !alive {
		b.FailCount.Add(1)
	} else {
		b.SuccessCount.Add(1)
	}

}
