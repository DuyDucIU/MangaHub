# Session 1: Project Setup & HTTP Basics — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A running HTTP API server with user auth (register/login) and manga browsing endpoints, backed by a seeded SQLite database — verified working with curl.

**Architecture:** Single Gin HTTP server binary at `cmd/api-server/`. Shared models and database packages in `pkg/` are reused by all future protocol servers (TCP, UDP, gRPC). Auth uses bcrypt + JWT HS256. Manga data seeds from `data/manga.json` on first run if the table is empty.

**Tech Stack:** Go 1.21+, `github.com/gin-gonic/gin`, `github.com/golang-jwt/jwt/v4`, `modernc.org/sqlite` (pure-Go SQLite — no CGO needed on Windows), `golang.org/x/crypto/bcrypt`, `github.com/google/uuid`, `github.com/stretchr/testify`

---

## File Map

| File | Responsibility |
|------|---------------|
| `go.mod` / `go.sum` | Module definition and dependency lock |
| `data/manga.json` | 30 seed manga entries across 4+ genres |
| `pkg/models/models.go` | Shared structs: User, Manga, UserProgress |
| `pkg/database/db.go` | Connect(), createTables(), SeedManga() |
| `pkg/database/db_test.go` | Verify tables exist after Connect() |
| `internal/manga/handler.go` | Search (GET /manga) and GetByID (GET /manga/:id) |
| `internal/manga/handler_test.go` | HTTP tests using httptest |
| `internal/auth/handler.go` | Register (POST /auth/register), Login (POST /auth/login) |
| `internal/auth/handler_test.go` | HTTP tests for register/login/middleware |
| `internal/auth/middleware.go` | JWTMiddleware() gin.HandlerFunc |
| `cmd/api-server/main.go` | Gin setup, route wiring, server start |

---

## Task 1: Initialize Go module, folder structure, and dependencies

**Files:**
- Create: `go.mod`, `go.sum`
- Create: all package directories

- [ ] **Step 1: Create project root and initialize Go module**

Open a terminal in the folder where you want to create the project, then:

```bash
mkdir mangahub
cd mangahub
go mod init mangahub
```

Expected: `go.mod` created containing `module mangahub` and your Go version.

- [ ] **Step 2: Create folder structure**

```bash
mkdir -p cmd/api-server
mkdir -p internal/auth
mkdir -p internal/manga
mkdir -p pkg/models
mkdir -p pkg/database
mkdir -p data
mkdir -p docs
```

- [ ] **Step 3: Install all dependencies**

```bash
go get github.com/gin-gonic/gin@latest
go get github.com/golang-jwt/jwt/v4@latest
go get modernc.org/sqlite@latest
go get golang.org/x/crypto@latest
go get github.com/google/uuid@latest
go get github.com/stretchr/testify@latest
```

Expected: `go.sum` created, `go.mod` updated with all require lines.

- [ ] **Step 4: Initialize git and commit**

```bash
git init
git add go.mod go.sum
git commit -m "feat: initialize Go module and install dependencies"
```

---

## Task 2: Write seed manga data

**Files:**
- Create: `data/manga.json`

- [ ] **Step 1: Create `data/manga.json`**

Create the file `data/manga.json` with this exact content (30 entries, covers shounen, shoujo, seinen, josei):

