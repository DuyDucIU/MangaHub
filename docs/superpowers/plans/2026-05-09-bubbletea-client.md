# Bubbletea TUI Client Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rewrite `cmd/client/` as a full-screen bubbletea TUI with a split-pane layout, "Berserk Dark" color theme, real-time TCP/UDP notifications in a persistent footer, paginated search results, and a full-screen chat room.

**Architecture:** Single `Model` struct with a `currentView` int enum dispatching to per-view `update*` and `render*` functions in dedicated files. Background TCP/UDP/WebSocket goroutines communicate with bubbletea exclusively via self-renewing `tea.Cmd` loops — no `fmt.Printf` from goroutines. `http.go` is the only existing file kept unchanged.

**Tech Stack:** `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/bubbles` (textinput, viewport), `github.com/charmbracelet/lipgloss`, `github.com/gorilla/websocket` (existing), Go 1.26

---

## File Map

| File | Role |
|---|---|
| `cmd/client/main.go` | `tea.NewProgram` entry point, `getenv` helper |
| `cmd/client/model.go` | `Model` struct, all shared types, all `tea.Msg` types, lipgloss styles, `Init`/`Update`/`View`, `renderLayout` |
| `cmd/client/view_menu.go` | `updateMenu`, `renderSidebar`, `renderMenu`, `sidebarItems` |
| `cmd/client/view_auth.go` | `updateAuth`, `renderAuth`, `cmdLogin`, `cmdRegister`, `initLoginInputs`, `initRegisterInputs`, `parseJWTClaims` |
| `cmd/client/view_search.go` | `updateSearch`, `renderSearch`, `cmdSearch`, `cmdFetchDetail`, `cmdAddToLibrary`, `cmdUpdateProgress`, `initSearchInputs`, `totalPages` |
| `cmd/client/view_library.go` | `updateLibrary`, `renderLibrary`, `cmdFetchLibrary`, `flattenLibrary` |
| `cmd/client/view_chat.go` | `updateChat`, `renderChatScreen`, `cmdSendWSMessage`, `formatChatMsg` |
| `cmd/client/tcp.go` | `cmdConnectTCP`, `waitForTCP` |
| `cmd/client/udp.go` | `cmdConnectUDP`, `waitForUDP` |
| `cmd/client/ws.go` | `cmdConnectWS`, `waitForWS` |
| `cmd/client/http.go` | Unchanged |

**Deleted:** old `main.go`, `auth.go`, `manga.go`, `tcp.go`, `udp.go`, `ws.go`

---

## Task 1: Add dependencies and remove old files

**Files:**
- Delete: `cmd/client/main.go`, `cmd/client/auth.go`, `cmd/client/manga.go`, `cmd/client/tcp.go`, `cmd/client/udp.go`, `cmd/client/ws.go`

- [ ] **Step 1: Add bubbletea dependencies**

```
go get github.com/charmbracelet/bubbletea@latest
go get github.com/charmbracelet/bubbles@latest
go get github.com/charmbracelet/lipgloss@latest
go mod tidy
```

- [ ] **Step 2: Delete old client files**

```
del cmd\client\main.go cmd\client\auth.go cmd\client\manga.go cmd\client\tcp.go cmd\client\udp.go cmd\client\ws.go
```

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore(client): add bubbletea/bubbles/lipgloss deps, remove old client files"
```

---

## Task 2: Scaffold — model.go + main.go + stub view files

Everything in this task must compile and display a blank TUI on `go run ./cmd/client/`. All view functions are stubs; they will be filled in later tasks.

**Files:**
- Create: `cmd/client/model.go`
- Create: `cmd/client/main.go`
- Create: `cmd/client/view_menu.go` (stubs)
- Create: `cmd/client/view_auth.go` (stubs)
- Create: `cmd/client/view_search.go` (stubs)
- Create: `cmd/client/view_library.go` (stubs)
- Create: `cmd/client/view_chat.go` (stubs)
- Create: `cmd/client/tcp.go` (stubs)
- Create: `cmd/client/udp.go` (stubs)
- Create: `cmd/client/ws.go` (stubs)
- Create: `cmd/client/model_test.go`

- [ ] **Step 1: Write failing tests**

Create `cmd/client/model_test.go`:

```go
package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	m := New("http://localhost:8080")
	assert.Equal(t, viewMenu, m.currentView)
	assert.Equal(t, 0, m.sidebarIdx)
	assert.Empty(t, m.token)
	assert.Equal(t, "http://localhost:8080", m.baseURL)
}

func TestWindowResize(t *testing.T) {
	m := New("http://localhost:8080")
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m2 := next.(Model)
	assert.Equal(t, 120, m2.width)
	assert.Equal(t, 40, m2.height)
}

func TestTCPNotifNoConn(t *testing.T) {
	m := New("http://localhost:8080")
	next, cmd := m.Update(tcpNotifMsg{text: "progress update"})
	m2 := next.(Model)
	assert.Equal(t, "progress update", m2.notification)
	assert.Nil(t, cmd) // no conn → no re-subscribe
}

func TestUDPNotifNoConn(t *testing.T) {
	m := New("http://localhost:8080")
	next, cmd := m.Update(udpNotifMsg{text: "chapter released"})
	m2 := next.(Model)
	assert.Equal(t, "chapter released", m2.notification)
	assert.Nil(t, cmd)
}
```

- [ ] **Step 2: Run tests — confirm they fail**

```
go test ./cmd/client/ -run "TestNew|TestWindowResize|TestTCPNotif|TestUDPNotif" -v
```

Expected: compile error — package doesn't exist yet.

- [ ] **Step 3: Create model.go**

```go
package main

import (
	"net"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/gorilla/websocket"
)

// --- View enum ---

type view int

const (
	viewMenu view = iota
	viewLogin
	viewRegister
	viewSearch
	viewLibrary
	viewChat
)

// --- Sub-state enums ---

type searchState int

const (
	searchStateForm searchState = iota
	searchStateResults
	searchStateDetail
)

// --- Shared data types ---

type mangaItem struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Author        string   `json:"author"`
	Genres        []string `json:"genres"`
	Status        string   `json:"status"`
	TotalChapters int      `json:"total_chapters"`
	Description   string   `json:"description"`
	CoverURL      string   `json:"cover_url,omitempty"`
}

type libraryItem struct {
	MangaID        string `json:"manga_id"`
	Title          string `json:"title"`
	CurrentChapter int    `json:"current_chapter"`
	Status         string `json:"status"`
	UpdatedAt      string `json:"updated_at"`
}

type chatMessage struct {
	userID   string
	username string
	text     string
	isSystem bool
}

// --- tea.Msg types ---

type tcpNotifMsg    struct{ text string }
type udpNotifMsg    struct{ text string }
type tcpConnectedMsg struct{ conn net.Conn }
type udpConnectedMsg struct{ conn *net.UDPConn }
type wsConnectedMsg  struct{ conn *websocket.Conn }

type wsMsgReceived struct {
	userID   string
	username string
	text     string
}
type wsJoined struct{ username string }
type wsLeft   struct{ username string }

type searchResultMsg struct {
	results []mangaItem
	total   int
	page    int
	err     string
}
type detailResultMsg struct {
	manga mangaItem
	entry *libraryItem
	err   string
}
type libraryResultMsg struct {
	groups map[string][]libraryItem
	total  int
	err    string
}
type loginSuccessMsg struct {
	token    string
	userID   string
	username string
}
type registerSuccessMsg struct{}
type addLibraryMsg     struct{ err string }
type updateProgressMsg struct{ err string }
type errMsg            struct{ text string }

// --- Lipgloss styles ---

var (
	colorGold    = lipgloss.Color("#C9A84C")
	colorCrimson = lipgloss.Color("#8B1A1A")
	colorText    = lipgloss.Color("#D4CFBF")
	colorMuted   = lipgloss.Color("#4A5568")
	colorNotif   = lipgloss.Color("#A07800")

	styleHeader = lipgloss.NewStyle().
			Background(colorGold).
			Foreground(lipgloss.Color("#0F0F14")).
			Bold(true).
			Padding(0, 1)

	styleSidebarItem = lipgloss.NewStyle().
				Foreground(colorText).
				Padding(0, 1)

	styleSidebarSelected = lipgloss.NewStyle().
				Background(colorCrimson).
				Foreground(colorGold).
				Bold(true).
				Padding(0, 1)

	styleTitle = lipgloss.NewStyle().
			Foreground(colorGold).
			Bold(true)

	styleMutedText = lipgloss.NewStyle().
			Foreground(colorMuted)

	styleNotif = lipgloss.NewStyle().
			Foreground(colorNotif)

	styleNormal = lipgloss.NewStyle().
			Foreground(colorText)

	styleBorderBox = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(colorMuted)

	styleActiveBorderBox = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(colorGold)

	styleError = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF6B6B"))
)

const sidebarWidth = 22

// --- Model ---

