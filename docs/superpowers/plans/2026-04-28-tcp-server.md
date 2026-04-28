# TCP Progress Sync Server Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a TCP server that maintains persistent client connections and pushes reading progress updates to a user's active connection when they update progress via the HTTP API.

**Architecture:** Two separate binaries (per spec): `cmd/tcp-server` runs a TCP listener on `:9090` (client connections) and an internal HTTP server on `:9099` (broadcast trigger). When `PUT /users/progress` succeeds, the HTTP API fires a goroutine that POSTs to `:9099/internal/broadcast`. The TCP server receives this, looks up the user's connection, and writes the JSON update.

**Tech Stack:** Go stdlib `net`, `encoding/json`, `sync`, `io` — `github.com/golang-jwt/jwt/v4` for JWT validation — `github.com/stretchr/testify` for assertions — `net.Pipe()` for in-memory TCP testing.

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/tcp/server.go` | Create | `ProgressSyncServer`, `ProgressUpdate`, `New`, `Register`, `Unregister`, `BroadcastToUser`, `Run`, `handleConn`, `validateJWT`, private helpers |
| `internal/tcp/server_test.go` | Create | Tests for all server.go functions |
| `internal/tcp/handler.go` | Create | `InternalHandler()`, `handleBroadcast` — internal HTTP broadcast endpoint |
| `internal/tcp/handler_test.go` | Create | Tests for handler.go |
| `cmd/tcp-server/main.go` | Create | Entry point — wires `ProgressSyncServer`, starts TCP + internal HTTP |
| `internal/user/handler.go` | Modify | Add `notifyTCPServer` helper + call it (goroutine) after successful DB update in `UpdateProgress` |

---

## Task 1: TCP server core — struct, New, Register, Unregister, BroadcastToUser

**Files:**
- Create: `internal/tcp/server_test.go`
- Create: `internal/tcp/server.go`

- [ ] **Step 1: Write failing tests for Register, Unregister, BroadcastToUser**

Create `internal/tcp/server_test.go`:

```go
package tcp

import (
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// readMsg reads one JSON line from conn with a 2s deadline.
func readMsg(t *testing.T, conn net.Conn) map[string]any {
	t.Helper()
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	defer conn.SetDeadline(time.Time{})
	var msg map[string]any
	if err := json.NewDecoder(conn).Decode(&msg); err != nil {
		t.Fatalf("readMsg: %v", err)
	}
	return msg
}

func TestRegister(t *testing.T) {
	srv := New("9090")
	s, c := net.Pipe()
	defer s.Close()
	defer c.Close()

	srv.Register("user1", s)

	assert.Len(t, srv.Connections, 1)
	assert.Equal(t, s, srv.Connections["user1"])
}

func TestRegister_ReplacesExistingConnection(t *testing.T) {
	srv := New("9090")
	s1, c1 := net.Pipe()
	s2, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	srv.Register("user1", s1)
	srv.Register("user1", s2) // should close s1 and replace

	assert.Len(t, srv.Connections, 1)
	assert.Equal(t, s2, srv.Connections["user1"])

	buf := make([]byte, 1)
	s1.SetDeadline(time.Now().Add(100 * time.Millisecond))
	_, err := s1.Read(buf)
	assert.Error(t, err, "old connection should be closed")
}

func TestUnregister(t *testing.T) {
	srv := New("9090")
	s, c := net.Pipe()
	defer s.Close()
	defer c.Close()

	srv.Register("user1", s)
	srv.Unregister("user1")

	assert.Empty(t, srv.Connections)
}

func TestUnregister_NoOp(t *testing.T) {
	srv := New("9090")
	// should not panic
	srv.Unregister("nobody")
}

func TestBroadcastToUser(t *testing.T) {
	srv := New("9090")
	s, c := net.Pipe()
	defer s.Close()
	defer c.Close()

	srv.Register("user1", s)

	update := ProgressUpdate{UserID: "user1", MangaID: "one-piece", Chapter: 95, Timestamp: 1000}
	srv.BroadcastToUser(update)

	msg := readMsg(t, c)
	assert.Equal(t, "progress_update", msg["type"])
	assert.Equal(t, "user1", msg["user_id"])
	assert.Equal(t, "one-piece", msg["manga_id"])
	assert.Equal(t, float64(95), msg["chapter"])
	assert.Equal(t, float64(1000), msg["timestamp"])
}

func TestBroadcastToUser_UnknownUser(t *testing.T) {
	srv := New("9090")
	// should not panic
	srv.BroadcastToUser(ProgressUpdate{UserID: "nobody"})
}

func TestBroadcastToUser_DeadConnection(t *testing.T) {
	srv := New("9090")
	s, c := net.Pipe()
	c.Close() // close client side so write on server side fails

	srv.Register("user1", s)
	srv.BroadcastToUser(ProgressUpdate{UserID: "user1", MangaID: "one-piece", Chapter: 1, Timestamp: 1000})

	// failed write should remove the connection
	assert.Empty(t, srv.Connections)
	s.Close()
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd d:/Code/MangaHub
go test ./internal/tcp/... -v
```

Expected: compile error — package `tcp` does not exist yet.

- [ ] **Step 3: Create `internal/tcp/server.go` with core structs and methods**

```go
package tcp

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

// ProgressUpdate is the message sent to TCP clients when a user's reading progress changes.
type ProgressUpdate struct {
	UserID    string `json:"user_id"`
	MangaID   string `json:"manga_id"`
	Chapter   int    `json:"chapter"`
	Timestamp int64  `json:"timestamp"`
}

// ProgressSyncServer manages persistent TCP client connections and broadcasts progress updates.
type ProgressSyncServer struct {
	Port           string
	Connections    map[string]net.Conn // user_id → active connection
	Broadcast      chan ProgressUpdate
	MaxConnections int
	mu             sync.RWMutex
}

// New creates a ProgressSyncServer with sensible defaults.
func New(port string) *ProgressSyncServer {
	return &ProgressSyncServer{
		Port:           port,
		Connections:    make(map[string]net.Conn),
		Broadcast:      make(chan ProgressUpdate, 100),
		MaxConnections: 30,
	}
}

// Register adds or replaces the connection for userID. Closes any existing connection.
func (s *ProgressSyncServer) Register(userID string, conn net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, ok := s.Connections[userID]; ok {
		old.Close()
	}
	s.Connections[userID] = conn
}

// Unregister removes the connection for userID.
func (s *ProgressSyncServer) Unregister(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.Connections, userID)
}

// count returns the current number of active connections.
func (s *ProgressSyncServer) count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.Connections)
}

