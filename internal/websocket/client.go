package websocket

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	gorillaws "github.com/gorilla/websocket"
	"mangahub/pkg/jwtutil"
)

const (
	writeWait  = 10 * time.Second // max time to write a frame to the client
	pongWait   = 60 * time.Second // max time to wait for a pong from the client
	pingPeriod = 54 * time.Second // send ping at this interval (must be < pongWait)
	maxMsgSize = 512              // max bytes for an inbound message
)

// Client represents a single connected WebSocket user.
type Client struct {
	hub         *ChatHub
	conn        *gorillaws.Conn
	send        chan []byte // hub writes here; writePump drains it
	userID      string
	username    string
	roomID      string
	jwtSecret   string
	authTimeout time.Duration // 0 → defaults to 10s
	authed      bool
}

// readPump pumps inbound messages from the WebSocket to the hub's broadcast channel.
// The first message must be {"token":"<jwt>"} to authenticate; subsequent messages
// are broadcast as chat. Runs in its own goroutine.
func (c *Client) readPump() {
	defer func() {
		if c.authed {
			c.hub.unregister <- c
		}
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMsgSize)

	authTimeout := 10 * time.Second
	if c.authTimeout > 0 {
		authTimeout = c.authTimeout
	}
	c.conn.SetReadDeadline(time.Now().Add(authTimeout))

	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			if !c.authed {
				close(c.send) // terminate writePump — hub never registered this client
			}
			break
		}

		if !c.authed {
			var authMsg struct {
				Token string `json:"token"`
			}
			if err := json.Unmarshal(raw, &authMsg); err != nil || authMsg.Token == "" {
				c.conn.WriteMessage(gorillaws.CloseMessage,
					gorillaws.FormatCloseMessage(4001, "auth required"))
				close(c.send)
				return
			}
			claims, err := jwtutil.ValidateToken(authMsg.Token, c.jwtSecret)
			if err != nil {
				c.conn.WriteMessage(gorillaws.CloseMessage,
					gorillaws.FormatCloseMessage(4001, "invalid token"))
				close(c.send)
				return
			}
			c.userID = claims.UserID
			c.username = claims.Username
			if c.username == "" {
				c.username = c.userID
			}
			c.authed = true
			c.conn.SetReadDeadline(time.Now().Add(pongWait))
			c.conn.SetPongHandler(func(string) error {
				c.conn.SetReadDeadline(time.Now().Add(pongWait))
				return nil
			})
			c.hub.register <- c
			continue
		}

		// Accept plain text or {"message":"..."} JSON.
		text := strings.TrimSpace(string(raw))
		if text == "" {
			continue
		}
		var in struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(raw, &in); err == nil && in.Message != "" {
			text = in.Message
		}
		c.hub.broadcast <- ChatMessage{
			Type:      "message",
			UserID:    c.userID,
			Username:  c.username,
			RoomID:    c.roomID,
			Message:   text,
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
