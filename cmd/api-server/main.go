package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"mangahub/internal/auth"
	mangagrpc "mangahub/internal/grpc"
	"mangahub/internal/manga"
	"mangahub/internal/user"
	wschat "mangahub/internal/websocket"
	"mangahub/pkg/database"
)

func main() {
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "mangahub-dev-secret"
	}

	grpcAddr := os.Getenv("GRPC_ADDR")
	if grpcAddr == "" {
		grpcAddr = "localhost:50051"
	}

	db, err := database.Connect("./data/mangahub.db")
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	if err := database.SeedManga(db, "./data/manga.json"); err != nil {
		log.Printf("seed warning: %v", err)
	}

	grpcClient, err := mangagrpc.NewClient(grpcAddr)
	if err != nil {
		log.Fatalf("grpc client: %v", err)
	}
	defer grpcClient.Close() //nolint:errcheck
	log.Printf("gRPC client connected to %s", grpcAddr)

	authHandler := &auth.Handler{DB: db, JWTSecret: jwtSecret}
	mangaHandler := &manga.Handler{DB: db, GRPCClient: grpcClient}
	userHandler := &user.Handler{DB: db, GRPCClient: grpcClient}

	hub := wschat.NewHub()
	go hub.Run()
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

	log.Println("HTTP API server running on :8080")
	if err := r.Run(":8080"); err != nil {
		log.Fatalf("server: %v", err)
	}
}
