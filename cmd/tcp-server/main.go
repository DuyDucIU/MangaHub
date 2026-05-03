package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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

	httpSrv := &http.Server{Addr: internalAddr, Handler: srv.InternalHandler()}
	go func() {
		log.Printf("tcp: internal HTTP on %s", internalAddr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("tcp: internal HTTP: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		srv.Shutdown()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(ctx); err != nil {
			log.Printf("tcp: internal HTTP shutdown: %v", err)
		}
	}()

	srv.Run()
	log.Println("tcp: server stopped")
}
