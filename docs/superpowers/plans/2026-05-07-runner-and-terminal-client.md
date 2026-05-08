# Runner + Terminal Client Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `cmd/runner` (one command starts all 5 servers) and `cmd/client` (interactive terminal client for all protocols).

**Architecture:** Runner imports all existing internal packages and starts each server as a goroutine inside one process; one `Ctrl+C` gracefully stops everything. Client is a numbered-menu terminal app split across focused files in `package main` — written sequentially, `main.go` last so it can reference all functions.

**Tech Stack:** Go stdlib (`net`, `net/http`, `bufio`, `encoding/json`, `os/signal`), `github.com/gorilla/websocket` (already in go.mod), `github.com/gin-gonic/gin`, `google.golang.org/grpc` — no new dependencies.

---

## File Map

| File | Created/Modified | Responsibility |
|---|---|---|
| `cmd/runner/main.go` | Create | Start all servers as goroutines, graceful shutdown |
| `cmd/client/http.go` | Create | `postJSON`, `getJSON`, `putJSON` HTTP helpers |
| `cmd/client/auth.go` | Create | `doRegister`, `doLogin`, `doLogout`, JWT claim parser |
| `cmd/client/manga.go` | Create | `doSearch`, `doViewDetails`, `doLibrary`, `doAddToLibrary`, `doUpdateProgress` |
| `cmd/client/tcp.go` | Create | `connectTCP`, `listenTCP` background goroutine |
| `cmd/client/udp.go` | Create | `connectUDP`, `listenUDP` background goroutine |
| `cmd/client/ws.go` | Create | `enterChatRoom` WebSocket session |
| `cmd/client/main.go` | Create | `App` struct, `run`, `guestMenu`, `mainMenu`, `prompt`, `cleanup`, `getenv` |

No existing files are modified.

---

## Task 1: Runner

**Files:**
- Create: `cmd/runner/main.go`

The runner replicates the setup from each existing `cmd/*/main.go` but runs all servers as goroutines in one process. It shares one `*sql.DB` between the gRPC service and HTTP handlers — `database/sql` manages a connection pool internally and is goroutine-safe. WebSocket needs no extra goroutine; it is already mounted on the HTTP server at `GET /ws/chat`.

- [ ] **Step 1: Create `cmd/runner/main.go`**

