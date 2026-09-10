package backend

import (
	"fmt"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/iamonah/loadbalancer/config"
)

type Backend struct {
	URL *url.URL

	mutex             sync.RWMutex
	IsAlive           atomic.Bool
	ActiveConnections atomic.Int32
	Metadata          map[string]string
	FailCount         atomic.Int32
	SuccessCount      atomic.Int32
}

// GetMetaOrDefault returns the value associated with the given key in the
// metadata, or returns the default
func (s *Backend) GetMetaOrDefault(key, def string) string {
	v, ok := s.Metadata[key]
	if !ok {
		return def
	}
	return v
}

// GetMetaOrDefaultInt returns the int value associated with the given key in the
// metadata, or returns the default
func (s *Backend) GetMetaOrDefaultInt(key string, def int) int {
	v := s.GetMetaOrDefault(key, fmt.Sprintf("%d", def))
	a, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return a
}

func (s *Backend) GetActiveConnections() int32 {
	return s.ActiveConnections.Load()
}

func NewBackend(cfg *config.Replica) (*Backend, error) {
	parsedURL, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse URL: %w", err)
	}
	metadata := make(map[string]string)
	if cfg.Metadata.Weight != nil {
		metadata["weight"] = fmt.Sprintf("%d", *cfg.Metadata.Weight)
	}
	backend := &Backend{
		URL:      parsedURL,
		Metadata: metadata,
	}

	return backend, nil
}
