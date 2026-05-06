package websocket

import (
	"encoding/json"
	"log"
	"time"
)

const historyMax = 20

// ChatMessage is the wire format for all outbound WebSocket messages.
type ChatMessage struct {
	Type      string `json:"type"`             // "message" | "join" | "leave"
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	RoomID    string `json:"room_id"`
	Message   string `json:"message,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

type room struct {
	clients map[*Client]bool
	history []ChatMessage // capped at historyMax; oldest dropped when full
}

// ChatHub owns all room state. Only its Run() goroutine reads/writes hub.rooms.
type ChatHub struct {
	rooms      map[string]*room
	broadcast  chan ChatMessage
	register   chan *Client
	unregister chan *Client
}

func NewHub() *ChatHub {
	return &ChatHub{
		rooms:      make(map[string]*room),
		broadcast:  make(chan ChatMessage, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// Run processes hub events sequentially. Must be started in a goroutine.
func (h *ChatHub) Run() {
	for {
		select {
		case client := <-h.register:
			r := h.rooms[client.roomID]
			if r == nil {
				r = &room{clients: make(map[*Client]bool)}
				h.rooms[client.roomID] = r
			}
			// notify existing clients before adding the new one
			h.fanOut(r, ChatMessage{
				Type:      "join",
				UserID:    client.userID,
				Username:  client.username,
				RoomID:    client.roomID,
				Timestamp: time.Now().Unix(),
			})
			r.clients[client] = true
			// send history to the new client
			for _, msg := range r.history {
				data, _ := json.Marshal(msg)
				client.send <- data
			}

		case client := <-h.unregister:
			r := h.rooms[client.roomID]
			if r == nil {
				continue
			}
			if _, ok := r.clients[client]; !ok {
				continue
			}
			delete(r.clients, client)
			close(client.send)
			if len(r.clients) == 0 {
				delete(h.rooms, client.roomID)
			} else {
				h.fanOut(r, ChatMessage{
					Type:      "leave",
					UserID:    client.userID,
					Username:  client.username,
					RoomID:    client.roomID,
					Timestamp: time.Now().Unix(),
				})
			}

		case msg := <-h.broadcast:
			r := h.rooms[msg.RoomID]
			if r == nil {
				continue
			}
			r.history = append(r.history, msg)
			if len(r.history) > historyMax {
				r.history = r.history[len(r.history)-historyMax:]
			}
			h.fanOut(r, msg)
		}
	}
}

// fanOut sends msg to every client in r. Slow clients (full send channel) are dropped immediately.
func (h *ChatHub) fanOut(r *room, msg ChatMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("ws: fanOut marshal: %v", err)
		return
	}
	for c := range r.clients {
		select {
		case c.send <- data:
		default:
			delete(r.clients, c)
			close(c.send)
		}
	}
}
