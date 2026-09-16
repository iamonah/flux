package main

import (
	"bytes"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/iamonah/loadbalancer/config"
	"github.com/iamonah/loadbalancer/l7"
	"github.com/iamonah/loadbalancer/util/consul"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	configFile = flag.String("config-path", "config.yaml", "path to config file")
)

func main() {
	flag.Parse()

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	file, err := os.ReadFile(*configFile)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to read config file")
	}

	cfg, err := config.LoadConfig(bytes.NewReader(file))
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}

	lb, err := NewFlux(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create load balancer")
	}

	server := http.Server{
		Addr:    ":" + strconv.Itoa(cfg.FluxPort),
		Handler: lb,
	}

	log.Info().Int("port", cfg.FluxPort).Msg("starting load balancer")

	if cfg.TLS.Enabled && cfg.TLS.CertFile != "" && cfg.TLS.KeyFile != "" {
		log.Info().Msg("TLS termination enabled")

		err = server.ListenAndServeTLS(cfg.TLS.CertFile, cfg.TLS.KeyFile)
	} else {
		err = server.ListenAndServe()
	}

	if err != nil && err != http.ErrServerClosed {
		log.Fatal().Err(err).Msg("failed to start Flux server")
	}
}

type flux interface {
	ServeHTTP(w http.ResponseWriter, r *http.Request)
}

func NewFlux(cfg *config.Config) (flux, error) {
	if cfg.Mode == nil {
		return nil, fmt.Errorf("load balancer mode is not specified in the config")
	}

	switch *cfg.Mode {
	case "l7":
		registry, err := consul.NewRegistry(cfg.Consul.Address)
		if err != nil {
			return nil, fmt.Errorf("failed to create service discovery: %w", err)
		}

		return l7.Newfluxl7(cfg, registry)

	case "l4":
		return nil, fmt.Errorf("l4 mode is not implemented yet")

	default:
		return nil, fmt.Errorf("unsupported load balancer mode: %s", *cfg.Mode)
	}
}