```go
package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	grpclib "google.golang.org/grpc"
	"mangahub/internal/auth"
	mangagrpc "mangahub/internal/grpc"
	"mangahub/internal/manga"
	"mangahub/internal/tcp"
	"mangahub/internal/udp"
	"mangahub/internal/user"
	wschat "mangahub/internal/websocket"
	"mangahub/pkg/database"
	pb "mangahub/proto/manga"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	jwtSecret   := getenv("JWT_SECRET", "mangahub-dev-secret")
	dbPath      := getenv("DB_PATH", "./data/mangahub.db")
	grpcAddr    := getenv("GRPC_ADDR", "localhost:50051")
	grpcPort    := getenv("GRPC_PORT", "50051")
	tcpPort     := getenv("TCP_PORT", "9090")
	tcpInternal := getenv("TCP_INTERNAL_ADDR", ":9099")
	udpPort     := getenv("UDP_PORT", "9091")
	udpInternal := getenv("UDP_INTERNAL_ADDR", ":9094")

	db, err := database.Connect(dbPath)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer db.Close()

	if err := database.SeedManga(db, "./data/manga.json"); err != nil {
		log.Printf("seed warning: %v", err)
	}

	// --- gRPC server ---
	grpcLis, err := net.Listen("tcp", ":"+grpcPort)
	if err != nil {
		log.Fatalf("[gRPC] listen: %v", err)
	}
	grpcSrv := grpclib.NewServer()
	pb.RegisterMangaServiceServer(grpcSrv, &mangagrpc.Service{DB: db})
	go func() {
		log.Printf("[gRPC] listening on :%s", grpcPort)
		if err := grpcSrv.Serve(grpcLis); err != nil {
			log.Printf("[gRPC] stopped: %v", err)
		}
	}()

	// --- TCP server ---
	tcpSrv := tcp.New(tcpPort)
	tcpHTTP := &http.Server{Addr: tcpInternal, Handler: tcpSrv.InternalHandler()}
	go func() {
		log.Printf("[TCP ] internal HTTP on %s", tcpInternal)
		if err := tcpHTTP.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[TCP ] internal HTTP error: %v", err)
		}
	}()
	go func() {
		log.Printf("[TCP ] listening on :%s", tcpPort)
		tcpSrv.Run()
	}()

	// --- UDP server ---
	udpSrv := udp.New(udpPort)
	udpHTTP := &http.Server{Addr: udpInternal, Handler: udpSrv.InternalHandler()}
	go func() {
		log.Printf("[UDP ] internal HTTP on %s", udpInternal)
		if err := udpHTTP.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[UDP ] internal HTTP error: %v", err)
		}
	}()
	go func() {
		log.Printf("[UDP ] listening on :%s", udpPort)
		udpSrv.Run()
	}()

	// --- HTTP API + WebSocket ---
	grpcClient, err := mangagrpc.NewClient(grpcAddr)
	if err != nil {
		log.Fatalf("[HTTP] grpc client: %v", err)
	}
	defer grpcClient.Close() //nolint:errcheck

	hub := wschat.NewHub()
	go hub.Run()

	authHandler  := &auth.Handler{DB: db, JWTSecret: jwtSecret}
	mangaHandler := &manga.Handler{DB: db, GRPCClient: grpcClient}
	userHandler  := &user.Handler{DB: db, GRPCClient: grpcClient}
	wsHandler    := &wschat.Handler{Hub: hub, JWTSecret: jwtSecret}

	r := gin.Default()
	r.POST("/auth/register",            authHandler.Register)
	r.POST("/auth/login",               authHandler.Login)
	r.GET("/manga",                     mangaHandler.Search)
	r.GET("/manga/:id",                 mangaHandler.GetByID)
	r.GET("/ws/chat",                   wsHandler.ServeWS)

	protected := r.Group("/")
	protected.Use(authHandler.JWTMiddleware())
	protected.POST("/manga",                     mangaHandler.Create)
	protected.POST("/users/library",             userHandler.AddToLibrary)
	protected.GET("/users/library",              userHandler.GetLibrary)
	protected.DELETE("/users/library/:manga_id", userHandler.RemoveFromLibrary)
	protected.PUT("/users/progress",             userHandler.UpdateProgress)

	httpSrv := &http.Server{Addr: ":8080", Handler: r}
	go func() {
		log.Printf("[HTTP] API + WebSocket listening on :8080")
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[HTTP] stopped: %v", err)
		}
	}()

	// --- wait for shutdown signal ---
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("[runner] shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	httpSrv.Shutdown(ctx)  //nolint:errcheck
	grpcSrv.GracefulStop()
	tcpSrv.Shutdown()
	tcpHTTP.Shutdown(ctx)  //nolint:errcheck
	udpSrv.Shutdown()
	udpHTTP.Shutdown(ctx)  //nolint:errcheck

	log.Println("[runner] stopped")
}
```

- [ ] **Step 2: Build and verify**

```bash
go build ./cmd/runner
```

Expected: no output (clean build). If errors appear, check import paths match `mangahub/...` module prefix.

- [ ] **Step 3: Smoke-test the runner**

```bash
go run ./cmd/runner
```

Expected output (order may vary slightly):
```
[gRPC] listening on :50051
[TCP ] internal HTTP on :9099
[TCP ] listening on :9090
[UDP ] internal HTTP on :9094
[UDP ] listening on :9091
[HTTP] API + WebSocket listening on :8080
```

Press `Ctrl+C`. Expected:
```
[runner] shutting down...
[runner] stopped
```

- [ ] **Step 4: Commit**

```bash
git add cmd/runner/main.go
git commit -m "feat(runner): single-command all-server launcher"
```

---

## Task 2: Client HTTP Utilities

**Files:**
- Create: `cmd/client/http.go`

Three helpers used by auth and manga tasks. All errors from transport are returned; HTTP error bodies (4xx/5xx) are decoded into `dest` by the caller.

