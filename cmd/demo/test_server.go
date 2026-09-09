package main

import (
	"flag"
	"fmt"
	"net/http"
)

var flagPort1 = flag.Int("port1", 9081, "listening port")
var flagPort2 = flag.Int("port2", 9082, "listening port")
var flagPort3 = flag.Int("port3", 9083, "listening port")

type DemoServer1 struct {
}

func (ds *DemoServer1) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Println(r.Method, r.URL.Path, r.RemoteAddr, r.Header.Get("X-Forwarded-For"), r.Header.Get("X-Forwarded-Host"), r.Header.Get("X-Forwarded-Proto"))
	w.Write([]byte("Hello from demo server 1!"))
}

type DemoServer2 struct {
}

func (ds *DemoServer2) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Hello from demo server 2!"))
}

type DemoServer3 struct {
}

func (ds *DemoServer3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Hello from demo server 3!"))
}
func main() {
	flag.Parse()
	ds1 := &DemoServer1{}
	ds2 := &DemoServer2{}
	ds3 := &DemoServer3{}
	go http.ListenAndServe(fmt.Sprintf(":%d", *flagPort1), ds1)
	go http.ListenAndServe(fmt.Sprintf(":%d", *flagPort3), ds3)
	err := http.ListenAndServe(
		fmt.Sprintf(":%d", *flagPort2),
		ds2,
	)

	fmt.Println("server stopped:", err)
}
