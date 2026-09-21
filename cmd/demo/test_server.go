package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/iamonah/loadbalancer/util/consul"
)

var flagPort1 = flag.Int("port1", 8081, "payments-v1 listening port")
var flagPort2 = flag.Int("port2", 8082, "payments-v2 listening port")

func startServer(port int, name string, serviceName string, weight string) {
	ctx := context.Background()

	registry, err := consul.NewRegistry(os.Getenv("CONSUL_ADDRESS"))
	if err != nil {
		fmt.Printf("failed to create service registry: %v\n", err)
		return
	}

	instance := consul.Instance{
		ID:      consul.GenerateInstanceID(serviceName),
		SvcName: serviceName,
		Address: os.Getenv("SERVICE_ADDRESS"),
		Port:    port,
		Metadata: map[string]string{
			"weight": weight,
		},
	}

	if err := registry.Register(ctx, instance); err != nil {
		fmt.Printf("failed to register %s: %v\n", name, err)
		return
	}

	if err := registry.HealthCheck(ctx, instance.ID); err != nil {
		fmt.Printf("failed to start health check for %s: %v\n", name, err)
		return
	}

	fmt.Printf(
		"registered %s as %s at %s:%d\n",
		name,
		serviceName,
		instance.Address,
		instance.Port,
	)

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "Hello from %s!", name)
	})

	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), mux); err != nil {
		fmt.Printf("%s stopped: %v\n", name, err)
	}
}

func main() {
	flag.Parse()

	serviceAddress := os.Getenv("SERVICE_ADDRESS")

	go startServer(
		*flagPort1,
		serviceAddress+" payments-v1",
		"payments-v1",
		"1",
	)

	go startServer(
		*flagPort2,
		serviceAddress+" payments-v2",
		"payments-v2",
		"1",
	)

	select {}
}