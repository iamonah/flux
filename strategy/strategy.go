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

type Strategy interface {
	NextServer(servers []*backend.Backend) *backend.Backend
}

var strategyRegistry = make(map[StrategyType]func([]*backend.Backend) Strategy)

// TODO: implement LeastConnections
// case LeastConnections:
//
//	return NewLeastConnections(),
func init() {
	strategyRegistry = map[StrategyType]func([]*backend.Backend) Strategy{
		RoundRobin: func(servers []*backend.Backend) Strategy {
			return NewRoundRobin()
		},
		WeightedRoundRobin: func(servers []*backend.Backend) Strategy {
			return NewSmoothWRR(servers)
		},
	}
}

func NewStrategy(strategy *string, servers []*backend.Backend) (Strategy, error) {
	if strategy == nil {
		return NewRoundRobin(), nil
	}
	st, err := ParseStrategyType(*strategy)
	if err != nil {
		log.Warn().Str("strategy", *strategy).Msg("strategy initializer not found, falling back to round-robin")
		st = RoundRobin
	}
	strategyRegistry, ok := strategyRegistry[st]
	if !ok {
		return nil, fmt.Errorf("strategy not initialized: %s", *strategy)
	}

	return strategyRegistry(servers), nil
}

type roundRobinAlgo struct {
	Current atomic.Uint32
}

func NewRoundRobin() *roundRobinAlgo {
	rr := &roundRobinAlgo{}
	rr.Current.Store(0)
	return rr
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

type node struct {
	Server        *backend.Backend
	Weight        int32 // Fixed configured capacity weight
	CurrentWeight int32 // Dynamically adjusted runtime weight
}

// Smooth WRR provides smoother load distribution than standard WRR.
//
// It selects the next server using weights that represent
// the relative compute capacity of each server.
type smoothWRRAlgo struct {
	mu    sync.Mutex
	nodes []*node
}

func NewNode(server *backend.Backend, weight int32) *node {
	return &node{
		Server:        server,
		Weight:        weight,
		CurrentWeight: 0,
	}
}

func NewSmoothWRR(servers []*backend.Backend) *smoothWRRAlgo {
	nodes := make([]*node, 0, len(servers))

	for _, server := range servers {
		node := &node{
			Server: server,
		}

		node.Weight = int32(server.GetMetaOrDefaultInt("weight", 1))
		node.CurrentWeight = 0
		nodes = append(nodes, node)
	}

	wrr := &smoothWRRAlgo{
		nodes: nodes,
	}
	return wrr
}

// General overview of how the smooth WRR algorithm works:
// Server A → weight 5
// Server B → weight 1
// The total weight is:
// 5 + 1 = 6
// So over a sufficiently large number of requests, the target distribution is approximately:
// Server A → 5/6 ≈ 83.3% of the Traffic
// Server B → 1/6 ≈ 16.7% of the Traffic
func (s *smoothWRRAlgo) NextServer(servers []*backend.Backend) *backend.Backend {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.nodes) == 0 {
		return nil
	}

	if !sameServers(s.nodes, servers) {
		s.nodes = make([]*node, 0, len(servers))
		for _, server := range servers {
			node := &node{
				Server: server,
			}
			node.Weight = int32(server.GetMetaOrDefaultInt("weight", 1))
			node.CurrentWeight = 0
			s.nodes = append(s.nodes, node)
		}
	}
	var best *node
	var totalWeight int32

	for _, node := range s.nodes {
		node.CurrentWeight += node.Weight
		totalWeight += node.Weight

		if best == nil || node.CurrentWeight > best.CurrentWeight {
			best = node
		}
	}

	best.CurrentWeight -= totalWeight
	return best.Server
}

func sameServers(nodes []*node, servers []*backend.Backend) bool {
	if len(nodes) != len(servers) {
		return false
	}

	for i, server := range servers {
		if nodes[i].Server != server {
			return false
		}
	}

	return true
}
