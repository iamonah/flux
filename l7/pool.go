package l7

import (
	"fmt"
	"sync"

	"github.com/iamonah/loadbalancer/backend"
	"github.com/iamonah/loadbalancer/config"
	"github.com/iamonah/loadbalancer/strategy"
)

type BackendPool struct {
	serviceName string

	mutex    sync.RWMutex
	Backends []*backend.Backend
	Strategy strategy.Strategy
}

func (sp *BackendPool) AddSingleBackendToPool(backend *backend.Backend) {
	sp.mutex.Lock()
	defer sp.mutex.Unlock()
	sp.Backends = append(sp.Backends, backend)
	sp.Strategy.AddBackendCount(backend)
}

func (sp *BackendPool) AddMultipleBackendToPool(backend []*backend.Backend) {
	sp.mutex.Lock()
	defer sp.mutex.Unlock()
	sp.Backends = append(sp.Backends, backend...)
	for _, b := range backend {
		sp.Strategy.AddBackendCount(b)
	}
}

func (sp *BackendPool) RemoveBackendFromPool(backend *backend.Backend) {
	sp.mutex.Lock()
	defer sp.mutex.Unlock()
	for i, b := range sp.Backends {
		if b == backend {
			sp.Backends = append(sp.Backends[:i], sp.Backends[i+1:]...)
			return
		}
	}
}

func NewBackendPool(svcCfg *config.Service) (*BackendPool, error) {
	backends := make([]*backend.Backend, 0, len(svcCfg.Replicas))

	if len(svcCfg.Replicas) == 0 {
		return nil, fmt.Errorf("No replicas defined for service %s", svcCfg.Name)
	}
	for _, replica := range svcCfg.Replicas {

		backend, err := backend.NewBackend(&replica)
		if err != nil {
			return nil, fmt.Errorf("Failed to create backend: %w", err)
		}
		backends = append(backends, backend)
	}

	strategy, err := strategy.NewStrategy(svcCfg.Strategy, backends)
	if err != nil {
		return nil, fmt.Errorf("Failed to create strategy: %w", err)
	}

	return &BackendPool{serviceName: svcCfg.Name, Backends: backends, Strategy: strategy}, nil
}

func (sp *BackendPool) getNextBackend() *backend.Backend {
	sp.mutex.RLock()
	defer sp.mutex.RUnlock()
	return sp.Strategy.NextServer(sp.Backends)
}
