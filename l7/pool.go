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

	mutex           sync.RWMutex
	Backends        []*backend.Backend
	Strategy        strategy.Strategy
	HealthCheckPath *string
}

func (sp *BackendPool) GetBackends() []*backend.Backend {
	sp.mutex.RLock()
	defer sp.mutex.RUnlock()

	backends := make([]*backend.Backend, len(sp.Backends))
	copy(backends, sp.Backends)

	return backends
}

func (sp *BackendPool) GetHealthyBackends() []*backend.Backend {
	sp.mutex.RLock()
	defer sp.mutex.RUnlock()

	healthyBackends := make([]*backend.Backend, 0, len(sp.Backends))
	for _, b := range sp.Backends {
		if b.IsAlive.Load() {
			healthyBackends = append(healthyBackends, b)
		}
	}

	return healthyBackends
}

func (sp *BackendPool) GetServiceName() string {
	return sp.serviceName
}

func (sp *BackendPool) GetHealthCheckPath() *string {
	if sp.HealthCheckPath == nil {
		return nil
	}
	return sp.HealthCheckPath
}

func (sp *BackendPool) ReplaceBackends(backends []*backend.Backend) {
	sp.mutex.Lock()
	defer sp.mutex.Unlock()

	sp.Backends = backends
}

func (sp *BackendPool) AddSingleBackendToPool(b *backend.Backend) {
	sp.mutex.Lock()
	defer sp.mutex.Unlock()
	sp.Backends = append(sp.Backends, b)
}

func (sp *BackendPool) AddMultipleBackendToPool(backends []*backend.Backend) {
	sp.mutex.Lock()
	defer sp.mutex.Unlock()
	sp.Backends = append(sp.Backends, backends...)
}

func (sp *BackendPool) RemoveBackendFromPool(target *backend.Backend) {
	sp.mutex.Lock()
	defer sp.mutex.Unlock()
	for i, b := range sp.Backends {
		if b == target {
			sp.Backends = append(sp.Backends[:i], sp.Backends[i+1:]...)
			return
		}
	}
}

func NewBackendPool(svcCfg *config.Service) (*BackendPool, error) {
	var backends []*backend.Backend

	strat, err := strategy.NewStrategy(svcCfg.Strategy, backends)
	if err != nil {
		return nil, fmt.Errorf("failed to create strategy: %w", err)
	}

	if svcCfg.HealthCheck == nil {
		return &BackendPool{
			serviceName: svcCfg.Name,
			Backends:    backends,
			Strategy:    strat,
		}, nil
	}
	pool:=  &BackendPool{
		serviceName:     svcCfg.Name,
		Backends:        backends,
		Strategy:        strat,
		HealthCheckPath: &svcCfg.HealthCheck.Path,
	}
	
	if svcCfg.HealthCheck != nil {
		pool.HealthCheckPath = &svcCfg.HealthCheck.Path
	}
	return pool, nil
}

func (sp *BackendPool) getNextBackend(healthyBackends []*backend.Backend) *backend.Backend {
	sp.mutex.RLock()
	defer sp.mutex.RUnlock()
	return sp.Strategy.NextServer(healthyBackends)
}
