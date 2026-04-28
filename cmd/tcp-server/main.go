package main

import (
	"log"
	"net/http"
	"os"

	"mangahub/internal/tcp"
)

func main() {
	port := os.Getenv("TCP_PORT")
	if port == "" {
		port = "9090"
	}
	internalAddr := os.Getenv("TCP_INTERNAL_ADDR")
	if internalAddr == "" {
		internalAddr = ":9099"
	}

	srv := tcp.New(port)

	go func() {
		log.Printf("tcp: internal HTTP on %s", internalAddr)
		if err := http.ListenAndServe(internalAddr, srv.InternalHandler()); err != nil {
			log.Fatalf("tcp: internal HTTP: %v", err)
		}
	}()

	srv.Run()
}
