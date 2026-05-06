# gRPC Internal Service Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the gRPC internal service (UC-014, UC-015, UC-016) with C-full integration — the HTTP API calls gRPC for GetManga, SearchManga, and UpdateProgress — plus a standalone demo client binary.

**Architecture:** A single `MangaService` proto with 3 unary RPCs lives in `internal/grpc/` (both server implementation and thin client adapter). HTTP handlers receive a `GRPCClient` interface at construction time; the concrete `*grpc.Client` satisfies both the `manga.MangaGRPCClient` and `user.ProgressGRPCClient` interfaces via Go duck typing. The gRPC `UpdateProgress` method owns the TCP broadcast trigger (moved from the HTTP handler).

**Tech Stack:** `google.golang.org/grpc`, `google.golang.org/protobuf`, `protoc` + `protoc-gen-go` + `protoc-gen-go-grpc` for codegen, `modernc.org/sqlite` (existing), `github.com/stretchr/testify` (existing).

---

## File Map

| File | Action | Responsibility |
|---|---|---|
| `proto/manga/manga.proto` | Create | Source-of-truth proto definition |
| `proto/manga/manga.pb.go` | Generated | Protobuf message types |
| `proto/manga/manga_grpc.pb.go` | Generated | gRPC service stubs |
| `internal/grpc/service.go` | Create | `MangaServiceServer` — DB queries + TCP broadcast |
| `internal/grpc/service_test.go` | Create | Unit tests for the server implementation |
| `internal/grpc/client.go` | Create | Thin adapter: `pb.*` → `models.*`, gRPC status → sentinel errors |
| `internal/grpc/client_test.go` | Create | Adapter tests using in-process gRPC server |
| `cmd/grpc-server/main.go` | Create | Starts gRPC server on `:50051` |
| `cmd/grpc-client/main.go` | Create | Demo binary — calls all 3 RPC methods |
| `pkg/models/models.go` | Modify | Add `ErrNotFound`, `ErrInvalidArgument` sentinel errors |
| `internal/manga/handler.go` | Modify | Add `MangaGRPCClient` interface; `GetByID`/`Search` use it; `Search` adds pagination |
| `internal/manga/handler_test.go` | Modify | Inject mock `MangaGRPCClient`; update `Search` assertions for pagination |
| `internal/user/handler.go` | Modify | Add `ProgressGRPCClient` interface; `UpdateProgress` uses it |
| `internal/user/handler_test.go` | Modify | Inject mock `ProgressGRPCClient`; rewrite UpdateProgress tests |
| `cmd/api-server/main.go` | Modify | Create `*grpc.Client`, inject into both handlers |
| `go.mod` / `go.sum` | Modify | Add `google.golang.org/grpc` as direct dependency |

---

## Task 1: Add Dependencies and Install Protoc Tools

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add grpc to go.mod**

```powershell
go get google.golang.org/grpc@latest
go mod tidy
```

Expected: `go.mod` now lists `google.golang.org/grpc` as a direct dependency.

- [ ] **Step 2: Install protoc (Protocol Buffer compiler)**

Option A — Chocolatey (recommended on Windows):
```powershell
choco install protoc
```

Option B — Manual: download the latest `protoc-*.zip` for Windows from https://github.com/protocolbuffers/protobuf/releases, extract, and add the `bin/` directory to your `PATH`.

Verify:
```powershell
protoc --version
```
Expected output: `libprotoc 3.x.x` or higher.

- [ ] **Step 3: Install Go protoc plugins**

```powershell
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

Verify both are on `PATH` (they install to `$GOPATH/bin`):
```powershell
protoc-gen-go --version
protoc-gen-go-grpc --version
```

- [ ] **Step 4: Commit**

```powershell
git add go.mod go.sum
git commit -m "chore: add google.golang.org/grpc dependency"
```

---

## Task 2: Write Proto File and Generate Go Stubs

**Files:**
- Create: `proto/manga/manga.proto`
- Generated: `proto/manga/manga.pb.go`, `proto/manga/manga_grpc.pb.go`

- [ ] **Step 1: Create `proto/manga/manga.proto`**

```protobuf
syntax = "proto3";
package manga;
option go_package = "mangahub/proto/manga;pb";

service MangaService {
    rpc GetManga(GetMangaRequest)       returns (MangaResponse);
    rpc SearchManga(SearchRequest)      returns (SearchResponse);
    rpc UpdateProgress(ProgressRequest) returns (ProgressResponse);
}

message GetMangaRequest {
    string id = 1;
}

message SearchRequest {
    string q         = 1;
    string genre     = 2;
    string status    = 3;
    int32  page      = 4;
    int32  page_size = 5;
}

message ProgressRequest {
    string user_id         = 1;
    string manga_id        = 2;
    int32  current_chapter = 3;
    string status          = 4;
}

message MangaResponse {
    string          id             = 1;
    string          title          = 2;
    string          author         = 3;
    repeated string genres         = 4;
    string          status         = 5;
    int32           total_chapters = 6;
    string          description    = 7;
}

message SearchResponse {
    repeated MangaResponse results = 1;
    int32                  count   = 2;
    int32                  total   = 3;
}

message ProgressResponse {
    string manga_id        = 1;
    int32  current_chapter = 2;
    string status          = 3;
}
```

- [ ] **Step 2: Run protoc to generate Go code**

Run from the project root (`d:\Code\MangaHub`):

```powershell
protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative proto/manga/manga.proto
```

- [ ] **Step 3: Verify generated files exist**

```powershell
ls proto/manga/
```

Expected: `manga.proto`, `manga.pb.go`, `manga_grpc.pb.go`

- [ ] **Step 4: Commit**

```powershell
git add proto/
git commit -m "feat(grpc): add MangaService proto definition and generated stubs"
```

---

## Task 3: Implement gRPC Service (TDD)

**Files:**
- Create: `internal/grpc/service_test.go`
- Create: `internal/grpc/service.go`

- [ ] **Step 1: Write `internal/grpc/service_test.go`**

```go
package grpc_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	mangagrpc "mangahub/internal/grpc"
	"mangahub/pkg/database"
	pb "mangahub/proto/manga"
)

func setupDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := database.Connect(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func seedManga(t *testing.T, db *sql.DB) {
	t.Helper()
	genres, _ := json.Marshal([]string{"Action", "Shounen"})
	_, err := db.Exec(
		`INSERT INTO manga (id, title, author, genres, status, total_chapters, description) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"one-piece", "One Piece", "Oda Eiichiro", string(genres), "ongoing", 1100, "Pirates!",
	)
	require.NoError(t, err)
}

func seedUser(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO users (id, username, email, password_hash) VALUES (?, ?, ?, ?)`,
		"user1", "testuser", "test@test.com", "hash",
	)
	require.NoError(t, err)
}

func seedProgress(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO user_progress (user_id, manga_id, current_chapter, status) VALUES (?, ?, ?, ?)`,
		"user1", "one-piece", 10, "reading",
	)
	require.NoError(t, err)
}

func TestGetManga_Found(t *testing.T) {
	db := setupDB(t)
	seedManga(t, db)
	svc := &mangagrpc.Service{DB: db}

	resp, err := svc.GetManga(context.Background(), &pb.GetMangaRequest{Id: "one-piece"})
	require.NoError(t, err)
	assert.Equal(t, "one-piece", resp.Id)
	assert.Equal(t, "One Piece", resp.Title)
	assert.Equal(t, []string{"Action", "Shounen"}, resp.Genres)
	assert.Equal(t, int32(1100), resp.TotalChapters)
}

func TestGetManga_NotFound(t *testing.T) {
	db := setupDB(t)
	svc := &mangagrpc.Service{DB: db}

	_, err := svc.GetManga(context.Background(), &pb.GetMangaRequest{Id: "nonexistent"})
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestSearchManga_QueryFilter(t *testing.T) {
	db := setupDB(t)
	seedManga(t, db)
	svc := &mangagrpc.Service{DB: db}

	resp, err := svc.SearchManga(context.Background(), &pb.SearchRequest{Q: "one", Page: 1, PageSize: 20})
	require.NoError(t, err)
	assert.Equal(t, int32(1), resp.Total)
	assert.Len(t, resp.Results, 1)
	assert.Equal(t, int32(1), resp.Count)
}

func TestSearchManga_GenreFilter(t *testing.T) {
	db := setupDB(t)
	seedManga(t, db)
	svc := &mangagrpc.Service{DB: db}

	resp, err := svc.SearchManga(context.Background(), &pb.SearchRequest{Genre: "Shounen", Page: 1, PageSize: 20})
	require.NoError(t, err)
	assert.Equal(t, int32(1), resp.Total)

	resp2, err := svc.SearchManga(context.Background(), &pb.SearchRequest{Genre: "Romance", Page: 1, PageSize: 20})
	require.NoError(t, err)
	assert.Equal(t, int32(0), resp2.Total)
	assert.Empty(t, resp2.Results)
}

func TestSearchManga_Pagination(t *testing.T) {
	db := setupDB(t)
	seedManga(t, db)
	svc := &mangagrpc.Service{DB: db}

	resp, err := svc.SearchManga(context.Background(), &pb.SearchRequest{Page: 1, PageSize: 1})
	require.NoError(t, err)
	assert.Equal(t, int32(1), resp.Total)
	assert.Len(t, resp.Results, 1)

	// page 2 of a 1-result set is empty
	resp2, err := svc.SearchManga(context.Background(), &pb.SearchRequest{Page: 2, PageSize: 1})
	require.NoError(t, err)
	assert.Equal(t, int32(1), resp2.Total)
	assert.Empty(t, resp2.Results)
}

func TestUpdateProgress_Success(t *testing.T) {
	db := setupDB(t)
	seedManga(t, db)
	seedUser(t, db)
	seedProgress(t, db)
	svc := &mangagrpc.Service{DB: db}

	resp, err := svc.UpdateProgress(context.Background(), &pb.ProgressRequest{
		UserId: "user1", MangaId: "one-piece", CurrentChapter: 50, Status: "reading",
	})
	require.NoError(t, err)
	assert.Equal(t, int32(50), resp.CurrentChapter)
	assert.Equal(t, "one-piece", resp.MangaId)
	assert.Equal(t, "reading", resp.Status)
}

func TestUpdateProgress_MangaNotFound(t *testing.T) {
	db := setupDB(t)
	seedUser(t, db)
	svc := &mangagrpc.Service{DB: db}

	_, err := svc.UpdateProgress(context.Background(), &pb.ProgressRequest{
		UserId: "user1", MangaId: "nonexistent", CurrentChapter: 1,
	})
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestUpdateProgress_ExceedsTotal(t *testing.T) {
	db := setupDB(t)
	seedManga(t, db)
	seedUser(t, db)
	seedProgress(t, db)
	svc := &mangagrpc.Service{DB: db}

	_, err := svc.UpdateProgress(context.Background(), &pb.ProgressRequest{
		UserId: "user1", MangaId: "one-piece", CurrentChapter: 9999,
	})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestUpdateProgress_NotInLibrary(t *testing.T) {
	db := setupDB(t)
	seedManga(t, db)
	seedUser(t, db)
	// no progress seeded
	svc := &mangagrpc.Service{DB: db}

	_, err := svc.UpdateProgress(context.Background(), &pb.ProgressRequest{
		UserId: "user1", MangaId: "one-piece", CurrentChapter: 10,
	})
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}
```

- [ ] **Step 2: Run tests — expect compile error**

```powershell
go test ./internal/grpc/... -v
```

Expected: compilation error — `mangagrpc.Service` undefined (file doesn't exist yet).

- [ ] **Step 3: Create `internal/grpc/service.go`**

```go
package grpc

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	grpccodes "google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	pb "mangahub/proto/manga"
)

// Service implements pb.MangaServiceServer.
type Service struct {
	pb.UnimplementedMangaServiceServer
	DB *sql.DB
}

func (s *Service) GetManga(ctx context.Context, req *pb.GetMangaRequest) (*pb.MangaResponse, error) {
	var m pb.MangaResponse
	var genresStr string
	err := s.DB.QueryRowContext(ctx,
		"SELECT id, title, author, genres, status, total_chapters, description FROM manga WHERE id = ?",
		req.Id,
	).Scan(&m.Id, &m.Title, &m.Author, &genresStr, &m.Status, &m.TotalChapters, &m.Description)
	if err == sql.ErrNoRows {
		return nil, grpcstatus.Errorf(grpccodes.NotFound, "manga %q not found", req.Id)
	}
	if err != nil {
		return nil, grpcstatus.Errorf(grpccodes.Internal, "db: %v", err)
	}
	json.Unmarshal([]byte(genresStr), &m.Genres) //nolint:errcheck
	return &m, nil
}

func (s *Service) SearchManga(ctx context.Context, req *pb.SearchRequest) (*pb.SearchResponse, error) {
	page, pageSize := req.Page, req.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}

	query := "SELECT id, title, author, genres, status, total_chapters, description FROM manga WHERE 1=1"
	args := []interface{}{}
	if req.Q != "" {
		query += " AND (LOWER(title) LIKE LOWER(?) OR LOWER(author) LIKE LOWER(?))"
		like := "%" + req.Q + "%"
		args = append(args, like, like)
	}
	if req.Status != "" {
		query += " AND status = ?"
		args = append(args, req.Status)
	}

	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, grpcstatus.Errorf(grpccodes.Internal, "db: %v", err)
	}
	defer rows.Close()

	var all []*pb.MangaResponse
	for rows.Next() {
		var m pb.MangaResponse
		var genresStr string
		if err := rows.Scan(&m.Id, &m.Title, &m.Author, &genresStr, &m.Status, &m.TotalChapters, &m.Description); err != nil {
			continue
		}
		json.Unmarshal([]byte(genresStr), &m.Genres) //nolint:errcheck
		if req.Genre != "" {
			match := false
			for _, g := range m.Genres {
				if strings.EqualFold(g, req.Genre) {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		all = append(all, &m)
	}

	total := int32(len(all))
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	pageResults := all[start:end]
	if pageResults == nil {
		pageResults = []*pb.MangaResponse{}
	}
	return &pb.SearchResponse{Results: pageResults, Count: int32(len(pageResults)), Total: total}, nil
}

func (s *Service) UpdateProgress(ctx context.Context, req *pb.ProgressRequest) (*pb.ProgressResponse, error) {
	var totalChapters int32
	err := s.DB.QueryRowContext(ctx, "SELECT total_chapters FROM manga WHERE id = ?", req.MangaId).Scan(&totalChapters)
	if err == sql.ErrNoRows {
		return nil, grpcstatus.Errorf(grpccodes.NotFound, "manga %q not found", req.MangaId)
	}
	if err != nil {
		return nil, grpcstatus.Errorf(grpccodes.Internal, "db: %v", err)
	}
	if totalChapters > 0 && req.CurrentChapter > totalChapters {
		return nil, grpcstatus.Errorf(grpccodes.InvalidArgument, "chapter %d exceeds total (%d)", req.CurrentChapter, totalChapters)
	}

	var currentStatus string
	err = s.DB.QueryRowContext(ctx,
		"SELECT status FROM user_progress WHERE user_id = ? AND manga_id = ?",
		req.UserId, req.MangaId,
	).Scan(&currentStatus)
	if err == sql.ErrNoRows {
		return nil, grpcstatus.Errorf(grpccodes.NotFound, "manga not in library")
	}
	if err != nil {
		return nil, grpcstatus.Errorf(grpccodes.Internal, "db: %v", err)
	}

	newStatus := currentStatus
	if req.Status != "" {
		newStatus = req.Status
	}

	_, err = s.DB.ExecContext(ctx,
		`UPDATE user_progress SET current_chapter = ?, status = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE user_id = ? AND manga_id = ?`,
		req.CurrentChapter, newStatus, req.UserId, req.MangaId,
	)
	if err != nil {
		return nil, grpcstatus.Errorf(grpccodes.Internal, "db: %v", err)
	}

	go notifyTCPServer(req.UserId, req.MangaId, req.CurrentChapter)

	return &pb.ProgressResponse{MangaId: req.MangaId, CurrentChapter: req.CurrentChapter, Status: newStatus}, nil
}

var tcpClient = &http.Client{Timeout: time.Second}

func notifyTCPServer(userID, mangaID string, chapter int32) {
	addr := os.Getenv("TCP_INTERNAL_URL")
	if addr == "" {
		addr = "http://localhost:9099"
	}
	payload, _ := json.Marshal(struct {
		UserID    string `json:"user_id"`
		MangaID   string `json:"manga_id"`
		Chapter   int32  `json:"chapter"`
		Timestamp int64  `json:"timestamp"`
	}{UserID: userID, MangaID: mangaID, Chapter: chapter, Timestamp: time.Now().Unix()})
	resp, err := tcpClient.Post(addr+"/internal/broadcast", "application/json", bytes.NewReader(payload))
	if err != nil {
		log.Printf("grpc: TCP notify failed: %v", err)
		return
	}
	defer resp.Body.Close()
}
```

- [ ] **Step 4: Run tests — expect PASS**

```powershell
go test ./internal/grpc/... -v -run TestGetManga
go test ./internal/grpc/... -v -run TestSearchManga
go test ./internal/grpc/... -v -run TestUpdateProgress
```

Expected: all 8 service tests pass (TCP notify fails silently — no TCP server in test, that's expected).

- [ ] **Step 5: Commit**

```powershell
git add internal/grpc/service.go internal/grpc/service_test.go
git commit -m "feat(grpc): implement MangaServiceServer with GetManga, SearchManga, UpdateProgress"
```

---

## Task 4: Implement gRPC Client Adapter (TDD)

**Files:**
- Modify: `pkg/models/models.go`
- Create: `internal/grpc/client_test.go`
- Create: `internal/grpc/client.go`

- [ ] **Step 1: Add sentinel errors to `pkg/models/models.go`**

Add after the existing imports:

```go
import (
	"errors"
	"time"
)

var (
	ErrNotFound        = errors.New("not found")
	ErrInvalidArgument = errors.New("invalid argument")
)
```

The full updated file:

```go
package models

import (
	"errors"
	"time"
)

var (
	ErrNotFound        = errors.New("not found")
	ErrInvalidArgument = errors.New("invalid argument")
)

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

- [ ] **Step 2: Write `internal/grpc/client_test.go`**

```go
package grpc_test

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	grpclib "google.golang.org/grpc"
	mangagrpc "mangahub/internal/grpc"
	"mangahub/pkg/models"
	pb "mangahub/proto/manga"
)

// setupClientTest starts an in-process gRPC server backed by a real in-memory DB
// and returns a connected Client adapter for testing.
func setupClientTest(t *testing.T) (*mangagrpc.Client, func()) {
	t.Helper()
	db := setupDB(t)
	svc := &mangagrpc.Service{DB: db}

	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	s := grpclib.NewServer()
	pb.RegisterMangaServiceServer(s, svc)
	go s.Serve(lis) //nolint:errcheck

	client, err := mangagrpc.NewClient(lis.Addr().String())
	require.NoError(t, err)

	seedManga(t, db)
	seedUser(t, db)
	seedProgress(t, db)

	return client, func() {
		client.Close()  //nolint:errcheck
		s.Stop()
	}
}

func TestClient_GetManga_Found(t *testing.T) {
	client, cleanup := setupClientTest(t)
	defer cleanup()

	m, err := client.GetManga(context.Background(), "one-piece")
	require.NoError(t, err)
	assert.Equal(t, "one-piece", m.ID)
	assert.Equal(t, "One Piece", m.Title)
	assert.Equal(t, []string{"Action", "Shounen"}, m.Genres)
	assert.Equal(t, 1100, m.TotalChapters)
}

func TestClient_GetManga_NotFound(t *testing.T) {
	client, cleanup := setupClientTest(t)
	defer cleanup()

	_, err := client.GetManga(context.Background(), "nonexistent")
	require.Error(t, err)
	assert.True(t, errors.Is(err, models.ErrNotFound))
}

func TestClient_SearchManga_ReturnsResults(t *testing.T) {
	client, cleanup := setupClientTest(t)
	defer cleanup()

	results, total, err := client.SearchManga(context.Background(), "one", "", "", 1, 20)
	require.NoError(t, err)
	assert.Equal(t, int32(1), total)
	assert.Len(t, results, 1)
	assert.Equal(t, "one-piece", results[0].ID)
}

func TestClient_SearchManga_EmptyResults(t *testing.T) {
	client, cleanup := setupClientTest(t)
	defer cleanup()

	results, total, err := client.SearchManga(context.Background(), "zzznomatch", "", "", 1, 20)
	require.NoError(t, err)
	assert.Equal(t, int32(0), total)
	assert.Empty(t, results)
}

func TestClient_UpdateProgress_Success(t *testing.T) {
	client, cleanup := setupClientTest(t)
	defer cleanup()

	up, err := client.UpdateProgress(context.Background(), "user1", "one-piece", 50, "reading")
	require.NoError(t, err)
	assert.Equal(t, 50, up.CurrentChapter)
	assert.Equal(t, "one-piece", up.MangaID)
	assert.Equal(t, "reading", up.Status)
}

func TestClient_UpdateProgress_NotFound(t *testing.T) {
	client, cleanup := setupClientTest(t)
	defer cleanup()

	_, err := client.UpdateProgress(context.Background(), "user1", "nonexistent", 1, "reading")
	require.Error(t, err)
	assert.True(t, errors.Is(err, models.ErrNotFound))
}

func TestClient_UpdateProgress_InvalidArgument(t *testing.T) {
	client, cleanup := setupClientTest(t)
	defer cleanup()

	_, err := client.UpdateProgress(context.Background(), "user1", "one-piece", 9999, "reading")
	require.Error(t, err)
	assert.True(t, errors.Is(err, models.ErrInvalidArgument))
}
```

- [ ] **Step 3: Run tests — expect compile error**

```powershell
go test ./internal/grpc/... -v -run TestClient
```

Expected: compilation error — `mangagrpc.NewClient` undefined.

- [ ] **Step 4: Create `internal/grpc/client.go`**

```go
package grpc

import (
	"context"
	"fmt"

	grpclib "google.golang.org/grpc"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	grpcstatus "google.golang.org/grpc/status"
	"mangahub/pkg/models"
	pb "mangahub/proto/manga"
)

// Client is the thin adapter over the generated MangaServiceClient.
// It converts pb.* types to models.* and gRPC status codes to sentinel errors.
type Client struct {
	conn *grpclib.ClientConn
	svc  pb.MangaServiceClient
}

// NewClient dials addr with insecure credentials (no TLS).
func NewClient(addr string) (*Client, error) {
	conn, err := grpclib.NewClient(addr, grpclib.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("grpc dial %s: %w", addr, err)
	}
	return &Client{conn: conn, svc: pb.NewMangaServiceClient(conn)}, nil
}

// Close releases the underlying connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// GetManga fetches one manga by ID. Returns models.ErrNotFound if not found.
func (c *Client) GetManga(ctx context.Context, id string) (*models.Manga, error) {
	resp, err := c.svc.GetManga(ctx, &pb.GetMangaRequest{Id: id})
	if err != nil {
		if grpcstatus.Code(err) == grpccodes.NotFound {
			return nil, models.ErrNotFound
		}
		return nil, err
	}
	return &models.Manga{
		ID:            resp.Id,
		Title:         resp.Title,
		Author:        resp.Author,
		Genres:        resp.Genres,
		Status:        resp.Status,
		TotalChapters: int(resp.TotalChapters),
		Description:   resp.Description,
	}, nil
}

// SearchManga searches manga with optional filters and pagination.
// Returns (results, total, error) where total is the full match count before paging.
func (c *Client) SearchManga(ctx context.Context, q, genre, statusFilter string, page, pageSize int32) ([]models.Manga, int32, error) {
	resp, err := c.svc.SearchManga(ctx, &pb.SearchRequest{
		Q: q, Genre: genre, Status: statusFilter, Page: page, PageSize: pageSize,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]models.Manga, 0, len(resp.Results))
	for _, r := range resp.Results {
		out = append(out, models.Manga{
			ID:            r.Id,
			Title:         r.Title,
			Author:        r.Author,
			Genres:        r.Genres,
			Status:        r.Status,
			TotalChapters: int(r.TotalChapters),
			Description:   r.Description,
		})
	}
	return out, resp.Total, nil
}

// UpdateProgress updates reading progress.
// Returns models.ErrNotFound if manga or progress record is missing.
// Returns models.ErrInvalidArgument (wrapped) if chapter exceeds total.
func (c *Client) UpdateProgress(ctx context.Context, userID, mangaID string, chapter int32, newStatus string) (*models.UserProgress, error) {
	resp, err := c.svc.UpdateProgress(ctx, &pb.ProgressRequest{
		UserId: userID, MangaId: mangaID, CurrentChapter: chapter, Status: newStatus,
	})
	if err != nil {
		switch grpcstatus.Code(err) {
		case grpccodes.NotFound:
			return nil, models.ErrNotFound
		case grpccodes.InvalidArgument:
			return nil, fmt.Errorf("%w: %s", models.ErrInvalidArgument, grpcstatus.Convert(err).Message())
		}
		return nil, err
	}
	return &models.UserProgress{
		MangaID:        resp.MangaId,
		CurrentChapter: int(resp.CurrentChapter),
		Status:         resp.Status,
	}, nil
}
```

- [ ] **Step 5: Run all grpc tests — expect PASS**

```powershell
go test ./internal/grpc/... -v
```

Expected: all tests pass (14 total: 8 service + 6 client).

- [ ] **Step 6: Commit**

```powershell
git add pkg/models/models.go internal/grpc/client.go internal/grpc/client_test.go
git commit -m "feat(grpc): add client adapter with pb-to-models conversion and sentinel errors"
```

---

## Task 5: Create cmd/grpc-server

**Files:**
- Create: `cmd/grpc-server/main.go`

- [ ] **Step 1: Create `cmd/grpc-server/main.go`**

```go
package main

import (
	"log"
	"net"
	"os"

	grpclib "google.golang.org/grpc"
	mangagrpc "mangahub/internal/grpc"
	"mangahub/pkg/database"
	pb "mangahub/proto/manga"
)

func main() {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./data/mangahub.db"
	}

	db, err := database.Connect(dbPath)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer db.Close()

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	s := grpclib.NewServer()
	pb.RegisterMangaServiceServer(s, &mangagrpc.Service{DB: db})

	log.Println("gRPC server listening on :50051")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
```

- [ ] **Step 2: Build and verify**

```powershell
go build ./cmd/grpc-server/
```

Expected: no errors, produces `grpc-server.exe`.

- [ ] **Step 3: Commit**

```powershell
git add cmd/grpc-server/main.go
git commit -m "feat(grpc): add grpc-server binary"
```

---

## Task 6: Create cmd/grpc-client

**Files:**
- Create: `cmd/grpc-client/main.go`

- [ ] **Step 1: Create `cmd/grpc-client/main.go`**

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	mangagrpc "mangahub/internal/grpc"
)

func main() {
	addr    := flag.String("addr", "localhost:50051", "gRPC server address")
	userID  := flag.String("user", "", "user ID for UpdateProgress demo (optional)")
	mangaID := flag.String("manga", "one-piece", "manga ID for UpdateProgress demo")
	chapter := flag.Int("chapter", 100, "chapter number for UpdateProgress demo")
	status  := flag.String("status", "reading", "reading status for UpdateProgress demo")
	flag.Parse()

	client, err := mangagrpc.NewClient(*addr)
	if err != nil {
		log.Fatalf("connect to %s: %v", *addr, err)
	}
	defer client.Close() //nolint:errcheck

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// UC-014: GetManga
	fmt.Println("=== UC-014: GetManga (id=one-piece) ===")
	m, err := client.GetManga(ctx, "one-piece")
	if err != nil {
		log.Printf("GetManga error: %v", err)
	} else {
		fmt.Printf("ID:          %s\nTitle:       %s\nAuthor:      %s\nGenres:      %v\nStatus:      %s\nChapters:    %d\nDescription: %s\n\n",
			m.ID, m.Title, m.Author, m.Genres, m.Status, m.TotalChapters, m.Description)
	}

	// UC-015: SearchManga
	fmt.Println("=== UC-015: SearchManga (q=one, page=1, page_size=5) ===")
	results, total, err := client.SearchManga(ctx, "one", "", "", 1, 5)
	if err != nil {
		log.Printf("SearchManga error: %v", err)
	} else {
		fmt.Printf("Total matching: %d\n", total)
		for _, r := range results {
			fmt.Printf("  - [%s] %s by %s\n", r.ID, r.Title, r.Author)
		}
		fmt.Println()
	}

	// UC-016: UpdateProgress (only if --user provided)
	fmt.Println("=== UC-016: UpdateProgress ===")
	if *userID == "" {
		fmt.Println("Skipped — provide --user <user_id> to demo UpdateProgress")
		return
	}
	up, err := client.UpdateProgress(ctx, *userID, *mangaID, int32(*chapter), *status)
	if err != nil {
		log.Printf("UpdateProgress error: %v", err)
	} else {
		fmt.Printf("Updated: manga=%s chapter=%d status=%s\n", up.MangaID, up.CurrentChapter, up.Status)
	}
}
```

- [ ] **Step 2: Build and verify**

```powershell
go build ./cmd/grpc-client/
```

Expected: no errors, produces `grpc-client.exe`.

- [ ] **Step 3: Commit**

```powershell
git add cmd/grpc-client/main.go
git commit -m "feat(grpc): add grpc-client demo binary"
```

---

## Task 7: Modify manga.Handler (TDD)

**Files:**
- Modify: `internal/manga/handler.go`
- Modify: `internal/manga/handler_test.go`

- [ ] **Step 1: Update `internal/manga/handler_test.go`**

Replace the entire file with:

```go
package manga_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"mangahub/internal/auth"
	"mangahub/internal/manga"
	"mangahub/pkg/database"
	"mangahub/pkg/models"
)

// mockMangaGRPC implements manga.MangaGRPCClient for tests.
type mockMangaGRPC struct {
	manga  *models.Manga
	list   []models.Manga
	total  int32
	err    error
}

func (m *mockMangaGRPC) GetManga(_ context.Context, _ string) (*models.Manga, error) {
	return m.manga, m.err
}

func (m *mockMangaGRPC) SearchManga(_ context.Context, _, _, _ string, _, _ int32) ([]models.Manga, int32, error) {
	return m.list, m.total, m.err
}

var defaultMangaData = &models.Manga{
	ID:            "one-piece",
	Title:         "One Piece",
	Author:        "Oda Eiichiro",
	Genres:        []string{"Action", "Shounen"},
	Status:        "ongoing",
	TotalChapters: 1100,
	Description:   "Pirates!",
}

func defaultMock() *mockMangaGRPC {
	return &mockMangaGRPC{
		manga: defaultMangaData,
		list:  []models.Manga{*defaultMangaData},
		total: 1,
	}
}

func setupMangaRouterWithMock(t *testing.T, grpcMock manga.MangaGRPCClient) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := database.Connect(":memory:")
	if err != nil {
		t.Fatalf("db connect: %v", err)
	}
	// DB still seeded — needed for Create tests
	_, err = db.Exec(`INSERT INTO manga (id, title, author, genres, status, total_chapters, description)
		VALUES ('one-piece', 'One Piece', 'Oda Eiichiro', '["Action","Shounen"]', 'ongoing', 1100, 'Pirates!')`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	authH := &auth.Handler{DB: db, JWTSecret: "test-secret"}
	h := &manga.Handler{DB: db, GRPCClient: grpcMock}

	r := gin.New()
	r.POST("/auth/register", authH.Register)
	r.POST("/auth/login", authH.Login)
	r.GET("/manga", h.Search)
	r.GET("/manga/:id", h.GetByID)

	doPost(r, "/auth/register", `{"username":"tester","email":"t@t.com","password":"password123"}`)
	loginResp := doPost(r, "/auth/login", `{"username":"tester","password":"password123"}`)
	var loginBody map[string]interface{}
	json.NewDecoder(loginResp.Body).Decode(&loginBody)
	token := loginBody["token"].(string)

	protected := r.Group("/")
	protected.Use(authH.JWTMiddleware())
	protected.POST("/manga", h.Create)

	return r, token
}

func setupMangaRouter(t *testing.T) (*gin.Engine, string) {
	return setupMangaRouterWithMock(t, defaultMock())
}

func doPost(r *gin.Engine, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func authReq(r *gin.Engine, method, path, body, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// GET /manga

func TestSearch_ReturnsResults(t *testing.T) {
	r, _ := setupMangaRouter(t)
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

func TestSearch_ReturnsPaginationFields(t *testing.T) {
	r, _ := setupMangaRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/manga?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if _, ok := resp["total"]; !ok {
		t.Error("expected 'total' field in response")
	}
	if _, ok := resp["page"]; !ok {
		t.Error("expected 'page' field in response")
	}
}

func TestSearch_EmptyQuery_ReturnsAll(t *testing.T) {
	r, _ := setupMangaRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/manga", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSearch_GRPCError_Returns500(t *testing.T) {
	mock := &mockMangaGRPC{err: errors.New("grpc unavailable")}
	r, _ := setupMangaRouterWithMock(t, mock)
	req := httptest.NewRequest(http.MethodGet, "/manga", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestSearch_GenreFilter(t *testing.T) {
	r, _ := setupMangaRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/manga?genre=Shounen", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestGetByID_Found(t *testing.T) {
	r, _ := setupMangaRouter(t)
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
	mock := &mockMangaGRPC{err: models.ErrNotFound}
	r, _ := setupMangaRouterWithMock(t, mock)
	req := httptest.NewRequest(http.MethodGet, "/manga/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// POST /manga (unchanged — still uses DB)

func TestCreateManga_Success(t *testing.T) {
	r, token := setupMangaRouter(t)
	body := `{"id":"naruto","title":"Naruto","author":"Kishimoto Masashi","genres":["Action","Shounen"],"status":"completed","total_chapters":700,"description":"A ninja story."}`
	w := authReq(r, http.MethodPost, "/manga", body, token)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateManga_InvalidStatus(t *testing.T) {
	r, token := setupMangaRouter(t)
	body := `{"id":"test-manga","title":"Test","author":"Author","genres":["Action"],"status":"invalid"}`
	w := authReq(r, http.MethodPost, "/manga", body, token)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateManga_InvalidGenre(t *testing.T) {
	r, token := setupMangaRouter(t)
	body := `{"id":"test-manga","title":"Test","author":"Author","genres":["NotAGenre"],"status":"ongoing"}`
	w := authReq(r, http.MethodPost, "/manga", body, token)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateManga_Duplicate(t *testing.T) {
	r, token := setupMangaRouter(t)
	body := `{"id":"one-piece","title":"One Piece","author":"Oda","genres":["Action"],"status":"ongoing"}`
	w := authReq(r, http.MethodPost, "/manga", body, token)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateManga_NoAuth(t *testing.T) {
	r, _ := setupMangaRouter(t)
	body := `{"id":"new-manga","title":"New","author":"Author","genres":["Action"],"status":"ongoing"}`
	req := httptest.NewRequest(http.MethodPost, "/manga", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestCreateManga_MissingFields(t *testing.T) {
	r, token := setupMangaRouter(t)
	body := `{"id":"partial-manga","title":"Partial"}`
	w := authReq(r, http.MethodPost, "/manga", body, token)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
}
```

- [ ] **Step 2: Run tests — expect compile error**

```powershell
go test ./internal/manga/... -v
```

Expected: compilation error — `manga.MangaGRPCClient` undefined and `manga.Handler` has no `GRPCClient` field.

- [ ] **Step 3: Replace `internal/manga/handler.go`**

```go
package manga

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"mangahub/pkg/models"
)

// MangaGRPCClient is satisfied by *grpc.Client from internal/grpc.
type MangaGRPCClient interface {
	GetManga(ctx context.Context, id string) (*models.Manga, error)
	SearchManga(ctx context.Context, q, genre, statusFilter string, page, pageSize int32) ([]models.Manga, int32, error)
}

// AllowedGenres defines the set of accepted genre strings.
var AllowedGenres = map[string]bool{
	"Action": true, "Adventure": true, "Comedy": true, "Drama": true,
	"Fantasy": true, "Horror": true, "Mystery": true, "Psychological": true,
	"Romance": true, "Sci-Fi": true, "Slice of Life": true, "Sports": true,
	"Supernatural": true, "Thriller": true, "Historical": true, "Music": true,
	"School": true, "Magic": true, "Fashion": true,
	"Shounen": true, "Shoujo": true, "Seinen": true, "Josei": true,
}

// AllowedStatuses defines the accepted manga publication statuses.
var AllowedStatuses = map[string]bool{
	"ongoing": true, "completed": true, "hiatus": true,
}

type Handler struct {
	DB         *sql.DB         // used by Create
	GRPCClient MangaGRPCClient // used by GetByID, Search
}

func (h *Handler) Search(c *gin.Context) {
	q := c.Query("q")
	genre := c.Query("genre")
	status := c.Query("status")

	page := int32(1)
	pageSize := int32(20)
	if p, err := strconv.Atoi(c.DefaultQuery("page", "1")); err == nil && p > 0 {
		page = int32(p)
	}
	if ps, err := strconv.Atoi(c.DefaultQuery("page_size", "20")); err == nil && ps > 0 {
		pageSize = int32(ps)
	}

	results, total, err := h.GRPCClient.SearchManga(c.Request.Context(), q, genre, status, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "search failed"})
		return
	}
	if results == nil {
		results = []models.Manga{}
	}
	c.JSON(http.StatusOK, gin.H{
		"results":   results,
		"count":     len(results),
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (h *Handler) GetByID(c *gin.Context) {
	id := c.Param("id")
	m, err := h.GRPCClient.GetManga(c.Request.Context(), id)
	if errors.Is(err, models.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "manga not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get manga"})
		return
	}
	c.JSON(http.StatusOK, m)
}

type createMangaRequest struct {
	ID            string   `json:"id"             binding:"required"`
	Title         string   `json:"title"          binding:"required"`
	Author        string   `json:"author"         binding:"required"`
	Genres        []string `json:"genres"         binding:"required,min=1"`
	Status        string   `json:"status"         binding:"required"`
	TotalChapters int      `json:"total_chapters" binding:"min=0"`
	Description   string   `json:"description"`
}

func (h *Handler) Create(c *gin.Context) {
	var req createMangaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	if !AllowedStatuses[strings.ToLower(req.Status)] {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid status; must be one of: ongoing, completed, hiatus",
		})
		return
	}
	req.Status = strings.ToLower(req.Status)

	for _, g := range req.Genres {
		if !AllowedGenres[g] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown genre: " + g})
			return
		}
	}

	genres, _ := json.Marshal(req.Genres)
	_, err := h.DB.Exec(
		`INSERT INTO manga (id, title, author, genres, status, total_chapters, description)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		req.ID, req.Title, req.Author, string(genres), req.Status, req.TotalChapters, req.Description,
	)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "manga with this ID already exists"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "manga created", "id": req.ID})
}
```

- [ ] **Step 4: Run tests — expect PASS**

```powershell
go test ./internal/manga/... -v
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```powershell
git add internal/manga/handler.go internal/manga/handler_test.go
git commit -m "feat(grpc): manga.Handler Search and GetByID now call gRPC; add pagination to Search"
```

