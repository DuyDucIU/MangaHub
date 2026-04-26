package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"mangahub/internal/auth"
	"mangahub/internal/manga"
	"mangahub/pkg/database"
)

func main() {
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "mangahub-dev-secret"
	}

	db, err := database.Connect("./data/mangahub.db")
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	if err := database.SeedManga(db, "./data/manga.json"); err != nil {
		log.Printf("seed warning: %v", err)
	}

	authHandler := &auth.Handler{DB: db, JWTSecret: jwtSecret}
	mangaHandler := &manga.Handler{DB: db}

	r := gin.Default()

	r.POST("/auth/register", authHandler.Register)
	r.POST("/auth/login", authHandler.Login)

	r.GET("/manga", mangaHandler.Search)
	r.GET("/manga/:id", mangaHandler.GetByID)

	log.Println("HTTP API server running on :8080")
	if err := r.Run(":8080"); err != nil {
		log.Fatalf("server: %v", err)
	}
}