- [ ] **Step 1: Create `cmd/client/http.go`**

```go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

var httpClient = &http.Client{}

// postJSON marshals body as JSON, POSTs to url with optional Bearer token,
// decodes the response into dest, and returns the HTTP status code.
func postJSON(url, token string, body interface{}, dest interface{}) (int, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	json.NewDecoder(resp.Body).Decode(dest) //nolint:errcheck
	return resp.StatusCode, nil
}

// getJSON GETs url with optional Bearer token and decodes the response into dest.
func getJSON(url, token string, dest interface{}) (int, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	json.NewDecoder(resp.Body).Decode(dest) //nolint:errcheck
	return resp.StatusCode, nil
}

// putJSON marshals body as JSON, PUTs to url with optional Bearer token,
// and decodes the response into dest.
func putJSON(url, token string, body interface{}, dest interface{}) (int, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	json.NewDecoder(resp.Body).Decode(dest) //nolint:errcheck
	return resp.StatusCode, nil
}
```

- [ ] **Step 2: Note — cannot build alone**

`http.go` is `package main` but has no `main()` — it will only compile once `main.go` is added in Task 8. Skip the build check here; the final build in Task 8 covers it.

---

## Task 3: Client Auth

**Files:**
- Create: `cmd/client/auth.go`

Implements register, login, logout. Login also triggers `connectTCP` and `connectUDP` (defined in Tasks 5 and 6). The JWT claims are decoded without signature verification — the client trusts the server-issued token.

- [ ] **Step 1: Create `cmd/client/auth.go`**

```go
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

type registerResponse struct {
	Message string `json:"message"`
	UserID  string `json:"user_id"`
	Error   string `json:"error"`
}

type loginResponse struct {
	Token string `json:"token"`
	Error string `json:"error"`
}

func (a *App) doRegister() {
	fmt.Println("\n--- Register ---")
	username := a.prompt("Username (min 3 chars): ")
	email    := a.prompt("Email: ")
	password := a.prompt("Password (min 8 chars): ")

	var resp registerResponse
	status, err := postJSON(a.BaseURL+"/auth/register", "", map[string]string{
		"username": username,
		"email":    email,
		"password": password,
	}, &resp)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	if status != 201 {
		fmt.Println("Registration failed:", resp.Error)
		return
	}
	fmt.Println("Registered successfully! You can now log in.")
}

func (a *App) doLogin() {
	fmt.Println("\n--- Login ---")
	username := a.prompt("Username: ")
	password := a.prompt("Password: ")

	var resp loginResponse
	status, err := postJSON(a.BaseURL+"/auth/login", "", map[string]string{
		"username": username,
		"password": password,
	}, &resp)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	if status != 200 {
		fmt.Println("Login failed:", resp.Error)
		return
	}

	a.Token = resp.Token
	a.UserID, a.Username = parseJWTClaims(resp.Token)

	fmt.Printf("Welcome, %s!\n", a.Username)

	a.connectTCP()
	a.connectUDP()
}

func (a *App) doLogout() {
	a.cleanup()
	a.Token    = ""
	a.UserID   = ""
	a.Username = ""
	a.TCPConn  = nil
	a.UDPConn  = nil
	fmt.Println("Logged out.")
}

// parseJWTClaims decodes the JWT payload (middle segment) and extracts user_id and username.
// Does not verify the signature — the client trusts the server-issued token.
func parseJWTClaims(token string) (userID, username string) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return
	}
	var claims map[string]interface{}
	if err := json.Unmarshal(b, &claims); err != nil {
		return
	}
	userID, _   = claims["user_id"].(string)
	username, _ = claims["username"].(string)
	return
}
```

---

## Task 4: Client Manga Actions

**Files:**
- Create: `cmd/client/manga.go`

All manga operations go through the HTTP API. `doSearch` shows a numbered list and lets the user pick one to view details. `doViewDetails` is also called directly from search.

- [ ] **Step 1: Create `cmd/client/manga.go`**