// BroadcastToUser sends a progress_update message to the user's active connection.
// If the write fails, the connection is removed. No-op if user is not connected.
func (s *ProgressSyncServer) BroadcastToUser(update ProgressUpdate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	conn, ok := s.Connections[update.UserID]
	if !ok {
		return
	}
	msg := serverMsg{
		Type:      "progress_update",
		UserID:    update.UserID,
		MangaID:   update.MangaID,
		Chapter:   update.Chapter,
		Timestamp: update.Timestamp,
	}
	if err := writeMsg(conn, msg); err != nil {
		log.Printf("tcp: write failed for user %s: %v", update.UserID, err)
		delete(s.Connections, update.UserID)
	}
}

// Run starts the TCP listener and the broadcast consumer goroutine. Blocks until listener fails.
func (s *ProgressSyncServer) Run() {
	ln, err := net.Listen("tcp", ":"+s.Port)
	if err != nil {
		log.Fatalf("tcp: listen: %v", err)
	}
	defer ln.Close()
	log.Printf("tcp: listening on :%s", s.Port)

	go func() {
		for update := range s.Broadcast {
			s.BroadcastToUser(update)
		}
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("tcp: accept: %v", err)
			continue
		}
		go s.handleConn(conn)
	}
}

// handleConn runs in its own goroutine for each TCP client.
// It performs the auth handshake then keeps the connection open until the client disconnects.
func (s *ProgressSyncServer) handleConn(conn net.Conn) {
	defer conn.Close()

	// UC-007 A2: reject when at capacity
	if s.count() >= s.MaxConnections {
		writeMsg(conn, serverMsg{Type: "error", Message: "server at capacity"}) //nolint:errcheck
		return
	}

	// client must send auth message within 5 seconds
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	var msg authMsg
	if err := json.NewDecoder(conn).Decode(&msg); err != nil || msg.Type != "auth" {
		return
	}

	conn.SetDeadline(time.Time{}) // clear deadline after auth attempt

	userID, err := validateJWT(msg.Token)
	if err != nil {
		writeMsg(conn, serverMsg{Type: "auth_error", Message: "invalid token"}) //nolint:errcheck
		return
	}

	s.Register(userID, conn)
	defer s.Unregister(userID)

	writeMsg(conn, serverMsg{Type: "auth_ok", UserID: userID}) //nolint:errcheck

	// keep alive — read until client disconnects
	io.Copy(io.Discard, conn)
}

// --- private types and helpers ---

