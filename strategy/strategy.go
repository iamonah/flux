package strategy

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/iamonah/loadbalancer/config"
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
	NextServer() uint32
	AddBackendCount(length uint32)
}

func NewStrategy(strategy string, replicas *[]config.Replica) (Strategy, error) {
	st, err := ParseStrategyType(strategy)
	if err != nil {
		return nil, err
	}

	lenghtofReplicas := uint32(len(*replicas))
	switch st {
	case RoundRobin:
		return NewRoundRobin(lenghtofReplicas), nil

	case WeightedRoundRobin:
		weights := make([]uint32, lenghtofReplicas)
		for i, replica := range *replicas {
			if replica.Metadata.Weight != nil {
				weights[i] = *replica.Metadata.Weight
			} else {
				weights[i] = 1 // Default weight if not specified
			}
		}

		return NewSmoothWRR(replicas), nil

	// case LeastConnections:
	// return NewLeastConnections(), nil

	default:
		return nil, fmt.Errorf("unsupported strategy: %s", strategy)
	}
}

type roundRobinAlgo struct {
	Current          atomic.Uint32
	LengthofReplicas atomic.Uint32
}

func NewRoundRobin(length uint32) *roundRobinAlgo {
	rr := &roundRobinAlgo{
		Current:          atomic.Uint32{},
		LengthofReplicas: atomic.Uint32{},
	}
	rr.LengthofReplicas.Store(length)
	return rr
}

func (rr *roundRobinAlgo) NextServer() uint32 {
	length := uint32(rr.LengthofReplicas.Load())
	for {
		current := rr.Current.Load()
		next := current + 1

		if next >= length {
			next = 0
		}
		if rr.Current.CompareAndSwap(current, next) {
			return next
		}
	}
}

func (rr *roundRobinAlgo) AddBackendCount(length uint32) {
	newLength := rr.LengthofReplicas.Load() + length
	rr.LengthofReplicas.Store(newLength)
}

type node struct {
	Index         uint32       //current index in the pool
	Weight        atomic.Int32 // Fixed configured capacity weight
	CurrentWeight atomic.Int32 // Dynamically adjusted runtime weight
}

// Note: Weighted Round Robin can be implemented using Smooth WRR or Standard WRR.
// Smooth WRR provides smoother load distribution, while Standard WRR is simpler.
//
// SmoothWRR manages the load balancing pool.
type smoothWRRAlgo struct {
	mu            sync.Mutex
	nodes         []*node
	lengthofNodes atomic.Uint32
}

func NewSmoothWRR(replicas *[]config.Replica) *smoothWRRAlgo {
	s := make([]*node, 0, len(*replicas))
	for i, replica := range *replicas {
		node := &node{
			Index:         uint32(i),
			Weight:        atomic.Int32{},
			CurrentWeight: atomic.Int32{},
		}
		if replica.Metadata.Weight != nil {
			node.Weight.Store(int32(*replica.Metadata.Weight))
		} else {
			node.Weight.Store(1)
		}
		s = append(s, node)
	}

	lenghtofNodes := uint32(len(s))
	wrr := &smoothWRRAlgo{
		nodes:         s,
		lengthofNodes: atomic.Uint32{},
	}
	wrr.lengthofNodes.Store(lenghtofNodes)
	return wrr
}

func (s *smoothWRRAlgo) NextServer() uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()

	var best *node
	var totalWeight int32

	// 1. Increase the current weight of each peer by its base weight.
	for _, node := range s.nodes {
		weight := node.Weight.Load()
		currentWeight := node.CurrentWeight.Load()

		currentWeight += weight
		node.CurrentWeight.Store(currentWeight)

		totalWeight += weight

		// 2. Select the peer with the greatest current weight.
		if best == nil || currentWeight > best.CurrentWeight.Load() {
			best = node
		}
	}

	// 3. Reduce the best peer's current weight by the total weight.
	best.CurrentWeight.Store(
		best.CurrentWeight.Load() - totalWeight,
	)

	return best.Index
}

func (wrr *smoothWRRAlgo) AddBackendCount(weight uint32) {
	wrr.mu.Lock()
	defer wrr.mu.Unlock()

	// The current length is the next available index.
	index := uint32(len(wrr.nodes))

	newNode := &node{
		Index:         index,
		Weight:        atomic.Int32{},
		CurrentWeight: atomic.Int32{},
	}

	newNode.Weight.Store(int32(weight))
	newNode.CurrentWeight.Store(0)

	wrr.nodes = append(wrr.nodes, newNode)

	// Update the total number of nodes.
	wrr.lengthofNodes.Store(uint32(len(wrr.nodes)))
}
