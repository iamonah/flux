package strategy

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/iamonah/loadbalancer/backend"
	"github.com/rs/zerolog/log"
)

type StrategyType string

var (
	RoundRobin         StrategyType = newStrategyType("round-robin")
	WeightedRoundRobin StrategyType = newStrategyType("weighted-round-robin")
	LeastConnections   StrategyType = newStrategyType("least-connections")
)

var strategyTypes = make(map[string]StrategyType)

func newStrategyType(s string) StrategyType {
	st := StrategyType(s)
	strategyTypes[strings.ToLower(s)] = st
	return st
}

func ParseStrategyType(s string) (StrategyType, error) {
	st, ok := strategyTypes[strings.ToLower(s)]
	if !ok {
		return "", fmt.Errorf("invalid strategy type: %s", s)
	}

	return st, nil
}

func (s StrategyType) String() string {
	return string(s)
}

var strategyRegistry = map[StrategyType]func() backend.Strategy{
	RoundRobin: func() backend.Strategy {
		return NewRoundRobin()
	},
	WeightedRoundRobin: func() backend.Strategy {
		return NewSmoothWRR()
	},
}

// TODO: implement LeastConnections

func NewStrategy(strategy *string) (backend.Strategy, error) {
	if strategy == nil {
		return NewRoundRobin(), nil
	}

	st, err := ParseStrategyType(*strategy)
	if err != nil {
		log.Warn().Str("strategy", *strategy).Msg("strategy initializer not found, falling back to round-robin")
		st = RoundRobin
	}

	str, ok := strategyRegistry[st]
	if !ok {
		return nil, fmt.Errorf("strategy not initialized: %s", *strategy)
	}

	return str(), nil
}

type roundRobinAlgo struct {
	Current atomic.Uint32
}

func NewRoundRobin() *roundRobinAlgo {
	return &roundRobinAlgo{}
}

func (rr *roundRobinAlgo) NextServer(servers []*backend.Backend) *backend.Backend {
	length := uint32(len(servers))
	if length == 0 {
		return nil
	}
	for {
		current := rr.Current.Load()
		next := current + 1
		if next >= length {
			next = 0
		}
		if rr.Current.CompareAndSwap(current, next) {
			return servers[next]
		}
	}
}

type smoothWRRState struct {
	Server        *backend.Backend
	Weight        int32
	CurrentWeight int32
}

type smoothWRRAlgo struct {
	mu     sync.Mutex
	states map[*backend.Backend]*smoothWRRState
}

func NewSmoothWRR() *smoothWRRAlgo {
	return &smoothWRRAlgo{states: make(map[*backend.Backend]*smoothWRRState)}
}

func (s *smoothWRRAlgo) NextServer(servers []*backend.Backend) *backend.Backend {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(servers) == 0 {
		return nil
	}

	s.syncStates(servers)

	var best *smoothWRRState
	var totalWeight int32

	for _, server := range servers {
		state := s.states[server]

		state.CurrentWeight += state.Weight
		totalWeight += state.Weight

		if best == nil || state.CurrentWeight > best.CurrentWeight {
			best = state
		}
	}

	if best == nil {
		return nil
	}

	best.CurrentWeight -= totalWeight

	return best.Server
}

func (s *smoothWRRAlgo) syncStates(servers []*backend.Backend) {
	currentServers := make(map[*backend.Backend]struct{}, len(servers))

	for _, server := range servers {
		currentServers[server] = struct{}{}

		if _, exists := s.states[server]; exists {
			continue
		}

		s.states[server] = &smoothWRRState{
			Server: server,
			Weight: int32(server.GetMetaOrDefaultInt("weight", 1)),
		}
	}

	for server := range s.states {
		if _, exists := currentServers[server]; !exists {
			delete(s.states, server)
		}
	}
}