---

## Task 8: Modify user.Handler (TDD)

**Files:**
- Modify: `internal/user/handler.go`
- Modify: `internal/user/handler_test.go`

- [ ] **Step 1: Update `internal/user/handler_test.go`**

Replace the `setupUserRouter` function and all `TestUpdateProgress_*` tests. Keep all other tests (`TestAddToLibrary_*`, `TestGetLibrary_*`, `TestRemoveFromLibrary_*`) exactly as they are since those methods are unchanged.

```go
package user_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"mangahub/internal/auth"
	"mangahub/internal/user"
	"mangahub/pkg/database"
	"mangahub/pkg/models"
)

// mockProgressGRPC implements user.ProgressGRPCClient for tests.
type mockProgressGRPC struct {
	result *models.UserProgress
	err    error
}

func (m *mockProgressGRPC) UpdateProgress(_ context.Context, _, mangaID string, chapter int32, status string) (*models.UserProgress, error) {
	if m.result != nil {
		m.result.MangaID = mangaID
		m.result.CurrentChapter = int(chapter)
		m.result.Status = status
	}
	return m.result, m.err
}

func defaultProgressMock() *mockProgressGRPC {
	return &mockProgressGRPC{
		result: &models.UserProgress{},
	}
}

func setupUserRouterWithMock(t *testing.T, grpcMock user.ProgressGRPCClient) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := database.Connect(":memory:")
	if err != nil {
		t.Fatalf("db connect: %v", err)
	}
	_, err = db.Exec(`INSERT INTO manga (id, title, author, genres, status, total_chapters, description)
		VALUES ('one-piece', 'One Piece', 'Oda Eiichiro', '["Shounen"]', 'ongoing', 1100, 'Pirates!')`)
	if err != nil {
		t.Fatalf("seed manga: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	authH := &auth.Handler{DB: db, JWTSecret: "test-secret"}
	userH := &user.Handler{DB: db, GRPCClient: grpcMock}

	r := gin.New()
	r.POST("/auth/register", authH.Register)
	r.POST("/auth/login", authH.Login)

	protected := r.Group("/")
	protected.Use(authH.JWTMiddleware())
	protected.POST("/users/library", userH.AddToLibrary)
	protected.GET("/users/library", userH.GetLibrary)
	protected.DELETE("/users/library/:manga_id", userH.RemoveFromLibrary)
	protected.PUT("/users/progress", userH.UpdateProgress)

	doPost(r, "/auth/register", `{"username":"testuser","email":"test@test.com","password":"password123"}`)
	loginResp := doPost(r, "/auth/login", `{"username":"testuser","password":"password123"}`)
	var loginBody map[string]interface{}
	json.NewDecoder(loginResp.Body).Decode(&loginBody)
	token := loginBody["token"].(string)

	return r, token
}

func setupUserRouter(t *testing.T) (*gin.Engine, string) {
	return setupUserRouterWithMock(t, defaultProgressMock())
}

func doPost(r *gin.Engine, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func authReq(r *gin.Engine, method, path, body, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// POST /users/library (unchanged — keep original tests as-is)

func TestAddToLibrary_Success(t *testing.T) {
	r, token := setupUserRouter(t)
	w := authReq(r, http.MethodPost, "/users/library", `{"manga_id":"one-piece","status":"reading","current_chapter":10}`, token)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAddToLibrary_MangaNotFound(t *testing.T) {
	r, token := setupUserRouter(t)
	w := authReq(r, http.MethodPost, "/users/library", `{"manga_id":"nonexistent","status":"reading"}`, token)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestAddToLibrary_InvalidStatus(t *testing.T) {
	r, token := setupUserRouter(t)
	w := authReq(r, http.MethodPost, "/users/library", `{"manga_id":"one-piece","status":"invalid"}`, token)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestAddToLibrary_Duplicate(t *testing.T) {
	r, token := setupUserRouter(t)
	authReq(r, http.MethodPost, "/users/library", `{"manga_id":"one-piece","status":"reading"}`, token)
	w := authReq(r, http.MethodPost, "/users/library", `{"manga_id":"one-piece","status":"reading"}`, token)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

func TestAddToLibrary_NoAuth(t *testing.T) {
	r, _ := setupUserRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/users/library", bytes.NewBufferString(`{"manga_id":"one-piece","status":"reading"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// GET /users/library

func TestGetLibrary_Empty(t *testing.T) {
	r, token := setupUserRouter(t)
	w := authReq(r, http.MethodGet, "/users/library", "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["total"].(float64) != 0 {
		t.Errorf("expected total=0, got %v", resp["total"])
	}
}

func TestGetLibrary_WithEntries(t *testing.T) {
	r, token := setupUserRouter(t)
	authReq(r, http.MethodPost, "/users/library", `{"manga_id":"one-piece","status":"reading","current_chapter":50}`, token)
	w := authReq(r, http.MethodGet, "/users/library", "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["total"].(float64) != 1 {
		t.Errorf("expected total=1, got %v", resp["total"])
	}
}

// DELETE /users/library/:manga_id

func TestRemoveFromLibrary_Success(t *testing.T) {
	r, token := setupUserRouter(t)
	authReq(r, http.MethodPost, "/users/library", `{"manga_id":"one-piece","status":"reading"}`, token)
	w := authReq(r, http.MethodDelete, "/users/library/one-piece", "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRemoveFromLibrary_NotInLibrary(t *testing.T) {
	r, token := setupUserRouter(t)
	w := authReq(r, http.MethodDelete, "/users/library/one-piece", "", token)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRemoveFromLibrary_NoAuth(t *testing.T) {
	r, _ := setupUserRouter(t)
	req := httptest.NewRequest(http.MethodDelete, "/users/library/one-piece", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// PUT /users/progress (now via gRPC mock)

func TestUpdateProgress_Success(t *testing.T) {
	r, token := setupUserRouter(t)
	w := authReq(r, http.MethodPut, "/users/progress", `{"manga_id":"one-piece","current_chapter":100}`, token)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["manga_id"] != "one-piece" {
		t.Errorf("expected manga_id=one-piece, got %v", resp["manga_id"])
	}
}

func TestUpdateProgress_ExceedsTotal(t *testing.T) {
	mock := &mockProgressGRPC{
		err: fmt.Errorf("%w: chapter 9999 exceeds total (1100)", models.ErrInvalidArgument),
	}
	r, token := setupUserRouterWithMock(t, mock)
	w := authReq(r, http.MethodPut, "/users/progress", `{"manga_id":"one-piece","current_chapter":9999}`, token)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateProgress_NotInLibrary(t *testing.T) {
	mock := &mockProgressGRPC{err: models.ErrNotFound}
	r, token := setupUserRouterWithMock(t, mock)
	w := authReq(r, http.MethodPut, "/users/progress", `{"manga_id":"one-piece","current_chapter":100}`, token)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateProgress_MangaNotFound(t *testing.T) {
	mock := &mockProgressGRPC{err: models.ErrNotFound}
	r, token := setupUserRouterWithMock(t, mock)
	w := authReq(r, http.MethodPut, "/users/progress", `{"manga_id":"nonexistent","current_chapter":1}`, token)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}
```

- [ ] **Step 2: Run tests — expect compile error**

```powershell
go test ./internal/user/... -v
```

Expected: compilation error — `user.ProgressGRPCClient` undefined and `user.Handler` has no `GRPCClient` field.

- [ ] **Step 3: Replace `internal/user/handler.go`**

```go
package user

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"mangahub/pkg/models"
)

// ProgressGRPCClient is satisfied by *grpc.Client from internal/grpc.
type ProgressGRPCClient interface {
	UpdateProgress(ctx context.Context, userID, mangaID string, chapter int32, newStatus string) (*models.UserProgress, error)
}

type Handler struct {
	DB         *sql.DB              // used by AddToLibrary, GetLibrary, RemoveFromLibrary
	GRPCClient ProgressGRPCClient   // used by UpdateProgress
}

var validStatuses = map[string]bool{
	"reading":      true,
	"completed":    true,
	"plan_to_read": true,
	"on_hold":      true,
	"dropped":      true,
}

type addToLibraryRequest struct {
	MangaID        string `json:"manga_id"        binding:"required"`
	Status         string `json:"status"          binding:"required"`
	CurrentChapter int    `json:"current_chapter"`
}

type updateProgressRequest struct {
	MangaID        string `json:"manga_id"        binding:"required"`
	CurrentChapter int    `json:"current_chapter" binding:"min=0"`
	Status         string `json:"status"`
}

type libraryEntry struct {
	MangaID        string `json:"manga_id"`
	Title          string `json:"title"`
	CurrentChapter int    `json:"current_chapter"`
	Status         string `json:"status"`
	UpdatedAt      string `json:"updated_at"`
}

func (h *Handler) AddToLibrary(c *gin.Context) {
	userID := c.GetString("user_id")

	var req addToLibraryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	if !validStatuses[req.Status] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status, must be one of: reading, completed, plan_to_read, on_hold, dropped"})
		return
	}

	var exists int
	if err := h.DB.QueryRow("SELECT COUNT(*) FROM manga WHERE id = ?", req.MangaID).Scan(&exists); err != nil || exists == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "manga not found"})
		return
	}

	_, err := h.DB.Exec(
		`INSERT INTO user_progress (user_id, manga_id, current_chapter, status) VALUES (?, ?, ?, ?)`,
		userID, req.MangaID, req.CurrentChapter, req.Status,
	)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "manga already in library"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":         "added to library",
		"manga_id":        req.MangaID,
		"status":          req.Status,
		"current_chapter": req.CurrentChapter,
	})
}

