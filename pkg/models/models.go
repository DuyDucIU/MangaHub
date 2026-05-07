package models

import (
	"errors"
	"time"
)

var (
	ErrNotFound        = errors.New("not found")
	ErrInvalidArgument = errors.New("invalid argument")
)

type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

type Manga struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Author        string   `json:"author"`
	Genres        []string `json:"genres"`
	Status        string   `json:"status"`
	TotalChapters int      `json:"total_chapters"`
	Description   string   `json:"description"`
}

type UserProgress struct {
	UserID         string    `json:"user_id"`
	MangaID        string    `json:"manga_id"`
	CurrentChapter int       `json:"current_chapter"`
	Status         string    `json:"status"`
	UpdatedAt      time.Time `json:"updated_at"`
}
