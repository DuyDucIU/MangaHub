# Design Spec: Bubbletea Terminal Client

**Date:** 2026-05-09
**Status:** Approved

---

## Overview

Rewrite `cmd/client/` from a plain `bufio.Scanner + fmt` sequential script into a full-screen bubbletea TUI. The new client has a persistent split-pane layout, a color theme, real-time notifications from TCP/UDP in a footer bar, paginated search results, and a full-screen chat room.

`http.go` is the only existing file kept unchanged. All other files in `cmd/client/` are replaced.

---

## 1. Layout

```
┌─────────────────────────────────────────────┐
│  MangaHub                      ducuser      │  ← gold header
├───────────────┬─────────────────────────────┤
│               │                             │
│  > Search     │   (content panel)           │
│    Library    │                             │
│    Chat       │                             │
│    Logout     │                             │
│               │                             │
│               │                             │
├───────────────┴─────────────────────────────┤
│  Notification: Bleach ch.700 just released! │  ← muted gold
└─────────────────────────────────────────────┘
```

- **Header bar** — app name left, logged-in username right. Hidden before login (shows server address instead).
- **Sidebar** — always visible except during chat. Highlights the active item in deep red.
- **Content panel** — right side, changes per view.
- **Notification footer** — single line. Stays until replaced by the next TCP/UDP message.
- **Chat** — full-screen takeover: sidebar and header hidden, chat fills entire terminal.

---

## 2. Color Theme — "Berserk Dark"

| Role | Hex | Usage |
|---|---|---|
| Primary accent | `#C9A84C` | Header text, borders of active panel, selected item |
| Secondary accent | `#8B1A1A` | Sidebar highlight background |
| Normal text | `#D4CFBF` | All body text |
| Muted text | `#4A5568` | Labels, timestamps, metadata |
| Notification | `#A07800` | Footer notification text |
| Background | `#0F0F14` | Terminal background assumption |

All styles defined as lipgloss constants in `model.go`.

---

## 3. Architecture

### Model

Single `Model` struct. A `currentView` int enum controls what renders. All shared state lives on the model.

```go
type view int

const (
    viewMenu view = iota
    viewLogin
    viewRegister
    viewSearch
    viewLibrary
    viewChat
)

type Model struct {
    // navigation
    currentView  view
    sidebarIdx   int

    // auth
    token        string
    userID       string
    username     string

    // connections
    tcpConn      net.Conn
    udpConn      *net.UDPConn

    // notification footer (stays until replaced)
    notification string

    // terminal dimensions
    width, height int

    // search view
    searchState   searchState   // form / results / detail
    searchInputs  []textinput.Model
    searchResults []mangaItem
    searchCursor  int
    searchPage    int
    searchTotal   int
    detailManga   mangaItem
    detailEntry   *libraryItem  // nil if not in library

    // library view
    libraryItems  []libraryItem
    libraryCursor int

    // auth view
    authState     authState     // login / register
    authInputs    []textinput.Model
    authErr       string

    // chat view
    chatMangaID   string
    chatMessages  []chatMessage
    chatInput     textinput.Model
    chatViewport  viewport.Model
    chatConn      *websocket.Conn
    chatPrompting bool          // true while waiting for manga ID input
}
```

Top-level `Update()` dispatches to a per-view handler function. Top-level `View()` dispatches to a per-view render function. Both live in `model.go`.

### HTTP calls as tea.Cmd

All network calls are wrapped in `tea.Cmd` so they run off the main goroutine and deliver results as `tea.Msg`:

```go
func cmdSearch(baseURL, token string, params url.Values) tea.Cmd {
    return func() tea.Msg {
        // calls getJSON, returns searchResultMsg or errMsg
    }
}
```

Existing `getJSON` / `postJSON` / `putJSON` helpers in `http.go` are reused as-is.

---

## 4. Views

### 4.1 Menu (`view_menu.go`)

Default view after launch (before login) and after login (before selecting an action).

- **Guest sidebar:** Search / Register / Login
- **Auth sidebar:** Search / Library / Chat / Logout
- **Content panel:** welcome message + server address

Sidebar navigation: ↑↓ arrows or number keys. Enter activates the selected item and switches `currentView`.

### 4.2 Auth (`view_auth.go`)

Handles both Login and Register as sub-states (`authStateLogin` / `authStateRegister`).

- Stacked `textinput` components (one per field)
- Tab moves focus between fields
- Enter on the last field submits
- Validation errors displayed inline below the field
- On success: sets token/userID/username on model, issues `cmdConnectTCP` + `cmdConnectUDP`, switches to `viewMenu`

**Login fields:** Username, Password

**Register fields:** Username, Email, Password

### 4.3 Search (`view_search.go`)

Three sub-states:

**`searchStateForm`**
- Stacked textinputs: Title, Genre, Status
- Tab between fields, Enter submits → issues `cmdSearch` → transitions to `searchStateResults`

