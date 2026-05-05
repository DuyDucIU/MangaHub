# WebSocket Chat System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Integrate a per-manga-room WebSocket chat system into the existing HTTP API server, covering UC-011/012/013 (15 pts core) and the room management bonus (10 pts).

**Architecture:** A `ChatHub` goroutine owns all room state via a `rooms map[string]*room` (no mutexes needed). Each connected `Client` runs two goroutines — a read pump and a write pump — communicating with the hub via channels. Authentication uses JWT from the `?token=` query parameter validated before the WebSocket upgrade.

**Tech Stack:** `github.com/gorilla/websocket`, `github.com/golang-jwt/jwt/v4` (already present), Go standard library.

---

## File Map

| Action | File | Responsibility |
|--------|------|----------------|
| Create | `pkg/jwtutil/jwtutil.go` | Shared JWT parse/validate — replaces private `validateJWT` in tcp |
| Create | `pkg/jwtutil/jwtutil_test.go` | Unit tests for ValidateToken |
| Modify | `internal/tcp/server.go` | Remove private `validateJWT`, use `jwtutil.ValidateToken` |
| Create | `internal/websocket/hub.go` | `ChatHub`, `ChatMessage`, `room` types + `NewHub()` + `Run()` + `fanOut()` |
| Create | `internal/websocket/hub_test.go` | White-box hub tests (register, broadcast, history, slow-client drop) |
| Create | `internal/websocket/client.go` | `Client` struct + `readPump()` + `writePump()` |
| Create | `internal/websocket/handler.go` | `Handler` struct + `ServeWS` gin handler (JWT auth + WS upgrade) |
| Create | `internal/websocket/handler_test.go` | Handler tests (auth rejection, upgrade, default room) |
| Modify | `cmd/api-server/main.go` | Create hub, start `hub.Run()`, register `GET /ws/chat` route |

---

## Task 1: Add gorilla/websocket Dependency

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add the dependency**

```bash
go get github.com/gorilla/websocket
```

- [ ] **Step 2: Verify it appears in go.mod**

```bash
go list -m github.com/gorilla/websocket
```

Expected output (version may differ):
```
github.com/gorilla/websocket v1.5.x
```

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add gorilla/websocket dependency"
```

---

## Task 2: Create pkg/jwtutil

The `validateJWT` function in `internal/tcp/server.go` is private and duplicates what `internal/auth/handler.go` already does with the jwt library. Extract it to a shared package so the WebSocket handler can reuse it without a third copy.

**Files:**
- Create: `pkg/jwtutil/jwtutil.go`
- Create: `pkg/jwtutil/jwtutil_test.go`

- [ ] **Step 1: Write the failing tests**

Create `pkg/jwtutil/jwtutil_test.go`:

```go
package jwtutil_test

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"mangahub/pkg/jwtutil"
)

const testSecret = "test-secret"

func signedToken(userID, username, secret string, ttl time.Duration) string {
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":  userID,
		"username": username,
		"exp":      time.Now().Add(ttl).Unix(),
	})
	s, _ := tok.SignedString([]byte(secret))
	return s
}

func TestValidateToken_Valid(t *testing.T) {
	tok := signedToken("usr_abc", "alice", testSecret, time.Hour)
	claims, err := jwtutil.ValidateToken(tok, testSecret)
	assert.NoError(t, err)
	assert.Equal(t, "usr_abc", claims.UserID)
	assert.Equal(t, "alice", claims.Username)
}

func TestValidateToken_WrongSecret(t *testing.T) {
	tok := signedToken("usr_abc", "alice", "wrong-secret", time.Hour)
	_, err := jwtutil.ValidateToken(tok, testSecret)
	assert.Error(t, err)
}

func TestValidateToken_Expired(t *testing.T) {
	tok := signedToken("usr_abc", "alice", testSecret, -time.Hour)
	_, err := jwtutil.ValidateToken(tok, testSecret)
	assert.Error(t, err)
}

func TestValidateToken_Malformed(t *testing.T) {
	_, err := jwtutil.ValidateToken("not.a.jwt", testSecret)
	assert.Error(t, err)
}

func TestValidateToken_MissingUserID(t *testing.T) {
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	s, _ := tok.SignedString([]byte(testSecret))
	_, err := jwtutil.ValidateToken(s, testSecret)
	assert.Error(t, err)
}

