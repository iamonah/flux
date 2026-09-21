package main

import (
	"bytes"
	"flag"
	"net/http"
	"os"
	"strings"

	"github.com/iamonah/loadbalancer/config"
	"github.com/iamonah/loadbalancer/l4"
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

	if cfg.Mode == nil {
		log.Fatal().Msg("load balancer mode is not specified in config")
	}

	registry, err := consul.NewRegistry(cfg.Consul.Address)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create service discovery")
	}

	switch strings.ToLower(*cfg.Mode) {
	case "l7":
		startL7(cfg, registry)

	case "l4":
		startL4(cfg, registry)

	default:
		log.Fatal().Str("mode", *cfg.Mode).Msg("unsupported load balancer mode")
	}
}

func startL7(cfg *config.Config, registry consul.Discovery) {
	lb, err := l7.Newfluxl7(cfg, registry)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create L7 load balancer")
	}

	server := http.Server{
		Addr:    ":" + cfg.FluxPort,
		Handler: lb,
	}

	log.Info().Str("mode", "l7").Str("port", cfg.FluxPort).Msg("starting Flux")

	if cfg.TLS.Enabled && cfg.TLS.CertFile != "" && cfg.TLS.KeyFile != "" {
		log.Info().Msg("TLS termination enabled")
		err = server.ListenAndServeTLS(cfg.TLS.CertFile, cfg.TLS.KeyFile)
	} else {
		err = server.ListenAndServe()
	}

	if err != nil && err != http.ErrServerClosed {
		log.Fatal().Err(err).Msg("failed to start L7 server")
	}
}

func startL4(cfg *config.Config, registry consul.Discovery) {
	lb, err := l4.NewfluxL4(cfg, registry)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create L4 load balancer")
	}

	log.Info().Str("mode", "l4").Msg("starting Flux")

	if err := lb.StartL4Proxy(); err != nil {
		log.Fatal().Err(err).Msg("failed to start L4 proxy")
	}
}