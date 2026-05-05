package websocket

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	gorillaws "github.com/gorilla/websocket"
	"mangahub/pkg/jwtutil"
)

var upgrader = gorillaws.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// Handler wires the ChatHub into the Gin router.
type Handler struct {
	Hub       *ChatHub
	JWTSecret string
}

// ServeWS handles GET /ws/chat?token=<jwt>&manga_id=<id>.
// It validates the JWT, upgrades the connection, and hands off to the hub.
func (h *Handler) ServeWS(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	claims, err := jwtutil.ValidateToken(token, h.JWTSecret)
	if err != nil {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	mangaID := c.Query("manga_id")
	if mangaID == "" {
		mangaID = "general"
	}

	username := claims.Username
	if username == "" {
		username = claims.UserID // fallback for tokens without username claim
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("ws: upgrade: %v", err)
		return
	}

	client := &Client{
		hub:      h.Hub,
		conn:     conn,
		send:     make(chan []byte, 256),
		userID:   claims.UserID,
		username: username,
		roomID:   mangaID,
	}
	h.Hub.register <- client

	go client.writePump()
	go client.readPump()
}
