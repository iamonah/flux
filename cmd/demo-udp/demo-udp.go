package main

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/iamonah/loadbalancer/util/consul"
)

const healthProbe = "FLUX_HEALTH_CHECK"

func main() {
	ctx := context.Background()

	serviceAddress := os.Getenv("SERVICE_ADDRESS")
	consulAddress := os.Getenv("CONSUL_ADDRESS")

	registry, err := consul.NewRegistry(consulAddress)
	if err != nil {
		fmt.Printf("failed to create registry: %v\n", err)
		return
	}

	instance := consul.Instance{
		ID:      consul.GenerateInstanceID("dns"),
		SvcName: "dns",
		Address: serviceAddress,
		Port:    5353,
		Metadata: map[string]string{
			"weight": "1",
		},
	}

	if err := registry.Register(ctx, instance); err != nil {
		fmt.Printf("failed to register UDP server: %v\n", err)
		return
	}

	if err := registry.HealthCheck(ctx, instance.ID); err != nil {
		fmt.Printf("failed to start health check: %v\n", err)
		return
	}

	fmt.Printf(
		"registered UDP server %s as %s at %s:%d\n",
		serviceAddress,
		instance.SvcName,
		serviceAddress,
		instance.Port,
	)

	conn, err := net.ListenUDP("udp", &net.UDPAddr{
		IP:   net.IPv4zero,
		Port: 5353,
	})
	if err != nil {
		fmt.Printf("failed to start UDP server: %v\n", err)
		return
	}
	defer conn.Close()

	fmt.Printf("UDP server listening on %s:5353\n", serviceAddress)

	buffer := make([]byte, 65535)

	for {
		n, clientAddr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			fmt.Printf("failed to read UDP packet: %v\n", err)
			continue
		}

		message := string(buffer[:n])

		if message == healthProbe {
			_, err := conn.WriteToUDP(
				[]byte("OK"),
				clientAddr,
			)
			if err != nil {
				fmt.Printf("failed to respond to health probe: %v\n", err)
			}
			continue
		}

		fmt.Printf(
			"received from %s: %s\n",
			clientAddr.String(),
			message,
		)

		response := fmt.Sprintf(
			"Hello from %s UDP server!",
			serviceAddress,
		)

		if _, err := conn.WriteToUDP(
			[]byte(response),
			clientAddr,
		); err != nil {
			fmt.Printf("failed to write UDP response: %v\n", err)
		}
	}
}