package l4

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/iamonah/loadbalancer/backend"
	"github.com/iamonah/loadbalancer/strategy"
)

func startTCPTestBackend(t *testing.T) (string, func()) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}

			go func() {
				defer conn.Close()

				_, _ = io.Copy(conn, conn)
			}()
		}
	}()

	return listener.Addr().String(), func() {
		_ = listener.Close()
	}
}

func TestForwardTCPConnection(t *testing.T) {
	backendAddr, cleanup := startTCPTestBackend(t)
	defer cleanup()

	backendURL := fmt.Sprintf("http://%s", backendAddr)

	backendURLParsed, err := url.Parse(backendURL)
	if err != nil {
		t.Fatal(err)
	}

	b := &backend.Backend{
		URL: backendURLParsed,
	}

	lb := &fluxL4{}

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	done := make(chan error, 1)

	go func() {
		done <- lb.forwardTCPConnection(server, b)
	}()

	message := []byte("hello flux")

	if _, err := client.Write(message); err != nil {
		t.Fatal(err)
	}

	buffer := make([]byte, len(message))

	if _, err := io.ReadFull(client, buffer); err != nil {
		t.Fatal(err)
	}

	if string(buffer) != string(message) {
		t.Fatalf("expected %q, got %q", message, buffer)
	}

	client.Close()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("forwardTCPConnection returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for TCP forwarding")
	}
}

func TestUDPFlowKey(t *testing.T) {
	tuple := &strategy.FiveTuple{
		SrcIP:    net.ParseIP("127.0.0.1"),
		SrcPort:  50000,
		DstIP:    net.ParseIP("127.0.0.1"),
		DstPort:  8080,
		Protocol: strategy.UDP,
	}

	got := udpFlowKey(tuple)

	expected := "127.0.0.1:50000-127.0.0.1:8080-17"

	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestNormalizeProtocol(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"tcp", "tcp"},
		{"TCP", "tcp"},
		{" Tcp ", "tcp"},
		{"udp", "udp"},
		{"UDP", "udp"},
		{" UDP ", "udp"},
	}

	for _, tt := range tests {
		got := normalizeProtocol(tt.input)

		if got != tt.expected {
			t.Fatalf("normalizeProtocol(%q): expected %q, got %q", tt.input, tt.expected, got)
		}
	}
}