# Design Spec: All-in-One Runner + Terminal Client

**Date:** 2026-05-07  
**Status:** Approved

---

## Overview

Two new binaries:

1. `cmd/runner` — starts all 5 servers in one process, one command, one `Ctrl+C` to stop
2. `cmd/client` — interactive terminal client that connects to all services with a numbered menu

---

## 1. Runner (`cmd/runner/main.go`)

### What it does

Launches all servers as goroutines inside a single process. WebSocket does not need its own goroutine — it is already mounted on the HTTP server at `GET /ws/chat`.

**Servers started:**

| Goroutine | Address | Notes |
|---|---|---|
| HTTP (+ WebSocket) | `:8080` | Includes `/ws/chat` route |
| gRPC | `:50051` | |
| TCP | `:9090` | Internal HTTP sidecar at `:9099` |
| UDP | `:9091` | Internal HTTP sidecar at `:9094` |

**Log format:** each server prefixes its own log lines so output is readable in one terminal:

```
[HTTP ] API server listening on :8080
[gRPC ] server listening on :50051
[TCP  ] listening on :9090
[TCP  ] internal HTTP on :9099
[UDP  ] listening on :9091
[UDP  ] internal HTTP on :9094
```

### Shutdown

`Ctrl+C` sends `SIGINT`. The runner catches it and calls each server's existing `Shutdown()` / `GracefulStop()` method in sequence, then exits.

### Environment variables

Same as each individual server — all existing env vars (`JWT_SECRET`, `DB_PATH`, `GRPC_ADDR`, `TCP_PORT`, `UDP_PORT`, etc.) are respected. Sensible defaults apply if unset.

---

## 2. Client (`cmd/client/`)

Single binary, split across files in `package main`:

```
cmd/client/
  main.go      — App struct, menu loop, entry point
  auth.go      — register/login HTTP calls
  manga.go     — search, view details, library, progress HTTP calls
  tcp.go       — background TCP listener goroutine
  udp.go       — background UDP listener goroutine
  ws.go        — chat room WebSocket session
```

### App state

```go
type App struct {
    BaseURL  string       // default: http://localhost:8080
    Token    string       // JWT, populated after login
    UserID   string
    Username string
    TCPConn  net.Conn     // opened after login
    UDPConn  *net.UDPConn // opened after login
}
```

---

## 3. Menu flow

### Before login

```
=== MangaHub ===
1. Search manga
2. Register
3. Login
0. Exit
> _
```

Search is available before login because `GET /manga` is a public endpoint.

### After login

```
=== MangaHub === [logged in as: <username>]
1. Search manga
2. View my library
3. Update reading progress
4. Add manga to library
5. Enter chat room
0. Logout / Exit
> _
```

---

## 4. Protocol connection lifecycle

| Protocol | Connects | Disconnects |
|---|---|---|
| HTTP | Per request (stateless) | Automatic after each call |
| TCP | Immediately after successful login | On logout or app exit |
| UDP | Immediately after successful login | On logout or app exit |
| WebSocket | When user selects "Enter chat room" | When user types `/exit` in chat |
| gRPC | Never — client uses HTTP only | — |

### TCP background goroutine (`tcp.go`)

After login, `tcp.go` opens a TCP connection to `:9090` and sends the auth message:

```json
{"type": "auth", "token": "<jwt>"}
```

Then blocks in a goroutine reading lines from the socket. When a progress update arrives, it prints to the terminal:

```
Your reading progress updated: One Piece → chapter 1096
```

### UDP background goroutine (`udp.go`)

After login, `udp.go` opens a local UDP listener, sends a register packet to `:9091`:

```json
{"type": "register", "manga_ids": []}
```

Registers for all manga (empty filter = receive everything). Blocks reading in a goroutine. When a notification arrives, prints:

```
Notification: Bleach chapter 700 just released!
```

Both goroutines print inline to the terminal. No `[TCP]` / `[UDP]` prefix — notifications appear as plain messages between menu prompts.

---

## 5. Chat room flow (`ws.go`)

User selects "Enter chat room" from main menu:

```
Enter manga ID (or press Enter for general): one-piece

=== Chat Room: one-piece ===
(type a message and press Enter to send, type /exit to leave)

[Tanaka] hey anyone reading the latest chapter?
[You   ] just finished it, crazy ending
> _
```

**Steps inside `ws.go`:**
1. Connect to `ws://localhost:8080/ws/chat?manga_id=<id>`
2. Immediately send `{"token":"<jwt>"}` as first message (automatic, not user-visible)
3. Server sends last 20 messages as history — display them
4. Start two goroutines: one reads incoming messages and prints them, one reads `stdin` and sends
5. `/exit` closes the WebSocket, returns to main menu

---

## 6. Error handling

- HTTP errors (4xx, 5xx): print the error message and return to menu, no crash
- TCP connect failure after login: print warning, continue without TCP (progress updates won't arrive)
- UDP register failure: print warning, continue without UDP notifications
- WebSocket connect failure: print error, return to main menu
- All errors are non-fatal — the client keeps running

---

## 7. Files to create

| File | Description |
|---|---|
| `cmd/runner/main.go` | All-in-one server runner |
| `cmd/client/main.go` | App struct, menu loop |
| `cmd/client/auth.go` | Register / login |
| `cmd/client/manga.go` | Search, details, library, progress |
| `cmd/client/tcp.go` | TCP background listener |
| `cmd/client/udp.go` | UDP background listener |
| `cmd/client/ws.go` | WebSocket chat session |

No changes to any existing files.

---

## 8. Out of scope

- No TUI library (bubbletea, promptui) — plain `bufio.Scanner` + `fmt`
- No colour/styling
- No pagination UI (search returns first page, ~20 results)
- No admin notification trigger in the client (UDP notifications come from external trigger via `POST :9094/internal/notify`)