```go
package main

import (
	"fmt"
	"net/url"
	"strings"
)

type searchResponse struct {
	Results []mangaItem `json:"results"`
	Count   int         `json:"count"`
	Total   int         `json:"total"`
	Error   string      `json:"error"`
}

type mangaItem struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Author        string   `json:"author"`
	Genres        []string `json:"genres"`
	Status        string   `json:"status"`
	TotalChapters int      `json:"total_chapters"`
	Description   string   `json:"description"`
}

type libraryResponse struct {
	ReadingLists map[string][]libraryItem `json:"reading_lists"`
	Total        int                      `json:"total"`
	Error        string                   `json:"error"`
}

type libraryItem struct {
	MangaID        string `json:"manga_id"`
	Title          string `json:"title"`
	CurrentChapter int    `json:"current_chapter"`
	Status         string `json:"status"`
	UpdatedAt      string `json:"updated_at"`
}

type apiError struct {
	Error string `json:"error"`
}

func (a *App) doSearch() {
	fmt.Println("\n--- Search Manga ---")
	q      := a.prompt("Title / author (Enter to skip): ")
	genre  := a.prompt("Genre (Enter to skip): ")
	status := a.prompt("Status — ongoing/completed/hiatus (Enter to skip): ")

	params := url.Values{}
	if q != ""      { params.Set("q", q) }
	if genre != ""  { params.Set("genre", genre) }
	if status != "" { params.Set("status", status) }

	endpoint := a.BaseURL + "/manga"
	if len(params) > 0 {
		endpoint += "?" + params.Encode()
	}

	var resp searchResponse
	code, err := getJSON(endpoint, a.Token, &resp)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	if code != 200 {
		fmt.Println("Search failed:", resp.Error)
		return
	}
	if len(resp.Results) == 0 {
		fmt.Println("No results found.")
		return
	}

	fmt.Printf("\nFound %d result(s) (showing %d):\n\n", resp.Total, resp.Count)
	for i, m := range resp.Results {
		fmt.Printf("  %2d. %-35s  by %-20s  [%s]\n", i+1, m.Title, m.Author, m.ID)
	}

	choice := a.prompt("\nEnter number to view details (Enter to go back): ")
	if choice == "" {
		return
	}
	idx := 0
	fmt.Sscanf(choice, "%d", &idx)
	if idx < 1 || idx > len(resp.Results) {
		fmt.Println("Invalid selection.")
		return
	}
	a.doViewDetails(resp.Results[idx-1].ID)
}

func (a *App) doViewDetails(id string) {
	var m mangaItem
	code, err := getJSON(a.BaseURL+"/manga/"+id, a.Token, &m)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	if code != 200 {
		fmt.Println("Manga not found.")
		return
	}
	fmt.Printf("\n=== %s ===\n", m.Title)
	fmt.Printf("ID:       %s\n", m.ID)
	fmt.Printf("Author:   %s\n", m.Author)
	fmt.Printf("Genres:   %s\n", strings.Join(m.Genres, ", "))
	fmt.Printf("Status:   %s\n", m.Status)
	fmt.Printf("Chapters: %d\n", m.TotalChapters)
	if m.Description != "" {
		fmt.Printf("\n%s\n", m.Description)
	}
}

func (a *App) doLibrary() {
	var resp libraryResponse
	code, err := getJSON(a.BaseURL+"/users/library", a.Token, &resp)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	if code != 200 {
		fmt.Println("Failed to fetch library:", resp.Error)
		return
	}
	if resp.Total == 0 {
		fmt.Println("\nYour library is empty. Add manga with option 4.")
		return
	}
	fmt.Printf("\n=== My Library (%d total) ===\n", resp.Total)
	for _, status := range []string{"reading", "completed", "plan_to_read", "on_hold", "dropped"} {
		items := resp.ReadingLists[status]
		if len(items) == 0 {
			continue
		}
		label := strings.ToUpper(strings.ReplaceAll(status, "_", " "))
		fmt.Printf("\n[%s]\n", label)
		for _, item := range items {
			fmt.Printf("  %-30s  ch.%-4d  (%s)\n", item.Title, item.CurrentChapter, item.MangaID)
		}
	}
}

func (a *App) doAddToLibrary() {
	fmt.Println("\n--- Add to Library ---")
	mangaID := a.prompt("Manga ID: ")
	status  := a.prompt("Status (reading / completed / plan_to_read / on_hold / dropped): ")

	chStr := a.prompt("Current chapter (default 0): ")
	chapter := 0
	fmt.Sscanf(chStr, "%d", &chapter)

	var resp apiError
	code, err := postJSON(a.BaseURL+"/users/library", a.Token, map[string]interface{}{
		"manga_id":        mangaID,
		"status":          status,
		"current_chapter": chapter,
	}, &resp)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	if code != 201 {
		fmt.Println("Failed:", resp.Error)
		return
	}
	fmt.Printf("Added %q to library with status %q at chapter %d.\n", mangaID, status, chapter)
}

func (a *App) doUpdateProgress() {
	fmt.Println("\n--- Update Progress ---")
	mangaID := a.prompt("Manga ID: ")
	chStr   := a.prompt("Current chapter: ")
	chapter := 0
	fmt.Sscanf(chStr, "%d", &chapter)
	status := a.prompt("New status (Enter to keep current): ")

	body := map[string]interface{}{
		"manga_id":        mangaID,
		"current_chapter": chapter,
	}
	if status != "" {
		body["status"] = status
	}

	var resp apiError
	code, err := putJSON(a.BaseURL+"/users/progress", a.Token, body, &resp)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	if code != 200 {
		fmt.Println("Failed:", resp.Error)
		return
	}
	fmt.Printf("Progress updated: %s → chapter %d\n", mangaID, chapter)
}
```