**`searchStateResults`**
- Results list with ↑↓ navigation
- ←→ arrow keys change page (issues new `cmdSearch` with `page` param)
- Page indicator: `Page 1 of 43`
- Enter on item → issues `cmdFetchDetail` → transitions to `searchStateDetail`
- Page size: 20 results

**`searchStateDetail`**
- Shows full manga info (title, author, genres, status, chapters, description)
- If logged in and manga is in library: shows current chapter + status, offers Update Progress
- If logged in and not in library: offers Add to Library
- If not logged in: no action options
- Esc → back to results, retaining current page and cursor position

### 4.4 Library (`view_library.go`)

- Fetches library on view entry via `cmdFetchLibrary`
- Grouped by status: READING / COMPLETED / PLAN TO READ / ON HOLD / DROPPED
- ↑↓ to scroll through all items across groups
- Enter on an item → switches to `viewSearch` / `searchStateDetail` for that manga ID

### 4.5 Chat (`view_chat.go`)

Full-screen takeover — sidebar and header hidden while active.

```
┌─────────────────────────────────────┐
│  Chat Room: one-piece          /exit│
├─────────────────────────────────────┤
│ [Tanaka  ] hey anyone reading?      │
│ [You     ] just finished it         │
│                                     │
│                                     │
├─────────────────────────────────────┤
│ > type a message...                 │
└─────────────────────────────────────┘
```

- On enter: shows a manga ID textinput in the content area (`chatPrompting = true`); Enter confirms, blank input defaults to "general"
- Connects WebSocket, sends JWT as first message
- History (last 20 messages) rendered in a `viewport` component — scrollable with ↑↓
- New messages auto-scroll to bottom
- `textinput` at bottom for composing; Enter sends
- `/exit` closes WebSocket, returns to `viewMenu`
- Join/leave events printed as system lines in the message pane

---

## 5. TCP / UDP Integration

Background listeners communicate with bubbletea via self-renewing `tea.Cmd` — no `fmt.Printf` from goroutines.

### Pattern

```go
type tcpNotifMsg struct{ text string }
type udpNotifMsg struct{ text string }

// Returns a Cmd that blocks until one message arrives, then returns it as a Msg.
// Update() re-issues this Cmd after each message to keep listening.
func waitForTCP(conn net.Conn) tea.Cmd { ... }
func waitForUDP(conn *net.UDPConn) tea.Cmd { ... }
```

In `Update()`:
```go
case tcpNotifMsg:
    m.notification = msg.text
    return m, waitForTCP(m.tcpConn)   // re-subscribe

case udpNotifMsg:
    m.notification = msg.text
    return m, waitForUDP(m.udpConn)   // re-subscribe
```

Connection is established after login via `cmdConnectTCP` / `cmdConnectUDP`. On failure, a warning is written to `m.notification` and the app continues without that protocol.

### WebSocket

Same pattern — `waitForWS(conn)` blocks on one message, returns it as `wsMsgReceived` / `wsJoined` / `wsLeft`. Chat view re-subscribes after each message.

---

## 6. Pagination

- Page size: 20
- API called with `?page=N&limit=20` query params (in addition to existing filters)
- Model holds `searchPage int` and `searchTotal int`
- ← key: `searchPage--`, re-issues `cmdSearch` (no-op if already page 1)
- → key: `searchPage++`, re-issues `cmdSearch` (no-op if on last page)
- Page indicator rendered below results: `Page 1 of 43`

---

## 7. Error Handling

- HTTP errors: displayed inline in the content panel, no crash
- TCP connect failure: written to `notification`, app continues without TCP
- UDP connect failure: written to `notification`, app continues without UDP
- WebSocket connect failure: error shown in chat panel, returns to menu
- All errors non-fatal

---

## 8. Files

| File | Description |
|---|---|
| `cmd/client/main.go` | `tea.NewProgram` entry point, terminal size init |
| `cmd/client/model.go` | `Model` struct, `view` enum, `Init` / `Update` / `View`, lipgloss styles |
| `cmd/client/view_menu.go` | Sidebar render + welcome panel |
| `cmd/client/view_auth.go` | Login + register forms |
| `cmd/client/view_search.go` | Search form, results list, manga detail |
| `cmd/client/view_library.go` | Library grouped list |
| `cmd/client/view_chat.go` | Full-screen chat with viewport + textinput |
| `cmd/client/tcp.go` | `connectTCP`, `waitForTCP`, TCP msg types |
| `cmd/client/udp.go` | `connectUDP`, `waitForUDP`, UDP msg types |
| `cmd/client/ws.go` | `connectWS`, `waitForWS`, WebSocket msg types |
| `cmd/client/http.go` | Unchanged — all HTTP helpers |

**Deleted (replaced):** old `main.go`, `auth.go`, `manga.go`, `tcp.go`, `udp.go`, `ws.go`

**New dependencies:**
```
github.com/charmbracelet/bubbletea
github.com/charmbracelet/bubbles
github.com/charmbracelet/lipgloss
```

---

## 9. Out of scope

- Mouse support
- Keyboard shortcut help overlay
- Notification history / log
- Manga detail image rendering
- gRPC client (unchanged — not used by client)
