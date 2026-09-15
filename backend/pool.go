package backend

import (
	"sync"

	"github.com/iamonah/loadbalancer/config"
)

type Strategy interface {
	NextServer(servers []*Backend) *Backend
}

type BackendPool struct {
	ServiceName string

	mutex           sync.RWMutex
	Backends        []*Backend
	Strategy        Strategy
	HealthCheckPath *string
}

func (sp *BackendPool) GetBackends() []*Backend {
	sp.mutex.RLock()
	defer sp.mutex.RUnlock()

	backends := make([]*Backend, len(sp.Backends))
	copy(backends, sp.Backends)

	return backends
}

func (sp *BackendPool) GetHealthyBackends() []*Backend {
	sp.mutex.RLock()
	defer sp.mutex.RUnlock()

	healthyBackends := make([]*Backend, 0, len(sp.Backends))

	for _, b := range sp.Backends {
		if b.IsAlive.Load() {
			healthyBackends = append(healthyBackends, b)
		}
	}

	return healthyBackends
}

func (sp *BackendPool) GetServiceName() string {
	return sp.ServiceName
}

func (sp *BackendPool) GetHealthCheckPath() *string {
	return sp.HealthCheckPath
}

func (sp *BackendPool) ReplaceBackends(backends []*Backend) {
	sp.mutex.Lock()
	defer sp.mutex.Unlock()

	sp.Backends = backends
}

func NewBackendPool(svcCfg *config.Service, strategy Strategy) (*BackendPool, error) {
	backends := make([]*Backend, 0)

	pool := &BackendPool{
		ServiceName: svcCfg.Name,
		Backends:    backends,
		Strategy:    strategy,
	}

	if svcCfg.HealthCheck != nil {
		pool.HealthCheckPath = &svcCfg.HealthCheck.Path
	}

	return pool, nil
}

func (sp *BackendPool) GetNextBackend(healthyBackends []*Backend) *Backend {
	sp.mutex.RLock()
	defer sp.mutex.RUnlock()

	return sp.Strategy.NextServer(healthyBackends)
}