---

## Task 5: Client TCP Listener

**Files:**
- Create: `cmd/client/tcp.go`

Opens a TCP connection after login, sends the JWT auth handshake, then listens in a background goroutine. On progress update arrival it prints inline — the `\n> ` suffix re-draws the prompt so the user can keep typing.

- [ ] **Step 1: Create `cmd/client/tcp.go`**

```go
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
)

type tcpServerMsg struct {
	Type    string `json:"type"`
	MangaID string `json:"manga_id"`
	Chapter int    `json:"chapter"`
	Message string `json:"message"`
}

func (a *App) connectTCP() {
	addr := getenv("TCP_ADDR", "localhost:9090")
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		fmt.Println("Warning: could not connect to TCP server:", err)
		fmt.Println("Real-time progress updates will not be received.")
		return
	}

	authMsg, _ := json.Marshal(map[string]string{"type": "auth", "token": a.Token})
	if _, err := fmt.Fprintf(conn, "%s\n", authMsg); err != nil {
		conn.Close()
		fmt.Println("Warning: TCP auth send failed:", err)
		return
	}

	a.TCPConn = conn
	go a.listenTCP(conn)
}

func (a *App) listenTCP(conn net.Conn) {
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		var msg tcpServerMsg
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}
		switch msg.Type {
		case "auth_ok":
			// silent confirmation
		case "progress_update":
			fmt.Printf("\nProgress updated: %s → chapter %d\n> ", msg.MangaID, msg.Chapter)
		case "error":
			fmt.Printf("\nTCP server error: %s\n> ", msg.Message)
		}
	}
}
```

---

## Task 6: Client UDP Listener

**Files:**
- Create: `cmd/client/udp.go`

Binds a local UDP socket on an OS-assigned port (`:0`), registers with the server, then listens in a background goroutine. The same socket is used for sending (register) and receiving (notifications) — standard UDP behaviour.

- [ ] **Step 1: Create `cmd/client/udp.go`**

