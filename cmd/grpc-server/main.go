package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	grpclib "google.golang.org/grpc"
	mangagrpc "mangahub/internal/grpc"
	"mangahub/pkg/database"
	pb "mangahub/proto/manga"
)

func main() {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./data/mangahub.db"
	}

	db, err := database.Connect(dbPath)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer db.Close()

	grpcPort := os.Getenv("GRPC_PORT")
	if grpcPort == "" {
		grpcPort = "50051"
	}
	lis, err := net.Listen("tcp", ":"+grpcPort)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	s := grpclib.NewServer()
	pb.RegisterMangaServiceServer(s, &mangagrpc.Service{DB: db})

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		log.Println("gRPC server shutting down...")
		s.GracefulStop()
	}()

	log.Printf("gRPC server listening on :%s", grpcPort)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
