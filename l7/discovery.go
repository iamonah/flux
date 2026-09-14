package l7

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/iamonah/loadbalancer/backend"
	"github.com/iamonah/loadbalancer/util/consul"
	"github.com/rs/zerolog/log"
)

type ServiceDiscovery struct {
	discovery consul.Discovery
	pools     []*BackendPool
	interval  time.Duration
}

func NewServiceDiscovery(d consul.Discovery, pools []*BackendPool, interval time.Duration) *ServiceDiscovery {
	return &ServiceDiscovery{
		discovery: d,
		pools:     pools,
		interval:  interval,
	}
}

func (sd *ServiceDiscovery) Start(ctx context.Context) {
	sd.discover(ctx)

	ticker := time.NewTicker(sd.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sd.discover(ctx)

		case <-ctx.Done():
			return
		}
	}
}

func (sd *ServiceDiscovery) discover(ctx context.Context) {
	for _, pool := range sd.pools {
		serviceName := pool.GetServiceName()

		instances, err := sd.discovery.Discover(ctx, serviceName)
		if err != nil {
			log.Error().Err(err).Msgf("Failed to discover service %s", serviceName)
			continue
		}

		existing := pool.GetBackends()

		backends, err := instancesToBackends(instances, existing)
		if err != nil {
			log.Error().Err(err).Msgf("Failed to create backends for service %s", serviceName)
			continue
		}

		pool.ReplaceBackends(backends)
		log.Info().Msgf("Discovered %d backends for service %s", len(backends), serviceName)
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

		for key, value := range instance.Metadata {
			metadata[key] = value
		}

		b := &backend.Backend{
			ID:       instance.ID,
			URL:      parsedURL,
			Metadata: metadata,
		}

		backends = append(backends, b)
	}

	return backends, nil
}
