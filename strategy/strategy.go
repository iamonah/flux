package strategy

import (
	"fmt"
	"strings"
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

		return NewWeightedRoundRobin(lenghtofReplicas, weights), nil

	// case LeastConnections:
	// return NewLeastConnections(), nil

	default:
		return nil, fmt.Errorf("unsupported strategy: %s", strategy)
	}
}

type RoundRobinAlgo struct {
	Current          atomic.Uint32
	LengthofReplicas atomic.Uint32
}

func NewRoundRobin(length uint32) *RoundRobinAlgo {
	rr := &RoundRobinAlgo{
		Current:          atomic.Uint32{},
		LengthofReplicas: atomic.Uint32{},
	}
	rr.LengthofReplicas.Store(length)
	return rr
}

func (rr *RoundRobinAlgo) NextServer() uint32 {
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

func (rr *RoundRobinAlgo) AddBackendCount(length uint32) {
	newLength := rr.LengthofReplicas.Load() + length
	rr.LengthofReplicas.Store(newLength)
}

type WeightedRoundRobinAlgo struct {
	//index of the current backend server
	currentIndex     atomic.Uint32

	LengthofReplicas atomic.Uint32
	Weights          []uint32
	CurrentWeight    atomic.Uint32
	MaxWeight        atomic.Int32
	// GCD is the greatest common divisor of all weights
	//divides every weight without a remainder,
	GCD              uint32
}

func NewWeightedRoundRobin(length uint32, weights []uint32) *WeightedRoundRobinAlgo { return nil }
func (wrr *WeightedRoundRobinAlgo) NextServer() uint32                              { return 0 }
func (wrr *WeightedRoundRobinAlgo) AddBackendCount(length uint32)                   {}