func (h *Handler) GetLibrary(c *gin.Context) {
	userID := c.GetString("user_id")

	rows, err := h.DB.Query(`
		SELECT up.manga_id, m.title, up.current_chapter, up.status, up.updated_at
		FROM user_progress up
		JOIN manga m ON m.id = up.manga_id
		WHERE up.user_id = ?
		ORDER BY up.updated_at DESC
	`, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer rows.Close()

	lists := map[string][]libraryEntry{
		"reading":      {},
		"completed":    {},
		"plan_to_read": {},
		"on_hold":      {},
		"dropped":      {},
	}
	total := 0

	for rows.Next() {
		var e libraryEntry
		if err := rows.Scan(&e.MangaID, &e.Title, &e.CurrentChapter, &e.Status, &e.UpdatedAt); err != nil {
			continue
		}
		lists[e.Status] = append(lists[e.Status], e)
		total++
	}

	c.JSON(http.StatusOK, gin.H{"reading_lists": lists, "total": total})
}

func (h *Handler) RemoveFromLibrary(c *gin.Context) {
	userID := c.GetString("user_id")
	mangaID := c.Param("manga_id")

	result, err := h.DB.Exec(
		"DELETE FROM user_progress WHERE user_id = ? AND manga_id = ?",
		userID, mangaID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "manga not in library"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "removed from library", "manga_id": mangaID})
}

