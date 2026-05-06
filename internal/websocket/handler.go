package websocket

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	gorillaws "github.com/gorilla/websocket"
)

var upgrader = gorillaws.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// Handler wires the ChatHub into the Gin router.
type Handler struct {
	Hub         *ChatHub
	JWTSecret   string
	AuthTimeout time.Duration // 0 → defaults to 10s in readPump
}

// ServeWS handles GET /ws/chat?manga_id=<id>.
// The connection is upgraded immediately; the client must send
// {"token":"<jwt>"} as its first message to authenticate.
func (h *Handler) ServeWS(c *gin.Context) {
	mangaID := c.Query("manga_id")
	if mangaID == "" {
		mangaID = "general"
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("ws: upgrade: %v", err)
		return
	}

	client := &Client{
		hub:         h.Hub,
		conn:        conn,
		send:        make(chan []byte, 256),
		roomID:      mangaID,
		jwtSecret:   h.JWTSecret,
		authTimeout: h.AuthTimeout,
	}

	go client.writePump()
	go client.readPump()
}