type authMsg struct {
	Type  string `json:"type"`
	Token string `json:"token"`
}

type serverMsg struct {
	Type      string `json:"type"`
	UserID    string `json:"user_id,omitempty"`
	MangaID   string `json:"manga_id,omitempty"`
	Chapter   int    `json:"chapter,omitempty"`
	Timestamp int64  `json:"timestamp,omitempty"`
	Message   string `json:"message,omitempty"`
}

func writeMsg(conn net.Conn, msg serverMsg) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = conn.Write(append(data, '\n'))
	return err
}

func validateJWT(tokenStr string) (string, error) {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "mangahub-secret-key"
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

- [ ] **Step 4: Run tests and verify they pass**

```bash
go test ./internal/tcp/... -v -run "TestRegister|TestUnregister|TestBroadcast"
```

Expected: all 6 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tcp/server.go internal/tcp/server_test.go
git commit -m "feat(tcp): add ProgressSyncServer core — Register, Unregister, BroadcastToUser"
```

---

## Task 2: TCP connection handler — handleConn, auth, capacity

**Files:**
- Modify: `internal/tcp/server_test.go` — add handleConn tests

- [ ] **Step 1: Add failing tests for handleConn**

Append to `internal/tcp/server_test.go`:

```go
func makeToken(t *testing.T, userID string) string {
	t.Helper()
	t.Setenv("JWT_SECRET", "test-secret")
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":  userID,
		"username": "tester",
		"exp":      time.Now().Add(time.Hour).Unix(),
	})
	signed, err := tok.SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatalf("makeToken: %v", err)
	}
	return signed
}

func TestHandleConn_CapacityLimit(t *testing.T) {
	srv := New("9090")
	srv.MaxConnections = 1

	existing, _ := net.Pipe()
	defer existing.Close()
	srv.Register("existing", existing)

	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.handleConn(serverSide)
	}()

	msg := readMsg(t, clientSide)
	assert.Equal(t, "error", msg["type"])
	assert.Equal(t, "server at capacity", msg["message"])
	clientSide.Close()
	<-done
}

func TestHandleConn_InvalidToken(t *testing.T) {
	srv := New("9090")
	t.Setenv("JWT_SECRET", "test-secret")

	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.handleConn(serverSide)
	}()

	payload, _ := json.Marshal(authMsg{Type: "auth", Token: "bad.token.here"})
	clientSide.Write(append(payload, '\n'))

	msg := readMsg(t, clientSide)
	assert.Equal(t, "auth_error", msg["type"])
	<-done
}

