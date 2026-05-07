package manga

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"mangahub/pkg/models"
)

// MangaGRPCClient is satisfied by *grpc.Client from internal/grpc.
type MangaGRPCClient interface {
	GetManga(ctx context.Context, id string) (*models.Manga, error)
	SearchManga(ctx context.Context, q, genre, statusFilter string, page, pageSize int) ([]models.Manga, int, error)
}

// AllowedGenres defines the set of accepted genre strings.
var AllowedGenres = map[string]bool{
	"Action": true, "Adventure": true, "Comedy": true, "Drama": true,
	"Fantasy": true, "Horror": true, "Mystery": true, "Psychological": true,
	"Romance": true, "Sci-Fi": true, "Slice of Life": true, "Sports": true,
	"Supernatural": true, "Thriller": true, "Historical": true, "Music": true,
	"School": true, "Magic": true, "Fashion": true,
	"Shounen": true, "Shoujo": true, "Seinen": true, "Josei": true,
}

// AllowedStatuses defines the accepted manga publication statuses.
var AllowedStatuses = map[string]bool{
	"ongoing": true, "completed": true, "hiatus": true,
}

type Handler struct {
	DB         *sql.DB         // used by Create
	GRPCClient MangaGRPCClient // used by GetByID, Search
}

func (h *Handler) Search(c *gin.Context) {
	q := c.Query("q")
	genre := c.Query("genre")
	status := c.Query("status")

	page := 1
	pageSize := 20
	if p, err := strconv.Atoi(c.DefaultQuery("page", "1")); err == nil && p > 0 {
		page = p
	}
	if ps, err := strconv.Atoi(c.DefaultQuery("page_size", "20")); err == nil && ps > 0 {
		pageSize = ps
	}

	results, total, err := h.GRPCClient.SearchManga(c.Request.Context(), q, genre, status, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "search failed"})
		return
	}
	if results == nil {
		results = []models.Manga{}
	}
	c.JSON(http.StatusOK, gin.H{
		"results":   results,
		"count":     len(results),
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (h *Handler) GetByID(c *gin.Context) {
	id := c.Param("id")
	m, err := h.GRPCClient.GetManga(c.Request.Context(), id)
	if errors.Is(err, models.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "manga not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get manga"})
		return
	}
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

	if !AllowedStatuses[strings.ToLower(req.Status)] {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid status; must be one of: ongoing, completed, hiatus",
		})
		return
	}
	req.Status = strings.ToLower(req.Status)

	for _, g := range req.Genres {
		if !AllowedGenres[g] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown genre: " + g})
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

	c.JSON(http.StatusCreated, gin.H{"message": "manga created", "id": req.ID})
}
