package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	mangagrpc "mangahub/internal/grpc"
)

func main() {
	addr    := flag.String("addr", "localhost:50051", "gRPC server address")
	userID  := flag.String("user", "", "user ID for UpdateProgress demo (optional)")
	mangaID := flag.String("manga", "one-piece", "manga ID for UpdateProgress demo")
	chapter := flag.Int("chapter", 100, "chapter number for UpdateProgress demo")
	status  := flag.String("status", "reading", "reading status for UpdateProgress demo")
	flag.Parse()

	client, err := mangagrpc.NewClient(*addr)
	if err != nil {
		log.Fatalf("connect to %s: %v", *addr, err)
	}
	defer client.Close() //nolint:errcheck

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// UC-014: GetManga
	fmt.Println("=== UC-014: GetManga (id=one-piece) ===")
	m, err := client.GetManga(ctx, "one-piece")
	if err != nil {
		log.Printf("GetManga error: %v", err)
	} else {
		fmt.Printf("ID:          %s\nTitle:       %s\nAuthor:      %s\nGenres:      %v\nStatus:      %s\nChapters:    %d\nDescription: %s\n\n",
			m.ID, m.Title, m.Author, m.Genres, m.Status, m.TotalChapters, m.Description)
	}

	// UC-015: SearchManga
	fmt.Println("=== UC-015: SearchManga (q=one, page=1, page_size=5) ===")
	results, total, err := client.SearchManga(ctx, "one", "", "", 1, 5)
	if err != nil {
		log.Printf("SearchManga error: %v", err)
	} else {
		fmt.Printf("Total matching: %d\n", total)
		for _, r := range results {
			fmt.Printf("  - [%s] %s by %s\n", r.ID, r.Title, r.Author)
		}
		fmt.Println()
	}

	// UC-016: UpdateProgress (only if --user provided)
	fmt.Println("=== UC-016: UpdateProgress ===")
	if *userID == "" {
		fmt.Println("Skipped — provide --user <user_id> to demo UpdateProgress")
		return
	}
	up, err := client.UpdateProgress(ctx, *userID, *mangaID, int32(*chapter), *status)
	if err != nil {
		log.Printf("UpdateProgress error: %v", err)
	} else {
		fmt.Printf("Updated: manga=%s chapter=%d status=%s\n", up.MangaID, up.CurrentChapter, up.Status)
	}
}
