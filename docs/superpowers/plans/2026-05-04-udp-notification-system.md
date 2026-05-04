# UDP Notification System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a standalone UDP notification server that lets clients subscribe to chapter-release notifications filtered by manga ID, triggered via an internal HTTP endpoint on `:9094`.

**Architecture:** `NotificationServer` in `internal/udp/` manages a `map[string]clientEntry` keyed on `addr.String()`. A single run loop dispatches UDP register/unregister packets and drains a `Notify chan NotifyRequest` for broadcast fan-out. An HTTP handler feeds `NotifyRequest` values into that channel. `cmd/udp-server` wires both listeners with graceful SIGTERM/SIGINT shutdown; `cmd/udp-client` is a subscribe-only demo.

**Tech Stack:** Go stdlib (`net`, `encoding/json`, `sync`, `strconv`, `os/signal`), `github.com/stretchr/testify` (already in go.mod)

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/udp/server.go` | Create | All types, `New()`, `register()`, `unregister()`, `clientCount()`, `Run()`, `Shutdown()`, `handlePacket()`, `sendAck()`, `broadcast()`, `matchesFilter()` |
| `internal/udp/handler.go` | Create | `InternalHandler()` — HTTP mux with `POST /internal/notify` |
| `internal/udp/server_test.go` | Create | Unit tests (register/unregister) + integration tests (real UDPConn on random port) |
| `cmd/udp-server/main.go` | Create | Entry point: UDP listener + internal HTTP server, graceful shutdown |
| `cmd/udp-client/main.go` | Create | CLI demo: register with manga filter, print notifications, unregister on SIGINT |

---

### Task 1: Server scaffold — types, New(), register/unregister

**Files:**
- Create: `internal/udp/server_test.go`
- Create: `internal/udp/server.go`

- [ ] **Step 1: Write failing unit tests**

Create `internal/udp/server_test.go`:

```go
package udp

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustResolve(t *testing.T, addr string) *net.UDPAddr {
	t.Helper()
	a, err := net.ResolveUDPAddr("udp", addr)
	require.NoError(t, err)
	return a
}

func TestNew(t *testing.T) {
	srv := New("9091")
	assert.Equal(t, "9091", srv.Port)
	assert.NotNil(t, srv.clients)
	assert.NotNil(t, srv.Notify)
	assert.NotNil(t, srv.done)
}

func TestRegister(t *testing.T) {
	srv := New("0")
	addr := mustResolve(t, "127.0.0.1:10001")
	srv.register(addr, []string{"one-piece"})
	assert.Equal(t, 1, srv.clientCount())
	srv.mu.RLock()
	e := srv.clients[addr.String()]
	srv.mu.RUnlock()
	assert.Equal(t, []string{"one-piece"}, e.Filter)
}

func TestRegister_Upsert(t *testing.T) {
	srv := New("0")
	addr := mustResolve(t, "127.0.0.1:10001")
	srv.register(addr, []string{"one-piece"})
	srv.register(addr, []string{"naruto"})
	assert.Equal(t, 1, srv.clientCount())
	srv.mu.RLock()
	e := srv.clients[addr.String()]
	srv.mu.RUnlock()
	assert.Equal(t, []string{"naruto"}, e.Filter)
}

func TestUnregister(t *testing.T) {
	srv := New("0")
	addr := mustResolve(t, "127.0.0.1:10001")
	srv.register(addr, []string{"one-piece"})
	srv.unregister(addr)
	assert.Equal(t, 0, srv.clientCount())
}

func TestUnregister_NoOp(t *testing.T) {
	srv := New("0")
	addr := mustResolve(t, "127.0.0.1:10001")
	srv.unregister(addr) // must not panic
}
```

- [ ] **Step 2: Run tests — expect compile failure**

```
go test ./internal/udp/...
```
Expected: compile error — package `udp` does not exist yet.

- [ ] **Step 3: Create server.go with types, New(), register/unregister**

Create `internal/udp/server.go`:

```go
package udp

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strconv"
	"sync"
	"time"
)

type clientEntry struct {
	Addr   *net.UDPAddr
	Filter []string // manga IDs; empty = all manga
}

// NotificationServer manages registered UDP subscribers and fans out notifications.
type NotificationServer struct {
	Port      string
	clients   map[string]clientEntry // key: addr.String()
	Notify    chan NotifyRequest
	mu        sync.RWMutex
	conn      *net.UDPConn
	done      chan struct{}
	closeOnce sync.Once
}

