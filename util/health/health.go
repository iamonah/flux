package health

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/iamonah/loadbalancer/backend"
	"github.com/rs/zerolog/log"
)

type BackendPool interface {
	GetBackends() []*backend.Backend
	GetProtocol() string
	GetHealthCheckPath() *string
}

type HealthCheck struct {
	pools        []BackendPool
	Interval     time.Duration
	Client       *http.Client
	initialDelay time.Duration
	maxDelay     time.Duration
}

func NewHealthCheck(pools []BackendPool, interval time.Duration) (*HealthCheck, error) {
	if len(pools) == 0 {
		return nil, fmt.Errorf("no backend pools defined")
	}

	initialDelay := 100 * time.Millisecond
	maxDelay := 1 * time.Second

	return &HealthCheck{
		pools:        pools,
		Interval:     interval,
		initialDelay: initialDelay,
		maxDelay:     maxDelay,
		Client: &http.Client{
			Timeout: 2 * time.Second,
		},
	}, nil
}

// SyncAllHealthyBackends performs the initial health check synchronously.
// This ensures backend health is established before serving requests.
func (hc *HealthCheck) SyncAllHealthyBackends() {
	for _, pool := range hc.pools {
		for _, b := range pool.GetBackends() {
			hc.checkBackend(pool, b)
		}
	}
}

// Start continues health checks periodically in the background.
func (hc *HealthCheck) Start() {
	ticker := time.NewTicker(hc.Interval)
	defer ticker.Stop()

	for range ticker.C {
		hc.checkAllBackends()
	}
}

func (hc *HealthCheck) checkAllBackends() {
	for _, pool := range hc.pools {
		for _, b := range pool.GetBackends() {
			go hc.checkBackend(pool, b)
		}
	}
}

func (hc *HealthCheck) checkBackend(pool BackendPool, b *backend.Backend) {
	if path := pool.GetHealthCheckPath(); path != nil {
		hc.checkHTTP(b, *path)
		return
	}

	// If no health check path is provided, default to TCP or UDP checks based on the protocol.
	switch strings.ToLower(pool.GetProtocol()) {
	case "udp":
		hc.checkUDP(b)

	default:
		hc.checkTCP(b)
	}
}

func (hc *HealthCheck) checkTCP(b *backend.Backend) {
	urlStr := b.URL.String()

	log.Info().Msgf("Checking TCP backend: %s", urlStr)

	maxRetry := b.GetMetaOrDefaultInt("max_retries", 3)

	for i := 0; i < maxRetry; i++ {
		conn, err := net.DialTimeout("tcp", b.URL.Host, 2*time.Second)

		if err == nil {
			_ = conn.Close()
			b.IsAlive.Store(true)
			return
		}

		log.Error().Err(err).Msgf("TCP health check failed: backend %s", urlStr)

		if i == maxRetry-1 {
			break
		}

		hc.backoff(i)
	}

	b.IsAlive.Store(false)
}

func (hc *HealthCheck) checkUDP(b *backend.Backend) {
	urlStr := b.URL.String()

	log.Info().Msgf("Checking UDP backend: %s", urlStr)

	maxRetry := b.GetMetaOrDefaultInt("max_retries", 3)

	for i := 0; i < maxRetry; i++ {
		if hc.udpProbe(b) {
			b.IsAlive.Store(true)
			return
		}

		log.Error().Msgf("UDP health check failed: backend %s", urlStr)

		if i == maxRetry-1 {
			break
		}

		hc.backoff(i)
	}

	b.IsAlive.Store(false)
}

func (hc *HealthCheck) udpProbe(b *backend.Backend) bool {
	addr, err := net.ResolveUDPAddr("udp", b.URL.Host)
	if err != nil {
		log.Error().Err(err).Msgf("failed to resolve UDP backend: %s", b.URL.String())
		return false
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		log.Error().Err(err).Msgf("failed to dial UDP backend: %s", b.URL.String())
		return false
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	const probe = "FLUX_HEALTH_CHECK"

	if _, err := conn.Write([]byte(probe)); err != nil {
		log.Error().Err(err).Msgf("failed to send UDP health probe: backend %s", b.URL.String())
		return false
	}

	buffer := make([]byte, 1024)

	n, _, err := conn.ReadFromUDP(buffer)
	if err != nil {
		return false
	}

	return n > 0
}

func (hc *HealthCheck) checkHTTP(b *backend.Backend, path string) {
	target := b.URL.ResolveReference(&url.URL{Path: path})
	urlStr := target.String()

	log.Info().Msgf("Checking HTTP backend: %s", urlStr)

	maxRetry := b.GetMetaOrDefaultInt("max_retries", 3)
	for i := 0; i < maxRetry; i++ {
		resp, err := hc.Client.Get(urlStr)

		if err == nil {
			resp.Body.Close()

			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				b.IsAlive.Store(true)
				return
			}
			log.Error().Msgf("HTTP health check failed: backend %s returned status code %d", urlStr, resp.StatusCode)
		} else {
			log.Error().Err(err).Msgf("HTTP health check failed: backend %s", urlStr)
		}

		if i == maxRetry-1 {
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
