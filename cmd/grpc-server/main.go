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

	lis, err := net.Listen("tcp", ":50051")
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

	log.Println("gRPC server listening on :50051")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