// NotifyRequest is the payload the HTTP handler sends into the Notify channel.
type NotifyRequest struct {
	MangaID string `json:"manga_id"`
	Message string `json:"message"`
}

// New creates a NotificationServer with a buffered Notify channel (capacity 100).
func New(port string) *NotificationServer {
	return &NotificationServer{
		Port:    port,
		clients: make(map[string]clientEntry),
		Notify:  make(chan NotifyRequest, 100),
		done:    make(chan struct{}),
	}
}

func (s *NotificationServer) register(addr *net.UDPAddr, mangaIDs []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients[addr.String()] = clientEntry{Addr: addr, Filter: mangaIDs}
}

func (s *NotificationServer) unregister(addr *net.UDPAddr) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.clients, addr.String())
}

func (s *NotificationServer) clientCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.clients)
}

// --- packet types, Run, Shutdown, broadcast defined in later steps ---
var (
	_ = json.Marshal
	_ = fmt.Sprintf
	_ = log.Printf
	_ = net.ListenUDP
	_ = strconv.Atoi
	_ = time.Now
)
```

- [ ] **Step 4: Run tests — expect PASS**

```
go test ./internal/udp/... -run "TestNew|TestRegister|TestUnregister" -v
```
Expected: PASS (5 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/udp/server.go internal/udp/server_test.go
git commit -m "feat(udp): scaffold NotificationServer — types, New(), register/unregister"
```

---

### Task 2: Run() — packet read loop with register/unregister/ack

**Files:**
- Modify: `internal/udp/server.go`
- Modify: `internal/udp/server_test.go`

- [ ] **Step 1: Add test helpers and integration tests to server_test.go**

Add these imports to the import block in `internal/udp/server_test.go` (replace the existing import block):

```go
import (
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)
```

Then append the following to `internal/udp/server_test.go`:

```go
// startTestServer spins up a real server on a random OS-assigned port.
// Registers t.Cleanup to call Shutdown() when the test ends.
func startTestServer(t *testing.T) (*NotificationServer, *net.UDPAddr) {
	t.Helper()
	srv := New("0")
	go srv.Run()
	time.Sleep(20 * time.Millisecond) // let Run() bind
	t.Cleanup(srv.Shutdown)
	srv.mu.RLock()
	addr := srv.conn.LocalAddr().(*net.UDPAddr)
	srv.mu.RUnlock()
	return srv, addr
}

// newTestClient creates a UDP socket and registers cleanup.
func newTestClient(t *testing.T) *net.UDPConn {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{})
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	return conn
}

// sendPkt marshals v as JSON and sends it to dst.
func sendPkt(t *testing.T, conn *net.UDPConn, dst *net.UDPAddr, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	_, err = conn.WriteToUDP(data, dst)
	require.NoError(t, err)
}

// recvPkt reads one UDP packet (500 ms deadline) and returns it as a map.
func recvPkt(t *testing.T, conn *net.UDPConn) map[string]any {
	t.Helper()
	conn.SetDeadline(time.Now().Add(500 * time.Millisecond))
	defer conn.SetDeadline(time.Time{})
	buf := make([]byte, 4096)
	n, _, err := conn.ReadFromUDP(buf)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(buf[:n], &m))
	return m
}

// noPacket asserts that no UDP packet arrives within 100 ms.
func noPacket(t *testing.T, conn *net.UDPConn) {
	t.Helper()
	conn.SetDeadline(time.Now().Add(100 * time.Millisecond))
	defer conn.SetDeadline(time.Time{})
	buf := make([]byte, 4096)
	_, _, err := conn.ReadFromUDP(buf)
	require.Error(t, err, "expected no packet but one arrived")
}

func TestRun_Register_SendsAck(t *testing.T) {
	srv, srvAddr := startTestServer(t)
	client := newTestClient(t)

	sendPkt(t, client, srvAddr, map[string]any{
		"type":      "register",
		"manga_ids": []string{"one-piece", "naruto"},
	})

	msg := recvPkt(t, client)
	assert.Equal(t, "ack", msg["type"])
	assert.Equal(t, "registered for 2 manga", msg["message"])
	assert.Equal(t, 1, srv.clientCount())
}

func TestRun_Register_EmptyFilter_SendsAck(t *testing.T) {
	_, srvAddr := startTestServer(t)
	client := newTestClient(t)

	sendPkt(t, client, srvAddr, map[string]any{"type": "register", "manga_ids": []string{}})

	msg := recvPkt(t, client)
	assert.Equal(t, "ack", msg["type"])
	assert.Equal(t, "registered for all manga", msg["message"])
}

func TestRun_Unregister_SendsAck(t *testing.T) {
	srv, srvAddr := startTestServer(t)
	client := newTestClient(t)

	sendPkt(t, client, srvAddr, map[string]any{"type": "register", "manga_ids": []string{"one-piece"}})
	recvPkt(t, client) // consume ack

	sendPkt(t, client, srvAddr, map[string]any{"type": "unregister"})
	msg := recvPkt(t, client)
	assert.Equal(t, "ack", msg["type"])
	assert.Equal(t, "unregistered", msg["message"])
	assert.Equal(t, 0, srv.clientCount())
}

func TestRun_UnknownType_Ignored(t *testing.T) {
	_, srvAddr := startTestServer(t)
	client := newTestClient(t)

	sendPkt(t, client, srvAddr, map[string]any{"type": "bogus"})
	noPacket(t, client)
}

func TestRun_MalformedJSON_Ignored(t *testing.T) {
	_, srvAddr := startTestServer(t)
	client := newTestClient(t)

	data := []byte("not json at all")
	_, err := client.WriteToUDP(data, srvAddr)
	require.NoError(t, err)
	noPacket(t, client)
}
```

