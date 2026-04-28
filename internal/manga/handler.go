package manga

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"mangahub/pkg/models"
)

// AllowedGenres defines the set of accepted genre strings.
var AllowedGenres = map[string]bool{
	"Action": true, "Adventure": true, "Comedy": true, "Drama": true,
	"Fantasy": true, "Horror": true, "Mystery": true, "Psychological": true,
	"Romance": true, "Sci-Fi": true, "Slice of Life": true, "Sports": true,
	"Supernatural": true, "Thriller": true, "Historical": true, "Music": true,
	"School": true, "Magic": true, "Fashion": true,
	// Demographic tags — allowed as genre labels in our data model.
	"Shounen": true, "Shoujo": true, "Seinen": true, "Josei": true,
}

// AllowedStatuses defines the accepted manga publication statuses.
var AllowedStatuses = map[string]bool{
	"ongoing": true, "completed": true, "hiatus": true,
}

type Handler struct {
	DB *sql.DB
}

func (h *Handler) Search(c *gin.Context) {
	q := c.Query("q")
	genre := c.Query("genre")
	status := c.Query("status")

	query := "SELECT id, title, author, genres, status, total_chapters, description FROM manga WHERE 1=1"
	args := []interface{}{}

	if q != "" {
		query += " AND (LOWER(title) LIKE LOWER(?) OR LOWER(author) LIKE LOWER(?))"
		like := "%" + q + "%"
		args = append(args, like, like)
	}
	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}

	rows, err := h.DB.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer rows.Close()

	var results []models.Manga
	for rows.Next() {
		var m models.Manga
		var genresStr string
		if err := rows.Scan(&m.ID, &m.Title, &m.Author, &genresStr, &m.Status, &m.TotalChapters, &m.Description); err != nil {
			continue
		}
		json.Unmarshal([]byte(genresStr), &m.Genres)

		if genre != "" {
			match := false
			for _, g := range m.Genres {
				if strings.EqualFold(g, genre) {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		results = append(results, m)
	}

	if results == nil {
		results = []models.Manga{}
	}
	c.JSON(http.StatusOK, gin.H{"results": results, "count": len(results)})
}

func (h *Handler) GetByID(c *gin.Context) {
	id := c.Param("id")
	var m models.Manga
	var genresStr string

	err := h.DB.QueryRow(
		"SELECT id, title, author, genres, status, total_chapters, description FROM manga WHERE id = ?",
		id,
	).Scan(&m.ID, &m.Title, &m.Author, &genresStr, &m.Status, &m.TotalChapters, &m.Description)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "manga not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	json.Unmarshal([]byte(genresStr), &m.Genres)
	c.JSON(http.StatusOK, m)
}

type createMangaRequest struct {
	ID            string   `json:"id"             binding:"required"`
	Title         string   `json:"title"          binding:"required"`
	Author        string   `json:"author"         binding:"required"`
	Genres        []string `json:"genres"         binding:"required,min=1"`
	Status        string   `json:"status"         binding:"required"`
	TotalChapters int      `json:"total_chapters" binding:"min=0"`
	Description   string   `json:"description"`
}

func (h *Handler) Create(c *gin.Context) {
	var req createMangaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	// Validate status.
	if !AllowedStatuses[strings.ToLower(req.Status)] {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid status; must be one of: ongoing, completed, hiatus",
		})
		return
	}
	req.Status = strings.ToLower(req.Status)

	// Validate each genre.
	for _, g := range req.Genres {
		if !AllowedGenres[g] {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "unknown genre: " + g,
			})
			return
		}
	}

	genres, _ := json.Marshal(req.Genres)
	_, err := h.DB.Exec(
		`INSERT INTO manga (id, title, author, genres, status, total_chapters, description)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		req.ID, req.Title, req.Author, string(genres), req.Status, req.TotalChapters, req.Description,
	)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "manga with this ID already exists"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "manga created",
		"id":      req.ID,
	})
}
