package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/iamonah/loadbalancer/config"
	"github.com/iamonah/loadbalancer/l7"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	port       = flag.Int("port", 8080, "listening port")
	configFile = flag.String("config-path", "config.yaml", "path to config file")
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	flag.Parse()
	file, err := os.ReadFile(*configFile)
	if err != nil {
		log.Fatal().Msg("Failed to open config file: " + err.Error())
	}

	filedata := strings.NewReader(string(file))
	cfg, err := config.LoadConfig(filedata)
	if err != nil {
		log.Error().Msg("Failed to load config: " + err.Error())
		return
	}

	lb, err := NewFlux(cfg)
	if err != nil {
		log.Error().Msg("Failed to create load balancer: " + err.Error())
		return
	}
	server := http.Server{
		Addr:    ":" + strconv.Itoa(*port),
		Handler: lb,
	}
	log.Info().Msg(fmt.Sprintf("Starting load balancer on port %d", *port))

	if cfg.TLS.Enable && cfg.TLS.CertFile != "" && cfg.TLS.KeyFile != "" {
		log.Info().Msg("TLS Termination enabled.")
		err = server.ListenAndServeTLS(cfg.TLS.CertFile, cfg.TLS.KeyFile)
	} else {
		err = server.ListenAndServe()
	}

	if err != nil && err != http.ErrServerClosed {
		log.Fatal().Msg("Failed to start flux server: " + err.Error())
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
		return l7.Newfluxl7(cfg)
	case "l4":
		return nil, fmt.Errorf("l4 mode is not implemented yet")
	default:
		return nil, fmt.Errorf("unsupported load balancer mode: %s", *cfg.Mode)
	}
}