func TestDefaultSecret_FallsBackToDevDefault(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	assert.Equal(t, "mangahub-dev-secret", jwtutil.DefaultSecret())
}

func TestDefaultSecret_ReadsEnvVar(t *testing.T) {
	t.Setenv("JWT_SECRET", "my-prod-secret")
	assert.Equal(t, "my-prod-secret", jwtutil.DefaultSecret())
}
```

- [ ] **Step 2: Run tests to confirm they fail to compile**

```bash
go test ./pkg/jwtutil/...
```

Expected: compile error — `jwtutil` package does not exist yet.

- [ ] **Step 3: Implement pkg/jwtutil/jwtutil.go**

Create `pkg/jwtutil/jwtutil.go`:

```go
package jwtutil

import (
	"fmt"
	"os"

	"github.com/golang-jwt/jwt/v4"
)

// Claims holds the fields MangaHub embeds in every JWT.
type Claims struct {
	UserID   string
	Username string
}

// DefaultSecret returns the HMAC key from JWT_SECRET env var, or the dev default.
func DefaultSecret() string {
	if s := os.Getenv("JWT_SECRET"); s != "" {
		return s
	}
	return "mangahub-dev-secret"
}

// ValidateToken parses tokenStr using secret and returns the embedded claims.
func ValidateToken(tokenStr, secret string) (Claims, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		return Claims{}, fmt.Errorf("invalid token")
	}
	m, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return Claims{}, fmt.Errorf("invalid claims")
	}
	userID, _ := m["user_id"].(string)
	if userID == "" {
		return Claims{}, fmt.Errorf("missing user_id claim")
	}
	username, _ := m["username"].(string)
	return Claims{UserID: userID, Username: username}, nil
}
```

- [ ] **Step 4: Run tests and confirm they pass**

```bash
go test ./pkg/jwtutil/... -v
```

Expected: all 7 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/jwtutil/
git commit -m "feat: add pkg/jwtutil for shared JWT validation"
```

---

## Task 3: Update tcp/server.go to Use jwtutil

Remove the private `validateJWT` function from `internal/tcp/server.go` and replace its call site with `jwtutil.ValidateToken`.

**Files:**
- Modify: `internal/tcp/server.go`

- [ ] **Step 1: Add the import and update handleConn**

In `internal/tcp/server.go`, add `"mangahub/pkg/jwtutil"` to the import block and replace the call to `validateJWT` in `handleConn`:

Find this block (around line 179):
```go
	userID, err := validateJWT(msg.Token)
	if err != nil {
		writeMsg(conn, serverMsg{Type: "auth_error", Message: "invalid token"}) //nolint:errcheck
		return
	}
```

Replace with:
```go
	claims, err := jwtutil.ValidateToken(msg.Token, jwtutil.DefaultSecret())
	if err != nil {
		writeMsg(conn, serverMsg{Type: "auth_error", Message: "invalid token"}) //nolint:errcheck
		return
	}
	userID := claims.UserID
```

- [ ] **Step 2: Delete the private validateJWT function**

Remove the entire `validateJWT` function from the bottom of `internal/tcp/server.go` (lines 219–242):

```go
func validateJWT(tokenStr string) (string, error) {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "mangahub-dev-secret"
	}
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		return "", fmt.Errorf("invalid token")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", fmt.Errorf("invalid claims")
	}
	userID, ok := claims["user_id"].(string)
	if !ok || userID == "" {
		return "", fmt.Errorf("missing user_id claim")
	}
	return userID, nil
}
```

- [ ] **Step 3: Clean up unused imports in tcp/server.go**

The imports `"fmt"` and `"os"` were only used by `validateJWT` and the jwt library import. Remove:
- `"fmt"`
- `"os"`
- `"github.com/golang-jwt/jwt/v4"`

Add:
- `"mangahub/pkg/jwtutil"`

The final import block should be:

```go
import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"mangahub/pkg/jwtutil"
)
```

- [ ] **Step 4: Run all existing TCP tests to confirm nothing broke**

```bash
go test ./internal/tcp/... -v
```

Expected: all tests PASS (same behaviour, different implementation).

- [ ] **Step 5: Commit**

```bash
git add internal/tcp/server.go
git commit -m "refactor(tcp): use pkg/jwtutil for JWT validation"
```

