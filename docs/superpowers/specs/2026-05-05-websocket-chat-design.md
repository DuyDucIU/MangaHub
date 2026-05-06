# WebSocket Chat System — Design Spec

**Date:** 2026-05-05  
**Feature:** UC-011 / UC-012 / UC-013 — Real-time Chat System (core) + WebSocket Room Management (bonus)  
**Points:** 15 pts core + 10 pts bonus  

---

## 1. Scope

Implement a real-time WebSocket chat system integrated into the existing HTTP API server (`cmd/api-server`). Users connect to a per-manga chat room, send messages that are broadcast to all room participants, and receive join/leave notifications. Authentication uses JWT passed as a query parameter at upgrade time.

---

## 2. Architecture

The WebSocket feature is **integrated into `cmd/api-server`** (not a standalone binary). The API server creates a `ChatHub`, starts it as a goroutine, and registers the `/ws/chat` route on the existing Gin engine.

```
Browser Client
      │
      │  GET /ws/chat?token=<jwt>&manga_id=<id>   (HTTP → WS upgrade)
      ▼
cmd/api-server (Gin, :8080)
      │
      ├── internal/websocket/handler.go  (validates JWT, upgrades, registers client)
      │
      └── internal/websocket/hub.go      (ChatHub — owns all room state)
              │
              ├── rooms["one-piece"] → {client1, client2, ...}
              ├── rooms["naruto"]    → {client3, ...}
              └── rooms["general"]  → {client4, ...}
```

**New dependency:** `github.com/gorilla/websocket`  
**Shared utility extracted:** `pkg/jwtutil/jwtutil.go` — JWT validation currently duplicated between `internal/tcp` and `internal/auth`; extracted so WebSocket handler can reuse it cleanly.

---

## 3. File Map

| File | Purpose |
|---|---|
| `pkg/jwtutil/jwtutil.go` | Shared JWT validation used by TCP, auth, and WebSocket |
| `internal/websocket/hub.go` | `ChatHub` struct, `Run()` goroutine, room lifecycle |
| `internal/websocket/client.go` | `Client` struct, read pump goroutine, write pump goroutine |
| `internal/websocket/handler.go` | HTTP handler: JWT auth, WebSocket upgrade, client registration |
| `internal/websocket/hub_test.go` | Hub unit tests |
| `internal/websocket/handler_test.go` | Handler tests (auth rejection, upgrade) |
| `cmd/api-server/main.go` | Wire up `ChatHub`, register `GET /ws/chat` route |

---

## 4. Data Structures

### ChatHub

```go
type ChatHub struct {
    rooms      map[string]map[*Client]bool // manga_id → set of clients
    broadcast  chan ChatMessage
    register   chan *Client
    unregister chan *Client
}
```

Deviates from the spec's base `ChatHub` (which has a flat `Clients` map for a single room) by using a nested rooms map. This combines the base requirement with the bonus room management feature. The spec's bonus `ChatRoom` struct is folded into the hub directly rather than being a separate type.

The hub also tracks a per-room message history ring buffer (last 20 messages), sent to each client on join to satisfy UC-011 step 6.

```go
type room struct {
    clients map[*Client]bool
    history []ChatMessage // ring buffer, cap 20
}

type ChatHub struct {
    rooms      map[string]*room
    broadcast  chan ChatMessage
    register   chan *Client
    unregister chan *Client
}
```

### Client

```go
type Client struct {
    hub      *ChatHub
    conn     *websocket.Conn
    send     chan []byte  // buffered; hub writes here, write pump drains it
    UserID   string
    Username string
    RoomID   string       // manga_id from query string; defaults to "general"
}
```

### ChatMessage

```go
type ChatMessage struct {
    Type      string `json:"type"`      // "message" | "join" | "leave"
    UserID    string `json:"user_id"`
    Username  string `json:"username"`
    RoomID    string `json:"room_id"`
    Message   string `json:"message,omitempty"`
    Timestamp int64  `json:"timestamp"`
}
```

`Type` field is an addition beyond the spec's `ChatMessage` definition — it enables join/leave events over the same channel without needing a separate event envelope.

---

## 5. Connection Lifecycle

### Upgrade & Auth (handler.go)