```go
package main

import (
	"encoding/json"
	"fmt"
	"net"
)

type udpInPkt struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	MangaID string `json:"manga_id"`
}

func (a *App) connectUDP() {
	serverAddr, err := net.ResolveUDPAddr("udp", getenv("UDP_ADDR", "localhost:9091"))
	if err != nil {
		fmt.Println("Warning: could not resolve UDP server:", err)
		return
	}

	conn, err := net.ListenUDP("udp", &net.UDPAddr{})
	if err != nil {
		fmt.Println("Warning: could not start UDP listener:", err)
		fmt.Println("Chapter notifications will not be received.")
		return
	}

	reg, _ := json.Marshal(map[string]interface{}{"type": "register", "manga_ids": []string{}})
	if _, err := conn.WriteToUDP(reg, serverAddr); err != nil {
		conn.Close()
		fmt.Println("Warning: UDP registration failed:", err)
		return
	}

	a.UDPConn = conn
	go a.listenUDP(conn)
}

func (a *App) listenUDP(conn *net.UDPConn) {
	buf := make([]byte, 65535)
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			return // connection closed on logout
		}
		var pkt udpInPkt
		if err := json.Unmarshal(buf[:n], &pkt); err != nil {
			continue
		}
		switch pkt.Type {
		case "ack":
			fmt.Printf("\nNotifications active: %s\n> ", pkt.Message)
		case "notification":
			fmt.Printf("\nNotification: %s\n> ", pkt.Message)
		}
	}
}
```

---

## Task 7: Client WebSocket Chat

**Files:**
- Create: `cmd/client/ws.go`

Connects to the manga chat room when user selects option 5. Auth is sent automatically as the first message. Two goroutines run inside: one reads incoming frames and prints them; the main goroutine reads stdin and sends. `/exit` closes the connection and returns to the main menu.

The server accepts `{"message": "..."}` JSON for chat messages. Gorilla's default ping handler automatically responds to server pings — no extra configuration needed.

- [ ] **Step 1: Create `cmd/client/ws.go`**

```go
package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type wsInMsg struct {
	Type      string `json:"type"`
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
}

func (a *App) enterChatRoom() {
	mangaID := a.prompt("Enter manga ID (or press Enter for general): ")
	if mangaID == "" {
		mangaID = "general"
	}

	wsURL := strings.Replace(a.BaseURL, "http://", "ws://", 1) + "/ws/chat?manga_id=" + mangaID

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		fmt.Println("Error connecting to chat:", err)
		return
	}
	defer conn.Close()

	// first message must be the JWT token
	if err := conn.WriteJSON(map[string]string{"token": a.Token}); err != nil {
		fmt.Println("Error sending auth:", err)
		return
	}

	done := make(chan struct{})

	// reader goroutine: receives messages from server and prints them
	go func() {
		defer close(done)
		for {
			var msg wsInMsg
			if err := conn.ReadJSON(&msg); err != nil {
				return
			}
			switch msg.Type {
			case "message":
				label := msg.Username
				if msg.UserID == a.UserID {
					label = "You"
				}
				fmt.Printf("\r[%-8s] %s\n> ", label, msg.Message)
			case "join":
				fmt.Printf("\r%s joined the room\n> ", msg.Username)
			case "leave":
				fmt.Printf("\r%s left the room\n> ", msg.Username)
			}
		}
	}()

	fmt.Printf("\n=== Chat Room: %s ===\n", mangaID)
	fmt.Println("(type a message and press Enter, /exit to leave)")
	fmt.Println()

	for {
		fmt.Print("> ")
		a.scanner.Scan()
		text := strings.TrimSpace(a.scanner.Text())
		if text == "/exit" {
			break
		}
		if text == "" {
			continue
		}
		if err := conn.WriteJSON(map[string]interface{}{
			"message":   text,
			"timestamp": time.Now().Unix(),
		}); err != nil {
			fmt.Println("Error sending message:", err)
			break
		}
	}

	conn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	select {
	case <-done:
	case <-time.After(time.Second):
	}
}
```

---

## Task 8: Client Main + Final Build

**Files:**
- Create: `cmd/client/main.go`

Written last so it can reference all functions defined in the previous files. Holds `App`, the menu loops, `prompt`, `cleanup`, and `getenv`.

- [ ] **Step 1: Create `cmd/client/main.go`**

