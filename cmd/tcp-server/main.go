package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

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

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		srv.Shutdown()
	}()

	srv.Run()
	log.Println("tcp: server stopped")
}