func TestHandleConn_ValidAuth(t *testing.T) {
	srv := New("9090")
	token := makeToken(t, "user1")

	serverSide, clientSide := net.Pipe()

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.handleConn(serverSide)
	}()

	payload, _ := json.Marshal(authMsg{Type: "auth", Token: token})
	clientSide.Write(append(payload, '\n'))

	msg := readMsg(t, clientSide)
	assert.Equal(t, "auth_ok", msg["type"])
	assert.Equal(t, "user1", msg["user_id"])

	assert.Len(t, srv.Connections, 1)

	clientSide.Close() // trigger disconnect
	<-done             // wait for handleConn to clean up

	assert.Empty(t, srv.Connections)
}
```

Add import to server_test.go:
```go
import (
    // existing imports ...
    "github.com/golang-jwt/jwt/v4"
)
```

- [ ] **Step 2: Run tests and verify they pass**

```bash
go test ./internal/tcp/... -v -run "TestHandleConn"
```

Expected: all 3 tests PASS. (`handleConn` is already implemented in Task 1's server.go.)

- [ ] **Step 3: Commit**

```bash
git add internal/tcp/server_test.go
git commit -m "test(tcp): add handleConn tests — capacity, invalid auth, valid auth"
```

---

## Task 3: Internal HTTP broadcast handler

**Files:**
- Create: `internal/tcp/handler_test.go`
- Create: `internal/tcp/handler.go`

- [ ] **Step 1: Write failing tests for InternalHandler**

Create `internal/tcp/handler_test.go`:

```go
package tcp

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestHandleBroadcast_ValidPayload(t *testing.T) {
	srv := New("9090")
	s, c := net.Pipe()
	defer s.Close()
	defer c.Close()
	srv.Register("user1", s)

	// drain broadcast channel so handler doesn't block
	go func() {
		for update := range srv.Broadcast {
			srv.BroadcastToUser(update)
		}
	}()

	body := `{"user_id":"user1","manga_id":"one-piece","chapter":95,"timestamp":1000}`
	req := httptest.NewRequest(http.MethodPost, "/internal/broadcast", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.InternalHandler().ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// verify the update arrived at the client connection
	c.SetDeadline(time.Now().Add(2 * time.Second))
	var msg map[string]any
	json.NewDecoder(c).Decode(&msg)
	assert.Equal(t, "progress_update", msg["type"])
	assert.Equal(t, "one-piece", msg["manga_id"])
	assert.Equal(t, float64(95), msg["chapter"])
}

func TestHandleBroadcast_MalformedPayload(t *testing.T) {
	srv := New("9090")
	req := httptest.NewRequest(http.MethodPost, "/internal/broadcast", strings.NewReader("not json"))
	w := httptest.NewRecorder()

	srv.InternalHandler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleBroadcast_UnknownUser(t *testing.T) {
	srv := New("9090")

	// drain so handler doesn't block on channel send
	go func() {
		for range srv.Broadcast {
		}
	}()

	body := `{"user_id":"nobody","manga_id":"one-piece","chapter":1,"timestamp":1000}`
	req := httptest.NewRequest(http.MethodPost, "/internal/broadcast", strings.NewReader(body))
	w := httptest.NewRecorder()

	srv.InternalHandler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code) // silent 200 — user simply not connected
}

func TestHandleBroadcast_MethodNotAllowed(t *testing.T) {
	srv := New("9090")
	req := httptest.NewRequest(http.MethodGet, "/internal/broadcast", nil)
	w := httptest.NewRecorder()

	srv.InternalHandler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/tcp/... -v -run "TestHandleBroadcast"
```

Expected: compile error — `InternalHandler` not defined.

- [ ] **Step 3: Create `internal/tcp/handler.go`**

```go
package tcp

import (
	"encoding/json"
	"net/http"
)

// InternalHandler returns an HTTP handler for the internal broadcast endpoint.
// Only POST /internal/broadcast is accepted.
func (s *ProgressSyncServer) InternalHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/broadcast", s.handleBroadcast)
	return mux
}

func (s *ProgressSyncServer) handleBroadcast(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var update ProgressUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	s.Broadcast <- update
	w.WriteHeader(http.StatusOK)
}
```

- [ ] **Step 4: Run tests and verify they pass**

```bash
go test ./internal/tcp/... -v -run "TestHandleBroadcast"
```

Expected: all 4 tests PASS.

- [ ] **Step 5: Run full test suite for the tcp package**

```bash
go test ./internal/tcp/... -v
```

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tcp/handler.go internal/tcp/handler_test.go
git commit -m "feat(tcp): add internal HTTP broadcast handler"
```

---

## Task 4: TCP server entry point

**Files:**
- Create: `cmd/tcp-server/main.go`

- [ ] **Step 1: Create `cmd/tcp-server/main.go`**

```go
package main

import (
	"log"
	"net/http"
	"os"

	"mangahub/internal/tcp"
)

func main() {
	port := os.Getenv("TCP_PORT")
	if port == "" {
		port = "9090"
	}
	internalAddr := os.Getenv("TCP_INTERNAL_ADDR")
	if internalAddr == "" {
		internalAddr = ":9099"
	}

	srv := tcp.New(port)

	go func() {
		log.Printf("tcp: internal HTTP on %s", internalAddr)
		if err := http.ListenAndServe(internalAddr, srv.InternalHandler()); err != nil {
			log.Fatalf("tcp: internal HTTP: %v", err)
		}
	}()

	srv.Run()
}
```

- [ ] **Step 2: Build the binary to confirm it compiles**

```bash
go build ./cmd/tcp-server/...
```

Expected: no output, binary created.

- [ ] **Step 3: Smoke test with nc (manual)**

In terminal 1 — start servers:
```bash
# Start the HTTP API
go run ./cmd/api-server/

# Start the TCP server (separate terminal)
go run ./cmd/tcp-server/
```

In terminal 2 — register and get a JWT:
```bash
curl -s -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username":"tcptest","email":"tcp@test.com","password":"password123"}'

curl -s -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"tcptest","password":"password123"}'
# Copy the token from the response
```

In terminal 3 — connect via nc and authenticate:
```bash
nc localhost 9090
{"type":"auth","token":"<paste token here>"}
```

Expected response from server:
```json
{"type":"auth_ok","user_id":"usr_xxxxxxxx"}
```

- [ ] **Step 4: Commit**

```bash
git add cmd/tcp-server/main.go
git commit -m "feat(tcp): add tcp-server entry point"
```

---

## Task 5: HTTP API integration — notify TCP server on progress update

**Files:**
- Modify: `internal/user/handler.go`

- [ ] **Step 1: Add `notifyTCPServer` and wire into `UpdateProgress`**

Add these imports to `internal/user/handler.go` (merge with existing imports):
```go
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
```

Add `notifyTCPServer` function at the bottom of `internal/user/handler.go`:

```go
// notifyTCPServer fires a goroutine that POSTs the progress update to the TCP server's
// internal broadcast endpoint. Fire-and-forget: HTTP API returns 200 regardless of
// TCP server availability (UC-006 A2 — progress is already saved to DB).
func notifyTCPServer(userID, mangaID string, chapter int) {
	addr := os.Getenv("TCP_INTERNAL_ADDR")
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
	client := &http.Client{Timeout: time.Second}
	resp, err := client.Post(addr+"/internal/broadcast", "application/json", bytes.NewReader(payload))
	if err != nil {
		log.Printf("user: TCP notify failed: %v", err)
		return
	}
	resp.Body.Close()
}
```

In `UpdateProgress`, add one line after the successful DB exec (after line 193, before `c.JSON`):

```go
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
```

- [ ] **Step 2: Verify existing user handler tests still pass**

```bash
go test ./internal/user/... -v
```

Expected: all existing tests PASS. The `notifyTCPServer` goroutine will fail to connect (no TCP server in tests) and log an error — this is expected and does not affect test outcomes.

- [ ] **Step 3: Run the full test suite**

```bash
go test ./... -v
```

Expected: all tests PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/user/handler.go
git commit -m "feat(user): notify TCP server on progress update (fire-and-forget)"
```

---

## Task 6: End-to-end integration test (manual)

- [ ] **Step 1: Start both servers**

Terminal 1:
```bash
go run ./cmd/api-server/
```

Terminal 2:
```bash
go run ./cmd/tcp-server/
```

- [ ] **Step 2: Register user and get JWT**

```bash
curl -s -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username":"e2euser","email":"e2e@test.com","password":"password123"}'

TOKEN=$(curl -s -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"e2euser","password":"password123"}' | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
echo "Token: $TOKEN"
```

- [ ] **Step 3: Add manga to library**

```bash
curl -s -X POST http://localhost:8080/users/library \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"manga_id":"one-piece","status":"reading","current_chapter":1}'
```

- [ ] **Step 4: Connect TCP client and authenticate**

Terminal 3 (keep open):
```bash
nc localhost 9090
```
Type and press Enter:
```json
{"type":"auth","token":"<paste $TOKEN here>"}
```
Expected response:
```json
{"type":"auth_ok","user_id":"usr_xxxxxxxx"}
```

- [ ] **Step 5: Update progress via HTTP and observe TCP push**

Terminal 4:
```bash
curl -s -X PUT http://localhost:8080/users/progress \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"manga_id":"one-piece","current_chapter":95}'
```

Expected: Terminal 3 (nc) receives:
```json
{"type":"progress_update","user_id":"usr_xxxxxxxx","manga_id":"one-piece","chapter":95,"timestamp":1745800000}
```

- [ ] **Step 6: Final commit if any cleanup needed**

```bash
go test ./...
git add -A
git commit -m "feat: complete TCP progress sync server (Week 3-4)"
```

---

## Self-Review

**Spec coverage check:**

| Requirement | Task |
|------------|------|
| Accept multiple TCP connections | Task 1 — `Run()` loops on `ln.Accept()`, goroutine per conn |
| Broadcast progress updates to connected clients | Task 1 — `BroadcastToUser` |
| Handle client connections and disconnections | Task 1 — `Register`/`Unregister` + `defer` in `handleConn` |
| Basic JSON message protocol | Task 1 — newline-delimited JSON via `writeMsg` |
| Simple concurrent connection handling with goroutines | Task 1 — `go s.handleConn(conn)` |
| UC-007: Client sends auth, server validates | Task 2 — `handleConn` auth handshake |
| UC-007 A1: Auth fails → close connection | Task 2 — `validateJWT` error path |
| UC-007 A2: Server at capacity → reject | Task 2 — capacity check with `MaxConnections` |
| UC-008: Broadcast via channel | Task 3 — `Broadcast chan` → `BroadcastToUser` |
| UC-008 A1: Connection lost → remove from list | Task 1 — `defer Unregister` in `handleConn` |
| UC-008 A2: Send fails → log and continue | Task 1 — error path in `BroadcastToUser` |
| HTTP API triggers TCP broadcast | Task 5 — `go notifyTCPServer(...)` in `UpdateProgress` |

**No gaps found.**
