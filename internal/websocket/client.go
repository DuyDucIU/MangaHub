package websocket

import (
	"encoding/json"
	"log"
	"time"

	gorillaws "github.com/gorilla/websocket"
)

const (
	writeWait  = 10 * time.Second // max time to write a frame to the client
	pongWait   = 60 * time.Second // max time to wait for a pong from the client
	pingPeriod = 54 * time.Second // send ping at this interval (must be < pongWait)
	maxMsgSize = 512              // max bytes for an inbound message
)

// Client represents a single connected WebSocket user.
type Client struct {
	hub      *ChatHub
	conn     *gorillaws.Conn
	send     chan []byte // hub writes here; writePump drains it
	userID   string
	username string
	roomID   string
}

// readPump pumps inbound messages from the WebSocket to the hub's broadcast channel.
// Runs in its own goroutine; on return it unregisters the client.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMsgSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		var in struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(raw, &in); err != nil || in.Message == "" {
			continue
		}
		c.hub.broadcast <- ChatMessage{
			Type:      "message",
			UserID:    c.userID,
			Username:  c.username,
			RoomID:    c.roomID,
			Message:   in.Message,
			Timestamp: time.Now().Unix(),
		}
	}
}

// writePump pumps outbound messages from the client's send channel to the WebSocket.
// Runs in its own goroutine; also sends periodic pings to keep the connection alive.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case data, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// hub closed the channel — send a clean close frame
				c.conn.WriteMessage(gorillaws.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(gorillaws.TextMessage, data); err != nil {
				log.Printf("ws: write: %v", err)
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(gorillaws.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
