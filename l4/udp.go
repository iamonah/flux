package l4

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/iamonah/loadbalancer/strategy"
	"github.com/rs/zerolog/log"
)

type udpFlow struct {
	conn     *net.UDPConn
	client   *net.UDPAddr
	backend  *net.UDPAddr
	lastSeen time.Time
	mu       sync.Mutex
}

func (l *fluxL4) handleUDP(conn *net.UDPConn) {
	buffer := make([]byte, 65535)

	for {
		n, clientAddr, err := conn.ReadFromUDP(buffer)

		if err != nil {
			log.Error().Err(err).Msg("error reading UDP packet")
			continue
		}

		// Reuses the buffer, so copy the packet before
		// passing it to another goroutine.
		data := make([]byte, n)
		copy(data, buffer[:n])

		tuple := strategy.NewFiveTupleUDP(conn, clientAddr)
		go l.handleUDPPacket(conn, clientAddr, data, tuple)
	}
}

func (l *fluxL4) handleUDPPacket(conn *net.UDPConn, clientAddr *net.UDPAddr, data []byte, tuple *strategy.FiveTuple) {
	pool, found := l.findServicePool(tuple)

	if !found {
		log.Error().Uint16("port", tuple.DstPort).Msg("no L4 UDP service configured")
		return
	}

	flowKey := udpFlowKey(tuple)

	// If we already have a flow for this client, keep using
	// the same backend.
	if existing, ok := l.udpFlows.Load(flowKey); ok {
		flow := existing.(*udpFlow)

		flow.mu.Lock()
		flow.lastSeen = time.Now()
		_, err := flow.conn.Write(data)
		flow.mu.Unlock()

		if err != nil {
			log.Error().Err(err).Msg("failed to forward UDP packet")
			l.removeUDPFlow(flowKey, flow)
		}

		return
	}

	selectedBackend := l.selectBackend(pool, tuple)

	if selectedBackend == nil {
		log.Error().Str("service", pool.ServiceName).Msg("no healthy UDP backend available")
		return
	}

	backendAddr, err := net.ResolveUDPAddr("udp", selectedBackend.URL.Host)

	if err != nil {
		log.Error().Err(err).Str("backend", selectedBackend.URL.Host).Msg("failed to resolve UDP backend")
		return
	}

	backendConn, err := net.DialUDP("udp", nil, backendAddr)

	if err != nil {
		log.Error().Err(err).Str("backend", backendAddr.String()).Msg("failed to connect UDP backend")
		return
	}

	flow := &udpFlow{
		conn:     backendConn,
		client:   clientAddr,
		backend:  backendAddr,
		lastSeen: time.Now(),
	}

	actual, loaded := l.udpFlows.LoadOrStore(flowKey, flow)

	if loaded {
		_ = backendConn.Close()

		existing := actual.(*udpFlow)

		existing.mu.Lock()
		existing.lastSeen = time.Now()
		_, err = existing.conn.Write(data)
		existing.mu.Unlock()

		if err != nil {
			log.Error().Err(err).Msg("failed to forward UDP packet")
			l.removeUDPFlow(flowKey, existing)
		}

		return
	}

	if _, err := backendConn.Write(data); err != nil {
		log.Error().Err(err).Msg("failed to send initial UDP packet")
		l.removeUDPFlow(flowKey, flow)
		return
	}

	go l.readUDPBackend(conn, flowKey, flow)
}

func (l *fluxL4) readUDPBackend(clientConn *net.UDPConn, flowKey string, flow *udpFlow) {
	defer func() {
		l.removeUDPFlow(flowKey, flow)
	}()

	buffer := make([]byte, 65535)

	for {
		flow.mu.Lock()

		if time.Since(flow.lastSeen) > 2*time.Minute {
			flow.mu.Unlock()
			return
		}

		_ = flow.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		flow.mu.Unlock()

		n, err := flow.conn.Read(buffer)

		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}

			log.Error().Err(err).Str("backend", flow.backend.String()).Msg("error reading UDP backend")
			return
		}

		flow.mu.Lock()
		flow.lastSeen = time.Now()
		client := flow.client
		flow.mu.Unlock()

		if _, err := clientConn.WriteToUDP(buffer[:n], client); err != nil {
			log.Error().Err(err).Str("client", client.String()).Msg("failed to send UDP response to client")
			return
		}
	}
}

func (l *fluxL4) removeUDPFlow(key string, flow *udpFlow) {
	if !l.udpFlows.CompareAndDelete(key, flow) {
		return
	}

	flow.mu.Lock()
	defer flow.mu.Unlock()

	_ = flow.conn.Close()
}

// srcIP:srcPort-dstIP:dstPort-protocol
func udpFlowKey(tuple *strategy.FiveTuple) string {
	return fmt.Sprintf("%s:%d-%s:%d-%d", tuple.SrcIP.String(), tuple.SrcPort, tuple.DstIP.String(), tuple.DstPort, tuple.Protocol)
}