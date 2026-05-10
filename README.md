# MangaHub

A manga tracking and community system built in Go, demonstrating all five major network protocols — **HTTP, TCP, UDP, gRPC, and WebSocket** — working together in a single application. Includes a full-featured terminal UI (TUI) client powered by [BubbleTea](https://github.com/charmbracelet/bubbletea).

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        TUI Client                           │
│  (BubbleTea · HTTP · TCP · UDP · WebSocket)                 │
└────────┬────────┬────────┬────────────┬────────────────────┘
         │        │        │            │
    HTTP REST   TCP     UDP         WebSocket
    :8080      :9090   :9091        :8080/ws/chat
         │        │        │            │
┌────────▼────────▼────────▼────────────▼────────────────────┐
│                        Runner (cmd/runner)                  │
│        Starts all 5 services in a single process           │
├─────────────────────────────────────────────────────────────┤
│  HTTP API (Gin)  │  TCP Server  │  UDP Server  │  gRPC      │
│  :8080           │  :9090       │  :9091       │  :50051    │
└──────────────────┴──────┬───────┴──────────────┴────────────┘
                          │
                    SQLite Database
                   (data/mangahub.db)
```

| Protocol  | Port  | Purpose                                  |
|-----------|-------|------------------------------------------|
| HTTP REST | 8080  | Auth, manga search, user library         |
| WebSocket | 8080  | Real-time community chat (`/ws/chat`)    |
| TCP       | 9090  | Persistent connection for progress sync  |
| UDP       | 9091  | Chapter release notifications broadcast  |
| gRPC      | 50051 | Internal service — search & progress     |

---

## Prerequisites

- **Go 1.21+** (`go version`)
- **Git**
- [`protoc`](https://grpc.io/docs/protoc-installation/) + `protoc-gen-go` / `protoc-gen-go-grpc` — only needed if you modify `.proto` files

---

## Installation

```bash
git clone https://github.com/DuyDucIU/MangaHub.git
cd MangaHub

# Download all Go dependencies
go mod download
```

The SQLite database and manga seed data are created automatically on first run — no extra setup required.

---

## Running the Application

### Option 1 — All-in-one runner (recommended)

Starts HTTP, TCP, UDP, gRPC, and WebSocket services in a single process:

```bash
go run ./cmd/runner/main.go
```

### Option 2 — Individual services

```bash
go run ./cmd/api-server/main.go    # HTTP REST API + WebSocket  :8080
go run ./cmd/grpc-server/main.go   # gRPC service               :50051
go run ./cmd/tcp-server/main.go    # TCP progress sync          :9090
go run ./cmd/udp-server/main.go    # UDP notifications          :9091
```

### TUI Client

With the server(s) running, launch the terminal client:

```bash
go run ./cmd/client/main.go
```

---

## TUI Client Overview

Navigate with **arrow keys** (↑ ↓ ← →) and **Enter/Esc**. Press **`?`** at any time to open the help modal.

| Screen    | Description                                              |
|-----------|----------------------------------------------------------|
| Auth      | Register a new account or log in                        |
| Search    | Search manga; split-pane detail auto-fetches on select  |
| Library   | Your reading list with update-progress and remove modals|
| Dashboard | Continue-reading shortcuts and recent notifications      |
| Chat      | Full-screen WebSocket chat scoped to a manga series     |

---

## Configuration

All options are set via environment variables. Defaults work out of the box for local development.

| Variable           | Default                    | Description                          |
|--------------------|----------------------------|--------------------------------------|
| `HTTP_PORT`        | `8080`                     | HTTP API / WebSocket port            |
| `GRPC_PORT`        | `50051`                    | gRPC listen port                     |
| `GRPC_ADDR`        | `localhost:50051`          | gRPC address used by the API server  |
| `TCP_PORT`         | `9090`                     | TCP server listen port               |
| `TCP_INTERNAL_ADDR`| `:9099`                    | TCP internal HTTP (broadcast trigger)|
| `UDP_PORT`         | `9091`                     | UDP server listen port               |
| `UDP_INTERNAL_ADDR`| `:9094`                    | UDP internal HTTP (notify trigger)   |
| `DB_PATH`          | `./data/mangahub.db`       | SQLite database file path            |
| `JWT_SECRET`       | `mangahub-dev-secret`      | JWT signing secret — **change in production** |
| `API_URL`          | `http://localhost:8080`    | API base URL used by the TUI client  |

Example:

```bash
JWT_SECRET=my-secret HTTP_PORT=9000 go run ./cmd/runner/main.go
```

---

## Database

MangaHub uses **SQLite** with no migration tooling — the schema is created automatically on startup.

**Schema:**

```sql
users          -- id, username, email, password_hash, created_at
manga          -- id, title, author, genres, status, total_chapters, description, cover_url
user_progress  -- user_id, manga_id, current_chapter, status, updated_at
```

**Seed data:** `data/manga.json` contains 50+ manga entries (One Piece, Berserk, Attack on Titan, etc.) and is loaded automatically when the `manga` table is empty.

---

## API Reference

### Authentication

```
POST /auth/register    { "username", "email", "password" }
POST /auth/login       { "username", "password" }  →  { "token", "expires_at" }
```

All endpoints below require `Authorization: Bearer <token>`.

### Manga

```
GET  /manga           ?title=&genre=&status=&page=&limit=    Search manga
GET  /manga/:id                                               Get manga details
```

### User Library

```
POST   /users/library               { "manga_id", "status" }         Add to library
GET    /users/library                                                 Get library
PUT    /users/progress              { "manga_id", "chapter", "status" }  Update progress
DELETE /users/library/:manga_id                                       Remove from library
```

### Real-time

```
GET  /ws/chat?manga_id=<id>    WebSocket upgrade — joins chat room for that manga
POST /admin/notify             { "manga_id", "message" }  Trigger UDP notification (admin)
```

---

## Protocol Details

### TCP (Port 9090) — Progress Sync

Persistent JSON messages, newline-delimited.

```json
// Client → Server (authenticate first)
{"type": "auth", "token": "<jwt>"}

// Server → Client
{"type": "auth_ok", "user_id": "..."}
{"type": "progress_update", "manga_id": "...", "manga_title": "...", "chapter": 10, "timestamp": 0}
```

### UDP (Port 9091) — Notifications

```json
// Client → Server
{"type": "register", "manga_ids": ["one-piece", "naruto"]}

// Server → Client
{"type": "notification", "manga_id": "one-piece", "message": "Chapter 1234 released!", "timestamp": 0}
```

### WebSocket (`/ws/chat`) — Chat

```json
{"type": "message",  "username": "john", "room_id": "one-piece", "message": "Great chapter!"}
{"type": "join",     "username": "john", "room_id": "one-piece"}
{"type": "leave",    "username": "john", "room_id": "one-piece"}
```

### gRPC (Port 50051)

Defined in `proto/manga/manga.proto`:

```protobuf
service MangaService {
    rpc GetManga(GetMangaRequest)       returns (MangaResponse);
    rpc SearchManga(SearchRequest)      returns (SearchResponse);
    rpc UpdateProgress(ProgressRequest) returns (ProgressResponse);
}
```

---

## Building Binaries

```bash
# Build all binaries into ./bin/
go build -o bin/ ./cmd/...

# Run a built binary
./bin/runner
./bin/client
```

---

## Testing

```bash
# Run all tests
go test ./...

# Verbose output with coverage
go test -v -cover ./...
```

Test coverage spans HTTP handlers, TCP/UDP servers, WebSocket hub, gRPC service, JWT utilities, database layer, and TUI model.

---

## Project Structure

```
MangaHub/
├── cmd/
│   ├── runner/          # All-in-one entrypoint (all 5 protocols)
│   ├── api-server/      # HTTP + WebSocket only
│   ├── grpc-server/     # gRPC only
│   ├── tcp-server/      # TCP only
│   ├── udp-server/      # UDP only
│   ├── client/          # BubbleTea TUI client
│   ├── tcp-client/      # Example TCP client
│   ├── udp-client/      # Example UDP client
│   └── grpc-client/     # Example gRPC client
├── internal/
│   ├── auth/            # JWT middleware & register/login handlers
│   ├── manga/           # Manga search & detail handlers
│   ├── user/            # Library & progress handlers
│   ├── tcp/             # TCP server & broadcast logic
│   ├── udp/             # UDP server & notification logic
│   ├── websocket/       # WebSocket hub & chat handlers
│   └── grpc/            # gRPC service & client
├── pkg/
│   ├── database/        # SQLite connection & seeding
│   ├── models/          # Shared data structs
│   └── jwtutil/         # Token generation & validation
├── proto/manga/         # Protobuf definitions
├── data/
│   ├── manga.json       # 50+ manga seed entries
│   └── mangahub.db      # SQLite DB (auto-created)
└── docs/                # Project spec, use cases, CLI manual
```

---

## Example Clients

Standalone example clients for testing individual protocols:

```bash
go run ./cmd/tcp-client/main.go     # Connect via TCP
go run ./cmd/udp-client/main.go     # Receive UDP notifications
go run ./cmd/grpc-client/main.go    # Call gRPC endpoints
```
