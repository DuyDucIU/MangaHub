# gRPC Internal Service — Design Spec

**Date:** 2026-05-06  
**Protocol:** gRPC (5th and final protocol)  
**Use Cases:** UC-014, UC-015, UC-016  

---

## 1. Goal

Implement the gRPC internal service for MangaHub, covering manga retrieval, search, and reading progress updates. Wire the existing HTTP REST API to call gRPC for those 3 operations (C-full integration), and provide a standalone demo client binary.

---

## 2. Use Cases Covered

| Use Case | gRPC Method | Description |
|---|---|---|
| UC-014 | `GetManga` | Fetch a single manga by ID |
| UC-015 | `SearchManga` | Search/filter manga by query, genre, status |
| UC-016 | `UpdateProgress` | Update reading progress + trigger TCP broadcast |

UC-016 step 4 explicitly requires the gRPC server to trigger TCP broadcast for real-time sync — this moves responsibility from the HTTP handler into the gRPC service.

---

## 3. Integration Decision

**C-full with Approach 2 (thin adapter).**

- The HTTP API server calls gRPC for all 3 use-case operations instead of querying SQLite directly
- A thin adapter package (`internal/grpc/client.go`) wraps the generated protobuf stubs and converts `pb.*` types to `models.*` types — HTTP handlers never see protobuf types
- A standalone `cmd/grpc-client` binary demonstrates all 3 RPC calls explicitly for the demo session
- All other HTTP operations (auth, library management, manga creation) continue to use SQLite directly — no gRPC method exists for those

---

## 4. Data Flow

```
External user
      │
      ▼
cmd/api-server (:8080)
  manga.Handler { GRPCClient, DB }    ← DB kept for Create
  user.Handler  { GRPCClient, DB }    ← DB kept for library ops
      │
      │  GetManga / SearchManga / UpdateProgress
      ▼
internal/grpc/client.go               ← pb → models.* conversion
      │
      ▼
cmd/grpc-server (:50051)
  internal/grpc/service.go
      │
      ├──► SQLite (manga queries, progress updates)
      │
      └──► TCP server :9099            ← UC-016: fire-and-forget HTTP POST
               (same notifyTCPServer pattern as current user.Handler)

cmd/grpc-client                        ← standalone demo binary, dials :50051
```

---

## 5. Proto Definition

File: `proto/manga/manga.proto`

```protobuf
syntax = "proto3";
package manga;
option go_package = "mangahub/proto/manga";

service MangaService {
    rpc GetManga(GetMangaRequest)       returns (MangaResponse);
    rpc SearchManga(SearchRequest)      returns (SearchResponse);
    rpc UpdateProgress(ProgressRequest) returns (ProgressResponse);
}

message GetMangaRequest {
    string id = 1;
}

message SearchRequest {
    string q      = 1;
    string genre  = 2;
    string status = 3;
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
}

message ProgressResponse {
    string manga_id        = 1;
    int32  current_chapter = 2;
    string status          = 3;
}
```

- All RPCs are unary (no streaming) as required by the spec
- `SearchRequest` mirrors the same 3 filters (`q`, `genre`, `status`) as `GET /manga`
- `ProgressResponse` echoes back the updated fields, matching what `PUT /users/progress` currently returns
- No TLS — insecure credentials (localhost demo environment)

---

## 6. File Layout

Follows the project structure recommended in the spec:

### New files

```
proto/
└── manga/
    ├── manga.proto              # source of truth
    ├── manga.pb.go              # generated
    └── manga_grpc.pb.go         # generated

internal/
└── grpc/
    ├── service.go               # MangaServiceServer implementation
    ├── service_test.go          # unit tests
    ├── client.go                # thin adapter: pb → models.*, used by HTTP handlers
    └── client_test.go           # adapter tests

cmd/
├── grpc-server/
│   └── main.go                  # starts gRPC server on :50051
└── grpc-client/
    └── main.go                  # demo binary: calls all 3 RPC methods
```

### Modified files

```
internal/manga/handler.go        # GetByID, Search → call GRPCClient instead of DB
internal/user/handler.go         # UpdateProgress → call GRPCClient instead of DB
cmd/api-server/main.go           # create grpcclient, inject into handlers
go.mod / go.sum                  # add google.golang.org/grpc
```

---

## 7. HTTP Handler Changes

Only 3 handler methods change. Everything else is untouched.

| Handler | Method | Change |
|---|---|---|
| `manga.Handler` | `GetByID` | calls `GRPCClient.GetManga(id)` |
| `manga.Handler` | `Search` | calls `GRPCClient.SearchManga(q, genre, status)` |
| `user.Handler` | `UpdateProgress` | calls `GRPCClient.UpdateProgress(userID, mangaID, chapter, status)` |

`manga.Handler` keeps its `DB` field for the `Create` endpoint.  
`user.Handler` keeps its `DB` field for `AddToLibrary`, `GetLibrary`, `RemoveFromLibrary`.

---

## 8. TCP Broadcast Integration (UC-016)

The gRPC `UpdateProgress` method is responsible for triggering TCP broadcast after a successful DB write. It uses the same fire-and-forget HTTP POST to `:9099/internal/broadcast` that `user.Handler` currently uses. This moves the broadcast trigger from the HTTP layer into the gRPC service layer, which is correct since gRPC now owns the progress update operation.

The HTTP `UpdateProgress` handler no longer calls `notifyTCPServer()` — it delegates the entire operation including broadcast to gRPC.

---

## 9. Demo Script (cmd/grpc-client)

The demo client calls all 3 methods in sequence:

1. `GetManga("one-piece")` — prints manga details  
2. `SearchManga(q="attack")` — prints search results  
3. `UpdateProgress(userID, mangaID, chapter, status)` — prints updated progress (requires flags: `--user`, `--manga`, `--chapter`, `--status`)

This provides explicit, visible proof that gRPC is working during the live demo session.

---

## 10. Ports Summary

| Service | Port | Protocol |
|---|---|---|
| HTTP API | 8080 | HTTP/REST |
| TCP Sync | 9090 | TCP |
| TCP Internal | 9099 | HTTP (internal only) |
| UDP Notify | 9091 | UDP |
| WebSocket | 8080/ws/chat | WS (embedded in HTTP) |
| gRPC | 50051 | gRPC (HTTP/2) |
