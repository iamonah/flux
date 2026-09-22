package strategy

import (
	"encoding/binary"
	"hash/crc32"
	"net"
	"strconv"

	"github.com/iamonah/loadbalancer/backend"
)

// Used only by the L4 proxy. It uses the network connection's five-tuple
// as the input for hashing and selecting a backend server.
//
// FiveTuple represents a network connection's five-tuple: source IP address,
// source port, destination IP address, destination port, and protocol (TCP or UDP).
type Protocol uint8

const (
	TCP Protocol = 6
	UDP Protocol = 17
)

type FiveTuple struct {
	SrcIP    net.IP
	SrcPort  uint16
	DstIP    net.IP
	DstPort  uint16
	Protocol Protocol
}

func NewFiveTupleTCP(conn net.Conn) (*FiveTuple, error) {
	srcHost, srcPortStr, err := net.SplitHostPort(conn.RemoteAddr().String())
	if err != nil {
		return nil, err
	}

	srcPort, err := strconv.ParseUint(srcPortStr, 10, 16)
	if err != nil {
		return nil, err
	}

	dstHost, dstPortStr, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		return nil, err
	}

	dstPort, err := strconv.ParseUint(dstPortStr, 10, 16)
	if err != nil {
		return nil, err
	}

	return &FiveTuple{
		SrcIP:    net.ParseIP(srcHost),
		SrcPort:  uint16(srcPort),
		DstIP:    net.ParseIP(dstHost),
		DstPort:  uint16(dstPort),
		Protocol: TCP,
	}, nil
}

func NewFiveTupleUDP(conn *net.UDPConn, clientAddr *net.UDPAddr) *FiveTuple {
	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return &FiveTuple{
		SrcIP:    clientAddr.IP,
		SrcPort:  uint16(clientAddr.Port),
		DstIP:    localAddr.IP,
		DstPort:  uint16(localAddr.Port),
		Protocol: UDP,
	}
}

func (t FiveTuple) Hash() uint32 {
	h := crc32.NewIEEE()

	if t.SrcIP != nil {
		h.Write(t.SrcIP)
	}

	if t.DstIP != nil {
		h.Write(t.DstIP)
	}

	var buf [5]byte

	binary.BigEndian.PutUint16(buf[0:2], t.SrcPort)
	binary.BigEndian.PutUint16(buf[2:4], t.DstPort)
	buf[4] = byte(t.Protocol)

	h.Write(buf[:])

	return h.Sum32()
}

func (t FiveTuple) NextServer(servers []*backend.Backend) *backend.Backend {
	if len(servers) == 0 {
		return nil
	}

	hashValue := t.Hash()
	index := hashValue % uint32(len(servers))

	return servers[index]
}