func (h *Handler) UpdateProgress(c *gin.Context) {
	userID := c.GetString("user_id")

	var req updateProgressRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	if req.Status != "" && !validStatuses[req.Status] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
		return
	}

	up, err := h.GRPCClient.UpdateProgress(c.Request.Context(), userID, req.MangaID, int32(req.CurrentChapter), req.Status)
	if errors.Is(err, models.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "manga not found or not in library"})
		return
	}
	if errors.Is(err, models.ErrInvalidArgument) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":         "progress updated",
		"manga_id":        up.MangaID,
		"current_chapter": up.CurrentChapter,
		"status":          up.Status,
	})
}
```

- [ ] **Step 4: Run tests — expect PASS**

```powershell
go test ./internal/user/... -v
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```powershell
git add internal/user/handler.go internal/user/handler_test.go
git commit -m "feat(grpc): user.Handler UpdateProgress now delegates to gRPC; TCP broadcast moved to gRPC service"
```

---

## Task 9: Wire gRPC Client into api-server

**Files:**
- Modify: `cmd/api-server/main.go`

- [ ] **Step 1: Replace `cmd/api-server/main.go`**

```go
package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"mangahub/internal/auth"
	"mangahub/internal/manga"
	"mangahub/internal/user"
	wschat "mangahub/internal/websocket"
	mangagrpc "mangahub/internal/grpc"
	"mangahub/pkg/database"
)

func main() {
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "mangahub-dev-secret"
	}

	grpcAddr := os.Getenv("GRPC_ADDR")
	if grpcAddr == "" {
		grpcAddr = "localhost:50051"
	}

	db, err := database.Connect("./data/mangahub.db")
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	if err := database.SeedManga(db, "./data/manga.json"); err != nil {
		log.Printf("seed warning: %v", err)
	}

	grpcClient, err := mangagrpc.NewClient(grpcAddr)
	if err != nil {
		log.Fatalf("grpc client: %v", err)
	}
	defer grpcClient.Close() //nolint:errcheck
	log.Printf("gRPC client connected to %s", grpcAddr)

	authHandler := &auth.Handler{DB: db, JWTSecret: jwtSecret}
	mangaHandler := &manga.Handler{DB: db, GRPCClient: grpcClient}
	userHandler := &user.Handler{DB: db, GRPCClient: grpcClient}

	hub := wschat.NewHub()
	go hub.Run()
	wsHandler := &wschat.Handler{Hub: hub, JWTSecret: jwtSecret}

	r := gin.Default()

	r.POST("/auth/register", authHandler.Register)
	r.POST("/auth/login", authHandler.Login)

	r.GET("/manga", mangaHandler.Search)
	r.GET("/manga/:id", mangaHandler.GetByID)
	r.GET("/ws/chat", wsHandler.ServeWS)

	protected := r.Group("/")
	protected.Use(authHandler.JWTMiddleware())
	protected.POST("/manga", mangaHandler.Create)
	protected.POST("/users/library", userHandler.AddToLibrary)
	protected.GET("/users/library", userHandler.GetLibrary)
	protected.DELETE("/users/library/:manga_id", userHandler.RemoveFromLibrary)
	protected.PUT("/users/progress", userHandler.UpdateProgress)

	log.Println("HTTP API server running on :8080")
	if err := r.Run(":8080"); err != nil {
		log.Fatalf("server: %v", err)
	}
}
```

