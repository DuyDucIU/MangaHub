package udp

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
)

// AdminNotify is a Gin handler for POST /admin/notify.
// Requires manga_id and message in the JSON body; pushes to the Notify channel non-blocking.
func (s *NotificationServer) AdminNotify(c *gin.Context) {
	var req NotifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.MangaID == "" || req.Message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "manga_id and message are required"})
		return
	}
	select {
	case s.Notify <- req:
		c.JSON(http.StatusOK, gin.H{"message": "notification queued", "manga_id": req.MangaID})
	default:
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "notification queue full"})
	}
}

// InternalHandler returns an HTTP handler for the internal notify endpoint.
// Only POST /internal/notify is accepted.
func (s *NotificationServer) InternalHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/notify", s.handleNotify)
	return mux
}

// handleNotify handles POST /internal/notify. It rejects non-POST requests,
// decodes the JSON body into a NotifyRequest, and forwards it to the Notify
// channel non-blocking. Returns 503 if the server is shutting down or the
// channel is at capacity.
func (s *NotificationServer) handleNotify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req NotifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	select {
	case <-s.done:
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	default:
		select {
		case s.Notify <- req:
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "notify queue full", http.StatusServiceUnavailable)
		}
	}
}