---

## Task 4: Implement ChatHub with Tests

**Files:**
- Create: `internal/websocket/hub.go`
- Create: `internal/websocket/hub_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/websocket/hub_test.go` (package `websocket` — same package for white-box access):

```go
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
	// The unbuffered register channel guarantees the prior unregister was processed first.
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
```

- [ ] **Step 2: Run tests to confirm they fail to compile**

```bash
go test ./internal/websocket/... 
```

Expected: compile error — package does not exist.

- [ ] **Step 3: Implement internal/websocket/hub.go**

Create `internal/websocket/hub.go`:

```go
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
				select {
				case client.send <- data:
				default:
				}
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
```

- [ ] **Step 4: Run hub tests**

```bash
go test ./internal/websocket/... -run TestHub -v
```

Expected: all 7 hub tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/websocket/hub.go internal/websocket/hub_test.go
git commit -m "feat(websocket): add ChatHub with room management and history ring buffer"
```

---

## Task 5: Implement Client (Read and Write Pumps)

**Files:**
- Create: `internal/websocket/client.go`

No dedicated unit tests for pumps — they require a real WebSocket connection and are covered by the handler integration test in Task 6.

- [ ] **Step 1: Create internal/websocket/client.go**

```go
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
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./internal/websocket/...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/websocket/client.go
git commit -m "feat(websocket): add Client with read/write pumps"
```

---

## Task 6: Implement Handler with Tests

**Files:**
- Create: `internal/websocket/handler.go`
- Create: `internal/websocket/handler_test.go`

- [ ] **Step 1: Write the failing handler tests**

Create `internal/websocket/handler_test.go` (package `websocket` — same package):

```go
package websocket

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	gorillaws "github.com/gorilla/websocket"
	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
)

const handlerTestSecret = "handler-test-secret"

func signHandlerToken(userID, username string) string {
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":  userID,
		"username": username,
		"exp":      time.Now().Add(time.Hour).Unix(),
	})
	s, _ := tok.SignedString([]byte(handlerTestSecret))
	return s
}

func setupHandlerRouter(h *Handler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/ws/chat", h.ServeWS)
	return r
}

