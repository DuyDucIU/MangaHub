package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	grpclib "google.golang.org/grpc"
	"mangahub/internal/auth"
	mangagrpc "mangahub/internal/grpc"
	"mangahub/internal/manga"
	"mangahub/internal/tcp"
	"mangahub/internal/udp"
	"mangahub/internal/user"
	wschat "mangahub/internal/websocket"
	"mangahub/pkg/database"
	pb "mangahub/proto/manga"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	jwtSecret := getenv("JWT_SECRET", "mangahub-dev-secret")
	dbPath := getenv("DB_PATH", "./data/mangahub.db")
	httpPort := getenv("HTTP_PORT", "8080")
	grpcAddr := getenv("GRPC_ADDR", "localhost:50051")
	grpcPort := getenv("GRPC_PORT", "50051")
	tcpPort := getenv("TCP_PORT", "9090")
	tcpInternal := getenv("TCP_INTERNAL_ADDR", ":9099")
	udpPort := getenv("UDP_PORT", "9091")
	udpInternal := getenv("UDP_INTERNAL_ADDR", ":9094")

	db, err := database.Connect(dbPath)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer db.Close()

	if err := database.SeedManga(db, "./data/manga.json"); err != nil {
		log.Printf("seed warning: %v", err)
	}

	// --- TCP server ---
	tcpSrv := tcp.New(tcpPort)
	tcpHTTP := &http.Server{Addr: tcpInternal, Handler: tcpSrv.InternalHandler()}
	go func() {
		log.Printf("[TCP ] internal HTTP on %s", tcpInternal)
		if err := tcpHTTP.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[TCP ] internal HTTP error: %v", err)
		}
	}()
	go func() {
		log.Printf("[TCP ] listening on :%s", tcpPort)
		tcpSrv.Run()
	}()

	// --- gRPC server ---
	grpcLis, err := net.Listen("tcp", ":"+grpcPort)
	if err != nil {
		log.Fatalf("[gRPC] listen: %v", err)
	}
	grpcSrv := grpclib.NewServer()
	pb.RegisterMangaServiceServer(grpcSrv, &mangagrpc.Service{DB: db, TCPBroadcast: tcpSrv.Broadcast})
	go func() {
		log.Printf("[gRPC] listening on :%s", grpcPort)
		if err := grpcSrv.Serve(grpcLis); err != nil {
			log.Printf("[gRPC] stopped: %v", err)
		}
	}()

	// --- UDP server ---
	udpSrv := udp.New(udpPort)
	udpHTTP := &http.Server{Addr: udpInternal, Handler: udpSrv.InternalHandler()}
	go func() {
		log.Printf("[UDP ] internal HTTP on %s", udpInternal)
		if err := udpHTTP.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[UDP ] internal HTTP error: %v", err)
		}
	}()
	go func() {
		log.Printf("[UDP ] listening on :%s", udpPort)
		udpSrv.Run()
	}()

	// --- HTTP API + WebSocket ---
	grpcClient, err := mangagrpc.NewClient(grpcAddr)
	if err != nil {
		log.Fatalf("[HTTP] grpc client: %v", err)
	}
	defer grpcClient.Close() //nolint:errcheck

	hub := wschat.NewHub()
	go hub.Run()

	authHandler := &auth.Handler{DB: db, JWTSecret: jwtSecret}
	mangaHandler := &manga.Handler{DB: db, GRPCClient: grpcClient}
	userHandler := &user.Handler{DB: db, GRPCClient: grpcClient}
	wsHandler := &wschat.Handler{Hub: hub, JWTSecret: jwtSecret}

	r := gin.Default()
	r.POST("/auth/register", authHandler.Register)
	r.POST("/auth/login", authHandler.Login)
	r.GET("/manga", mangaHandler.Search)
	r.GET("/manga/:id", mangaHandler.GetByID)
	r.GET("/ws/chat", wsHandler.ServeWS)

	protected := r.Group("/")
	protected.Use(authHandler.JWTMiddleware())
	protected.POST("/manga", mangaHandler.Create)
	protected.POST("/users/library", userHandler.AddToLibrary)
	protected.GET("/users/library", userHandler.GetLibrary)
	protected.DELETE("/users/library/:manga_id", userHandler.RemoveFromLibrary)
	protected.PUT("/users/progress", userHandler.UpdateProgress)

	httpSrv := &http.Server{Addr: ":" + httpPort, Handler: r}
	go func() {
		log.Printf("[HTTP] API + WebSocket listening on :%s", httpPort)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[HTTP] stopped: %v", err)
		}
	}()

	// --- wait for shutdown signal ---
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("[runner] shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	for _, srv := range []*http.Server{httpSrv, tcpHTTP, udpHTTP} {
		wg.Add(1)
		go func(s *http.Server) { defer wg.Done(); s.Shutdown(ctx) }(srv) //nolint:errcheck
	}
	grpcDone := make(chan struct{})
	go func() { grpcSrv.GracefulStop(); close(grpcDone) }()
	select {
	case <-grpcDone:
	case <-ctx.Done():
		grpcSrv.Stop()
	}
	tcpSrv.Shutdown()
	udpSrv.Shutdown()
	wg.Wait()

	log.Println("[runner] stopped")
}
