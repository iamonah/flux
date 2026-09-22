package l4

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/iamonah/loadbalancer/backend"
	"github.com/iamonah/loadbalancer/config"
	"github.com/iamonah/loadbalancer/strategy"
	"github.com/iamonah/loadbalancer/util"
	"github.com/iamonah/loadbalancer/util/consul"
	"github.com/iamonah/loadbalancer/util/health"
	"github.com/rs/zerolog/log"
)

type listenerKey struct {
	Protocol string
	Port     uint16
}

type fluxL4 struct {
	servicePool map[listenerKey]*backend.BackendPool
	config      *config.Config
	udpFlows    sync.Map
}

func NewfluxL4(cfg *config.Config, serviceDiscovery consul.Discovery) (*fluxL4, error) {
	svcPools := make(map[listenerKey]*backend.BackendPool)
	pools := make([]health.BackendPool, 0, len(cfg.Services))

	for _, service := range cfg.Services {
		protocol := normalizeProtocol(service.Protocol)

		if protocol != "tcp" && protocol != "udp" {
			return nil, fmt.Errorf("invalid protocol %q for service %s", service.Protocol, service.Name)
		}

		if service.Port == 0 {
			return nil, fmt.Errorf("port is required for L4 service %s", service.Name)
		}

		key := listenerKey{
			Protocol: protocol,
			Port:     service.Port,
		}

		if _, exists := svcPools[key]; exists {
			return nil, fmt.Errorf("duplicate L4 listener: %s:%d", protocol, service.Port)
		}

		strategyType := strategy.RoundRobin

		if service.StrategyType != "" {
			parsed, err := strategy.ParseStrategyType(service.StrategyType)
			if err != nil {
				return nil, fmt.Errorf("invalid strategy %q for service %s: %w", service.StrategyType, service.Name, err)
			}

			strategyType = parsed
		}

		var lbStrategy backend.Strategy

		if strategyType != strategy.Hash {
			strategyName := strategyType.String()

			var err error
			lbStrategy, err = strategy.NewStrategy(strategyName)
			if err != nil {
				return nil, fmt.Errorf("failed to create strategy for service %s: %w", service.Name, err)
			}
		}

		pool, err := backend.NewBackendPool(service, lbStrategy)
		if err != nil {
			return nil, fmt.Errorf("failed to create backend pool for service %s: %w", service.Name, err)
		}

		svcPools[key] = pool
		pools = append(pools, pool)
	}

	if len(svcPools) == 0 {
		return nil, fmt.Errorf("no L4 services configured")
	}

	discoveryPools := make([]*backend.BackendPool, 0, len(svcPools))

	for _, pool := range svcPools {
		discoveryPools = append(discoveryPools, pool)
	}

	sd := util.NewServiceDiscovery(serviceDiscovery, discoveryPools, 10*time.Second)

	//sync the initial state of the backends before the health checker.
	sd.Discover(context.Background())

	hc, err := health.NewHealthCheck(pools, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to create health checker: %w", err)
	}

	//sync the initial state of the healthy backends.
	hc.SyncAllHealthyBackends()

	go hc.Start()
	go sd.Start(context.Background())

	return &fluxL4{
		servicePool: svcPools,
		config:      cfg,
	}, nil
}

func (l *fluxL4) StartL4Proxy() error {
	listeners := make([]net.Listener, 0, len(l.servicePool))
	udpConnections := make([]*net.UDPConn, 0, len(l.servicePool))

	for key := range l.servicePool {
		switch key.Protocol {
		case "tcp":
			listener, err := net.Listen("tcp", ":"+strconv.Itoa(int(key.Port)))

			if err != nil {
				for _, existing := range listeners {
					_ = existing.Close()
				}

				return fmt.Errorf("failed to listen on TCP port %d: %w", key.Port, err)
			}

			listeners = append(listeners, listener)
			log.Info().Str("protocol", "tcp").Uint16("port", key.Port).Msg("L4 TCP listener started")
			go l.handleTCP(listener)

		case "udp":
			conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: int(key.Port)})
			if err != nil {
				for _, existing := range udpConnections {
					_ = existing.Close()
				}

				return fmt.Errorf("failed to listen on UDP port %d: %w", key.Port, err)
			}

			udpConnections = append(udpConnections, conn)
			log.Info().Str("protocol", "udp").Uint16("port", key.Port).Msg("L4 UDP listener started")
			go l.handleUDP(conn)
		}
	}

	select {}
}

func (l *fluxL4) handleTCP(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Error().Err(err).Msg("error accepting TCP connection")
			continue
		}

		tuple, err := strategy.NewFiveTupleTCP(conn)
		if err != nil {
			log.Error().Err(err).Msg("error creating TCP five-tuple")
			_ = conn.Close()
			continue
		}

		go l.handleTCPConnection(conn, tuple)
	}
}

func (l *fluxL4) handleTCPConnection(conn net.Conn, tuple *strategy.FiveTuple) {
	defer conn.Close()

	pool, found := l.findServicePool(tuple)
	if !found {
		log.Error().Uint16("port", tuple.DstPort).Msg("no backend configured for L4 TCP listener")
		return
	}

	selectedBackend := l.selectBackend(pool, tuple)

	if selectedBackend == nil {
		log.Error().Str("service", pool.ServiceName).Msg("no healthy backend available")
		return
	}

	if err := l.forwardTCPConnection(conn, selectedBackend); err != nil {
		log.Error().Err(err).Str("service", pool.ServiceName).Str("backend", selectedBackend.URL.Host).Msg("error forwarding TCP connection")
	}
}

func (l *fluxL4) findServicePool(tuple *strategy.FiveTuple) (*backend.BackendPool, bool) {
	key := listenerKey{
		Protocol: protocolName(tuple.Protocol),
		Port:     tuple.DstPort,
	}

	service, ok := l.servicePool[key]
	return service, ok
}

func (l *fluxL4) selectBackend(service *backend.BackendPool, tuple *strategy.FiveTuple) *backend.Backend {
	healthyBackends := service.GetHealthyBackends()

	if len(healthyBackends) == 0 {
		log.Error().Str("service", service.ServiceName).Msg("no healthy backends available")
		return nil
	}

	if service.StrategyType == strategy.Hash.String() {
		return tuple.NextServer(healthyBackends)
	}

	// RR / WRR are already implemented as BackendPool strategies.
	return service.GetNextBackend(healthyBackends)
}

func (l *fluxL4) forwardTCPConnection(conn net.Conn, selectedBackend *backend.Backend) error {
	backendConn, err := net.DialTimeout("tcp", selectedBackend.URL.Host, 3*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to backend %s: %w", selectedBackend.URL.Host, err)
	}

	defer backendConn.Close()

	errCh := make(chan error, 2)

	go func() {
		_, err := io.Copy(backendConn, conn)
		errCh <- err
	}()

	go func() {
		_, err := io.Copy(conn, backendConn)
		errCh <- err
	}()

	return <-errCh
}

func normalizeProtocol(protocol string) string {
	return strings.ToLower(strings.TrimSpace(protocol))
}

func protocolName(protocol strategy.Protocol) string {
	switch protocol {
	case strategy.TCP:
		return "tcp"
	case strategy.UDP:
		return "udp"
	default:
		return ""
	}
}