func TestHandler_MissingToken_Returns401(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	h := &Handler{Hub: hub, JWTSecret: handlerTestSecret}

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/ws/chat", nil)
	setupHandlerRouter(h).ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHandler_InvalidToken_Returns401(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	h := &Handler{Hub: hub, JWTSecret: handlerTestSecret}

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/ws/chat?token=bad.token.value", nil)
	setupHandlerRouter(h).ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHandler_ValidToken_UpgradesConnection(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	h := &Handler{Hub: hub, JWTSecret: handlerTestSecret}

	srv := httptest.NewServer(setupHandlerRouter(h))
	defer srv.Close()

	tok := signHandlerToken("usr_abc", "alice")
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/chat?token=" + tok + "&manga_id=one-piece"

	conn, resp, err := gorillaws.DefaultDialer.Dial(url, nil)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	conn.Close()
}

func TestHandler_NoMangaID_JoinsGeneralRoom(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	h := &Handler{Hub: hub, JWTSecret: handlerTestSecret}

	// Pre-register a client in "general" to receive the join notification.
	watcher := newTestClient(hub, "general", "watcher")
	hub.register <- watcher

	srv := httptest.NewServer(setupHandlerRouter(h))
	defer srv.Close()

	tok := signHandlerToken("usr_xyz", "bob")
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/chat?token=" + tok // no manga_id

	conn, _, err := gorillaws.DefaultDialer.Dial(url, nil)
	assert.NoError(t, err)
	defer conn.Close()

	// Watcher should receive a join notification for bob in "general".
	msg := recv(t, watcher)
	assert.Equal(t, "join", msg.Type)
	assert.Equal(t, "usr_xyz", msg.UserID)
	assert.Equal(t, "general", msg.RoomID)
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/websocket/... -run TestHandler -v
```

Expected: compile error — `Handler` type and `ServeWS` not defined.

- [ ] **Step 3: Implement internal/websocket/handler.go**

```go
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
```

- [ ] **Step 4: Run all websocket tests**

```bash
go test ./internal/websocket/... -v
```

Expected: all 11 tests PASS (7 hub + 4 handler).

- [ ] **Step 5: Commit**

```bash
git add internal/websocket/handler.go internal/websocket/handler_test.go
git commit -m "feat(websocket): add Handler with JWT auth and WebSocket upgrade"
```

---

## Task 7: Wire ChatHub into cmd/api-server

**Files:**
- Modify: `cmd/api-server/main.go`

- [ ] **Step 1: Update cmd/api-server/main.go**

The current `main.go` ends at line 54. Add the hub setup and route before `r.Run`. Replace the full file with:

```go
package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"mangahub/internal/auth"
	"mangahub/internal/manga"
	"mangahub/internal/user"
	wschat "mangahub/internal/websocket"
	"mangahub/pkg/database"
)

func main() {
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "mangahub-dev-secret"
	}

	db, err := database.Connect("./data/mangahub.db")
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	if err := database.SeedManga(db, "./data/manga.json"); err != nil {
		log.Printf("seed warning: %v", err)
	}

	authHandler := &auth.Handler{DB: db, JWTSecret: jwtSecret}
	mangaHandler := &manga.Handler{DB: db}
	userHandler := &user.Handler{DB: db}

	hub := wschat.NewHub()
	go hub.Run()
	wsHandler := &wschat.Handler{Hub: hub, JWTSecret: jwtSecret}

	r := gin.Default()

	r.POST("/auth/register", authHandler.Register)
	r.POST("/auth/login", authHandler.Login)

	r.GET("/manga", mangaHandler.Search)
	r.GET("/manga/:id", mangaHandler.GetByID)
	r.GET("/ws/chat", wsHandler.ServeWS)

	protected := r.Group("/")
	protected.Use(authHandler.JWTMiddleware())
	protected.POST("/manga", mangaHandler.Create)
	protected.POST("/users/library", userHandler.AddToLibrary)
	protected.GET("/users/library", userHandler.GetLibrary)
	protected.DELETE("/users/library/:manga_id", userHandler.RemoveFromLibrary)
	protected.PUT("/users/progress", userHandler.UpdateProgress)

	log.Println("HTTP API server running on :8080")
	if err := r.Run(":8080"); err != nil {
		log.Fatalf("server: %v", err)
	}
}
```

Note: `/ws/chat` is outside the `protected` group — JWT is validated inside `ServeWS` via query param, not the Authorization header middleware.

- [ ] **Step 2: Build to confirm no compile errors**

```bash
go build ./cmd/api-server/...
```

Expected: no errors.

- [ ] **Step 3: Run the full test suite**

```bash
go test ./...
```

Expected: all tests PASS across all packages.

- [ ] **Step 4: Commit**

```bash
git add cmd/api-server/main.go
git commit -m "feat: wire WebSocket ChatHub into api-server on GET /ws/chat"
```

---

## Self-Review

### Spec Coverage Check

| Spec Requirement | Task |
|---|---|
| WebSocket connection handling | Task 6 (handler upgrade) |
| Real-time message broadcasting | Task 4 (hub fanOut) |
| User join/leave functionality | Task 4 (hub register/unregister events) |
| Basic connection management | Task 5 (read/write pumps, ping/pong) |
| JWT auth before joining | Task 6 (ServeWS validates token before upgrade) |
| Recent chat history on join | Task 4 (hub sends history on register) |
| Per-manga rooms (bonus) | Task 4 (rooms map keyed by manga_id) |
| Default room "general" | Task 6 (handler defaults empty manga_id) |
| Slow client drop | Task 4 (fanOut non-blocking send) |
| Route registered in api-server | Task 7 |

All 10 requirements covered.

### Type Consistency Check

- `Client.roomID` (string) — used as key in `hub.rooms` and set in `handler.go` from `c.Query("manga_id")` ✓
- `Client.userID` / `Client.username` — set from `jwtutil.Claims.UserID` / `Claims.Username` ✓
- `ChatMessage.RoomID` used as lookup key in `hub.broadcast` case — matches `Client.roomID` ✓
- `newTestClient` in `hub_test.go` matches the `Client` struct fields defined in `client.go` ✓
- `Handler.JWTSecret` passed to `jwtutil.ValidateToken` — matches `ValidateToken(tokenStr, secret string)` signature ✓

### Placeholder Scan

No TBD, TODO, or vague steps found. All code steps include complete, runnable code.