- [ ] **Step 2: Run tests — expect compile failure**

```
go test ./internal/udp/... -run "TestRun"
```
Expected: compile error — `Run` and `Shutdown` undefined.

- [ ] **Step 3: Replace server.go blank import stubs and add Run/Shutdown/handlePacket/sendAck**

Replace the entire `internal/udp/server.go` with:

```go
package udp

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strconv"
	"sync"
	"time"
)

type clientEntry struct {
	Addr   *net.UDPAddr
	Filter []string // manga IDs; empty = all manga
}

// NotificationServer manages registered UDP subscribers and fans out notifications.
type NotificationServer struct {
	Port      string
	clients   map[string]clientEntry // key: addr.String()
	Notify    chan NotifyRequest
	mu        sync.RWMutex
	conn      *net.UDPConn
	done      chan struct{}
	closeOnce sync.Once
}

// NotifyRequest is the payload the HTTP handler sends into the Notify channel.
type NotifyRequest struct {
	MangaID string `json:"manga_id"`
	Message string `json:"message"`
}

func New(port string) *NotificationServer {
	return &NotificationServer{
		Port:    port,
		clients: make(map[string]clientEntry),
		Notify:  make(chan NotifyRequest, 100),
		done:    make(chan struct{}),
	}
}

func (s *NotificationServer) register(addr *net.UDPAddr, mangaIDs []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients[addr.String()] = clientEntry{Addr: addr, Filter: mangaIDs}
}

func (s *NotificationServer) unregister(addr *net.UDPAddr) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.clients, addr.String())
}

func (s *NotificationServer) clientCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.clients)
}

// udpPkt is an inbound UDP datagram with its sender address.
type udpPkt struct {
	data []byte
	addr *net.UDPAddr
}

// inPkt is the JSON shape of client→server packets.
type inPkt struct {
	Type     string   `json:"type"`
	MangaIDs []string `json:"manga_ids"`
}

// outPkt is the JSON shape of server→client packets.
type outPkt struct {
	Type      string `json:"type"`
	Message   string `json:"message,omitempty"`
	MangaID   string `json:"manga_id,omitempty"`
	Timestamp int64  `json:"timestamp,omitempty"`
}

// Run opens the UDP listener and processes packets and Notify requests until Shutdown.
func (s *NotificationServer) Run() {
	port, _ := strconv.Atoi(s.Port)
	conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: port})
	if err != nil {
		log.Fatalf("udp: listen: %v", err)
	}
	s.mu.Lock()
	s.conn = conn
	s.mu.Unlock()
	defer conn.Close()
	log.Printf("udp: listening on %s", conn.LocalAddr())

	pktCh := make(chan udpPkt, 256)
	go func() {
		buf := make([]byte, 65535)
		for {
			n, addr, err := conn.ReadFromUDP(buf)
			if err != nil {
				select {
				case <-s.done:
				default:
					log.Printf("udp: read error: %v", err)
				}
				return
			}
			data := make([]byte, n)
			copy(data, buf[:n])
			select {
			case pktCh <- udpPkt{data: data, addr: addr}:
			case <-s.done:
				return
			}
		}
	}()

	for {
		select {
		case pkt := <-pktCh:
			s.handlePacket(pkt.data, pkt.addr)
		case req := <-s.Notify:
			s.broadcast(req)
		case <-s.done:
			return
		}
	}
}

// Shutdown closes the UDP connection and signals the run loop to stop.
func (s *NotificationServer) Shutdown() {
	s.closeOnce.Do(func() {
		log.Println("udp: shutting down...")
		close(s.done)
		s.mu.Lock()
		if s.conn != nil {
			s.conn.Close()
		}
		s.mu.Unlock()
	})
}

func (s *NotificationServer) handlePacket(data []byte, addr *net.UDPAddr) {
	var pkt inPkt
	if err := json.Unmarshal(data, &pkt); err != nil {
		log.Printf("udp: decode error from %s: %v", addr, err)
		return
	}
	switch pkt.Type {
	case "register":
		s.register(addr, pkt.MangaIDs)
		var msg string
		if len(pkt.MangaIDs) == 0 {
			msg = "registered for all manga"
		} else {
			msg = fmt.Sprintf("registered for %d manga", len(pkt.MangaIDs))
		}
		s.sendAck(addr, msg)
	case "unregister":
		s.unregister(addr)
		s.sendAck(addr, "unregistered")
	default:
		log.Printf("udp: unknown type %q from %s", pkt.Type, addr)
	}
}

func (s *NotificationServer) sendAck(addr *net.UDPAddr, message string) {
	data, _ := json.Marshal(outPkt{Type: "ack", Message: message})
	s.mu.RLock()
	conn := s.conn
	s.mu.RUnlock()
	if _, err := conn.WriteToUDP(data, addr); err != nil {
		log.Printf("udp: ack to %s failed: %v", addr, err)
	}
}

// broadcast is a placeholder — implemented in Task 3.
func (s *NotificationServer) broadcast(_ NotifyRequest) {}

// matchesFilter is a placeholder — implemented in Task 3.
func matchesFilter(_ []string, _ string) bool { return false }

// keep time imported until Task 3 uses it
var _ = time.Now
```

