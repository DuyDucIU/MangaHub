package user

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	DB *sql.DB
}

var tcpNotifyClient = &http.Client{Timeout: time.Second}

var validStatuses = map[string]bool{
	"reading":      true,
	"completed":    true,
	"plan_to_read": true,
	"on_hold":      true,
	"dropped":      true,
}

type addToLibraryRequest struct {
	MangaID        string `json:"manga_id"        binding:"required"`
	Status         string `json:"status"          binding:"required"`
	CurrentChapter int    `json:"current_chapter"`
}

type updateProgressRequest struct {
	MangaID        string `json:"manga_id"        binding:"required"`
	CurrentChapter int    `json:"current_chapter" binding:"min=0"`
	Status         string `json:"status"`
}

type libraryEntry struct {
	MangaID        string `json:"manga_id"`
	Title          string `json:"title"`
	CurrentChapter int    `json:"current_chapter"`
	Status         string `json:"status"`
	UpdatedAt      string `json:"updated_at"`
}

func (h *Handler) AddToLibrary(c *gin.Context) {
	userID := c.GetString("user_id")

	var req addToLibraryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	if !validStatuses[req.Status] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status, must be one of: reading, completed, plan_to_read, on_hold, dropped"})
		return
	}

	var exists int
	if err := h.DB.QueryRow("SELECT COUNT(*) FROM manga WHERE id = ?", req.MangaID).Scan(&exists); err != nil || exists == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "manga not found"})
		return
	}

	_, err := h.DB.Exec(
		`INSERT INTO user_progress (user_id, manga_id, current_chapter, status) VALUES (?, ?, ?, ?)`,
		userID, req.MangaID, req.CurrentChapter, req.Status,
	)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "manga already in library"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":         "added to library",
		"manga_id":        req.MangaID,
		"status":          req.Status,
		"current_chapter": req.CurrentChapter,
	})
}

func (h *Handler) GetLibrary(c *gin.Context) {
	userID := c.GetString("user_id")

	rows, err := h.DB.Query(`
		SELECT up.manga_id, m.title, up.current_chapter, up.status, up.updated_at
		FROM user_progress up
		JOIN manga m ON m.id = up.manga_id
		WHERE up.user_id = ?
		ORDER BY up.updated_at DESC
	`, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer rows.Close()

	lists := map[string][]libraryEntry{
		"reading":      {},
		"completed":    {},
		"plan_to_read": {},
		"on_hold":      {},
		"dropped":      {},
	}
	total := 0

	for rows.Next() {
		var e libraryEntry
		if err := rows.Scan(&e.MangaID, &e.Title, &e.CurrentChapter, &e.Status, &e.UpdatedAt); err != nil {
			continue
		}
		lists[e.Status] = append(lists[e.Status], e)
		total++
	}

	c.JSON(http.StatusOK, gin.H{"reading_lists": lists, "total": total})
}

func (h *Handler) RemoveFromLibrary(c *gin.Context) {
	userID := c.GetString("user_id")
	mangaID := c.Param("manga_id")

	result, err := h.DB.Exec(
		"DELETE FROM user_progress WHERE user_id = ? AND manga_id = ?",
		userID, mangaID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "manga not in library"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "removed from library", "manga_id": mangaID})
}

func (h *Handler) UpdateProgress(c *gin.Context) {
	userID := c.GetString("user_id")

	var req updateProgressRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	if req.Status != "" && !validStatuses[req.Status] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
		return
	}

	var totalChapters int
	if err := h.DB.QueryRow("SELECT total_chapters FROM manga WHERE id = ?", req.MangaID).Scan(&totalChapters); err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "manga not found"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	if totalChapters > 0 && req.CurrentChapter > totalChapters {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("chapter %d exceeds total chapters (%d)", req.CurrentChapter, totalChapters),
		})
		return
	}

	var currentStatus string
	if err := h.DB.QueryRow(
		"SELECT status FROM user_progress WHERE user_id = ? AND manga_id = ?",
		userID, req.MangaID,
	).Scan(&currentStatus); err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "manga not in library, add it first"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	newStatus := currentStatus
	if req.Status != "" {
		newStatus = req.Status
	}

	if _, err := h.DB.Exec(
		`UPDATE user_progress SET current_chapter = ?, status = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE user_id = ? AND manga_id = ?`,
		req.CurrentChapter, newStatus, userID, req.MangaID,
	); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	go notifyTCPServer(userID, req.MangaID, req.CurrentChapter) // fire-and-forget TCP sync

	c.JSON(http.StatusOK, gin.H{
		"message":         "progress updated",
		"manga_id":        req.MangaID,
		"current_chapter": req.CurrentChapter,
		"status":          newStatus,
	})
}

// notifyTCPServer fires a goroutine that POSTs the progress update to the TCP server's
// internal broadcast endpoint. Fire-and-forget: HTTP API returns 200 regardless of
// TCP server availability (UC-006 A2 — progress is already saved to DB).
func notifyTCPServer(userID, mangaID string, chapter int) {
	addr := os.Getenv("TCP_INTERNAL_URL")
	if addr == "" {
		addr = "http://localhost:9099"
	}
	payload, _ := json.Marshal(struct {
		UserID    string `json:"user_id"`
		MangaID   string `json:"manga_id"`
		Chapter   int    `json:"chapter"`
		Timestamp int64  `json:"timestamp"`
	}{
		UserID:    userID,
		MangaID:   mangaID,
		Chapter:   chapter,
		Timestamp: time.Now().Unix(),
	})
	resp, err := tcpNotifyClient.Post(addr+"/internal/broadcast", "application/json", bytes.NewReader(payload))
	if err != nil {
		log.Printf("user: TCP notify failed: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("user: TCP notify: unexpected status %d", resp.StatusCode)
	}
}
