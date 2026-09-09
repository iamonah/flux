package backend

import (
	"fmt"
	"net/url"
	"strconv"
	"sync/atomic"

	"github.com/iamonah/loadbalancer/config"
)

type Backend struct {
	URL               *url.URL
	IsAlive           atomic.Bool
	ActiveConnections atomic.Int32
	//metadata[weight]
	Metadata          map[string]string
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

func NewBackend(cfg *config.Replica) (*Backend, error) {
	parsedURL, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse URL: %w", err)
	}
	metadata := make(map[string]string)
	if cfg.Metadata.Weight != nil {
		metadata["weight"] = fmt.Sprintf("%d", *cfg.Metadata.Weight)
	}
	return &Backend{
		URL:      parsedURL,
		Metadata: metadata,
	}, nil
}
