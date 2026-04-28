// importer fetches popular manga from the MangaDx API and stores them in the
// local database. Run once to seed extra titles beyond the static manga.json.
//
// Usage:
//
//	go run ./cmd/importer --db ./data/mangahub.db --limit 20
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"mangahub/internal/mangadex"
	"mangahub/pkg/database"
	"mangahub/pkg/models"
)

func main() {
	dbPath := flag.String("db", "./data/mangahub.db", "Path to SQLite database")
	limit := flag.Int("limit", 20, "Number of manga to fetch from MangaDx")
	dryRun := flag.Bool("dry-run", false, "Print fetched manga without writing to DB")
	flag.Parse()

	client := mangadex.NewClient()

	log.Printf("Fetching %d manga from MangaDx API...", *limit)
	mangas, err := client.SearchPopular(*limit)
	if err != nil {
		log.Fatalf("fetch failed: %v", err)
	}
	log.Printf("Received %d results from MangaDx", len(mangas))

	if *dryRun {
		printMangas(mangas)
		return
	}

	db, err := database.Connect(*dbPath)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	inserted, skipped := importMangas(db, mangas)
	log.Printf("Done — inserted: %d, skipped (already exist): %d", inserted, skipped)
}

func importMangas(db *sql.DB, mangas []models.Manga) (inserted, skipped int) {
	for _, m := range mangas {
		if m.Title == "" || m.ID == "" {
			skipped++
			continue
		}
		genres, _ := json.Marshal(m.Genres)
		result, err := db.Exec(
			`INSERT OR IGNORE INTO manga (id, title, author, genres, status, total_chapters, description)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			m.ID, m.Title, m.Author, string(genres), m.Status, m.TotalChapters, m.Description,
		)
		if err != nil {
			log.Printf("skipping %q: %v", m.Title, err)
			skipped++
			continue
		}
		rows, _ := result.RowsAffected()
		if rows > 0 {
			inserted++
			log.Printf("  + %s (%s)", m.Title, m.ID)
		} else {
			skipped++
		}
	}
	return
}

func printMangas(mangas []models.Manga) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	for _, m := range mangas {
		fmt.Printf("---\nID: %s\nTitle: %s\nAuthor: %s\nGenres: %v\nStatus: %s\nChapters: %d\n",
			m.ID, m.Title, m.Author, m.Genres, m.Status, m.TotalChapters)
		if m.Description != "" {
			maxLen := 80
			desc := m.Description
			if len(desc) > maxLen {
				desc = desc[:maxLen] + "..."
			}
			fmt.Printf("Description: %s\n", desc)
		}
	}
	_ = enc
}