```json
[
  {"id":"one-piece","title":"One Piece","author":"Oda Eiichiro","genres":["Action","Adventure","Comedy","Shounen"],"status":"ongoing","total_chapters":1100,"description":"A young pirate's quest to find the ultimate treasure and become King of the Pirates."},
  {"id":"naruto","title":"Naruto","author":"Kishimoto Masashi","genres":["Action","Adventure","Shounen"],"status":"completed","total_chapters":700,"description":"A young ninja seeks recognition and dreams of becoming the Hokage."},
  {"id":"attack-on-titan","title":"Attack on Titan","author":"Isayama Hajime","genres":["Action","Drama","Fantasy","Seinen"],"status":"completed","total_chapters":139,"description":"Humanity fights for survival against giant humanoid Titans."},
  {"id":"death-note","title":"Death Note","author":"Ohba Tsugumi","genres":["Mystery","Psychological","Thriller","Shounen"],"status":"completed","total_chapters":108,"description":"A student finds a supernatural notebook that kills anyone whose name is written in it."},
  {"id":"fullmetal-alchemist","title":"Fullmetal Alchemist","author":"Arakawa Hiromu","genres":["Action","Adventure","Fantasy","Shounen"],"status":"completed","total_chapters":108,"description":"Two brothers use alchemy to try to restore their bodies after a failed ritual."},
  {"id":"jujutsu-kaisen","title":"Jujutsu Kaisen","author":"Akutami Gege","genres":["Action","Supernatural","Shounen"],"status":"completed","total_chapters":271,"description":"A boy swallows a cursed object and joins a secret school to fight evil spirits."},
  {"id":"demon-slayer","title":"Demon Slayer","author":"Gotouge Koyoharu","genres":["Action","Historical","Shounen"],"status":"completed","total_chapters":205,"description":"A boy becomes a demon slayer to avenge his family and cure his demon sister."},
  {"id":"my-hero-academia","title":"My Hero Academia","author":"Horikoshi Kohei","genres":["Action","School","Shounen"],"status":"completed","total_chapters":430,"description":"In a world of superheroes, a boy born without powers strives to be the greatest hero."},
  {"id":"dragon-ball","title":"Dragon Ball","author":"Toriyama Akira","genres":["Action","Adventure","Comedy","Shounen"],"status":"completed","total_chapters":519,"description":"A boy with a monkey tail searches for magical orbs that grant any wish."},
  {"id":"bleach","title":"Bleach","author":"Kubo Tite","genres":["Action","Supernatural","Shounen"],"status":"completed","total_chapters":686,"description":"A teenager gains Soul Reaper powers and defends humans from evil spirits."},
  {"id":"hunter-x-hunter","title":"Hunter x Hunter","author":"Togashi Yoshihiro","genres":["Action","Adventure","Fantasy","Shounen"],"status":"ongoing","total_chapters":401,"description":"A boy follows in his missing father's footsteps to become a professional Hunter."},
  {"id":"vinland-saga","title":"Vinland Saga","author":"Yukimura Makoto","genres":["Action","Historical","Adventure","Seinen"],"status":"ongoing","total_chapters":200,"description":"A young Viking warrior seeks revenge for his father's death in medieval Europe."},
  {"id":"berserk","title":"Berserk","author":"Miura Kentaro","genres":["Action","Fantasy","Horror","Seinen"],"status":"ongoing","total_chapters":374,"description":"A lone mercenary fights demons in a dark medieval fantasy world."},
  {"id":"vagabond","title":"Vagabond","author":"Inoue Takehiko","genres":["Action","Historical","Drama","Seinen"],"status":"ongoing","total_chapters":327,"description":"The fictionalized journey of Miyamoto Musashi, Japan's greatest swordsman."},
  {"id":"tokyo-ghoul","title":"Tokyo Ghoul","author":"Ishida Sui","genres":["Action","Horror","Psychological","Seinen"],"status":"completed","total_chapters":143,"description":"A student becomes half-ghoul and struggles with his new nature."},
  {"id":"slam-dunk","title":"Slam Dunk","author":"Inoue Takehiko","genres":["Sports","Comedy","Drama","Shounen"],"status":"completed","total_chapters":276,"description":"A delinquent joins his school's basketball team and discovers his passion for the sport."},
  {"id":"haikyuu","title":"Haikyuu!!","author":"Furudate Haruichi","genres":["Sports","Drama","School","Shounen"],"status":"completed","total_chapters":402,"description":"A short boy with a big dream fights to reach the top of high school volleyball."},
  {"id":"fruits-basket","title":"Fruits Basket","author":"Takaya Natsuki","genres":["Romance","Fantasy","Drama","Shoujo"],"status":"completed","total_chapters":136,"description":"An orphaned girl discovers a family cursed to transform into zodiac animals."},
  {"id":"sailor-moon","title":"Sailor Moon","author":"Takeuchi Naoko","genres":["Magic","Romance","Action","Shoujo"],"status":"completed","total_chapters":60,"description":"A clumsy girl transforms into a sailor guardian to protect Earth from evil."},
  {"id":"card-captor-sakura","title":"Cardcaptor Sakura","author":"CLAMP","genres":["Magic","Romance","Adventure","Shoujo"],"status":"completed","total_chapters":50,"description":"A girl accidentally releases magical cards and must capture them all."},
  {"id":"nana","title":"Nana","author":"Yazawa Ai","genres":["Romance","Drama","Music","Josei"],"status":"ongoing","total_chapters":84,"description":"Two women named Nana become unlikely friends in Tokyo."},
  {"id":"paradise-kiss","title":"Paradise Kiss","author":"Yazawa Ai","genres":["Romance","Drama","Fashion","Josei"],"status":"completed","total_chapters":40,"description":"A studious girl falls into the world of fashion and a complicated romance."},
  {"id":"skip-beat","title":"Skip Beat!","author":"Nakamura Yoshiki","genres":["Romance","Comedy","Drama","Shoujo"],"status":"ongoing","total_chapters":300,"description":"A girl enters show business to take revenge on the boy who betrayed her."},
  {"id":"chainsaw-man","title":"Chainsaw Man","author":"Fujimoto Tatsuki","genres":["Action","Horror","Supernatural","Shounen"],"status":"ongoing","total_chapters":190,"description":"A boy merges with a chainsaw devil and hunts other devils for a secret agency."},
  {"id":"spy-x-family","title":"Spy x Family","author":"Endo Tatsuya","genres":["Action","Comedy","Slice of Life","Shounen"],"status":"ongoing","total_chapters":100,"description":"A spy, an assassin, and a telepath pretend to be a family for their respective missions."},
  {"id":"black-clover","title":"Black Clover","author":"Tabata Yuki","genres":["Action","Fantasy","Magic","Shounen"],"status":"ongoing","total_chapters":370,"description":"A boy born without magic vows to become the Wizard King."},
  {"id":"ao-haru-ride","title":"Ao Haru Ride","author":"Sakisaka Io","genres":["Romance","Drama","School","Shoujo"],"status":"completed","total_chapters":49,"description":"A girl reunites with her first love in high school to find he has completely changed."},
  {"id":"grand-blue","title":"Grand Blue Dreaming","author":"Inoue Kenji","genres":["Comedy","Slice of Life","Sports","Seinen"],"status":"ongoing","total_chapters":80,"description":"A college student joins a diving club that turns out to be mostly about drinking and chaos."},
  {"id":"mushishi","title":"Mushishi","author":"Urushibara Yuki","genres":["Adventure","Fantasy","Mystery","Seinen"],"status":"completed","total_chapters":50,"description":"A traveler studies mysterious beings that exist on the boundary between life and death."},
  {"id":"bokutachi-wa-benkyou","title":"We Never Learn","author":"Tsutsui Taishi","genres":["Romance","Comedy","School","Shounen"],"status":"completed","total_chapters":187,"description":"A hardworking student tutors three genius girls in their weakest subjects."}
]
```

