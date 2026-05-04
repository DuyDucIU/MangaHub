# UDP Notification System — Design Spec

**Date:** 2026-05-04
**Feature:** UDP chapter-release notification system (UC-009, UC-010)
**Spec points:** 15 pts (UDP Notifications)

---

## Overview

A standalone UDP server that lets clients subscribe to chapter-release notifications for specific manga. An admin client fires a broadcast trigger via a special UDP packet; the server fans the notification out only to subscribers whose filter includes that manga. Everything is pure UDP — no HTTP side-channel on the server process.

---

## Architecture

Three new files:

```
cmd/udp-server/main.go     — server entry point, SIGTERM/SIGINT graceful shutdown
cmd/udp-client/main.go     — two-mode demo tool (subscribe | broadcast)
internal/udp/server.go      — NotificationServer core logic
internal/udp/server_test.go — unit tests
```

Mirrors the TCP layout (`cmd/tcp-server`, `cmd/tcp-client`, `internal/tcp/server.go`) with one deliberate difference: no `handler.go` because there is no inter-service HTTP endpoint. All communication goes through the single UDP socket.

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
    mu        sync.RWMutex
    conn      *net.UDPConn
    done      chan struct{}
    closeOnce sync.Once
}
```

`clients` is keyed on `addr.String()` so re-registration from the same address is an upsert (idempotent).

---

## UDP Packet Protocol

All packets are JSON objects. The `type` field is the discriminator.

### Client → Server

| `type` | Additional fields | Purpose |
|--------|-------------------|---------|
| `register` | `manga_ids []string` | UC-009: subscribe; empty list = all manga |
| `unregister` | — | remove from registry |
| `admin_broadcast` | `manga_id string`, `message string` | UC-010: trigger chapter notification |

### Server → Client

| `type` | Additional fields | Purpose |
|--------|-------------------|---------|
| `ack` | `message string` | confirm register / unregister |
| `notification` | `manga_id string`, `message string`, `timestamp int64` | chapter release notification |

### Example Exchange

```
# Subscribe
Client → Server:  {"type":"register","manga_ids":["one-piece","naruto"]}
Server → Client:  {"type":"ack","message":"registered for 2 manga"}

# Admin triggers broadcast
Admin  → Server:  {"type":"admin_broadcast","manga_id":"one-piece","message":"Chapter 1101 released!"}
Server → Client:  {"type":"notification","manga_id":"one-piece","message":"Chapter 1101 released!","timestamp":1714000000}

# Unsubscribe
Client → Server:  {"type":"unregister"}
Server → Client:  {"type":"ack","message":"unregistered"}
```

The `notification` packet satisfies the spec's `Notification` struct exactly (`type`, `manga_id`, `message`, `timestamp`).

---

## Server Logic

`Run()` opens a single `*net.UDPConn` on `UDP_PORT` (default `9091`) and loops on `ReadFromUDP`. Packet dispatch runs in the read loop — no per-client goroutines (UDP is connectionless, no persistent connection state to manage per-sender).

```
ReadFromUDP → decode JSON → switch type:
  "register"        → upsert clientEntry, send ack
  "unregister"      → delete clientEntry, send ack
  "admin_broadcast" → fan out notification to matching subscribers
  unknown           → log and ignore
```

**Broadcast fan-out:** Iterate `clients`; for each entry where `Filter` contains `manga_id` or `Filter` is empty, call `WriteToUDP`. On write failure: log error, remove client from map, continue to remaining clients (UC-010 A1 — unreachable clients do not halt the broadcast).

**Shutdown:** `closeOnce.Do` closes the `UDPConn`. The blocked `ReadFromUDP` call returns an error; `Run()` checks if `done` is closed and exits cleanly. Same `sync.Once` + `done` channel pattern as the TCP server.

---

## cmd/udp-client Modes

Selected by `--mode` flag:

**Subscribe mode** (`--mode subscribe --manga-ids one-piece,naruto`):
1. Sends `register` packet with the manga filter
2. Loops on `ReadFromUDP`, printing each `notification` as it arrives
3. On Ctrl+C (SIGINT): sends `unregister`, then exits

**Broadcast mode** (`--mode broadcast --manga-id one-piece --message "Chapter 1101 released!"`):
1. Sends `admin_broadcast` packet
2. Exits immediately (fire-and-forget; server logs confirm delivery)

---

## Configuration

| Env var | Default | Used by |
|---------|---------|---------|
| `UDP_PORT` | `9091` | server |
| `UDP_SERVER_ADDR` | `localhost:9091` | client |

---

## Error Handling

| Scenario | Handling |
|----------|----------|
| `WriteToUDP` fails (stale client) | Log, remove from map, continue broadcast |
| Unknown packet `type` | Log `"udp: unknown type <type> from <addr>"`, ignore |
| Malformed JSON | Log `"udp: decode error from <addr>: <err>"`, ignore |
| `ReadFromUDP` error on shutdown | Check `done` channel; if closed, exit cleanly |
| `ReadFromUDP` error mid-run | Log, continue loop |

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
UDP_PORT=9091 go run ./cmd/udp-server

# Terminal 2 — subscribe to one-piece notifications
UDP_SERVER_ADDR=localhost:9091 go run ./cmd/udp-client \
  --mode subscribe --manga-ids one-piece

# Terminal 3 — admin triggers a broadcast
UDP_SERVER_ADDR=localhost:9091 go run ./cmd/udp-client \
  --mode broadcast --manga-id one-piece --message "Chapter 1101 released!"

# Terminal 2 prints:
# [notification] one-piece — Chapter 1101 released!
```
