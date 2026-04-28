# TCP Progress Sync Server — Design Spec

**Date:** 2026-04-28
**Phase:** Week 3–4 (Phase 2 — Network Protocols)
**Points:** 20 pts

---

## Overview

A TCP server that maintains persistent connections from clients and pushes reading progress updates to the correct user's connections. When a user updates their progress via the HTTP API, the API notifies the TCP server via an internal HTTP endpoint, which then pushes the update to all active TCP connections belonging to that user.

---

## Architecture

Two binaries remain separate per spec:

```
cmd/api-server/    → HTTP API (:8080)       — existing
cmd/tcp-server/    → TCP listener (:9090)   — new
                     Internal HTTP (:9099)  — new (broadcast trigger)
```

**Broadcast flow:**

```
User → PUT /users/progress (:8080)
     → Update DB (existing)
     → POST localhost:9099/internal/broadcast  (fire-and-forget, 1s timeout)
          → ProgressSyncServer.Broadcast channel
               → find conn by user_id in Connections map
                    → write JSON to net.Conn
```

**New packages:**

```
internal/tcp/
    server.go    — ProgressSyncServer, Run(), Register(), Unregister(), BroadcastToUser()
    handler.go   — internal HTTP handler (POST /internal/broadcast)
cmd/tcp-server/
    main.go      — starts TCP listener and internal HTTP server
```

---

## Data Structures

As defined in the project spec, extended with `MaxConnections` to support UC-007 A2 (capacity rejection):

```go
type ProgressSyncServer struct {
    Port           string
    Connections    map[string]net.Conn  // user_id → single active connection
    Broadcast      chan ProgressUpdate
    MaxConnections int                  // default 30 (spec target: 20–30 concurrent TCP connections)
}

type ProgressUpdate struct {
    UserID    string `json:"user_id"`
    MangaID   string `json:"manga_id"`
    Chapter   int    `json:"chapter"`
    Timestamp int64  `json:"timestamp"`
}
```

`Connections` maps one `user_id` to one `net.Conn`. If the same user reconnects, the new connection replaces the old one.

---

## Message Protocol

All TCP messages are newline-delimited JSON (`\n` terminated).

**Client → Server (on connect, must arrive within 5s):**
```json
{"type": "auth", "token": "<jwt>"}
```

**Server → Client (auth response):**
```json
{"type": "auth_ok", "user_id": "usr_abc123"}
{"type": "auth_error", "message": "invalid token"}
```

**Server → Client (progress push):**
```json
{"type": "progress_update", "user_id": "usr_abc123", "manga_id": "one-piece", "chapter": 95, "timestamp": 1745800000}
```

JWT validation reuses the same secret as the HTTP API (`JWT_SECRET` env var).

---

## Concurrency

- One goroutine per TCP client connection (handles auth handshake + keeps connection alive)
- `ProgressSyncServer.Connections` protected by `sync.RWMutex`
- `Broadcast` channel consumed by the server's main Run() goroutine
- Internal HTTP server runs in its own goroutine

---

## Error Handling

| Scenario | Behavior | Source |
|----------|----------|--------|
| Server at capacity (`len(Connections) >= MaxConnections`) | Send `{"type":"error","message":"server at capacity"}`, close connection | UC-007 A2 |
| Client fails to auth within 5s | Close connection, log warning | our decision |
| Invalid JWT | Send `auth_error`, close connection | UC-007 A1 |
| Write to dead connection | Remove from `Connections`, log, continue | UC-008 A2 |
| Client disconnects | `Unregister` cleans up map entry | UC-008 A1 |
| TCP server down (HTTP API side) | Log warning, return HTTP 200 anyway — progress saved to DB | our decision (UC-006 A2 specifies queuing, out of scope for academic timeline) |
| Unknown `user_id` on broadcast | Return 200 silently — user not connected, not an error | our decision |
| Malformed broadcast payload | Return 400, log error | our decision |

HTTP API timeout to TCP server: **1 second** (non-blocking to end user).

---

## Integration with HTTP API

In `internal/user/handler.go`, after a successful DB update in `UpdateProgress`:

```go
go notifyTCPServer(update)  // fire-and-forget goroutine
```

```go
func notifyTCPServer(update ProgressUpdate) {
    body, _ := json.Marshal(update)
    client := &http.Client{Timeout: time.Second}
    client.Post("http://localhost:9099/internal/broadcast", "application/json", bytes.NewReader(body))
}
```

The TCP server address (`localhost:9099`) is read from `TCP_INTERNAL_ADDR` env var with fallback to `localhost:9099`.

---

## Ports

| Server | Port | Purpose |
|--------|------|---------|
| HTTP API | :8080 | Existing — user-facing REST API |
| TCP Sync | :9090 | Client TCP connections |
| TCP Internal | :9099 | Internal broadcast trigger (HTTP POST) |

---

## Out of Scope

- **Message queuing when TCP server is unavailable** — UC-006 A2 specifies queuing, but this is excluded for academic timeline. Fire-and-forget is used instead.
- Multiple connections per user — spec defines `map[string]net.Conn`, one connection per user
- TLS on TCP connections
- TCP client library / CLI demo client — can be tested with `nc` or `telnet`