type Model struct {
	// core
	baseURL     string
	currentView view
	sidebarIdx  int

	// auth state
	token    string
	userID   string
	username string

	// background connections
	tcpConn net.Conn
	udpConn *net.UDPConn

	// notification footer (stays until replaced)
	notification string

	// terminal dimensions
	width, height int

	// auth forms (shared for login + register; re-initialised on view switch)
	authInputs []textinput.Model
	authFocus  int
	authErr    string

	// search view
	searchState   searchState
	searchInputs  []textinput.Model
	searchFocus   int
	searchResults []mangaItem
	searchCursor  int
	searchPage    int
	searchTotal   int
	detailManga   mangaItem
	detailEntry   *libraryItem
	detailFocus   int // 0=action button, irrelevant when no actions

	// library view
	libraryGroups map[string][]libraryItem
	libraryFlat   []libraryItem
	libraryCursor int

	// chat view
	chatMangaID     string
	chatMessages    []chatMessage
	chatInput       textinput.Model
	chatViewport    viewport.Model
	chatConn        *websocket.Conn
	chatPrompting   bool
	chatPromptInput textinput.Model
}

func New(baseURL string) Model {
	return Model{
		baseURL:     baseURL,
		currentView: viewMenu,
		sidebarIdx:  0,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// resize chat viewport if active
		if m.currentView == viewChat {
			m.chatViewport.Width = msg.Width
			m.chatViewport.Height = msg.Height - 4
		}
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			closeConns(m)
			return m, tea.Quit
		}

	case tcpNotifMsg:
		m.notification = msg.text
		if m.tcpConn != nil {
			return m, waitForTCP(m.tcpConn)
		}
		return m, nil

	case udpNotifMsg:
		m.notification = msg.text
		if m.udpConn != nil {
			return m, waitForUDP(m.udpConn)
		}
		return m, nil

	case tcpConnectedMsg:
		m.tcpConn = msg.conn
		return m, waitForTCP(msg.conn)

	case udpConnectedMsg:
		m.udpConn = msg.conn
		return m, waitForUDP(msg.conn)
	}

	switch m.currentView {
	case viewMenu:
		return updateMenu(m, msg)
	case viewLogin, viewRegister:
		return updateAuth(m, msg)
	case viewSearch:
		return updateSearch(m, msg)
	case viewLibrary:
		return updateLibrary(m, msg)
	case viewChat:
		return updateChat(m, msg)
	}
	return m, nil
}

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	if m.currentView == viewChat {
		return renderChatScreen(m)
	}
	return renderLayout(m)
}

