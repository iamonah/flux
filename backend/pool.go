package backend

import (
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/iamonah/loadbalancer/config"
)

type Strategy interface {
	NextServer(servers []*Backend) *Backend
}

type BackendPool struct {
	ServiceName string
	Protocol    string

	backends     [2][]*Backend
	readCounters [2]atomic.Int32
	activeIndex  atomic.Int32
	writerMutex  sync.Mutex

	StrategyType    string
	Strategy        Strategy
	HealthCheckPath *string
}

func (sp *BackendPool) GetBackends() []*Backend {
	for {
		active := sp.activeIndex.Load()

		sp.readCounters[active].Add(1)

		if sp.activeIndex.Load() != active {
			sp.readCounters[active].Add(-1)
			continue
		}

		backends := make([]*Backend, len(sp.backends[active]))
		copy(backends, sp.backends[active])

		sp.readCounters[active].Add(-1)

		return backends
	}
}

func (sp *BackendPool) GetHealthyBackends() []*Backend {
	for {
		active := sp.activeIndex.Load()

		sp.readCounters[active].Add(1)

		if sp.activeIndex.Load() != active {
			sp.readCounters[active].Add(-1)
			continue
		}

		healthyBackends := make([]*Backend, 0, len(sp.backends[active]))

		for _, b := range sp.backends[active] {
			if b.IsAlive.Load() {
				healthyBackends = append(healthyBackends, b)
			}
		}

		sp.readCounters[active].Add(-1)

		return healthyBackends
	}
}

func (sp *BackendPool) GetServiceName() string {
	return sp.ServiceName
}

func (sp *BackendPool) GetProtocol() string {
	return sp.Protocol
}

func (sp *BackendPool) GetHealthCheckPath() *string {
	return sp.HealthCheckPath
}

func (sp *BackendPool) ReplaceBackends(backends []*Backend) {
	sp.writerMutex.Lock()
	defer sp.writerMutex.Unlock()

	active := sp.activeIndex.Load()
	inactive := 1 - active

	newBackends := make([]*Backend, len(backends))
	copy(newBackends, backends)

	sp.backends[inactive] = newBackends

	sp.activeIndex.Store(inactive)

	for sp.readCounters[active].Load() > 0 {
		runtime.Gosched()
	}
}

func NewBackendPool(svcCfg *config.Service, strategy Strategy) (*BackendPool, error) {
	pool := &BackendPool{
		ServiceName: svcCfg.Name,
		Protocol:    svcCfg.Protocol,
		backends: [2][]*Backend{
			make([]*Backend, 0),
			make([]*Backend, 0),
		},
		Strategy:     strategy,
		StrategyType: svcCfg.StrategyType,
	}

	if svcCfg.HealthCheck != nil {
		pool.HealthCheckPath = &svcCfg.HealthCheck.Path
	}

	return pool, nil
}

func (sp *BackendPool) GetNextBackend(healthyBackends []*Backend) *Backend {
	return sp.Strategy.NextServer(healthyBackends)
}