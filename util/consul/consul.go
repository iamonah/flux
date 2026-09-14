package consul

import (
	"context"
	"time"

	consul "github.com/hashicorp/consul/api"
	"github.com/rs/zerolog/log"
)

type Registry struct {
	client *consul.Client
}

func NewRegistry(addr string) (*Registry, error) {
	cfg := consul.DefaultConfig()

	if addr != "" {
		cfg.Address = addr
	}

	client, err := consul.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	return &Registry{client: client}, nil
}

// Register registers a service instance with Consul.
func (r *Registry) Register(ctx context.Context, instance Instance) error {
	checkID := "service:" + instance.ID

	reg := &consul.AgentServiceRegistration{
		ID:      instance.ID,
		Name:    instance.SvcName,
		Address: instance.Address,
		Port:    instance.Port,
		Meta:    instance.Metadata,
		Check: &consul.AgentServiceCheck{
			CheckID:                        checkID,
			TTL:                            "10s",
			DeregisterCriticalServiceAfter: "2m",
		},
	}

	return r.client.Agent().ServiceRegister(reg)
}

// DeRegister removes the service instance from Consul.
func (r *Registry) DeRegister(ctx context.Context, instanceID string) error {
	log.Info().Msgf("deregistering service %s", instanceID)

	return r.client.Agent().ServiceDeregister(instanceID)
}

// Discover returns healthy instances for a service.
func (r *Registry) Discover(ctx context.Context, serviceName string) ([]Instance, error) {
	entries, _, err := r.client.Health().Service(serviceName, "", true, nil)
	if err != nil {
		return nil, err
	}

	instances := make([]Instance, 0, len(entries))

	for _, entry := range entries {
		addr := entry.Service.Address

		if addr == "" {
			addr = entry.Node.Address
		}

		instance := Instance{
			ID:       entry.Service.ID,
			SvcName:  entry.Service.Service,
			Address:  addr,
			Port:     entry.Service.Port,
			Metadata: entry.Service.Meta,
		}

		instances = append(instances, instance)
	}

	return instances, nil
}

// HealthCheck starts a background goroutine which updates the TTL check.
func (r *Registry) HealthCheck(ctx context.Context, instanceID string) error {
	checkID := "service:" + instanceID

	update := func() {
		if err := r.client.Agent().UpdateTTL(checkID, "passing", consul.HealthPassing); err != nil {
			log.Error().Msgf("consul: update ttl failed: %v", err)
		}
	}

	update()

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				update()

			case <-ctx.Done():
				_ = r.client.Agent().UpdateTTL(
					checkID,
					"stopped",
					consul.HealthCritical,
				)
				return
			}
		}
	}()

	return nil
}
