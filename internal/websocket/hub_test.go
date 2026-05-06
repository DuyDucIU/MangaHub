package websocket

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// newTestClient creates a Client with a buffered send channel and no conn (hub tests only).
func newTestClient(hub *ChatHub, roomID, userID string) *Client {
	return &Client{
		hub:      hub,
		send:     make(chan []byte, 16),
		userID:   userID,
		username: userID,
		roomID:   roomID,
	}
}

// recv reads the next JSON message from c.send with a 500ms timeout.
func recv(t *testing.T, c *Client) ChatMessage {
	t.Helper()
	select {
	case data := <-c.send:
		var msg ChatMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			t.Fatalf("recv: unmarshal: %v", err)
		}
		return msg
	case <-time.After(500 * time.Millisecond):
		t.Fatal("recv: timeout waiting for message")
		return ChatMessage{}
	}
}

// expectEmpty asserts no message arrives on c.send within 100ms.
func expectEmpty(t *testing.T, c *Client) {
	t.Helper()
	select {
	case data := <-c.send:
		t.Fatalf("expected empty send channel, got: %s", data)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestHub_Register_NotifiesExistingClients(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	c1 := newTestClient(hub, "room1", "u1")
	hub.register <- c1
	expectEmpty(t, c1) // empty room — no join notification sent, no history

	c2 := newTestClient(hub, "room1", "u2")
	hub.register <- c2
	msg := recv(t, c1) // c1 gets join notification for c2
	assert.Equal(t, "join", msg.Type)
	assert.Equal(t, "u2", msg.UserID)
	assert.Equal(t, "room1", msg.RoomID)
	expectEmpty(t, c2) // c2 does not receive its own join; no history yet
}

func TestHub_Register_SendsHistoryToNewClient(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	c1 := newTestClient(hub, "room1", "u1")
	hub.register <- c1

	hub.broadcast <- ChatMessage{Type: "message", UserID: "u1", Username: "u1", RoomID: "room1", Message: "hello", Timestamp: 1}
	recv(t, c1) // consume the broadcast delivered to c1

	c2 := newTestClient(hub, "room1", "u2")
	hub.register <- c2
	recv(t, c1) // consume join notification on c1

	msg := recv(t, c2) // c2 receives the history message
	assert.Equal(t, "message", msg.Type)
	assert.Equal(t, "hello", msg.Message)
}

func TestHub_Broadcast_OnlyDeliveredToSameRoom(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	c1 := newTestClient(hub, "room1", "u1")
	c2 := newTestClient(hub, "room2", "u2")
	hub.register <- c1
	hub.register <- c2

	hub.broadcast <- ChatMessage{Type: "message", UserID: "u1", Username: "u1", RoomID: "room1", Message: "hi", Timestamp: 1}
	msg := recv(t, c1)
	assert.Equal(t, "hi", msg.Message)
	expectEmpty(t, c2) // different room — should receive nothing
}

func TestHub_Unregister_SendsLeaveToRemainingClients(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	c1 := newTestClient(hub, "room1", "u1")
	c2 := newTestClient(hub, "room1", "u2")
	hub.register <- c1
	hub.register <- c2
	recv(t, c1) // consume join notification for c2

	hub.unregister <- c2
	msg := recv(t, c1)
	assert.Equal(t, "leave", msg.Type)
	assert.Equal(t, "u2", msg.UserID)
}

func TestHub_Unregister_DeletesEmptyRoom(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	c1 := newTestClient(hub, "room1", "u1")
	hub.register <- c1
	hub.unregister <- c1

	// Probe: register c2 to the same room; it should get no history (room was wiped).
	c2 := newTestClient(hub, "room1", "u2")
	hub.register <- c2
	expectEmpty(t, c2)
}

func TestHub_History_CappedAt20(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	c1 := newTestClient(hub, "room1", "u1")
	hub.register <- c1

	for i := 0; i < 25; i++ {
		hub.broadcast <- ChatMessage{Type: "message", UserID: "u1", Username: "u1", RoomID: "room1", Message: "msg", Timestamp: int64(i)}
		recv(t, c1) // drain so c1's buffer never fills
	}

	c2 := newTestClient(hub, "room1", "u2")
	hub.register <- c2
	recv(t, c1) // consume join notification on c1

	count := 0
	for {
		select {
		case <-c2.send:
			count++
		case <-time.After(100 * time.Millisecond):
			assert.Equal(t, 20, count, "history should be capped at 20 messages")
			return
		}
	}
}

func TestHub_SlowClient_DroppedWithoutBlockingHub(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	slow := &Client{hub: hub, send: make(chan []byte, 1), userID: "slow", username: "slow", roomID: "room1"}
	fast := newTestClient(hub, "room1", "fast")
	hub.register <- slow
	hub.register <- fast
	recv(t, slow) // consume join notification for fast

	// Three broadcasts: slow's buffer (size 1) will fill on msg1, overflow on msg2 (drop).
	for i := 0; i < 3; i++ {
		hub.broadcast <- ChatMessage{Type: "message", UserID: "fast", Username: "fast", RoomID: "room1", Message: "msg", Timestamp: int64(i)}
		recv(t, fast)
	}

	// Hub should still work fine after dropping slow.
	hub.broadcast <- ChatMessage{Type: "message", UserID: "fast", Username: "fast", RoomID: "room1", Message: "after drop", Timestamp: 99}
	msg := recv(t, fast)
	assert.Equal(t, "after drop", msg.Message)
}
