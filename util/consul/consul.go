package consul

import (
	"context"
	"uuid"
)

type Instance struct {
	ID       string
	SvcName  string
	Address  string
	Port     int
	Metadata map[string]string
}
type Discovery interface {
	Discover(ctx context.Context, svcName string) ([]Instance, error)
}

type Registrar interface {
	Register(ctx context.Context, instance Instance) error
	DeRegister(ctx context.Context, instanceID string) error
	HealthCheck(ctx context.Context, instanceID string) error
}

func GenerateInstanceID(svcName string) string {
	return uuid.NewV7().String() + "-" + svcName
}