- [ ] **Step 4: Run tests — expect PASS**

```
go test ./internal/udp/... -run "TestNew|TestRegister|TestUnregister|TestRun" -v
```
Expected: PASS (10 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/udp/server.go internal/udp/server_test.go
git commit -m "feat(udp): implement Run() loop with register/unregister/ack packet handling"
```

---

### Task 3: Broadcast fan-out via Notify channel

**Files:**
- Modify: `internal/udp/server.go`
- Modify: `internal/udp/server_test.go`

- [ ] **Step 1: Write broadcast tests**

Append to `internal/udp/server_test.go`:

```go
func TestBroadcast_MatchingSubscriber(t *testing.T) {
	srv, srvAddr := startTestServer(t)
	client := newTestClient(t)

	sendPkt(t, client, srvAddr, map[string]any{"type": "register", "manga_ids": []string{"one-piece"}})
	recvPkt(t, client) // consume ack

	srv.Notify <- NotifyRequest{MangaID: "one-piece", Message: "Chapter 1101 released!"}

	msg := recvPkt(t, client)
	assert.Equal(t, "notification", msg["type"])
	assert.Equal(t, "one-piece", msg["manga_id"])
	assert.Equal(t, "Chapter 1101 released!", msg["message"])
	assert.NotZero(t, msg["timestamp"])
}

func TestBroadcast_NonMatchingSubscriber(t *testing.T) {
	srv, srvAddr := startTestServer(t)
	client := newTestClient(t)

	sendPkt(t, client, srvAddr, map[string]any{"type": "register", "manga_ids": []string{"naruto"}})
	recvPkt(t, client)

	srv.Notify <- NotifyRequest{MangaID: "one-piece", Message: "Chapter 1101 released!"}
	noPacket(t, client)
}

func TestBroadcast_EmptyFilter_ReceivesAll(t *testing.T) {
	srv, srvAddr := startTestServer(t)
	client := newTestClient(t)

	sendPkt(t, client, srvAddr, map[string]any{"type": "register", "manga_ids": []string{}})
	recvPkt(t, client)

	srv.Notify <- NotifyRequest{MangaID: "one-piece", Message: "new chapter"}

	msg := recvPkt(t, client)
	assert.Equal(t, "notification", msg["type"])
	assert.Equal(t, "one-piece", msg["manga_id"])
}

func TestBroadcast_MultipleClients_OnlyMatchingNotified(t *testing.T) {
	srv, srvAddr := startTestServer(t)
	c1 := newTestClient(t) // subscribes to one-piece
	c2 := newTestClient(t) // subscribes to naruto

	sendPkt(t, c1, srvAddr, map[string]any{"type": "register", "manga_ids": []string{"one-piece"}})
	recvPkt(t, c1)
	sendPkt(t, c2, srvAddr, map[string]any{"type": "register", "manga_ids": []string{"naruto"}})
	recvPkt(t, c2)

	srv.Notify <- NotifyRequest{MangaID: "one-piece", Message: "new chapter"}

	msg := recvPkt(t, c1)
	assert.Equal(t, "notification", msg["type"])
	assert.Equal(t, "one-piece", msg["manga_id"])

	noPacket(t, c2)
}
```

- [ ] **Step 2: Run tests — expect FAIL**

```
go test ./internal/udp/... -run "TestBroadcast" -v
```
Expected: FAIL — `recvPkt` times out because `broadcast` is a no-op.

- [ ] **Step 3: Replace broadcast() and matchesFilter() placeholders in server.go**

Replace in `internal/udp/server.go`:

```go
// broadcast sends a notification packet to all subscribers whose filter matches req.MangaID.
// On write failure the subscriber is removed from the map (stale client cleanup).
func (s *NotificationServer) broadcast(req NotifyRequest) {
	data, _ := json.Marshal(outPkt{
		Type:      "notification",
		MangaID:   req.MangaID,
		Message:   req.Message,
		Timestamp: time.Now().Unix(),
	})
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, entry := range s.clients {
		if !matchesFilter(entry.Filter, req.MangaID) {
			continue
		}
		if _, err := s.conn.WriteToUDP(data, entry.Addr); err != nil {
			log.Printf("udp: write to %s failed: %v — removing", key, err)
			delete(s.clients, key)
		}
	}
}

func matchesFilter(filter []string, mangaID string) bool {
	if len(filter) == 0 {
		return true
	}
	for _, id := range filter {
		if id == mangaID {
			return true
		}
	}
	return false
}
```

Also remove the `var _ = time.Now` stub line — `time` is now used directly in `broadcast`.

- [ ] **Step 4: Run tests — expect PASS**

```
go test ./internal/udp/... -run "TestBroadcast" -v
```
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/udp/server.go internal/udp/server_test.go
git commit -m "feat(udp): implement broadcast fan-out with manga filter matching"
```

---

### Task 4: Stale client removal + Shutdown

**Files:**
- Modify: `internal/udp/server_test.go`

- [ ] **Step 1: Write stale client test**

Append to `internal/udp/server_test.go`:

```go
func TestBroadcast_StaleClient_RemovedAndOthersNotified(t *testing.T) {
	srv, srvAddr := startTestServer(t)
	goodClient := newTestClient(t)

	sendPkt(t, goodClient, srvAddr, map[string]any{"type": "register", "manga_ids": []string{"one-piece"}})
	recvPkt(t, goodClient) // consume ack

	// Inject a stale entry: open a UDP socket, note its address, then close it.
	// Writes to this address will fail (OS reports port unreachable after close).
	staleConn, err := net.ListenUDP("udp", &net.UDPAddr{})
	require.NoError(t, err)
	staleAddr := staleConn.LocalAddr().(*net.UDPAddr)
	staleConn.Close()

	srv.mu.Lock()
	srv.clients[staleAddr.String()] = clientEntry{Addr: staleAddr, Filter: []string{"one-piece"}}
	srv.mu.Unlock()

	// First broadcast: good client must receive; stale write may or may not fail immediately.
	srv.Notify <- NotifyRequest{MangaID: "one-piece", Message: "Chapter 1101 released!"}
	msg := recvPkt(t, goodClient)
	assert.Equal(t, "notification", msg["type"])

	// Second broadcast: if stale client wasn't removed on the first attempt (UDP ICMP is
	// asynchronous on some platforms), it will fail here and be removed.
	if srv.clientCount() == 2 {
		srv.Notify <- NotifyRequest{MangaID: "one-piece", Message: "retry"}
		recvPkt(t, goodClient)
	}

	assert.Equal(t, 1, srv.clientCount(), "stale client must be removed from map")
}
```

- [ ] **Step 2: Write Shutdown tests**

Append to `internal/udp/server_test.go`:

```go
func TestShutdown_StopsRun(t *testing.T) {
	srv := New("0")
	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.Run()
	}()
	time.Sleep(20 * time.Millisecond)
	srv.Shutdown()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Run() did not stop after Shutdown()")
	}
}

func TestShutdown_Idempotent(t *testing.T) {
	srv := New("0")
	go srv.Run()
	time.Sleep(20 * time.Millisecond)
	srv.Shutdown()
	srv.Shutdown() // must not panic
}
```

- [ ] **Step 3: Run all tests**

```
go test ./internal/udp/... -v -count=1 -race
```
Expected: all PASS, no data races detected.

- [ ] **Step 4: Commit**

```bash
git add internal/udp/server_test.go
git commit -m "test(udp): add stale client removal and shutdown tests"
```

---

### Task 5: HTTP handler

**Files:**
- Create: `internal/udp/handler.go`
- Modify: `internal/udp/server_test.go`

- [ ] **Step 1: Write HTTP handler tests**

Add `"bytes"` and `"net/http"` and `"net/http/httptest"` to the import block in `internal/udp/server_test.go`:

```go
import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)
```

Then append to `internal/udp/server_test.go`:

```go
func TestInternalHandler_ValidPayload(t *testing.T) {
	srv := New("0")
	body := `{"manga_id":"one-piece","message":"Chapter 1101 released!"}`
	req := httptest.NewRequest(http.MethodPost, "/internal/notify", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.InternalHandler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	select {
	case got := <-srv.Notify:
		assert.Equal(t, "one-piece", got.MangaID)
		assert.Equal(t, "Chapter 1101 released!", got.Message)
	default:
		t.Fatal("expected NotifyRequest on channel but got none")
	}
}

func TestInternalHandler_InvalidJSON(t *testing.T) {
	srv := New("0")
	req := httptest.NewRequest(http.MethodPost, "/internal/notify", bytes.NewBufferString("not json"))
	w := httptest.NewRecorder()
	srv.InternalHandler().ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestInternalHandler_ChannelFull(t *testing.T) {
	srv := New("0")
	for i := 0; i < 100; i++ {
		srv.Notify <- NotifyRequest{MangaID: "one-piece", Message: "fill"}
	}
	req := httptest.NewRequest(http.MethodPost, "/internal/notify",
		bytes.NewBufferString(`{"manga_id":"one-piece","message":"overflow"}`))
	w := httptest.NewRecorder()
	srv.InternalHandler().ServeHTTP(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestInternalHandler_ServerShuttingDown(t *testing.T) {
	srv := New("0")
	close(srv.done) // simulate shutdown without calling Shutdown() to avoid close-of-nil conn
	req := httptest.NewRequest(http.MethodPost, "/internal/notify",
		bytes.NewBufferString(`{"manga_id":"one-piece","message":"test"}`))
	w := httptest.NewRecorder()
	srv.InternalHandler().ServeHTTP(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestInternalHandler_MethodNotAllowed(t *testing.T) {
	srv := New("0")
	req := httptest.NewRequest(http.MethodGet, "/internal/notify", nil)
	w := httptest.NewRecorder()
	srv.InternalHandler().ServeHTTP(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}
```

- [ ] **Step 2: Run tests — expect compile failure**

```
go test ./internal/udp/... -run "TestInternalHandler"
```
Expected: compile error — `InternalHandler` undefined.

- [ ] **Step 3: Create handler.go**

Create `internal/udp/handler.go`:

```go
package udp

import (
	"encoding/json"
	"net/http"
)

// InternalHandler returns an HTTP handler for the internal notify endpoint.
// Only POST /internal/notify is accepted.
func (s *NotificationServer) InternalHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/notify", s.handleNotify)
	return mux
}

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
```

- [ ] **Step 4: Run all tests**

```
go test ./internal/udp/... -v -count=1 -race
```
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/udp/handler.go internal/udp/server_test.go
git commit -m "feat(udp): add internal HTTP handler POST /internal/notify"
```

---

### Task 6: cmd/udp-server/main.go

**Files:**
- Create: `cmd/udp-server/main.go`

- [ ] **Step 1: Implement entry point**

Create `cmd/udp-server/main.go`:

```go
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mangahub/internal/udp"
)