```go
package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
)

// App holds all shared client state. All methods on App live in other files in this package.
type App struct {
	BaseURL  string
	Token    string
	UserID   string
	Username string
	TCPConn  net.Conn
	UDPConn  *net.UDPConn
	scanner  *bufio.Scanner
}

func main() {
	app := &App{
		BaseURL: getenv("API_URL", "http://localhost:8080"),
		scanner: bufio.NewScanner(os.Stdin),
	}
	fmt.Println("=== MangaHub Terminal Client ===")
	fmt.Println("Server:", app.BaseURL)
	app.run()
}

func (a *App) run() {
	for {
		if a.Token == "" {
			if !a.guestMenu() {
				break
			}
		} else {
			if !a.mainMenu() {
				break
			}
		}
	}
	a.cleanup()
	fmt.Println("Goodbye!")
}

func (a *App) guestMenu() bool {
	fmt.Println()
	fmt.Println("1. Search manga")
	fmt.Println("2. Register")
	fmt.Println("3. Login")
	fmt.Println("0. Exit")
	switch a.prompt("> ") {
	case "1":
		a.doSearch()
	case "2":
		a.doRegister()
	case "3":
		a.doLogin()
	case "0":
		return false
	default:
		fmt.Println("Invalid choice.")
	}
	return true
}

func (a *App) mainMenu() bool {
	fmt.Printf("\n=== MangaHub === [%s]\n", a.Username)
	fmt.Println("1. Search manga")
	fmt.Println("2. View my library")
	fmt.Println("3. Update reading progress")
	fmt.Println("4. Add manga to library")
	fmt.Println("5. Enter chat room")
	fmt.Println("0. Logout / Exit")
	switch a.prompt("> ") {
	case "1":
		a.doSearch()
	case "2":
		a.doLibrary()
	case "3":
		a.doUpdateProgress()
	case "4":
		a.doAddToLibrary()
	case "5":
		a.enterChatRoom()
	case "0":
		a.doLogout()
		return false
	default:
		fmt.Println("Invalid choice.")
	}
	return true
}

func (a *App) prompt(p string) string {
	fmt.Print(p)
	a.scanner.Scan()
	return strings.TrimSpace(a.scanner.Text())
}

func (a *App) cleanup() {
	if a.TCPConn != nil {
		a.TCPConn.Close()
	}
	if a.UDPConn != nil {
		a.UDPConn.Close()
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
```

- [ ] **Step 2: Build the client**

```bash
go build ./cmd/client
```

Expected: no output (clean build).

- [ ] **Step 3: Build everything together**

```bash
go build ./...
```

Expected: no output. All packages compile cleanly.

- [ ] **Step 4: Run all tests**

```bash
go test ./...
```

Expected: all existing test packages pass. No new tests are added for `cmd/` packages (convention in this codebase).

- [ ] **Step 5: End-to-end smoke test**

Terminal 1:
```bash
go run ./cmd/runner
```
Wait for all `[HTTP]`, `[gRPC]`, `[TCP ]`, `[UDP ]` lines to appear.

Terminal 2:
```bash
go run ./cmd/client
```

Walk through the following flow:
1. Choose `1` (Search manga) — enter `one` as title. Should show results including "One Piece".
2. Choose `3` (Login) — use an existing account or register first with `2`.
3. After login — verify the terminal prints "Notifications active: ..." (UDP ack).
4. Choose `3` (Update progress) — enter a manga ID and chapter. Verify "Progress updated: ..." appears.
5. Choose `5` (Chat room) — enter `one-piece`. Send a message. Type `/exit` to leave.
6. Choose `0` (Logout).

- [ ] **Step 6: Commit**

```bash
git add cmd/client/
git commit -m "feat(client): interactive terminal client — HTTP, TCP, UDP, WebSocket"
```

---

## Environment Variables Reference

| Variable | Default | Used by |
|---|---|---|
| `JWT_SECRET` | `mangahub-dev-secret` | runner |
| `DB_PATH` | `./data/mangahub.db` | runner |
| `GRPC_ADDR` | `localhost:50051` | runner (HTTP→gRPC client) |
| `GRPC_PORT` | `50051` | runner (gRPC server) |
| `TCP_PORT` | `9090` | runner |
| `TCP_INTERNAL_ADDR` | `:9099` | runner |
| `UDP_PORT` | `9091` | runner |
| `UDP_INTERNAL_ADDR` | `:9094` | runner |
| `API_URL` | `http://localhost:8080` | client |
| `TCP_ADDR` | `localhost:9090` | client |
| `UDP_ADDR` | `localhost:9091` | client |
