package util

import (
	"context"
	"fmt"
	"maps"
	"net/url"
	"sync"
	"time"

	"github.com/iamonah/loadbalancer/backend"
	"github.com/iamonah/loadbalancer/util/consul"
	"github.com/rs/zerolog/log"
)

type ServiceDiscovery struct {
	discovery consul.Discovery
	pools     []*backend.BackendPool
	interval  time.Duration
}

func NewServiceDiscovery(d consul.Discovery, pools []*backend.BackendPool, interval time.Duration) *ServiceDiscovery {
	return &ServiceDiscovery{
		discovery: d,
		pools:     pools,
		interval:  interval,
	}
}

func (sd *ServiceDiscovery) Start(ctx context.Context) {
	ticker := time.NewTicker(sd.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sd.Discover(ctx,false)

		case <-ctx.Done():
			return
		}
	}
}

func (sd *ServiceDiscovery) discoverPool(ctx context.Context, pool *backend.BackendPool) {
	serviceName := pool.GetServiceName()

	instances, err := sd.discovery.Discover(ctx, serviceName)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to discover service %s", serviceName)
		return
	}

	existing := pool.GetBackends()

	backends, err := instancesToBackends(instances, existing)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to create backends for service %s", serviceName)
		return
	}

	pool.ReplaceBackends(backends)

	log.Info().Msgf("Discovered %d backends for service %s", len(backends), serviceName)
}

func (sd *ServiceDiscovery) Discover(ctx context.Context, wait bool) {
	var wg sync.WaitGroup

	for _, pool := range sd.pools {
		wg.Add(1)

		go func() {
			defer wg.Done()
			sd.discoverPool(ctx, pool)
		}()
	}

	if wait {
		wg.Wait()
	}
}
func instancesToBackends(instances []consul.Instance, existing []*backend.Backend) ([]*backend.Backend, error) {
	existingByID := make(map[string]*backend.Backend)

	for _, b := range existing {
		if b.ID != "" {
			existingByID[b.ID] = b
		}
	}

	backends := make([]*backend.Backend, 0, len(instances))

	for _, instance := range instances {
		if existingBackend, ok := existingByID[instance.ID]; ok {
			backends = append(backends, existingBackend)
			continue
		}

		rawURL := fmt.Sprintf("http://%s:%d", instance.Address, instance.Port)

		parsedURL, err := url.Parse(rawURL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse backend URL %s: %w", rawURL, err)
		}

		metadata := make(map[string]string)

		maps.Copy(metadata, instance.Metadata)
		b := &backend.Backend{
			ID:       instance.ID,
			URL:      parsedURL,
			Metadata: metadata,
		}

		backends = append(backends, b)
	}

	return backends, nil
}
