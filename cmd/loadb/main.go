package main

import (
	"flag"
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

	lb, err := l7.NewLoadBalancer(cfg)
	if err != nil {
		log.Error().Msg("Failed to create load balancer: " + err.Error())
		return
	}
	server := http.Server{
		Addr:    ":" + strconv.Itoa(*port),
		Handler: lb,
	}
	if err := server.ListenAndServe(); err != nil {
		log.Error().Msg("Failed to start server: " + err.Error())
	}
}