- [ ] **Step 2: Build the api-server**

```powershell
go build ./cmd/api-server/
```

Expected: no errors.

- [ ] **Step 3: Run the full test suite**

```powershell
go test ./... -v
```

Expected: all tests pass.

- [ ] **Step 4: Commit**

```powershell
git add cmd/api-server/main.go
git commit -m "feat(grpc): wire gRPC client into api-server; manga and user handlers now call gRPC"
```

---

## Task 10: End-to-End Smoke Test

**No code changes — verify everything works together.**

- [ ] **Step 1: Start the gRPC server** (terminal 1)

```powershell
go run ./cmd/grpc-server/
```

Expected output: `gRPC server listening on :50051`

- [ ] **Step 2: Start the TCP server** (terminal 2)

```powershell
go run ./cmd/tcp-server/
```

Expected output: `tcp: listening on :9090`

- [ ] **Step 3: Start the HTTP API server** (terminal 3)

```powershell
go run ./cmd/api-server/
```

Expected output:
```
gRPC client connected to localhost:50051
HTTP API server running on :8080
```

- [ ] **Step 4: Run the gRPC demo client** (terminal 4)

```powershell
go run ./cmd/grpc-client/
```

Expected output:
```
=== UC-014: GetManga (id=one-piece) ===
ID:       one-piece
Title:    One Piece
...

=== UC-015: SearchManga (q=one, page=1, page_size=5) ===
Total matching: 1
  - [one-piece] One Piece by Oda Eiichiro

=== UC-016: UpdateProgress ===
Skipped — provide --user <user_id> to demo UpdateProgress
```

