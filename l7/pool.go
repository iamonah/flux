package l7

import (
	"fmt"
	"net/url"
	"sync"

	"github.com/iamonah/loadbalancer/config"
	"github.com/iamonah/loadbalancer/strategy"
)

type BackendPool struct {
	serviceName string

	mutex    sync.RWMutex
	Backends []*Backend
	Strategy strategy.Strategy
}

func (sp *BackendPool) AddBackendToPool(backend []*Backend) {
	sp.mutex.Lock()
	defer sp.mutex.Unlock()
	sp.Backends = append(sp.Backends, backend...)
	sp.Strategy.AddBackendCount(uint32(len(backend)))
}

func (sp *BackendPool) RemoveBackendFromPool(backend *Backend) {
	sp.mutex.Lock()
	defer sp.mutex.Unlock()
	for i, b := range sp.Backends {
		if b == backend {
			sp.Backends = append(sp.Backends[:i], sp.Backends[i+1:]...)
			return
		}
	}
}

func NewServerPool(svcCfg *config.Service) (*BackendPool, error) {
	backends := make([]*Backend, 0, len(svcCfg.Replicas))

	if len(svcCfg.Replicas) == 0 {
		return nil, fmt.Errorf("No replicas defined for service %s", svcCfg.Name)
	}
	for _, replica := range svcCfg.Replicas {
		parsedURL, err := url.Parse(replica.URL)
		if err != nil {
			return nil, fmt.Errorf("Failed to parse URL: %w", err)
		}

		backend := NewBackend(parsedURL, svcCfg.Matcher, replica.Metadata.Weight)
		backends = append(backends, backend)
	}

	// If no strategy is defined, default to round-robin
	if svcCfg.Strategy == nil {
		defaultStrategy := "round-robin"
		svcCfg.Strategy = &defaultStrategy
	}

	strategy, err := strategy.NewStrategy(*svcCfg.Strategy, &svcCfg.Replicas)
	if err != nil {
		return nil, fmt.Errorf("Failed to create strategy: %w", err)
	}
	return &BackendPool{serviceName: svcCfg.Name, Backends: backends, Strategy: strategy}, nil
}

func (sp *BackendPool) getNextBackend() *Backend {
	nextIndex := sp.Strategy.NextServer()
	sp.mutex.RLock()
	defer sp.mutex.RUnlock()
	return sp.Backends[nextIndex]
}
