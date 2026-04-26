package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"

	_ "modernc.org/sqlite"

	"mangahub/pkg/models"
)

func Connect(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("pragma foreign_keys: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		return nil, fmt.Errorf("pragma journal_mode: %w", err)
	}
	if err := createTables(db); err != nil {
		return nil, fmt.Errorf("create tables: %w", err)
	}
	return db, nil
}

func createTables(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id            TEXT PRIMARY KEY,
			username      TEXT UNIQUE NOT NULL,
			email         TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS manga (
			id             TEXT PRIMARY KEY,
			title          TEXT NOT NULL,
			author         TEXT NOT NULL,
			genres         TEXT NOT NULL,
			status         TEXT NOT NULL,
			total_chapters INTEGER NOT NULL,
			description    TEXT
		);

		CREATE TABLE IF NOT EXISTS user_progress (
			user_id         TEXT NOT NULL,
			manga_id        TEXT NOT NULL,
			current_chapter INTEGER NOT NULL DEFAULT 0,
			status          TEXT NOT NULL,
			updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (user_id, manga_id),
			FOREIGN KEY (user_id)  REFERENCES users(id),
			FOREIGN KEY (manga_id) REFERENCES manga(id)
		);
	`)
	return err
}

func SeedManga(db *sql.DB, dataPath string) error {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM manga").Scan(&count); err != nil {
		return fmt.Errorf("count manga: %w", err)
	}
	if count > 0 {
		return nil
	}

	data, err := os.ReadFile(dataPath)
	if err != nil {
		return fmt.Errorf("read seed file: %w", err)
	}

	var mangaList []models.Manga
	if err := json.Unmarshal(data, &mangaList); err != nil {
		return fmt.Errorf("parse seed data: %w", err)
	}

	for _, m := range mangaList {
		genres, _ := json.Marshal(m.Genres)
		_, err := db.Exec(
			`INSERT OR IGNORE INTO manga (id, title, author, genres, status, total_chapters, description)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			m.ID, m.Title, m.Author, string(genres), m.Status, m.TotalChapters, m.Description,
		)
		if err != nil {
			return fmt.Errorf("insert manga %q: %w", m.ID, err)
		}
	}
	return nil
}