- [ ] **Step 2: Verify the JSON is valid**

```bash
# PowerShell
Get-Content data/manga.json | python -m json.tool
```

Expected: pretty-printed JSON with no errors.

- [ ] **Step 3: Commit**

```bash
git add data/manga.json
git commit -m "feat: add seed manga data (30 entries, 4+ genres)"
```

---

## Task 3: Write shared models

**Files:**
- Create: `pkg/models/models.go`

- [ ] **Step 1: Create `pkg/models/models.go`**

```go
package models

import "time"

type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
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

- [ ] **Step 2: Verify it compiles**

```bash
go build ./pkg/models/...
```

Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add pkg/models/models.go
git commit -m "feat: add shared data models (User, Manga, UserProgress)"
```

---

## Task 4: Write and test the database layer

**Files:**
- Create: `pkg/database/db.go`
- Create: `pkg/database/db_test.go`

- [ ] **Step 1: Write the failing test**

Create `pkg/database/db_test.go`:

```go
package database_test

import (
	"testing"

	"mangahub/pkg/database"
)

func TestConnect_CreatesAllTables(t *testing.T) {
	db, err := database.Connect(":memory:")
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer db.Close()

	tables := []string{"users", "manga", "user_progress"}
	for _, table := range tables {
		var name string
		err := db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./pkg/database/... -v
```