- [ ] **Step 5: Verify HTTP endpoints call gRPC**

```powershell
# Search — goes through gRPC
Invoke-WebRequest -Uri "http://localhost:8080/manga?q=one" -Method GET | Select-Object -ExpandProperty Content

# GetByID — goes through gRPC
Invoke-WebRequest -Uri "http://localhost:8080/manga/one-piece" -Method GET | Select-Object -ExpandProperty Content
```

Expected: both return manga data with `total` and `page` fields in the search response.

- [ ] **Step 6: Demo UC-016 with a real user**

Register and login via HTTP, add a manga to library, then demo UpdateProgress via grpc-client:

```powershell
# Register
$body = '{"username":"demouser","email":"demo@test.com","password":"password123"}'
$reg = Invoke-WebRequest -Uri "http://localhost:8080/auth/register" -Method POST -Body $body -ContentType "application/json"

# Login — get token
$login = Invoke-WebRequest -Uri "http://localhost:8080/auth/login" -Method POST -Body '{"username":"demouser","password":"password123"}' -ContentType "application/json"
$token = ($login.Content | ConvertFrom-Json).token

# Add manga to library
$headers = @{Authorization = "Bearer $token"}
Invoke-WebRequest -Uri "http://localhost:8080/users/library" -Method POST -Headers $headers -Body '{"manga_id":"one-piece","status":"reading","current_chapter":10}' -ContentType "application/json"

# Get the user ID from the token (it's in the JWT payload)
# Then run grpc-client with --user flag:
go run ./cmd/grpc-client/ --user <user_id> --manga one-piece --chapter 50 --status reading
```

