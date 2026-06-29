package main

import (
	"net"
	"os"
	"time"
)

// healthcheck verifies the MITM proxy is accepting connections on its port.
func main() {
	port := os.Getenv("PROXY_PORT")
	if port == "" {
		port = "8080"
	}
	conn, err := net.DialTimeout("tcp", "localhost:"+port, 2*time.Second)
	if err != nil {
		os.Exit(1)
	}
	conn.Close()
}