Expected: FAIL — `package database` not found or compile error.

- [ ] **Step 3: Write `pkg/database/db.go`**

```go
package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"

	_ "modernc.org/sqlite"

	"mangahub/pkg/models"
)

func Connect(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("pragma foreign_keys: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		return nil, fmt.Errorf("pragma journal_mode: %w", err)
	}
	if err := createTables(db); err != nil {
		return nil, fmt.Errorf("create tables: %w", err)
	}
	return db, nil
}

func createTables(db *sql.DB) error {
	_, err := db.Exec(`
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
			genres         TEXT NOT NULL,
			status         TEXT NOT NULL,
			total_chapters INTEGER NOT NULL,
			description    TEXT
		);

		CREATE TABLE IF NOT EXISTS user_progress (
			user_id         TEXT NOT NULL,
			manga_id        TEXT NOT NULL,
			current_chapter INTEGER NOT NULL DEFAULT 0,
			status          TEXT NOT NULL,
			updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (user_id, manga_id),
			FOREIGN KEY (user_id)  REFERENCES users(id),
			FOREIGN KEY (manga_id) REFERENCES manga(id)
		);
	`)
	return err
}

func SeedManga(db *sql.DB, dataPath string) error {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM manga").Scan(&count); err != nil {
		return fmt.Errorf("count manga: %w", err)
	}
	if count > 0 {
		return nil
	}

	data, err := os.ReadFile(dataPath)
	if err != nil {
		return fmt.Errorf("read seed file: %w", err)
	}

	var mangaList []models.Manga
	if err := json.Unmarshal(data, &mangaList); err != nil {
		return fmt.Errorf("parse seed data: %w", err)
	}

	for _, m := range mangaList {
		genres, _ := json.Marshal(m.Genres)
		_, err := db.Exec(
			`INSERT OR IGNORE INTO manga (id, title, author, genres, status, total_chapters, description)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			m.ID, m.Title, m.Author, string(genres), m.Status, m.TotalChapters, m.Description,
		)
		if err != nil {
			return fmt.Errorf("insert manga %q: %w", m.ID, err)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./pkg/database/... -v
```

Expected:
```
--- PASS: TestConnect_CreatesAllTables (0.00s)
PASS
```

- [ ] **Step 5: Commit**

```bash
git add pkg/database/db.go pkg/database/db_test.go
git commit -m "feat: add database layer with SQLite connect, schema, and seed"
```

---

## Task 5: Write and test manga HTTP handlers

**Files:**
- Create: `internal/manga/handler.go`
- Create: `internal/manga/handler_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/manga/handler_test.go`:

```go
package manga_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"mangahub/internal/manga"
	"mangahub/pkg/database"
)

func setupMangaRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := database.Connect(":memory:")
	if err != nil {
		t.Fatalf("db connect: %v", err)
	}
	_, err = db.Exec(`INSERT INTO manga (id, title, author, genres, status, total_chapters, description)
		VALUES ('one-piece', 'One Piece', 'Oda Eiichiro', '["Action","Shounen"]', 'ongoing', 1100, 'Pirates!')`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	h := &manga.Handler{DB: db}
	r := gin.New()
	r.GET("/manga", h.Search)
	r.GET("/manga/:id", h.GetByID)
	return r
}

func TestSearch_ReturnsResults(t *testing.T) {
	r := setupMangaRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/manga?q=one", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if count, ok := resp["count"].(float64); !ok || count == 0 {
		t.Error("expected at least 1 result")
	}
}

func TestSearch_EmptyQuery_ReturnsAll(t *testing.T) {
	r := setupMangaRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/manga", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestGetByID_Found(t *testing.T) {
	r := setupMangaRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/manga/one-piece", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var m map[string]interface{}
	json.NewDecoder(w.Body).Decode(&m)
	if m["id"] != "one-piece" {
		t.Errorf("expected id 'one-piece', got %v", m["id"])
	}
}

func TestGetByID_NotFound(t *testing.T) {
	r := setupMangaRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/manga/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/manga/... -v
```

Expected: FAIL — `manga.Handler` not defined.

- [ ] **Step 3: Write `internal/manga/handler.go`**

```go
package manga

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"mangahub/pkg/models"
)

