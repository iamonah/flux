package strategy

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/iamonah/loadbalancer/backend"
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
	AddBackendCount(server *backend.Backend)
}

func NewStrategy(strategy *string, replicas []*backend.Backend) (Strategy, error) {
	if strategy == nil {
		defaultStrategy := "round-robin"
		strategy = &defaultStrategy
	}

	st, err := ParseStrategyType(*strategy)
	if err != nil {
		return nil, err
	}

	lengthOfReplicas := uint32(len(replicas))

	switch st {
	case RoundRobin:
		return NewRoundRobin(lengthOfReplicas), nil

	case WeightedRoundRobin:
		return NewSmoothWRR(replicas), nil

	// case LeastConnections:
	// 	return NewLeastConnections(), nil

	default:
		return nil, fmt.Errorf("unsupported strategy: %s", *strategy)
	}
}

type roundRobinAlgo struct {
	Current          atomic.Uint32
	LengthofReplicas atomic.Uint32
}

func NewRoundRobin(length uint32) *roundRobinAlgo {
	rr := &roundRobinAlgo{}

	rr.LengthofReplicas.Store(length)

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

func (rr *roundRobinAlgo) AddBackendCount(server *backend.Backend) {
	newLength := rr.LengthofReplicas.Load() + 1
	rr.LengthofReplicas.Store(newLength)
}

type node struct {
	Server        *backend.Backend
	Weight        atomic.Int32 // Fixed configured capacity weight
	CurrentWeight atomic.Int32 // Dynamically adjusted runtime weight
}

// Smooth WRR provides smoother load distribution than standard WRR.
type smoothWRRAlgo struct {
	mu            sync.Mutex
	nodes         []*node
	lengthofNodes atomic.Uint32
}

func NewSmoothWRR(replicas []*backend.Backend) *smoothWRRAlgo {
	nodes := make([]*node, 0, len(replicas))

	for _, replica := range replicas {
		node := &node{
			Server: replica,
		}

		node.Weight.Store(int32(replica.GetMetaOrDefaultInt("weight", 1)))
		nodes = append(nodes, node)
	}

	wrr := &smoothWRRAlgo{
		nodes: nodes,
	}
	wrr.lengthofNodes.Store(uint32(len(nodes)))
	return wrr
}

func (s *smoothWRRAlgo) NextServer(servers []*backend.Backend) *backend.Backend {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.nodes) == 0 {
		return nil
	}

	var best *node
	var totalWeight int32

	// Increase the current weight of each peer by its base weight.
	for _, node := range s.nodes {
		weight := node.Weight.Load()

		currentWeight := node.CurrentWeight.Load()
		currentWeight += weight
		node.CurrentWeight.Store(currentWeight)

		totalWeight += weight

		// Select the peer with the greatest current weight.
		if best == nil || currentWeight > best.CurrentWeight.Load() {
			best = node
		}
	}

	// Reduce the best peer's current weight by the total weight.
	best.CurrentWeight.Store(
		best.CurrentWeight.Load() - totalWeight,
	)

	return best.Server
}

func (wrr *smoothWRRAlgo) AddBackendCount(server *backend.Backend) {
	wrr.mu.Lock()
	defer wrr.mu.Unlock()
	newNode := &node{}
	newNode.Server = server
	newNode.Weight.Store(int32(server.GetMetaOrDefaultInt("weight", 1)))
	wrr.nodes = append(wrr.nodes, newNode)
	wrr.lengthofNodes.Store(uint32(len(wrr.nodes)))
}