// renderLayout composes header + (sidebar | content) + footer.
func renderLayout(m Model) string {
	bodyHeight := m.height - 2 // 1 header + 1 footer
	contentWidth := m.width - sidebarWidth - 1 // -1 for left border of content

	header := renderHeader(m)
	sidebar := lipgloss.NewStyle().
		Width(sidebarWidth).
		Height(bodyHeight).
		Render(renderSidebar(m, sidebarWidth, bodyHeight))
	content := lipgloss.NewStyle().
		Width(contentWidth).
		Height(bodyHeight).
		BorderLeft(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(colorMuted).
		Render(renderContent(m, contentWidth-2, bodyHeight-1))
	footer := renderFooter(m)

	body := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, content)
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

func renderHeader(m Model) string {
	left := "  MangaHub"
	right := ""
	if m.username != "" {
		right = m.username + "  "
	}
	gap := m.width - len(left) - len(right)
	if gap < 0 {
		gap = 0
	}
	return styleHeader.Width(m.width).Render(left + strings.Repeat(" ", gap) + right)
}

func renderFooter(m Model) string {
	text := ""
	if m.notification != "" {
		text = "  " + m.notification
	}
	return styleNotif.Width(m.width).Render(text)
}

// renderContent dispatches to the active view's render function.
func renderContent(m Model, width, height int) string {
	switch m.currentView {
	case viewLogin, viewRegister:
		return renderAuth(m, width, height)
	case viewSearch:
		return renderSearch(m, width, height)
	case viewLibrary:
		return renderLibrary(m, width, height)
	default:
		return renderMenu(m, width, height)
	}
}

// closeConns closes all open background connections.
func closeConns(m Model) {
	if m.tcpConn != nil {
		m.tcpConn.Close()
	}
	if m.udpConn != nil {
		m.udpConn.Close()
	}
	if m.chatConn != nil {
		m.chatConn.Close()
	}
}
```

- [ ] **Step 4: Create main.go**

```go
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	baseURL := getenv("API_URL", "http://localhost:8080")
	p := tea.NewProgram(New(baseURL), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
```

- [ ] **Step 5: Create stub view_menu.go**

```go
package main

import tea "github.com/charmbracelet/bubbletea"

func sidebarItems(m Model) []string {
	if m.token == "" {
		return []string{"Search", "Register", "Login"}
	}
	return []string{"Search", "Library", "Chat", "Logout"}
}

func updateMenu(m Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	return m, nil
}

func renderSidebar(m Model, width, height int) string {
	return ""
}

func renderMenu(m Model, width, height int) string {
	return ""
}
```

- [ ] **Step 6: Create stub view_auth.go**

```go
package main

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func initLoginInputs() []textinput.Model    { return nil }
func initRegisterInputs() []textinput.Model { return nil }

func cmdLogin(baseURL, username, password string) tea.Cmd    { return nil }
func cmdRegister(baseURL, username, email, password string) tea.Cmd { return nil }

func parseJWTClaims(token string) (userID, username string, ok bool) { return }

func updateAuth(m Model, msg tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func renderAuth(m Model, width, height int) string         { return "" }
```

- [ ] **Step 7: Create stub view_search.go**

```go
package main

import (
	"net/url"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
)

func initSearchInputs() []textinput.Model { return nil }

func cmdSearch(baseURL, token, q, genre, status string, page int) tea.Cmd { return nil }
func cmdFetchDetail(baseURL, token, id string) tea.Cmd                    { return nil }
func cmdAddToLibrary(baseURL, token, mangaID, status string, chapter int) tea.Cmd { return nil }
func cmdUpdateProgress(baseURL, token, mangaID, status string, chapter int) tea.Cmd { return nil }

func totalPages(total, pageSize int) int {
	if pageSize == 0 {
		return 1
	}
	p := total / pageSize
	if total%pageSize > 0 {
		p++
	}
	return p
}

var _ = url.Values{} // suppress unused import until Task 8

func updateSearch(m Model, msg tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func renderSearch(m Model, width, height int) string         { return "" }
```

- [ ] **Step 8: Create stub view_library.go**

```go
package main

import tea "github.com/charmbracelet/bubbletea"

func cmdFetchLibrary(baseURL, token string) tea.Cmd { return nil }

func flattenLibrary(groups map[string][]libraryItem) []libraryItem {
	order := []string{"reading", "completed", "plan_to_read", "on_hold", "dropped"}
	var flat []libraryItem
	for _, s := range order {
		flat = append(flat, groups[s]...)
	}
	return flat
}

func updateLibrary(m Model, msg tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func renderLibrary(m Model, width, height int) string         { return "" }
```

- [ ] **Step 9: Create stub view_chat.go**

```go
package main

import (
	"github.com/gorilla/websocket"
	tea "github.com/charmbracelet/bubbletea"
)

func cmdSendWSMessage(conn *websocket.Conn, text string) tea.Cmd { return nil }

func formatChatMsg(msg chatMessage, myUserID string) string { return "" }

func updateChat(m Model, msg tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func renderChatScreen(m Model) string                      { return "" }
```

- [ ] **Step 10: Create stub tcp.go**

```go
package main

import (
	"net"
	tea "github.com/charmbracelet/bubbletea"
)

func cmdConnectTCP(addr, token string) tea.Cmd { return nil }
func waitForTCP(conn net.Conn) tea.Cmd         { return nil }
```

- [ ] **Step 11: Create stub udp.go**

```go
package main

import (
	"net"
	tea "github.com/charmbracelet/bubbletea"
)

func cmdConnectUDP(serverAddr string) tea.Cmd      { return nil }
func waitForUDP(conn *net.UDPConn) tea.Cmd         { return nil }
```

- [ ] **Step 12: Create stub ws.go**

```go
package main

import (
	"github.com/gorilla/websocket"
	tea "github.com/charmbracelet/bubbletea"
)

func cmdConnectWS(baseURL, token, mangaID string) tea.Cmd  { return nil }
func waitForWS(conn *websocket.Conn) tea.Cmd               { return nil }
```

- [ ] **Step 13: Run tests — confirm they pass**

```
go test ./cmd/client/ -run "TestNew|TestWindowResize|TestTCPNotif|TestUDPNotif" -v
```

Expected output:
```
--- PASS: TestNew
--- PASS: TestWindowResize
--- PASS: TestTCPNotifNoConn
--- PASS: TestUDPNotifNoConn
PASS
```

- [ ] **Step 14: Verify it compiles**

```
go build ./cmd/client/
```

Expected: no errors.

- [ ] **Step 15: Commit**

```bash
git add cmd/client/
git commit -m "feat(client): scaffold bubbletea model, all types, styles, and stub views"
```

---

## Task 3: tcp.go — TCP listener as tea.Cmd

**Files:**
- Modify: `cmd/client/tcp.go`
- Create: `cmd/client/tcp_test.go`

- [ ] **Step 1: Write failing test**

Create `cmd/client/tcp_test.go`:

```go
package main

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTCPNotifWithConn(t *testing.T) {
	// net.Pipe gives a synchronous in-memory connection pair
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	m := New("http://localhost:8080")
	m.tcpConn = client

	next, cmd := m.Update(tcpNotifMsg{text: "One Piece → chapter 1096"})
	m2 := next.(Model)
	assert.Equal(t, "One Piece → chapter 1096", m2.notification)
	assert.NotNil(t, cmd) // has conn → re-subscribes
}
```

- [ ] **Step 2: Run — confirm fail**

```
go test ./cmd/client/ -run TestTCPNotifWithConn -v
```

Expected: FAIL — cmd is nil because `waitForTCP` is a stub returning nil.

- [ ] **Step 3: Implement tcp.go**

```go
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"

	tea "github.com/charmbracelet/bubbletea"
)

type tcpServerMsg struct {
	Type    string `json:"type"`
	MangaID string `json:"manga_id"`
	Chapter int    `json:"chapter"`
	Message string `json:"message"`
}

// cmdConnectTCP dials the TCP server, sends the auth message, and returns
// tcpConnectedMsg on success or tcpNotifMsg with a warning on failure.
func cmdConnectTCP(addr, token string) tea.Cmd {
	return func() tea.Msg {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			return tcpNotifMsg{text: "Warning: TCP unavailable — progress updates disabled"}
		}
		auth, _ := json.Marshal(map[string]string{"type": "auth", "token": token})
		if _, err := fmt.Fprintf(conn, "%s\n", auth); err != nil {
			conn.Close()
			return tcpNotifMsg{text: "Warning: TCP auth failed — progress updates disabled"}
		}
		return tcpConnectedMsg{conn: conn}
	}
}

// waitForTCP blocks until one message arrives on conn, then returns it as a
// tcpNotifMsg. The caller must re-issue this Cmd after each message.
func waitForTCP(conn net.Conn) tea.Cmd {
	return func() tea.Msg {
		scanner := bufio.NewScanner(conn)
		if !scanner.Scan() {
			return tcpNotifMsg{text: "TCP connection closed"}
		}
		var msg tcpServerMsg
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			return tcpNotifMsg{text: "TCP: unreadable message"}
		}
		switch msg.Type {
		case "auth_ok":
			return tcpNotifMsg{text: ""}
		case "progress_update":
			return tcpNotifMsg{text: fmt.Sprintf("Progress updated: %s → chapter %d", msg.MangaID, msg.Chapter)}
		case "error":
			return tcpNotifMsg{text: "TCP: " + msg.Message}
		default:
			return tcpNotifMsg{text: ""}
		}
	}
}
```

- [ ] **Step 4: Run — confirm pass**

```
go test ./cmd/client/ -run "TestTCPNotif" -v
```

Expected: both `TestTCPNotifNoConn` and `TestTCPNotifWithConn` PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/client/tcp.go cmd/client/tcp_test.go
git commit -m "feat(client): implement TCP listener as tea.Cmd"
```

---

## Task 4: udp.go — UDP listener as tea.Cmd

**Files:**
- Modify: `cmd/client/udp.go`
- Create: `cmd/client/udp_test.go`

- [ ] **Step 1: Write failing test**

Create `cmd/client/udp_test.go`:

```go
package main

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUDPNotifWithConn(t *testing.T) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Skip("cannot open UDP socket:", err)
	}
	defer conn.Close()

	m := New("http://localhost:8080")
	m.udpConn = conn

	next, cmd := m.Update(udpNotifMsg{text: "Bleach chapter 700 released!"})
	m2 := next.(Model)
	assert.Equal(t, "Bleach chapter 700 released!", m2.notification)
	assert.NotNil(t, cmd) // has conn → re-subscribes
}
```

- [ ] **Step 2: Run — confirm fail**

```
go test ./cmd/client/ -run TestUDPNotifWithConn -v
```

Expected: FAIL — cmd is nil because `waitForUDP` is still a stub.

- [ ] **Step 3: Implement udp.go**

```go
package main

import (
	"encoding/json"
	"fmt"
	"net"

	tea "github.com/charmbracelet/bubbletea"
)

type udpInPkt struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	MangaID string `json:"manga_id"`
}

// cmdConnectUDP opens a local UDP socket, sends a register packet to serverAddr,
// and returns udpConnectedMsg on success or udpNotifMsg with a warning on failure.
func cmdConnectUDP(serverAddr string) tea.Cmd {
	return func() tea.Msg {
		srv, err := net.ResolveUDPAddr("udp", serverAddr)
		if err != nil {
			return udpNotifMsg{text: "Warning: UDP unavailable — chapter notifications disabled"}
		}
		conn, err := net.ListenUDP("udp", &net.UDPAddr{})
		if err != nil {
			return udpNotifMsg{text: "Warning: UDP unavailable — chapter notifications disabled"}
		}
		reg, _ := json.Marshal(map[string]interface{}{"type": "register", "manga_ids": []string{}})
		if _, err := conn.WriteToUDP(reg, srv); err != nil {
			conn.Close()
			return udpNotifMsg{text: "Warning: UDP registration failed — chapter notifications disabled"}
		}
		return udpConnectedMsg{conn: conn}
	}
}

// waitForUDP blocks until one UDP packet arrives, then returns it as a
// udpNotifMsg. The caller must re-issue this Cmd after each message.
func waitForUDP(conn *net.UDPConn) tea.Cmd {
	return func() tea.Msg {
		buf := make([]byte, 65535)
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			return udpNotifMsg{text: "UDP connection closed"}
		}
		var pkt udpInPkt
		if err := json.Unmarshal(buf[:n], &pkt); err != nil {
			return udpNotifMsg{text: ""}
		}
		switch pkt.Type {
		case "ack":
			return udpNotifMsg{text: fmt.Sprintf("Notifications active: %s", pkt.Message)}
		case "notification":
			return udpNotifMsg{text: fmt.Sprintf("Notification: %s", pkt.Message)}
		default:
			return udpNotifMsg{text: ""}
		}
	}
}
```

- [ ] **Step 4: Run — confirm pass**

```
go test ./cmd/client/ -run "TestUDPNotif" -v
```

Expected: both `TestUDPNotifNoConn` and `TestUDPNotifWithConn` PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/client/udp.go cmd/client/udp_test.go
git commit -m "feat(client): implement UDP listener as tea.Cmd"
```

---

## Task 5: ws.go — WebSocket as tea.Cmd

No unit tests for WebSocket goroutine behavior (requires a live server). The message types and `waitForWS` pattern are exercised by the chat view tests in Task 10.

**Files:**
- Modify: `cmd/client/ws.go`

- [ ] **Step 1: Implement ws.go**

```go
package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gorilla/websocket"
)

type wsInMsg struct {
	Type      string `json:"type"`
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
}

// cmdConnectWS dials the WebSocket server, sends the JWT auth message, and
// returns wsConnectedMsg on success or errMsg on failure.
func cmdConnectWS(baseURL, token, mangaID string) tea.Cmd {
	return func() tea.Msg {
		wsURL := strings.Replace(baseURL, "http://", "ws://", 1) +
			"/ws/chat?manga_id=" + mangaID
		dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
		conn, _, err := dialer.Dial(wsURL, nil)
		if err != nil {
			return errMsg{text: "Chat connect failed: " + err.Error()}
		}
		if err := conn.WriteJSON(map[string]string{"token": token}); err != nil {
			conn.Close()
			return errMsg{text: "Chat auth failed: " + err.Error()}
		}
		return wsConnectedMsg{conn: conn}
	}
}

// waitForWS blocks until one WebSocket message arrives, then returns it as
// wsMsgReceived / wsJoined / wsLeft. The caller must re-issue this Cmd.
func waitForWS(conn *websocket.Conn) tea.Cmd {
	return func() tea.Msg {
		var msg wsInMsg
		if err := conn.ReadJSON(&msg); err != nil {
			return errMsg{text: "Chat disconnected"}
		}
		switch msg.Type {
		case "message":
			return wsMsgReceived{
				userID:   msg.UserID,
				username: msg.Username,
				text:     msg.Message,
			}
		case "join":
			return wsJoined{username: msg.Username}
		case "leave":
			return wsLeft{username: msg.Username}
		default:
			return wsMsgReceived{}
		}
	}
}

// cmdSendWSMessage sends one message over the WebSocket.
func cmdSendWSMessage(conn *websocket.Conn, text string) tea.Cmd {
	return func() tea.Msg {
		err := conn.WriteJSON(map[string]interface{}{
			"message":   text,
			"timestamp": fmt.Sprintf("%d", time.Now().Unix()),
		})
		if err != nil {
			return errMsg{text: "Send failed: " + err.Error()}
		}
		return nil
	}
}
```

- [ ] **Step 2: Verify it compiles**

```
go build ./cmd/client/
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add cmd/client/ws.go
git commit -m "feat(client): implement WebSocket connect/listen as tea.Cmd"
```

---

## Task 6: view_menu.go — sidebar, welcome panel, layout

**Files:**
- Modify: `cmd/client/view_menu.go`
- Create: `cmd/client/view_menu_test.go`

- [ ] **Step 1: Write failing tests**

Create `cmd/client/view_menu_test.go`:

```go
package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestSidebarItemsGuest(t *testing.T) {
	m := New("http://localhost:8080")
	items := sidebarItems(m)
	assert.Equal(t, []string{"Search", "Register", "Login"}, items)
}

func TestSidebarItemsAuth(t *testing.T) {
	m := New("http://localhost:8080")
	m.token = "tok"
	m.username = "alice"
	items := sidebarItems(m)
	assert.Equal(t, []string{"Search", "Library", "Chat", "Logout"}, items)
}

func TestMenuNavDown(t *testing.T) {
	m := New("http://localhost:8080")
	m.sidebarIdx = 0
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m2 := next.(Model)
	assert.Equal(t, 1, m2.sidebarIdx)
}

func TestMenuNavSelectSearch(t *testing.T) {
	m := New("http://localhost:8080")
	m.width, m.height = 100, 40
	m.sidebarIdx = 0 // Search
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := next.(Model)
	assert.Equal(t, viewSearch, m2.currentView)
}

func TestMenuSelectLoginSetsInputs(t *testing.T) {
	m := New("http://localhost:8080")
	m.width, m.height = 100, 40
	// guest sidebar: 0=Search,1=Register,2=Login
	m.sidebarIdx = 2
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := next.(Model)
	assert.Equal(t, viewLogin, m2.currentView)
	assert.Len(t, m2.authInputs, 2) // username + password
}
```

- [ ] **Step 2: Run — confirm fail**

```
go test ./cmd/client/ -run "TestSidebar|TestMenuNav|TestMenuSelect" -v
```

Expected: FAIL — stubs return nothing useful.

- [ ] **Step 3: Implement view_menu.go**

```go
package main

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func sidebarItems(m Model) []string {
	if m.token == "" {
		return []string{"Search", "Register", "Login"}
	}
	return []string{"Search", "Library", "Chat", "Logout"}
}

func updateMenu(m Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		items := sidebarItems(m)
		switch msg.String() {
		case "up", "k":
			if m.sidebarIdx > 0 {
				m.sidebarIdx--
			}
		case "down", "j":
			if m.sidebarIdx < len(items)-1 {
				m.sidebarIdx++
			}
		case "1", "2", "3", "4":
			idx := int(msg.Runes[0]-'1')
			if idx < len(items) {
				m.sidebarIdx = idx
				return activateSidebarItem(m)
			}
		case "enter":
			return activateSidebarItem(m)
		}
	}
	return m, nil
}

func activateSidebarItem(m Model) (Model, tea.Cmd) {
	items := sidebarItems(m)
	if m.sidebarIdx >= len(items) {
		return m, nil
	}
	switch items[m.sidebarIdx] {
	case "Search":
		m.currentView = viewSearch
		m.searchState = searchStateForm
		m.searchInputs = initSearchInputs()
		m.searchFocus = 0
		m.searchResults = nil
		m.searchPage = 1
		m.searchTotal = 0
	case "Register":
		m.currentView = viewRegister
		m.authInputs = initRegisterInputs()
		m.authFocus = 0
		m.authErr = ""
	case "Login":
		m.currentView = viewLogin
		m.authInputs = initLoginInputs()
		m.authFocus = 0
		m.authErr = ""
	case "Library":
		m.currentView = viewLibrary
		m.libraryCursor = 0
		return m, cmdFetchLibrary(m.baseURL, m.token)
	case "Chat":
		m.currentView = viewChat
		m.chatPrompting = true
		m.chatMessages = nil
		inp := newChatPromptInput()
		m.chatPromptInput = inp
		return m, textinput.Blink
	case "Logout":
		if m.tcpConn != nil {
			m.tcpConn.Close()
			m.tcpConn = nil
		}
		if m.udpConn != nil {
			m.udpConn.Close()
			m.udpConn = nil
		}
		m.token = ""
		m.userID = ""
		m.username = ""
		m.sidebarIdx = 0
		m.currentView = viewMenu
	}
	return m, nil
}

func renderSidebar(m Model, width, height int) string {
	items := sidebarItems(m)
	var sb strings.Builder
	for i, item := range items {
		label := "  " + item
		if i == m.sidebarIdx {
			sb.WriteString(styleSidebarSelected.Width(width).Render(label) + "\n")
		} else {
			sb.WriteString(styleSidebarItem.Width(width).Render(label) + "\n")
		}
	}
	// pad to fill height
	written := len(items)
	for i := written; i < height; i++ {
		sb.WriteString(styleSidebarItem.Width(width).Render("") + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

func renderMenu(m Model, width, height int) string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(styleTitle.Render("  Welcome to MangaHub") + "\n\n")
	sb.WriteString(styleMutedText.Render("  Server: "+m.baseURL) + "\n\n")
	if m.token == "" {
		sb.WriteString(styleNormal.Render("  Select an option from the menu.") + "\n")
	} else {
		sb.WriteString(styleNormal.Render("  Logged in as: "+m.username) + "\n")
	}
	return lipgloss.NewStyle().Width(width).Render(sb.String())
}
```

Note: `textinput.Blink` and `newChatPromptInput()` are referenced here — `newChatPromptInput` will be defined in `view_chat.go`. Add a temporary stub in view_chat.go now:

In `view_chat.go`, add:
```go
import "github.com/charmbracelet/bubbles/textinput"

func newChatPromptInput() textinput.Model {
	inp := textinput.New()
	inp.Placeholder = "manga ID (blank = general)"
	inp.Focus()
	return inp
}
```

Also add `textinput` import to view_menu.go:
```go
import (
	"strings"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)
```

- [ ] **Step 4: Run tests — confirm pass**

```
go test ./cmd/client/ -run "TestSidebar|TestMenuNav|TestMenuSelect" -v
```

Expected: all 5 tests PASS.

- [ ] **Step 5: Build check**

```
go build ./cmd/client/
```

- [ ] **Step 6: Commit**

```bash
git add cmd/client/view_menu.go cmd/client/view_menu_test.go cmd/client/view_chat.go
git commit -m "feat(client): implement sidebar navigation and menu view"
```

---

## Task 7: view_auth.go — login and register forms

**Files:**
- Modify: `cmd/client/view_auth.go`
- Create: `cmd/client/view_auth_test.go`

- [ ] **Step 1: Write failing tests**

Create `cmd/client/view_auth_test.go`:

```go
package main

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func makeTestToken(userID, username string) string {
	payload, _ := json.Marshal(map[string]string{"user_id": userID, "username": username})
	enc := base64.RawURLEncoding.EncodeToString(payload)
	return "header." + enc + ".sig"
}

func TestLoginSuccessSetsState(t *testing.T) {
	m := New("http://localhost:8080")
	m.currentView = viewLogin
	m.authInputs = initLoginInputs()

	tok := makeTestToken("u1", "alice")
	next, cmds := m.Update(loginSuccessMsg{token: tok, userID: "u1", username: "alice"})
	m2 := next.(Model)

	assert.Equal(t, "alice", m2.username)
	assert.Equal(t, "u1", m2.userID)
	assert.Equal(t, tok, m2.token)
	assert.Equal(t, viewMenu, m2.currentView)
	assert.NotNil(t, cmds) // should fire cmdConnectTCP + cmdConnectUDP
}

func TestLoginErrSetsAuthErr(t *testing.T) {
	m := New("http://localhost:8080")
	m.currentView = viewLogin
	m.authInputs = initLoginInputs()

	next, _ := m.Update(errMsg{text: "invalid credentials"})
	m2 := next.(Model)
	assert.Equal(t, "invalid credentials", m2.authErr)
	assert.Equal(t, viewLogin, m2.currentView)
}

func TestRegisterSuccessSwitchesToLogin(t *testing.T) {
	m := New("http://localhost:8080")
	m.currentView = viewRegister
	m.authInputs = initRegisterInputs()

	next, _ := m.Update(registerSuccessMsg{})
	m2 := next.(Model)
	assert.Equal(t, viewLogin, m2.currentView)
	assert.Len(t, m2.authInputs, 2) // re-initialised as login form
}

func TestParseJWTClaims(t *testing.T) {
	tok := makeTestToken("u42", "bob")
	uid, uname, ok := parseJWTClaims(tok)
	assert.True(t, ok)
	assert.Equal(t, "u42", uid)
	assert.Equal(t, "bob", uname)
}
```

- [ ] **Step 2: Run — confirm fail**

```
go test ./cmd/client/ -run "TestLogin|TestRegister|TestParseJWT" -v
```

Expected: FAIL — stubs return nothing.

- [ ] **Step 3: Implement view_auth.go**

```go
package main

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func initLoginInputs() []textinput.Model {
	username := textinput.New()
	username.Placeholder = "username"
	username.Focus()
	username.Width = 30

	password := textinput.New()
	password.Placeholder = "password"
	password.EchoMode = textinput.EchoPassword
	password.Width = 30

	return []textinput.Model{username, password}
}

func initRegisterInputs() []textinput.Model {
	username := textinput.New()
	username.Placeholder = "username (min 3 chars)"
	username.Focus()
	username.Width = 30

	email := textinput.New()
	email.Placeholder = "email"
	email.Width = 30

	password := textinput.New()
	password.Placeholder = "password (min 8 chars)"
	password.EchoMode = textinput.EchoPassword
	password.Width = 30

	return []textinput.Model{username, email, password}
}

func cmdLogin(baseURL, username, password string) tea.Cmd {
	return func() tea.Msg {
		var resp struct {
			Token string `json:"token"`
			Error string `json:"error"`
		}
		code, err := postJSON(baseURL+"/auth/login", "", map[string]string{
			"username": username,
			"password": password,
		}, &resp)
		if err != nil {
			return errMsg{text: "Request failed: " + err.Error()}
		}
		if code != 200 {
			return errMsg{text: resp.Error}
		}
		uid, uname, ok := parseJWTClaims(resp.Token)
		if !ok {
			return errMsg{text: "Could not parse server token"}
		}
		return loginSuccessMsg{token: resp.Token, userID: uid, username: uname}
	}
}

func cmdRegister(baseURL, username, email, password string) tea.Cmd {
	return func() tea.Msg {
		var resp struct {
			Message string `json:"message"`
			Error   string `json:"error"`
		}
		code, err := postJSON(baseURL+"/auth/register", "", map[string]string{
			"username": username,
			"email":    email,
			"password": password,
		}, &resp)
		if err != nil {
			return errMsg{text: "Request failed: " + err.Error()}
		}
		if code != 201 {
			return errMsg{text: resp.Error}
		}
		return registerSuccessMsg{}
	}
}

func parseJWTClaims(token string) (userID, username string, ok bool) {
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
	userID, _ = claims["user_id"].(string)
	username, _ = claims["username"].(string)
	ok = true
	return
}

func updateAuth(m Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case loginSuccessMsg:
		m.token = msg.token
		m.userID = msg.userID
		m.username = msg.username
		m.currentView = viewMenu
		m.sidebarIdx = 0
		tcpAddr := getenv("TCP_ADDR", "localhost:9090")
		udpAddr := getenv("UDP_ADDR", "localhost:9091")
		return m, tea.Batch(
			cmdConnectTCP(tcpAddr, m.token),
			cmdConnectUDP(udpAddr),
		)

	case registerSuccessMsg:
		m.currentView = viewLogin
		m.authInputs = initLoginInputs()
		m.authFocus = 0
		m.authErr = "Registered! Please log in."
		return m, nil

	case errMsg:
		m.authErr = msg.text
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.currentView = viewMenu
			m.authErr = ""
			return m, nil

		case "tab", "down":
			m.authInputs[m.authFocus].Blur()
			m.authFocus = (m.authFocus + 1) % len(m.authInputs)
			m.authInputs[m.authFocus].Focus()
			return m, textinput.Blink

		case "shift+tab", "up":
			m.authInputs[m.authFocus].Blur()
			m.authFocus = (m.authFocus - 1 + len(m.authInputs)) % len(m.authInputs)
			m.authInputs[m.authFocus].Focus()
			return m, textinput.Blink

		case "enter":
			if m.authFocus < len(m.authInputs)-1 {
				// advance to next field
				m.authInputs[m.authFocus].Blur()
				m.authFocus++
				m.authInputs[m.authFocus].Focus()
				return m, textinput.Blink
			}
			// last field — submit
			return submitAuth(m)
		}

		// pass keystrokes to focused input
		var cmd tea.Cmd
		m.authInputs[m.authFocus], cmd = m.authInputs[m.authFocus].Update(msg)
		return m, cmd
	}

	// propagate to all inputs (handles cursor blink etc.)
	var cmds []tea.Cmd
	for i := range m.authInputs {
		var c tea.Cmd
		m.authInputs[i], c = m.authInputs[i].Update(msg)
		cmds = append(cmds, c)
	}
	return m, tea.Batch(cmds...)
}

func submitAuth(m Model) (Model, tea.Cmd) {
	if m.currentView == viewLogin {
		username := strings.TrimSpace(m.authInputs[0].Value())
		password := strings.TrimSpace(m.authInputs[1].Value())
		if username == "" || password == "" {
			m.authErr = "Username and password are required."
			return m, nil
		}
		return m, cmdLogin(m.baseURL, username, password)
	}
	// register
	username := strings.TrimSpace(m.authInputs[0].Value())
	email := strings.TrimSpace(m.authInputs[1].Value())
	password := strings.TrimSpace(m.authInputs[2].Value())
	var errs []string
	if len(username) < 3 {
		errs = append(errs, "Username must be at least 3 chars")
	}
	if !strings.Contains(email, "@") {
		errs = append(errs, "Email is invalid")
	}
	if len(password) < 8 {
		errs = append(errs, "Password must be at least 8 chars")
	}
	if len(errs) > 0 {
		m.authErr = strings.Join(errs, " · ")
		return m, nil
	}
	return m, cmdRegister(m.baseURL, username, email, password)
}

func renderAuth(m Model, width, height int) string {
	title := "Login"
	if m.currentView == viewRegister {
		title = "Register"
	}
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(styleTitle.Render("  "+title) + "\n\n")

	for i, inp := range m.authInputs {
		label := ""
		switch {
		case m.currentView == viewLogin && i == 0:
			label = "Username"
		case m.currentView == viewLogin && i == 1:
			label = "Password"
		case m.currentView == viewRegister && i == 0:
			label = "Username"
		case m.currentView == viewRegister && i == 1:
			label = "Email"
		case m.currentView == viewRegister && i == 2:
			label = "Password"
		}
		sb.WriteString(styleMutedText.Render("  "+label+":") + "\n")
		sb.WriteString("  " + inp.View() + "\n\n")
	}

	if m.authErr != "" {
		sb.WriteString(styleError.Render("  "+m.authErr) + "\n\n")
	}
	sb.WriteString(styleMutedText.Render("  Tab/↑↓ move · Enter submit · Esc back") + "\n")
	return lipgloss.NewStyle().Width(width).Render(sb.String())
}
```

- [ ] **Step 4: Run tests — confirm pass**

```
go test ./cmd/client/ -run "TestLogin|TestRegister|TestParseJWT" -v
```

Expected: all 4 tests PASS.

- [ ] **Step 5: Build check**

```
go build ./cmd/client/
```

- [ ] **Step 6: Commit**

```bash
git add cmd/client/view_auth.go cmd/client/view_auth_test.go
git commit -m "feat(client): implement login and register forms"
```

---

## Task 8: view_search.go — search form, results, pagination, detail

**Files:**
- Modify: `cmd/client/view_search.go`
- Create: `cmd/client/view_search_test.go`

- [ ] **Step 1: Write failing tests**

Create `cmd/client/view_search_test.go`:

```go
package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func newSearchModel() Model {
	m := New("http://localhost:8080")
	m.currentView = viewSearch
	m.searchState = searchStateForm
	m.searchInputs = initSearchInputs()
	m.width, m.height = 120, 40
	return m
}

func TestSearchResultsMsgSwitchesToResults(t *testing.T) {
	m := newSearchModel()
	results := []mangaItem{
		{ID: "one-piece", Title: "One Piece", Author: "Oda"},
		{ID: "naruto", Title: "Naruto", Author: "Kishimoto"},
	}
	next, _ := m.Update(searchResultMsg{results: results, total: 2, page: 1})
	m2 := next.(Model)
	assert.Equal(t, searchStateResults, m2.searchState)
	assert.Len(t, m2.searchResults, 2)
	assert.Equal(t, 0, m2.searchCursor)
	assert.Equal(t, 1, m2.searchPage)
	assert.Equal(t, 2, m2.searchTotal)
}

func TestSearchPaginationNext(t *testing.T) {
	m := newSearchModel()
	m.searchState = searchStateResults
	m.searchResults = make([]mangaItem, 20)
	m.searchPage = 1
	m.searchTotal = 50 // 3 pages of 20

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m2 := next.(Model)
	assert.Equal(t, 2, m2.searchPage)
	assert.NotNil(t, cmd) // fetches next page
}

func TestSearchPaginationPrevOnFirstPage(t *testing.T) {
	m := newSearchModel()
	m.searchState = searchStateResults
	m.searchPage = 1
	m.searchTotal = 50

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m2 := next.(Model)
	assert.Equal(t, 1, m2.searchPage) // stays on page 1
	assert.Nil(t, cmd)
}

func TestSearchDetailMsgSwitchesToDetail(t *testing.T) {
	m := newSearchModel()
	m.searchState = searchStateResults
	manga := mangaItem{ID: "one-piece", Title: "One Piece", Author: "Oda"}
	next, _ := m.Update(detailResultMsg{manga: manga, entry: nil})
	m2 := next.(Model)
	assert.Equal(t, searchStateDetail, m2.searchState)
	assert.Equal(t, "one-piece", m2.detailManga.ID)
	assert.Nil(t, m2.detailEntry)
}

func TestTotalPages(t *testing.T) {
	assert.Equal(t, 1, totalPages(0, 20))
	assert.Equal(t, 1, totalPages(20, 20))
	assert.Equal(t, 2, totalPages(21, 20))
	assert.Equal(t, 3, totalPages(50, 20))
}
```

- [ ] **Step 2: Run — confirm fail**

```
go test ./cmd/client/ -run "TestSearch|TestTotalPages" -v
```

Expected: FAIL.

- [ ] **Step 3: Implement view_search.go**

```go
package main

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const searchPageSize = 20

type searchResponse struct {
	Results  []mangaItem `json:"results"`
	Count    int         `json:"count"`
	Total    int         `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
	Error    string      `json:"error"`
}

type libraryResponse struct {
	ReadingLists map[string][]libraryItem `json:"reading_lists"`
	Total        int                      `json:"total"`
	Error        string                   `json:"error"`
}

type apiError struct {
	Error string `json:"error"`
}

func initSearchInputs() []textinput.Model {
	title := textinput.New()
	title.Placeholder = "title or author"
	title.Focus()
	title.Width = 30

	genre := textinput.New()
	genre.Placeholder = "e.g. Action, Romance"
	genre.Width = 30

	status := textinput.New()
	status.Placeholder = "ongoing / completed"
	status.Width = 30

	return []textinput.Model{title, genre, status}
}

func totalPages(total, pageSize int) int {
	if total == 0 || pageSize == 0 {
		return 1
	}
	p := total / pageSize
	if total%pageSize > 0 {
		p++
	}
	return p
}

func cmdSearch(baseURL, token, q, genre, status string, page int) tea.Cmd {
	return func() tea.Msg {
		params := url.Values{}
		if q != "" {
			params.Set("q", q)
		}
		if genre != "" {
			params.Set("genre", genre)
		}
		if status != "" {
			params.Set("status", status)
		}
		params.Set("page", strconv.Itoa(page))
		params.Set("page_size", strconv.Itoa(searchPageSize))

		endpoint := baseURL + "/manga?" + params.Encode()
		var resp searchResponse
		code, err := getJSON(endpoint, token, &resp)
		if err != nil {
			return searchResultMsg{err: err.Error()}
		}
		if code != 200 {
			return searchResultMsg{err: resp.Error}
		}
		return searchResultMsg{results: resp.Results, total: resp.Total, page: resp.Page}
	}
}

func cmdFetchDetail(baseURL, token, id string) tea.Cmd {
	return func() tea.Msg {
		var manga mangaItem
		code, err := getJSON(baseURL+"/manga/"+id, token, &manga)
		if err != nil {
			return detailResultMsg{err: err.Error()}
		}
		if code != 200 {
			return detailResultMsg{err: "manga not found"}
		}
		var entry *libraryItem
		if token != "" {
			entry = fetchLibraryEntry(baseURL, token, id)
		}
		return detailResultMsg{manga: manga, entry: entry}
	}
}

// fetchLibraryEntry checks the user's library for a specific manga ID.
func fetchLibraryEntry(baseURL, token, mangaID string) *libraryItem {
	var resp libraryResponse
	code, err := getJSON(baseURL+"/users/library", token, &resp)
	if err != nil || code != 200 {
		return nil
	}
	for _, items := range resp.ReadingLists {
		for i := range items {
			if items[i].MangaID == mangaID {
				return &items[i]
			}
		}
	}
	return nil
}

func cmdAddToLibrary(baseURL, token, mangaID, status string, chapter int) tea.Cmd {
	return func() tea.Msg {
		var resp apiError
		code, err := postJSON(baseURL+"/users/library", token, map[string]interface{}{
			"manga_id":        mangaID,
			"status":          status,
			"current_chapter": chapter,
		}, &resp)
		if err != nil {
			return addLibraryMsg{err: err.Error()}
		}
		if code != 201 {
			return addLibraryMsg{err: resp.Error}
		}
		return addLibraryMsg{}
	}
}

func cmdUpdateProgress(baseURL, token, mangaID, status string, chapter int) tea.Cmd {
	return func() tea.Msg {
		body := map[string]interface{}{
			"manga_id":        mangaID,
			"current_chapter": chapter,
		}
		if status != "" {
			body["status"] = status
		}
		var resp apiError
		code, err := putJSON(baseURL+"/users/progress", token, body, &resp)
		if err != nil {
			return updateProgressMsg{err: err.Error()}
		}
		if code != 200 {
			return updateProgressMsg{err: resp.Error}
		}
		return updateProgressMsg{}
	}
}

func updateSearch(m Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case searchResultMsg:
		if msg.err != "" {
			m.notification = "Search error: " + msg.err
			return m, nil
		}
		m.searchResults = msg.results
		m.searchTotal = msg.total
		m.searchPage = msg.page
		m.searchCursor = 0
		m.searchState = searchStateResults
		return m, nil

	case detailResultMsg:
		if msg.err != "" {
			m.notification = "Error: " + msg.err
			return m, nil
		}
		m.detailManga = msg.manga
		m.detailEntry = msg.entry
		m.searchState = searchStateDetail
		m.detailFocus = 0
		return m, nil

	case addLibraryMsg:
		if msg.err != "" {
			m.notification = "Add failed: " + msg.err
		} else {
			m.notification = fmt.Sprintf("Added %q to library.", m.detailManga.Title)
			// refresh entry
			return m, cmdFetchDetail(m.baseURL, m.token, m.detailManga.ID)
		}
		return m, nil

	case updateProgressMsg:
		if msg.err != "" {
			m.notification = "Update failed: " + msg.err
		} else {
			m.notification = fmt.Sprintf("Progress updated for %q.", m.detailManga.Title)
			return m, cmdFetchDetail(m.baseURL, m.token, m.detailManga.ID)
		}
		return m, nil

	case tea.KeyMsg:
		switch m.searchState {
		case searchStateForm:
			return updateSearchForm(m, msg)
		case searchStateResults:
			return updateSearchResults(m, msg)
		case searchStateDetail:
			return updateSearchDetail(m, msg)
		}
	}
	// propagate to focused input when in form state
	if m.searchState == searchStateForm && len(m.searchInputs) > 0 {
		var cmd tea.Cmd
		m.searchInputs[m.searchFocus], cmd = m.searchInputs[m.searchFocus].Update(msg)
		return m, cmd
	}
	return m, nil
}

func updateSearchForm(m Model, msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.currentView = viewMenu
		return m, nil
	case "tab", "down":
		m.searchInputs[m.searchFocus].Blur()
		m.searchFocus = (m.searchFocus + 1) % len(m.searchInputs)
		m.searchInputs[m.searchFocus].Focus()
		return m, textinput.Blink
	case "shift+tab", "up":
		m.searchInputs[m.searchFocus].Blur()
		m.searchFocus = (m.searchFocus - 1 + len(m.searchInputs)) % len(m.searchInputs)
		m.searchInputs[m.searchFocus].Focus()
		return m, textinput.Blink
	case "enter":
		q := strings.TrimSpace(m.searchInputs[0].Value())
		genre := strings.TrimSpace(m.searchInputs[1].Value())
		status := strings.TrimSpace(m.searchInputs[2].Value())
		m.searchPage = 1
		return m, cmdSearch(m.baseURL, m.token, q, genre, status, 1)
	default:
		var cmd tea.Cmd
		m.searchInputs[m.searchFocus], cmd = m.searchInputs[m.searchFocus].Update(msg)
		return m, cmd
	}
}

func updateSearchResults(m Model, msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.searchState = searchStateForm
	case "up", "k":
		if m.searchCursor > 0 {
			m.searchCursor--
		}
	case "down", "j":
		if m.searchCursor < len(m.searchResults)-1 {
			m.searchCursor++
		}
	case "right", "l":
		pages := totalPages(m.searchTotal, searchPageSize)
		if m.searchPage < pages {
			m.searchPage++
			q := strings.TrimSpace(m.searchInputs[0].Value())
			genre := strings.TrimSpace(m.searchInputs[1].Value())
			status := strings.TrimSpace(m.searchInputs[2].Value())
			return m, cmdSearch(m.baseURL, m.token, q, genre, status, m.searchPage)
		}
	case "left", "h":
		if m.searchPage > 1 {
			m.searchPage--
			q := strings.TrimSpace(m.searchInputs[0].Value())
			genre := strings.TrimSpace(m.searchInputs[1].Value())
			status := strings.TrimSpace(m.searchInputs[2].Value())
			return m, cmdSearch(m.baseURL, m.token, q, genre, status, m.searchPage)
		}
	case "enter":
		if m.searchCursor < len(m.searchResults) {
			id := m.searchResults[m.searchCursor].ID
			return m, cmdFetchDetail(m.baseURL, m.token, id)
		}
	}
	return m, nil
}

func updateSearchDetail(m Model, msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.searchState = searchStateResults
	case "1":
		// action 1: Add to library (if not in library) or Update progress (if in library)
		if m.token == "" {
			return m, nil
		}
		if m.detailEntry == nil {
			// add with defaults
			return m, cmdAddToLibrary(m.baseURL, m.token, m.detailManga.ID, "reading", 0)
		}
		// increment chapter by 1 as a quick update
		return m, cmdUpdateProgress(m.baseURL, m.token, m.detailManga.ID, "", m.detailEntry.CurrentChapter+1)
	}
	return m, nil
}

func renderSearch(m Model, width, height int) string {
	switch m.searchState {
	case searchStateForm:
		return renderSearchForm(m, width, height)
	case searchStateResults:
		return renderSearchResults(m, width, height)
	case searchStateDetail:
		return renderSearchDetail(m, width, height)
	}
	return ""
}

func renderSearchForm(m Model, width, height int) string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(styleTitle.Render("  Search Manga") + "\n\n")
	labels := []string{"Title / Author", "Genre", "Status"}
	for i, inp := range m.searchInputs {
		sb.WriteString(styleMutedText.Render("  "+labels[i]+":") + "\n")
		sb.WriteString("  " + inp.View() + "\n\n")
	}
	sb.WriteString(styleMutedText.Render("  Tab/↑↓ move · Enter search · Esc back") + "\n")
	return lipgloss.NewStyle().Width(width).Render(sb.String())
}

func renderSearchResults(m Model, width, height int) string {
	var sb strings.Builder
	pages := totalPages(m.searchTotal, searchPageSize)
	sb.WriteString(fmt.Sprintf("\n  Found %d result(s) — Page %d of %d\n\n",
		m.searchTotal, m.searchPage, pages))

	for i, item := range m.searchResults {
		line := fmt.Sprintf("  %-35s  %s", truncate(item.Title, 35), styleMutedText.Render(item.Author))
		if i == m.searchCursor {
			sb.WriteString(styleSidebarSelected.Width(width).Render(line) + "\n")
		} else {
			sb.WriteString(styleNormal.Render(line) + "\n")
		}
	}
	sb.WriteString("\n")
	sb.WriteString(styleMutedText.Render("  ↑↓ navigate · ←→ page · Enter detail · Esc back") + "\n")
	return lipgloss.NewStyle().Width(width).Render(sb.String())
}

func renderSearchDetail(m Model, width, height int) string {
	m2 := m.detailManga
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(styleTitle.Render("  "+m2.Title) + "\n\n")
	sb.WriteString(styleNormal.Render(fmt.Sprintf("  Author:   %s", m2.Author)) + "\n")
	sb.WriteString(styleNormal.Render(fmt.Sprintf("  Genres:   %s", strings.Join(m2.Genres, ", "))) + "\n")
	sb.WriteString(styleNormal.Render(fmt.Sprintf("  Status:   %s", m2.Status)) + "\n")
	sb.WriteString(styleNormal.Render(fmt.Sprintf("  Chapters: %d", m2.TotalChapters)) + "\n")
	if m2.CoverURL != "" {
		sb.WriteString(styleMutedText.Render("  Cover:    "+m2.CoverURL) + "\n")
	}
	if m2.Description != "" {
		sb.WriteString("\n" + styleNormal.Render("  "+truncate(m2.Description, width-4)) + "\n")
	}
	sb.WriteString("\n")
	if m.token != "" {
		if m.detailEntry != nil {
			sb.WriteString(styleNormal.Render(fmt.Sprintf(
				"  [In library] ch.%d · %s", m.detailEntry.CurrentChapter, m.detailEntry.Status)) + "\n\n")
			sb.WriteString(styleSidebarSelected.Render("  1. Quick +1 chapter") + "\n")
		} else {
			sb.WriteString(styleSidebarSelected.Render("  1. Add to library (reading, ch.0)") + "\n")
		}
	}
	sb.WriteString("\n" + styleMutedText.Render("  Esc back to results") + "\n")
	return lipgloss.NewStyle().Width(width).Render(sb.String())
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
```

- [ ] **Step 4: Run tests — confirm pass**

```
go test ./cmd/client/ -run "TestSearch|TestTotalPages" -v
```

Expected: all 5 tests PASS.

- [ ] **Step 5: Build check**

```
go build ./cmd/client/
```

- [ ] **Step 6: Commit**

```bash
git add cmd/client/view_search.go cmd/client/view_search_test.go
git commit -m "feat(client): implement search view with form, results, pagination, and detail"
```

---

## Task 9: view_library.go — library list

**Files:**
- Modify: `cmd/client/view_library.go`
- Create: `cmd/client/view_library_test.go`

- [ ] **Step 1: Write failing tests**

Create `cmd/client/view_library_test.go`:

```go
package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestLibraryResultMsg(t *testing.T) {
	m := New("http://localhost:8080")
	m.currentView = viewLibrary
	m.token = "tok"

	groups := map[string][]libraryItem{
		"reading":   {{MangaID: "one-piece", Title: "One Piece", CurrentChapter: 1096}},
		"completed": {{MangaID: "naruto", Title: "Naruto", CurrentChapter: 700}},
	}
	next, _ := m.Update(libraryResultMsg{groups: groups, total: 2})
	m2 := next.(Model)
	assert.Equal(t, 2, len(m2.libraryFlat))
	assert.Equal(t, 0, m2.libraryCursor)
}

func TestLibraryNavDown(t *testing.T) {
	m := New("http://localhost:8080")
	m.currentView = viewLibrary
	m.libraryFlat = []libraryItem{
		{MangaID: "one-piece"},
		{MangaID: "naruto"},
	}
	m.libraryCursor = 0

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m2 := next.(Model)
	assert.Equal(t, 1, m2.libraryCursor)
}

func TestFlattenLibrary(t *testing.T) {
	groups := map[string][]libraryItem{
		"reading":   {{MangaID: "a"}},
		"completed": {{MangaID: "b"}},
		"on_hold":   {{MangaID: "c"}},
	}
	flat := flattenLibrary(groups)
	// reading comes first, then completed
	assert.Equal(t, "a", flat[0].MangaID)
	assert.Equal(t, "b", flat[1].MangaID)
}
```

- [ ] **Step 2: Run — confirm fail**

```
go test ./cmd/client/ -run "TestLibrary|TestFlatten" -v
```

Expected: FAIL.

- [ ] **Step 3: Implement view_library.go**

```go
package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var libraryStatusOrder = []string{"reading", "completed", "plan_to_read", "on_hold", "dropped"}

func cmdFetchLibrary(baseURL, token string) tea.Cmd {
	return func() tea.Msg {
		var resp libraryResponse
		code, err := getJSON(baseURL+"/users/library", token, &resp)
		if err != nil {
			return libraryResultMsg{err: err.Error()}
		}
		if code != 200 {
			return libraryResultMsg{err: resp.Error}
		}
		return libraryResultMsg{groups: resp.ReadingLists, total: resp.Total}
	}
}

func flattenLibrary(groups map[string][]libraryItem) []libraryItem {
	var flat []libraryItem
	for _, s := range libraryStatusOrder {
		flat = append(flat, groups[s]...)
	}
	return flat
}

func updateLibrary(m Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case libraryResultMsg:
		if msg.err != "" {
			m.notification = "Library error: " + msg.err
			return m, nil
		}
		m.libraryGroups = msg.groups
		m.libraryFlat = flattenLibrary(msg.groups)
		m.libraryCursor = 0
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.currentView = viewMenu
		case "up", "k":
			if m.libraryCursor > 0 {
				m.libraryCursor--
			}
		case "down", "j":
			if m.libraryCursor < len(m.libraryFlat)-1 {
				m.libraryCursor++
			}
		case "enter":
			if m.libraryCursor < len(m.libraryFlat) {
				id := m.libraryFlat[m.libraryCursor].MangaID
				m.currentView = viewSearch
				m.searchState = searchStateDetail
				return m, cmdFetchDetail(m.baseURL, m.token, id)
			}
		}
	}
	return m, nil
}

func renderLibrary(m Model, width, height int) string {
	if m.libraryFlat == nil {
		return lipgloss.NewStyle().Width(width).Render(
			"\n" + styleMutedText.Render("  Loading library..."))
	}
	if len(m.libraryFlat) == 0 {
		return lipgloss.NewStyle().Width(width).Render(
			"\n" + styleNormal.Render("  Your library is empty. Add manga via Search."))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n  %s (%d total)\n",
		styleTitle.Render("My Library"), len(m.libraryFlat)))

	flatIdx := 0
	for _, status := range libraryStatusOrder {
		items := m.libraryGroups[status]
		if len(items) == 0 {
			continue
		}
		label := strings.ToUpper(strings.ReplaceAll(status, "_", " "))
		sb.WriteString("\n  " + styleMutedText.Render("["+label+"]") + "\n")
		for _, item := range items {
			line := fmt.Sprintf("  %-30s  ch.%-4d", truncate(item.Title, 30), item.CurrentChapter)
			if flatIdx == m.libraryCursor {
				sb.WriteString(styleSidebarSelected.Width(width).Render(line) + "\n")
			} else {
				sb.WriteString(styleNormal.Render(line) + "\n")
			}
			flatIdx++
		}
	}
	sb.WriteString("\n" + styleMutedText.Render("  ↑↓ navigate · Enter view detail · Esc back") + "\n")
	return lipgloss.NewStyle().Width(width).Render(sb.String())
}
```

- [ ] **Step 4: Run tests — confirm pass**

```
go test ./cmd/client/ -run "TestLibrary|TestFlatten" -v
```

Expected: all 3 tests PASS.

- [ ] **Step 5: Build check**

```
go build ./cmd/client/
```

- [ ] **Step 6: Commit**

```bash
git add cmd/client/view_library.go cmd/client/view_library_test.go
git commit -m "feat(client): implement library view"
```

---

## Task 10: view_chat.go — full-screen chat room

**Files:**
- Modify: `cmd/client/view_chat.go`
- Create: `cmd/client/view_chat_test.go`

- [ ] **Step 1: Write failing tests**

Create `cmd/client/view_chat_test.go`:

```go
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func newChatModel() Model {
	m := New("http://localhost:8080")
	m.currentView = viewChat
	m.token = "tok"
	m.userID = "u1"
	m.username = "alice"
	m.width, m.height = 100, 40
	m.chatPrompting = false
	m.chatInput = newChatInput()
	m.chatViewport = newChatViewport(100, 35)
	return m
}

func TestChatMessageAppended(t *testing.T) {
	m := newChatModel()
	next, cmd := m.Update(wsMsgReceived{userID: "u2", username: "bob", text: "hey!"})
	m2 := next.(Model)
	assert.Len(t, m2.chatMessages, 1)
	assert.Equal(t, "hey!", m2.chatMessages[0].text)
	assert.False(t, m2.chatMessages[0].isSystem)
	assert.NotNil(t, cmd) // re-subscribes to WS
}

func TestChatJoinAppendsSystemMsg(t *testing.T) {
	m := newChatModel()
	next, _ := m.Update(wsJoined{username: "carol"})
	m2 := next.(Model)
	assert.Len(t, m2.chatMessages, 1)
	assert.True(t, m2.chatMessages[0].isSystem)
	assert.Contains(t, m2.chatMessages[0].text, "carol")
}

func TestChatLeftAppendsSystemMsg(t *testing.T) {
	m := newChatModel()
	next, _ := m.Update(wsLeft{username: "carol"})
	m2 := next.(Model)
	assert.Len(t, m2.chatMessages, 1)
	assert.True(t, m2.chatMessages[0].isSystem)
}

func TestFormatChatMsgSelf(t *testing.T) {
	msg := chatMessage{userID: "u1", username: "alice", text: "hello"}
	rendered := formatChatMsg(msg, "u1")
	assert.Contains(t, rendered, "You")
	assert.Contains(t, rendered, "hello")
}

func TestFormatChatMsgOther(t *testing.T) {
	msg := chatMessage{userID: "u2", username: "bob", text: "world"}
	rendered := formatChatMsg(msg, "u1")
	assert.Contains(t, rendered, "bob")
	assert.Contains(t, rendered, "world")
}
```

- [ ] **Step 2: Run — confirm fail**

```
go test ./cmd/client/ -run "TestChat|TestFormat" -v
```

Expected: FAIL — stubs.

- [ ] **Step 3: Implement view_chat.go**

```go
package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gorilla/websocket"
)

func newChatInput() textinput.Model {
	inp := textinput.New()
	inp.Placeholder = "type a message... (/exit to leave)"
	inp.Focus()
	inp.Width = 80
	return inp
}

func newChatPromptInput() textinput.Model {
	inp := textinput.New()
	inp.Placeholder = "manga ID (blank = general)"
	inp.Focus()
	inp.Width = 40
	return inp
}

func newChatViewport(width, height int) viewport.Model {
	vp := viewport.New(width, height)
	return vp
}

func formatChatMsg(msg chatMessage, myUserID string) string {
	if msg.isSystem {
		return styleMutedText.Render("  ── " + msg.text + " ──")
	}
	label := fmt.Sprintf("%-8s", msg.username)
	if msg.userID == myUserID {
		label = fmt.Sprintf("%-8s", "You")
	}
	return fmt.Sprintf("[%s] %s",
		styleTitle.Render(label),
		styleNormal.Render(msg.text))
}

func updateChat(m Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case wsConnectedMsg:
		m.chatConn = msg.conn
		return m, waitForWS(msg.conn)

	case wsMsgReceived:
		if msg.text != "" || msg.userID != "" {
			m.chatMessages = append(m.chatMessages, chatMessage{
				userID:   msg.userID,
				username: msg.username,
				text:     msg.text,
			})
			m.chatViewport.SetContent(renderMessages(m))
			m.chatViewport.GotoBottom()
		}
		if m.chatConn != nil {
			return m, waitForWS(m.chatConn)
		}
		return m, nil

	case wsJoined:
		m.chatMessages = append(m.chatMessages, chatMessage{
			text:     msg.username + " joined the room",
			isSystem: true,
		})
		m.chatViewport.SetContent(renderMessages(m))
		if m.chatConn != nil {
			return m, waitForWS(m.chatConn)
		}
		return m, nil

	case wsLeft:
		m.chatMessages = append(m.chatMessages, chatMessage{
			text:     msg.username + " left the room",
			isSystem: true,
		})
		m.chatViewport.SetContent(renderMessages(m))
		if m.chatConn != nil {
			return m, waitForWS(m.chatConn)
		}
		return m, nil

	case errMsg:
		if m.currentView == viewChat {
			m.chatMessages = append(m.chatMessages, chatMessage{
				text:     msg.text,
				isSystem: true,
			})
			m.chatViewport.SetContent(renderMessages(m))
		}
		return m, nil

	case tea.KeyMsg:
		if m.chatPrompting {
			return updateChatPrompt(m, msg)
		}
		return updateChatActive(m, msg)
	}

	// propagate to input/viewport
	var cmds []tea.Cmd
	if m.chatPrompting {
		var c tea.Cmd
		m.chatPromptInput, c = m.chatPromptInput.Update(msg)
		cmds = append(cmds, c)
	} else {
		var c tea.Cmd
		m.chatInput, c = m.chatInput.Update(msg)
		cmds = append(cmds, c)
		var c2 tea.Cmd
		m.chatViewport, c2 = m.chatViewport.Update(msg)
		cmds = append(cmds, c2)
	}
	return m, tea.Batch(cmds...)
}

func updateChatPrompt(m Model, msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.currentView = viewMenu
		m.chatPrompting = false
		return m, nil
	case "enter":
		mangaID := strings.TrimSpace(m.chatPromptInput.Value())
		if mangaID == "" {
			mangaID = "general"
		}
		m.chatMangaID = mangaID
		m.chatPrompting = false
		m.chatInput = newChatInput()
		m.chatViewport = newChatViewport(m.width, m.height-4)
		return m, cmdConnectWS(m.baseURL, m.token, mangaID)
	default:
		var c tea.Cmd
		m.chatPromptInput, c = m.chatPromptInput.Update(msg)
		return m, c
	}
}

func updateChatActive(m Model, msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		text := strings.TrimSpace(m.chatInput.Value())
		if text == "/exit" {
			if m.chatConn != nil {
				m.chatConn.WriteMessage( //nolint:errcheck
					websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				m.chatConn.Close()
				m.chatConn = nil
			}
			m.currentView = viewMenu
			m.chatMessages = nil
			return m, nil
		}
		if text == "" {
			return m, nil
		}
		m.chatInput.SetValue("")
		if m.chatConn != nil {
			return m, cmdSendWSMessage(m.chatConn, text)
		}
		return m, nil
	default:
		var c tea.Cmd
		m.chatInput, c = m.chatInput.Update(msg)
		return m, c
	}
}

func renderMessages(m Model) string {
	var lines []string
	for _, msg := range m.chatMessages {
		lines = append(lines, formatChatMsg(msg, m.userID))
	}
	return strings.Join(lines, "\n")
}

func renderChatScreen(m Model) string {
	if m.chatPrompting {
		return renderChatPrompt(m)
	}

	titleBar := styleHeader.Width(m.width).Render(
		fmt.Sprintf("  Chat: %s   (type /exit to leave)", m.chatMangaID))
	divider := styleMutedText.Width(m.width).Render(strings.Repeat("─", m.width))
	inputLine := "  > " + m.chatInput.View()

	return lipgloss.JoinVertical(lipgloss.Left,
		titleBar,
		m.chatViewport.View(),
		divider,
		inputLine,
	)
}

func renderChatPrompt(m Model) string {
	var sb strings.Builder
	sb.WriteString("\n\n")
	sb.WriteString(styleTitle.Render("  Enter Chat Room") + "\n\n")
	sb.WriteString(styleMutedText.Render("  Manga ID:") + "\n")
	sb.WriteString("  " + m.chatPromptInput.View() + "\n\n")
	sb.WriteString(styleMutedText.Render("  Enter to connect · Esc to cancel") + "\n")
	return lipgloss.NewStyle().Width(m.width).Height(m.height).Render(sb.String())
}
```

- [ ] **Step 4: Run tests — confirm pass**

```
go test ./cmd/client/ -run "TestChat|TestFormat" -v
```

Expected: all 5 tests PASS.

- [ ] **Step 5: Run all client tests**

```
go test ./cmd/client/ -v
```

Expected: all tests PASS.

- [ ] **Step 6: Build check**

```
go build ./cmd/client/
```

- [ ] **Step 7: Run full project tests**

```
go test ./...
```

Expected: all tests PASS.

- [ ] **Step 8: Commit**

```bash
git add cmd/client/view_chat.go cmd/client/view_chat_test.go
git commit -m "feat(client): implement full-screen chat view with WebSocket integration"
```

---

## Final smoke test

Start the runner and verify the TUI launches:

```
go run ./cmd/runner/
```

In a second terminal:

```
go run ./cmd/client/
```

Verify:
- [ ] TUI launches with split-pane layout, gold header, dark background
- [ ] Sidebar shows Search / Register / Login
- [ ] Arrow keys navigate sidebar, Enter selects
- [ ] Search form shows 3 stacked fields
- [ ] Login/Register forms work and log in successfully
- [ ] After login sidebar shows Search / Library / Chat / Logout
- [ ] TCP/UDP notifications appear in footer without corrupting layout
- [ ] Chat room enters full-screen, `/exit` returns to menu
- [ ] `Ctrl+C` exits cleanly