type Handler struct {
	DB *sql.DB
}

func (h *Handler) Search(c *gin.Context) {
	q := c.Query("q")
	genre := c.Query("genre")
	status := c.Query("status")

	query := "SELECT id, title, author, genres, status, total_chapters, description FROM manga WHERE 1=1"
	args := []interface{}{}

	if q != "" {
		query += " AND (LOWER(title) LIKE LOWER(?) OR LOWER(author) LIKE LOWER(?))"
		like := "%" + q + "%"
		args = append(args, like, like)
	}
	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}

	rows, err := h.DB.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer rows.Close()

	var results []models.Manga
	for rows.Next() {
		var m models.Manga
		var genresStr string
		if err := rows.Scan(&m.ID, &m.Title, &m.Author, &genresStr, &m.Status, &m.TotalChapters, &m.Description); err != nil {
			continue
		}
		json.Unmarshal([]byte(genresStr), &m.Genres)

		if genre != "" {
			match := false
			for _, g := range m.Genres {
				if strings.EqualFold(g, genre) {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		results = append(results, m)
	}

	if results == nil {
		results = []models.Manga{}
	}
	c.JSON(http.StatusOK, gin.H{"results": results, "count": len(results)})
}

func (h *Handler) GetByID(c *gin.Context) {
	id := c.Param("id")
	var m models.Manga
	var genresStr string

	err := h.DB.QueryRow(
		"SELECT id, title, author, genres, status, total_chapters, description FROM manga WHERE id = ?",
		id,
	).Scan(&m.ID, &m.Title, &m.Author, &genresStr, &m.Status, &m.TotalChapters, &m.Description)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "manga not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	json.Unmarshal([]byte(genresStr), &m.Genres)
	c.JSON(http.StatusOK, m)
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/manga/... -v
```

Expected:
```
--- PASS: TestSearch_ReturnsResults (0.00s)
--- PASS: TestSearch_EmptyQuery_ReturnsAll (0.00s)
--- PASS: TestGetByID_Found (0.00s)
--- PASS: TestGetByID_NotFound (0.00s)
PASS
```

- [ ] **Step 5: Commit**

```bash
git add internal/manga/handler.go internal/manga/handler_test.go
git commit -m "feat: add manga search and detail endpoints with tests"
```

---

## Task 6: Write and test auth handlers

**Files:**
- Create: `internal/auth/handler.go`
- Create: `internal/auth/handler_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/auth/handler_test.go`:

```go
package auth_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"mangahub/internal/auth"
	"mangahub/pkg/database"
)

func setupAuthRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := database.Connect(":memory:")
	if err != nil {
		t.Fatalf("db connect: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	h := &auth.Handler{DB: db, JWTSecret: "test-secret"}
	r := gin.New()
	r.POST("/auth/register", h.Register)
	r.POST("/auth/login", h.Login)
	return r
}

func postJSON(r *gin.Engine, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestRegister_Success(t *testing.T) {
	r := setupAuthRouter(t)
	w := postJSON(r, "/auth/register", `{"username":"testuser","email":"test@test.com","password":"password123"}`)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["user_id"] == "" {
		t.Error("expected user_id in response")
	}
}

func TestRegister_DuplicateUsername(t *testing.T) {
	r := setupAuthRouter(t)
	body := `{"username":"testuser","email":"test@test.com","password":"password123"}`
	postJSON(r, "/auth/register", body)
	w := postJSON(r, "/auth/register", body)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestRegister_InvalidEmail(t *testing.T) {
	r := setupAuthRouter(t)
	w := postJSON(r, "/auth/register", `{"username":"testuser","email":"notanemail","password":"password123"}`)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}
}

func TestLogin_Success(t *testing.T) {
	r := setupAuthRouter(t)
	postJSON(r, "/auth/register", `{"username":"testuser","email":"test@test.com","password":"password123"}`)
	w := postJSON(r, "/auth/login", `{"username":"testuser","password":"password123"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["token"] == nil {
		t.Error("expected token in response")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	r := setupAuthRouter(t)
	postJSON(r, "/auth/register", `{"username":"testuser","email":"test@test.com","password":"password123"}`)
	w := postJSON(r, "/auth/login", `{"username":"testuser","password":"wrongpassword"}`)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestLogin_UserNotFound(t *testing.T) {
	r := setupAuthRouter(t)
	w := postJSON(r, "/auth/login", `{"username":"nobody","password":"password123"}`)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/auth/... -v
```

Expected: FAIL — `auth.Handler` not defined.

- [ ] **Step 3: Write `internal/auth/handler.go`**

```go
package auth

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type Handler struct {
	DB        *sql.DB
	JWTSecret string
}

type registerRequest struct {
	Username string `json:"username" binding:"required,min=3,max=30"`
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *Handler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	userID := "usr_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:8]

	_, err = h.DB.Exec(
		"INSERT INTO users (id, username, email, password_hash) VALUES (?, ?, ?, ?)",
		userID, req.Username, req.Email, string(hash),
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username or email already exists"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Account created", "user_id": userID})
}

func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	var userID, username, passwordHash string
	err := h.DB.QueryRow(
		"SELECT id, username, password_hash FROM users WHERE username = ?", req.Username,
	).Scan(&userID, &username, &passwordHash)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":  userID,
		"username": username,
		"exp":      expiresAt.Unix(),
	})

	tokenString, err := token.SignedString([]byte(h.JWTSecret))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": tokenString, "expires_at": expiresAt})
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/auth/... -v
```

Expected:
```
--- PASS: TestRegister_Success (0.00s)
--- PASS: TestRegister_DuplicateUsername (0.00s)
--- PASS: TestRegister_InvalidEmail (0.00s)
--- PASS: TestLogin_Success (0.00s)
--- PASS: TestLogin_WrongPassword (0.00s)
--- PASS: TestLogin_UserNotFound (0.00s)
PASS
```

- [ ] **Step 5: Commit**

```bash
git add internal/auth/handler.go internal/auth/handler_test.go
git commit -m "feat: add auth register and login endpoints with tests"
```

---

## Task 7: Write JWT middleware

**Files:**
- Create: `internal/auth/middleware.go`
- Modify: `internal/auth/handler_test.go` (add 2 middleware tests)

- [ ] **Step 1: Add the failing middleware tests**

Append these two test functions to the bottom of `internal/auth/handler_test.go`:

```go
func TestJWTMiddleware_MissingToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := database.Connect(":memory:")
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	defer db.Close()

	h := &auth.Handler{DB: db, JWTSecret: "test-secret"}
	r := gin.New()
	r.GET("/protected", h.JWTMiddleware(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestJWTMiddleware_ValidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := database.Connect(":memory:")
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	defer db.Close()

	h := &auth.Handler{DB: db, JWTSecret: "test-secret"}
	r := gin.New()
	r.POST("/auth/register", h.Register)
	r.POST("/auth/login", h.Login)
	r.GET("/protected", h.JWTMiddleware(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"user_id": c.GetString("user_id")})
	})

	postJSON(r, "/auth/register", `{"username":"mwuser","email":"mw@test.com","password":"password123"}`)
	loginResp := postJSON(r, "/auth/login", `{"username":"mwuser","password":"password123"}`)
	var loginBody map[string]interface{}
	json.NewDecoder(loginResp.Body).Decode(&loginBody)
	token := loginBody["token"].(string)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
```

- [ ] **Step 2: Run tests to verify the new ones fail**

```bash
go test ./internal/auth/... -v
```

Expected: FAIL — `JWTMiddleware` not defined.

- [ ] **Step 3: Write `internal/auth/middleware.go`**

```go
package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
)

func (h *Handler) JWTMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing or invalid token"})
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(h.JWTSecret), nil
		})

		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}

		claims := token.Claims.(jwt.MapClaims)
		c.Set("user_id", claims["user_id"].(string))
		c.Set("username", claims["username"].(string))
		c.Next()
	}
}
```

- [ ] **Step 4: Run all auth tests to verify all 8 pass**

```bash
go test ./internal/auth/... -v
```

Expected: all 8 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/middleware.go internal/auth/handler_test.go
git commit -m "feat: add JWT middleware with tests"
```

---

## Task 8: Wire main.go and run the server

**Files:**
- Create: `cmd/api-server/main.go`

- [ ] **Step 1: Write `cmd/api-server/main.go`**

```go
package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"mangahub/internal/auth"
	"mangahub/internal/manga"
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

	r := gin.Default()

	r.POST("/auth/register", authHandler.Register)
	r.POST("/auth/login", authHandler.Login)

	r.GET("/manga", mangaHandler.Search)
	r.GET("/manga/:id", mangaHandler.GetByID)

	log.Println("HTTP API server running on :8080")
	if err := r.Run(":8080"); err != nil {
		log.Fatalf("server: %v", err)
	}
}
```

- [ ] **Step 2: Build to verify it compiles**

```bash
go build ./cmd/api-server/...
```

Expected: no errors, binary created.

- [ ] **Step 3: Run the server**

```bash
go run ./cmd/api-server/
```

Expected output:
```
[GIN-debug] POST   /auth/register
[GIN-debug] POST   /auth/login
[GIN-debug] GET    /manga
[GIN-debug] GET    /manga/:id
HTTP API server running on :8080
```

Keep this terminal open. Open a **second terminal** for Task 9.

- [ ] **Step 4: Commit**

```bash
git add cmd/api-server/main.go
git commit -m "feat: wire HTTP server with all Session 1 routes"
```

---

## Task 9: End-to-end verification with curl

Run all checks in a second terminal while the server is running from Task 8.

- [ ] **Step 1: Register a user**

```bash
curl -s -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"testuser\",\"email\":\"test@test.com\",\"password\":\"password123\"}"
```

Expected:
```json
{"message":"Account created","user_id":"usr_xxxxxxxx"}
```

- [ ] **Step 2: Login and save the token**

```bash
curl -s -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"testuser\",\"password\":\"password123\"}"
```

Expected: `{"token":"eyJ...","expires_at":"..."}`. Copy the token value — you will need it in Session 2.

- [ ] **Step 3: Search manga by title**

```bash
curl -s "http://localhost:8080/manga?q=one+piece"
```

Expected: `{"count":1,"results":[{...One Piece...}]}`

- [ ] **Step 4: Search manga by genre**

```bash
curl -s "http://localhost:8080/manga?genre=Shoujo"
```

Expected: results containing Fruits Basket, Sailor Moon, Cardcaptor Sakura, Ao Haru Ride, Skip Beat.

- [ ] **Step 5: Search manga by status**

```bash
curl -s "http://localhost:8080/manga?status=ongoing"
```

Expected: results containing One Piece, Hunter x Hunter, Berserk, etc.

- [ ] **Step 6: Get manga by ID**

```bash
curl -s http://localhost:8080/manga/one-piece
```

Expected: full One Piece object with `genres` as an array (not a string).

- [ ] **Step 7: Verify error cases**

```bash
# Duplicate registration → 400
curl -s -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"testuser\",\"email\":\"test@test.com\",\"password\":\"password123\"}"

# Wrong password → 401
curl -s -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"testuser\",\"password\":\"wrongpassword\"}"

# Non-existent manga → 404
curl -s http://localhost:8080/manga/nonexistent
```

Expected: `400`, `401`, `404` responses respectively.

- [ ] **Step 8: Run the full test suite**

```bash
go test ./... -v
```

Expected: all tests across all packages PASS, zero failures.

- [ ] **Step 9: Final commit**

```bash
git add .
git commit -m "feat: session 1 complete — HTTP API with auth and manga endpoints"
```

---

## Session 1 Complete

What's working after this session:
- SQLite database with 3 tables + 30 seeded manga entries (4+ genres)
- `POST /auth/register` — bcrypt password hashing, UUID user ID
- `POST /auth/login` — JWT token (24h expiry, HS256)
- `GET /manga` — search by title/author, filter by genre/status
- `GET /manga/:id` — manga detail with genres as array
- `JWTMiddleware()` ready for Session 2 protected routes
- Full unit test coverage on database, manga, and auth layers

**Session 2 adds:** `POST /users/library`, `GET /users/library`, `PUT /users/progress` (these use `JWTMiddleware()` and write to the `user_progress` table built today)
