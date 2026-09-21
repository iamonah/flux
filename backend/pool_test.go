package backend

import (
	"sync"
	"testing"

	"github.com/iamonah/loadbalancer/config"
)

func TestBackendPoolConcurrentReadsAndWrites(t *testing.T) {
	strategy := Strategy(nil)

	svcCfg := &config.Service{
		Name: "test-service",
	}

	pool, err := NewBackendPool(
		svcCfg,
		strategy,
	)
	if err != nil {
		t.Fatal(err)
	}

	backends := make([]*Backend, 10)

	for i := range backends {
		backends[i] = &Backend{
			ID: "backend-" + string(rune('0'+i)),
		}
		backends[i].IsAlive.Store(true)
	}

	pool.ReplaceBackends(backends)

	var wg sync.WaitGroup

	// Start many concurrent readers.
	for i := 0; i < 100; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for j := 0; j < 10_000; j++ {
				result := pool.GetHealthyBackends()

				if len(result) != len(backends) {
					t.Errorf(
						"expected %d healthy backends, got %d",
						len(backends),
						len(result),
					)
				}
			}
		}()
	}

	// Start concurrent writers.
	for i := 0; i < 10; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for j := 0; j < 1_000; j++ {
				pool.ReplaceBackends(backends)
			}
		}()
	}

	wg.Wait()
}