func main() {
	port := os.Getenv("UDP_PORT")
	if port == "" {
		port = "9091"
	}
	internalAddr := os.Getenv("UDP_INTERNAL_ADDR")
	if internalAddr == "" {
		internalAddr = ":9094"
	}

	srv := udp.New(port)

	httpSrv := &http.Server{Addr: internalAddr, Handler: srv.InternalHandler()}
	go func() {
		log.Printf("udp: internal HTTP on %s", internalAddr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("udp: internal HTTP: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		srv.Shutdown()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(ctx); err != nil {
			log.Printf("udp: internal HTTP shutdown: %v", err)
		}
	}()

	srv.Run()
	log.Println("udp: server stopped")
}
```

- [ ] **Step 2: Build to verify compilation**

```
go build ./cmd/udp-server/...
```
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add cmd/udp-server/main.go
git commit -m "feat(udp): add cmd/udp-server entry point with graceful shutdown"
```

---

### Task 7: cmd/udp-client/main.go

**Files:**
- Create: `cmd/udp-client/main.go`

- [ ] **Step 1: Implement subscribe-only UDP client**

Create `cmd/udp-client/main.go`:

```go
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

func main() {
	mangaFlag := flag.String("manga-ids", "", "comma-separated manga IDs to subscribe to (empty = all)")
	flag.Parse()

	serverAddr := os.Getenv("UDP_SERVER_ADDR")
	if serverAddr == "" {
		serverAddr = "localhost:9091"
	}

	srv, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		log.Fatalf("udp-client: resolve %s: %v", serverAddr, err)
	}

	conn, err := net.ListenUDP("udp", &net.UDPAddr{})
	if err != nil {
		log.Fatalf("udp-client: listen: %v", err)
	}
	defer conn.Close()

	var mangaIDs []string
	if *mangaFlag != "" {
		mangaIDs = strings.Split(*mangaFlag, ",")
	}

	payload, _ := json.Marshal(map[string]any{"type": "register", "manga_ids": mangaIDs})
	if _, err := conn.WriteToUDP(payload, srv); err != nil {
		log.Fatalf("udp-client: register: %v", err)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		unreg, _ := json.Marshal(map[string]string{"type": "unregister"})
		conn.WriteToUDP(unreg, srv) //nolint:errcheck
		conn.Close()
	}()

	fmt.Printf("subscribed to %v — waiting for notifications (Ctrl+C to quit)...\n", mangaIDs)

	buf := make([]byte, 4096)
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			return // closed by signal handler
		}
		var msg map[string]any
		if err := json.Unmarshal(buf[:n], &msg); err != nil {
			continue
		}
		switch msg["type"] {
		case "notification":
			fmt.Printf("[notification] %s — %s\n", msg["manga_id"], msg["message"])
		case "ack":
			fmt.Printf("[ack] %s\n", msg["message"])
		}
	}
}
```

- [ ] **Step 2: Build to verify compilation**

```
go build ./cmd/udp-client/...
```
Expected: no errors.

- [ ] **Step 3: Run full test suite with race detector**

```
go test ./internal/udp/... -v -count=1 -race
```
Expected: all PASS, zero races.

- [ ] **Step 4: Commit**

```bash
git add cmd/udp-client/main.go
git commit -m "feat(udp): add cmd/udp-client subscribe-only demo tool"
```

---

## Demo

After all tasks are complete, verify the full flow:

```bash
# Terminal 1
go run ./cmd/udp-server

# Terminal 2
go run ./cmd/udp-client --manga-ids one-piece

# Terminal 3 — triggers the notification
curl -X POST http://localhost:9094/internal/notify \
  -H "Content-Type: application/json" \
  -d '{"manga_id":"one-piece","message":"Chapter 1101 released!"}'

# Terminal 2 should print:
# [notification] one-piece — Chapter 1101 released!
```