1. Extract `token` query param — missing or empty → HTTP 401, connection rejected
2. Validate JWT via `pkg/jwtutil` — invalid or expired → HTTP 401
3. Extract `manga_id` query param — empty → defaults to `"general"`
4. Upgrade HTTP connection to WebSocket (`gorilla/websocket` Upgrader)
5. Create `Client{UserID, Username, RoomID}`
6. Send `hub.register <- client`
7. Launch client's read pump and write pump goroutines

### Hub Run() Goroutine (hub.go)

Single goroutine owns all room state — no mutexes needed on the rooms map.

```
select {
case client := <-hub.register:
    add client to rooms[client.RoomID]
    send room history (last 20 msgs) to client.send
    broadcast join message to room

case client := <-hub.unregister:
    remove client from rooms[client.RoomID]
    close(client.send)
    if room is empty → delete room
    broadcast leave message to remaining clients

case msg := <-hub.broadcast:
    append msg to rooms[msg.RoomID].history (cap 20, drop oldest)
    for each client in rooms[msg.RoomID]:
        non-blocking send to client.send
        if send blocked → unregister client (slow/dead)
}
```

### Read Pump (per client goroutine)

- Max incoming message size: 512 bytes
- Pong handler resets read deadline to keep connection alive
- On read → wrap in `ChatMessage{Type: "message"}` → `hub.broadcast <- msg`
- On any error → `hub.unregister <- client` → goroutine exits

### Write Pump (per client goroutine)

- Drains `client.send` channel → writes JSON text frames to WebSocket
- Sends WebSocket ping every 54 seconds
- If `client.send` is closed by hub → closes WebSocket connection → goroutine exits

---

## 6. Room Management (Bonus)

- **Room key:** `manga_id` query parameter (string)
- **Default room:** `"general"` when no `manga_id` is provided
- **Auto-create:** room map entry is created when first client joins
- **Auto-delete:** room map entry is deleted when last client leaves
- **Isolation:** broadcast only fans out to clients in the same room; cross-room messages are impossible by design

---

## 7. Error Handling

| Scenario | Handling |
|---|---|
| Missing/invalid JWT | HTTP 401 before WebSocket upgrade |
| Empty manga_id | Defaults to "general" room |
| Message too long (>512 bytes) | Read deadline exceeded, client disconnected |
| Slow/unresponsive client | Non-blocking send; client dropped if send channel full |
| Client closes tab/disconnects | Read pump detects EOF → unregister → leave broadcast |
| Hub shutting down | Close all client send channels → write pumps exit cleanly |

---

## 8. Integration into cmd/api-server

```go
hub := websocket.NewHub()
go hub.Run()

wsHandler := &websocket.Handler{Hub: hub, JWTSecret: jwtSecret}
r.GET("/ws/chat", wsHandler.ServeWS)
```

No changes to existing routes or middleware. The WebSocket handler is a standard `gin.HandlerFunc`.

---

## 9. Testing

### hub_test.go
- Client registers → added to correct room
- New client receives history (last ≤20 messages) on join
- Client unregisters → removed, room deleted if empty
- Message broadcast → delivered only to clients in the same room
- History ring buffer caps at 20, oldest dropped
- Join/leave events broadcast correctly
- Slow client (full send channel) → dropped without blocking hub

### handler_test.go
- Missing token → HTTP 401
- Invalid/expired token → HTTP 401
- Valid token + valid manga_id → WebSocket upgrade succeeds
- Valid token + no manga_id → joins "general" room

---

## 10. Decisions Made Beyond the Spec

| Decision | Reason |
|---|---|
| Integrated into api-server instead of standalone binary | WebSocket is HTTP-upgraded; no reason for a separate process |
| JWT in query string (not first-message auth) | Natural for HTTP upgrade; simpler browser client code |
| Per-manga rooms instead of single global room | Covers bonus 10 pts with minimal extra code |
| `type` field on ChatMessage | Needed to distinguish message/join/leave events on one channel |
| `pkg/jwtutil` extraction | Avoids triplicate JWT validation code across tcp/auth/websocket |
| Default room "general" | Graceful fallback when manga_id not provided |
| Slow client drop (non-blocking send) | Prevents one lagging client from blocking the hub goroutine |
| In-memory ring buffer (last 20 msgs/room) | Satisfies UC-011 step 6 (user receives recent chat history on join) |
| `room` struct wrapping clients + history | Cleaner than two parallel maps; history naturally scoped to its room |
