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

	"mangahub/internal/udp"
)

func main() {
	port := os.Getenv("UDP_PORT")
	if port == "" {
		port = "9091"
	}
	internalAddr := os.Getenv("UDP_INTERNAL_ADDR")
	if internalAddr == "" {
		internalAddr = ":9094"
	}

	srv := udp.New(port)

	httpSrv := &http.Server{Addr: internalAddr, Handler: srv.InternalHandler()}
	go func() {
		log.Printf("udp: internal HTTP on %s", internalAddr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("udp: internal HTTP: %v", err)
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
			log.Printf("udp: internal HTTP shutdown: %v", err)
		}
	}()

	srv.Run()
	log.Println("udp: server stopped")
}
