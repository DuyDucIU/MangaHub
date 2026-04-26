# Session 1 Design: Project Setup & HTTP Basics

**Date:** 2026-04-26
**Scope:** Go project scaffold, SQLite database layer, HTTP auth endpoints, manga read endpoints
**Goal:** A running HTTP server that can register users, issue JWT tokens, and serve manga data from a seeded SQLite database — fully verified with curl before Session 2 begins.

---

## Context

MangaHub is a 5-protocol manga tracking system (HTTP, TCP, UDP, WebSocket, gRPC) built in Go for an academic networking course. This is Session 1 of 7. Later sessions add protocols on top of the foundation built here.

**Overall architecture (for reference):**
- 4 binaries share one SQLite file: `api-server` (HTTP+WebSocket), `tcp-server`, `udp-server`, `grpc-server`
- Ports: HTTP :8080, TCP :9090, UDP :9091, gRPC :9092, WebSocket at HTTP /ws
- Session 1 only builds the HTTP binary and the shared database/models packages

---

## Folder Structure

```
mangahub/
├── cmd/
│   └── api-server/
│       └── main.go              # Gin setup, route wiring, server start
├── internal/
│   └── auth/
│       ├── handler.go           # Register + Login HTTP handlers
│       └── middleware.go        # JWT validation middleware
├── pkg/
│   ├── models/
│   │   └── models.go            # Shared structs: User, Manga, UserProgress
│   └── database/
│       └── db.go                # Connect(), CreateTables(), SeedManga()
├── data/
│   └── manga.json               # ~30 seed manga entries
├── go.mod
└── go.sum
```

---

## Database Schema

SQLite file at `./data/mangahub.db` (configurable).

```sql
CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY,
    username      TEXT UNIQUE NOT NULL,
    email         TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS manga (
    id             TEXT PRIMARY KEY,
    title          TEXT NOT NULL,
    author         TEXT NOT NULL,
    genres         TEXT NOT NULL,   -- JSON array as text e.g. ["Action","Shounen"]
    status         TEXT NOT NULL,   -- "ongoing" | "completed"
    total_chapters INTEGER NOT NULL,
    description    TEXT
);

CREATE TABLE IF NOT EXISTS user_progress (
    user_id         TEXT NOT NULL,
    manga_id        TEXT NOT NULL,
    current_chapter INTEGER NOT NULL DEFAULT 0,
    status          TEXT NOT NULL,  -- "reading"|"completed"|"plan_to_read"|"on_hold"|"dropped"
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, manga_id),
    FOREIGN KEY (user_id) REFERENCES users(id),
    FOREIGN KEY (manga_id) REFERENCES manga(id)
);
```

SQLite pragmas applied on connect:
- `PRAGMA foreign_keys = ON`
- `PRAGMA journal_mode = WAL`

---

## Shared Models (`pkg/models/models.go`)

```go
type User struct {
    ID           string    `json:"id"`
    Username     string    `json:"username"`
    Email        string    `json:"email"`
    PasswordHash string    `json:"-"`         // never serialized
    CreatedAt    time.Time `json:"created_at"`
}

type Manga struct {
    ID            string   `json:"id"`
    Title         string   `json:"title"`
    Author        string   `json:"author"`
    Genres        []string `json:"genres"`
    Status        string   `json:"status"`
    TotalChapters int      `json:"total_chapters"`
    Description   string   `json:"description"`
}

type UserProgress struct {
    UserID         string    `json:"user_id"`
    MangaID        string    `json:"manga_id"`
    CurrentChapter int       `json:"current_chapter"`
    Status         string    `json:"status"`
    UpdatedAt      time.Time `json:"updated_at"`
}
```

---

## HTTP Endpoints (Session 1 scope only)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/auth/register` | No | Create user account |
| POST | `/auth/login` | No | Authenticate, return JWT |
| GET | `/manga` | No | List/search manga (query: `q`, `genre`, `status`) |
| GET | `/manga/:id` | No | Get single manga detail |

Endpoints deferred to Session 2: `POST /users/library`, `GET /users/library`, `PUT /users/progress`

### Request / Response Shapes

```
POST /auth/register
  Body:    {"username":"johndoe","email":"john@example.com","password":"secret123"}
  Success: 201 {"message":"Account created","user_id":"usr_abc123"}
  Errors:  400 username/email already exists | 400 weak password | 422 invalid input

POST /auth/login
  Body:    {"username":"johndoe","password":"secret123"}
  Success: 200 {"token":"<jwt>","expires_at":"2026-04-27T10:30:00Z"}
  Errors:  401 invalid credentials | 404 account not found

GET /manga?q=one+piece&genre=shounen&status=ongoing
  Success: 200 {"results":[...],"count":3}

GET /manga/:id
  Success: 200 {"id":"one-piece","title":"One Piece",...}
  Errors:  404 not found
```

### JWT

- Payload claims: `user_id`, `username`, `exp` (24h from issue)
- Algorithm: HS256, secret from environment variable `JWT_SECRET`
- Clients send: `Authorization: Bearer <token>`
- Middleware injects `user_id` into Gin context on success

### Password

- Hashed with `bcrypt` (cost 12) on registration
- Compared with `bcrypt.CompareHashAndPassword` on login
- User ID generated as `"usr_" + first 8 chars of UUID`

---

## Seed Data (`data/manga.json`)

Minimum 30 entries covering at least 4 genres (shounen, shoujo, seinen, josei).
Format matches the `Manga` struct. Loaded once by `database.SeedManga()` if the manga table is empty.

---

## Go Libraries (Session 1)

```
github.com/gin-gonic/gin          HTTP framework
github.com/golang-jwt/jwt/v4      JWT signing/validation
github.com/mattn/go-sqlite3       SQLite driver (requires CGO)
golang.org/x/crypto/bcrypt        Password hashing
github.com/google/uuid            User ID generation
```

---

## Verification Plan

After the session, all 4 checks must pass before marking Session 1 complete:

```bash
# 1. Register a new user
curl -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username":"testuser","email":"test@test.com","password":"password123"}'
# Expected: 201 with user_id

# 2. Login and capture token
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"testuser","password":"password123"}'
# Expected: 200 with JWT token

# 3. Search manga
curl "http://localhost:8080/manga?q=one+piece"
# Expected: 200 with at least 1 result

# 4. Get manga by ID
curl http://localhost:8080/manga/one-piece
# Expected: 200 with full manga detail

# Failure cases to verify:
# - Register same username twice → 400
# - Login with wrong password → 401
# - GET /manga/nonexistent → 404
```

---

## Out of Scope (Session 1)

- `POST /users/library`, `GET /users/library`, `PUT /users/progress` → Session 2
- TCP, UDP, WebSocket, gRPC → Sessions 3–6
- Docker, CI/CD, rate limiting → optional bonus
