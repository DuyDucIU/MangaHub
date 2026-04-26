package manga

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"mangahub/pkg/models"
)

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
