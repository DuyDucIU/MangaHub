# UDP Notification System — Design Spec

**Date:** 2026-05-04
**Feature:** UDP chapter-release notification system (UC-009, UC-010)
**Spec points:** 15 pts (UDP Notifications)

---

## Overview

A standalone UDP server that lets clients subscribe to chapter-release notifications for specific manga. An admin triggers a broadcast via an internal HTTP endpoint (same pattern as the TCP server); the server fans the notification out via UDP only to subscribers whose filter includes that manga.

---

## Architecture

Five new files, fully mirroring the TCP layout:

```
cmd/udp-server/main.go      — server entry point, spins up UDP listener + internal HTTP server, SIGTERM/SIGINT graceful shutdown
cmd/udp-client/main.go      — subscribe-only demo tool
internal/udp/server.go      — NotificationServer core logic
internal/udp/handler.go     — internal HTTP handler (POST /internal/notify)
internal/udp/server_test.go — unit tests
```

`cmd/udp-server` runs two listeners in parallel:
- UDP `:9091` — client registration and notification delivery
- Internal HTTP `:9094` — admin broadcast trigger (same pattern as TCP's `:9099`)

---

## Data Structures

```go
// clientEntry holds a registered subscriber's address and manga filter.
type clientEntry struct {
    Addr   *net.UDPAddr
    Filter []string  // manga IDs; empty slice = all manga
}

// NotificationServer manages registered UDP clients and broadcasts notifications.
type NotificationServer struct {
    Port      string
    clients   map[string]clientEntry  // key: addr.String()
    Notify    chan NotifyRequest       // receives broadcast triggers from HTTP handler
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
```

`clients` is keyed on `addr.String()` so re-registration from the same address is an upsert (idempotent).

---

## UDP Packet Protocol

All packets are JSON objects. The `type` field is the discriminator.

### Client → Server (UDP)

| `type` | Additional fields | Purpose |
|--------|-------------------|---------|
| `register` | `manga_ids []string` | UC-009: subscribe; empty list = all manga |
| `unregister` | — | remove from registry |

### Server → Client (UDP)

| `type` | Additional fields | Purpose |
|--------|-------------------|---------|
| `ack` | `message string` | confirm register / unregister |
| `notification` | `manga_id string`, `message string`, `timestamp int64` | chapter release notification |

### Example UDP Exchange

```
# Subscribe
Client → Server:  {"type":"register","manga_ids":["one-piece","naruto"]}
Server → Client:  {"type":"ack","message":"registered for 2 manga"}

# Server delivers notification (triggered via HTTP)
Server → Client:  {"type":"notification","manga_id":"one-piece","message":"Chapter 1101 released!","timestamp":1714000000}

# Unsubscribe
Client → Server:  {"type":"unregister"}
Server → Client:  {"type":"ack","message":"unregistered"}
```

The `notification` packet satisfies the spec's `Notification` struct exactly (`type`, `manga_id`, `message`, `timestamp`).

---

## Internal HTTP Trigger

`POST /internal/notify` on `:9094` — mirrors TCP's `POST /internal/broadcast` on `:9099`.

**Request body:**
```json
{"manga_id": "one-piece", "message": "Chapter 1101 released!"}
```

**Response:** `200 OK` on success, `400` on invalid payload, `503` if notify channel is full or server is shutting down.

The handler decodes the request and sends a `NotifyRequest` into the `Notify` channel (buffered, capacity 100). The run loop picks it up and fans out UDP notifications to matching subscribers.

---

## Server Logic

`Run()` opens a `*net.UDPConn` on `UDP_PORT` (default `9091`) and loops on `ReadFromUDP`, also selecting on the `Notify` channel for broadcast triggers:

```
select:
  ReadFromUDP   → decode JSON → switch type:
                    "register"   → upsert clientEntry, send ack
                    "unregister" → delete clientEntry, send ack
                    unknown      → log and ignore
  <-Notify      → fan out notification to matching subscribers
  <-done        → exit
```

**Broadcast fan-out:** Iterate `clients`; for each entry where `Filter` contains `manga_id` or `Filter` is empty, call `WriteToUDP`. On write failure: log error, remove client from map, continue to remaining clients (UC-010 A1).

**Shutdown:** `closeOnce.Do` closes the `UDPConn` and signals `done`. The HTTP server is shut down separately in `cmd/udp-server/main.go` with a timeout context, identical to TCP's shutdown sequence.

---

## cmd/udp-client

Subscribe-only tool (no broadcast mode — admin uses curl or the HTTP handler directly):

```bash
go run ./cmd/udp-client --manga-ids one-piece,naruto
```

1. Sends `register` packet with the manga filter
2. Loops on `ReadFromUDP`, printing each `notification` as it arrives
3. On Ctrl+C (SIGINT): sends `unregister`, then exits

---

## Configuration

| Env var | Default | Used by |
|---------|---------|---------|
| `UDP_PORT` | `9091` | server (UDP listener) |
| `UDP_INTERNAL_ADDR` | `:9094` | server (internal HTTP) |
| `UDP_SERVER_ADDR` | `localhost:9091` | client |

---

## Error Handling

| Scenario | Handling |
|----------|----------|
| `WriteToUDP` fails (stale client) | Log, remove from map, continue broadcast |
| Unknown UDP packet `type` | Log `"udp: unknown type <type> from <addr>"`, ignore |
| Malformed UDP JSON | Log `"udp: decode error from <addr>: <err>"`, ignore |
| `ReadFromUDP` error on shutdown | Check `done` channel; if closed, exit cleanly |
| `ReadFromUDP` error mid-run | Log, continue loop |
| HTTP handler: invalid JSON body | Return `400 Bad Request` |
| HTTP handler: notify channel full | Return `503 Service Unavailable` |

---

## Testing (`internal/udp/server_test.go`)

Follows `internal/tcp/server_test.go` style — spins up a real `UDPConn` on a random port, no mocks.

| Test case | Asserts |
|-----------|---------|
| Register + ack | Client map updated, ack received |
| Broadcast to matching subscriber | `notification` packet received |
| Broadcast to non-matching subscriber | No packet received |
| Broadcast to multiple clients, one matching | Only matching client notified |
| Unregister | Client removed from map, ack received |
| Stale client on broadcast | Removed from map, other clients still notified |

---

## Demo Flow (UC-009 + UC-010)

```bash
# Terminal 1 — start server
go run ./cmd/udp-server

# Terminal 2 — subscribe to one-piece notifications
go run ./cmd/udp-client --manga-ids one-piece

# Terminal 3 — admin triggers a broadcast via HTTP
curl -X POST http://localhost:9094/internal/notify \
  -H "Content-Type: application/json" \
  -d '{"manga_id":"one-piece","message":"Chapter 1101 released!"}'

# Terminal 2 prints:
# [notification] one-piece — Chapter 1101 released!
```