Expected: `Updated: manga=one-piece chapter=50 status=reading`
Also verify TCP server logs show the broadcast was received.

- [ ] **Step 7: Final commit**

```powershell
git add .
git commit -m "chore: gRPC service complete — all 5 protocols implemented and integrated"
```

---

## Self-Review

**Spec coverage check:**

| Requirement | Covered by |
|---|---|
| Protocol Buffer definitions | Task 2 |
| Basic gRPC server implementation | Task 3 + Task 5 |
| Simple client integration | Task 4 + Task 6 + Task 7 + Task 8 + Task 9 |
| Unary RPC calls | Task 2 (all 3 RPCs are unary) |
| UC-014 GetManga | Task 3, Task 4 |
| UC-015 SearchManga with pagination | Task 3, Task 4, Task 7 |
| UC-016 UpdateProgress + TCP broadcast | Task 3 |
| Connect all protocols (Phase 3) | Task 9 (HTTP → gRPC → DB + TCP) |

**Placeholder scan:** None found. All code blocks are complete.

**Type consistency check:**
- `MangaGRPCClient.SearchManga` signature: `(ctx, q, genre, statusFilter string, page, pageSize int32) ([]models.Manga, int32, error)` — consistent across interface definition (Task 7), mock (Task 7), and concrete implementation (Task 4).
- `ProgressGRPCClient.UpdateProgress` signature: `(ctx, userID, mangaID string, chapter int32, newStatus string) (*models.UserProgress, error)` — consistent across interface (Task 8), mock (Task 8), and concrete implementation (Task 4).
- `models.ErrNotFound` and `models.ErrInvalidArgument` defined in Task 4 Step 1, used in Tasks 4, 7, 8.
- `mangagrpc.Service{DB: db}` — `Service` struct has exported `DB` field, used in Tasks 3 and 4.
