package main

import (
	"flag"
	"fmt"
	"net/http"
)


// var flagPort1 = flag.Int("port1", 8081, "listening port")
var flagPort2 = flag.Int("port2", 8082, "listening port")
var flagPort3 = flag.Int("port3", 8083, "listening port")
var flagPort4 = flag.Int("port4", 9081, "listening port")
var flagPort5 = flag.Int("port5", 9082, "listening port")
var flagPort6 = flag.Int("port6", 9083, "listening port")

func startServer(port int, name string) {
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

	// go startServer(*flagPort1, "demo server 1")
	go startServer(*flagPort2, "demo server 2")
	go startServer(*flagPort3, "demo server 3")
	go startServer(*flagPort4, "demo server 4")
	go startServer(*flagPort5, "demo server 5")
	go startServer(*flagPort6, "demo server 6")

	select {}
}