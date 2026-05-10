# TUI Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Redesign the BubbleTea TUI client with a modal/overlay system, split-pane Search and Library views, a home dashboard, and proper loading/empty states.

**Architecture:** Modal infrastructure is built first (Task 1–2) so all subsequent features use it from day one. The flat `Model` struct is extended with new fields. The sequential search state machine (`searchState` enum) is removed and replaced with a split-pane layout. Global `libraryResultMsg` routing in `Update` feeds both the library view and the dashboard simultaneously.

**Tech Stack:** Go, `github.com/charmbracelet/bubbletea` v1.3.10, `github.com/charmbracelet/bubbles` v1.0.0 (textinput, viewport, spinner), `github.com/charmbracelet/lipgloss` v1.1.0, `github.com/stretchr/testify`

---

## File Map

| File | Action | Responsibility |
|---|---|---|
| `cmd/client/model.go` | Modify | New enums/fields, global Update routing, notification history, spinner init |
| `cmd/client/http.go` | Modify | Add `deleteJSON` helper |
| `cmd/client/view_modal.go` | **Create** | `updateModal`, `renderModal`, all modal open helpers, `cmdRemoveFromLibrary` |
| `cmd/client/view_search.go` | Modify | Split-pane layout, remove `searchState`, auto detail-fetch on cursor move |
| `cmd/client/view_library.go` | Modify | Split-pane layout, `libraryLoading`, modal wiring for update/remove |
| `cmd/client/view_menu.go` | Modify | Dashboard rendering, sidebar Logout opens confirm modal |
| `cmd/client/view_auth.go` | Modify | `loginSuccessMsg` also dispatches `cmdFetchLibrary` |
| `cmd/client/model_test.go` | Modify | Update notif tests, remove stale `m.notification` references |
| `cmd/client/view_modal_test.go` | **Create** | Modal open/close/submit state tests |
| `cmd/client/view_search_test.go` | Modify | Rewrite for split-pane model, remove `searchState` references |
| `cmd/client/view_library_test.go` | Modify | Arrow key nav, modal wiring, loading state |
| `cmd/client/view_menu_test.go` | Modify | Dashboard population, arrow key nav |

---

## Task 1: Model Foundations

Add new enums and fields, replace `notification string` with `notifications []string`, add `spinner.Model`, remove `searchState` enum, add `removeLibraryMsg` type, add `deleteJSON` to http.go. Update existing tests that reference removed/changed fields.

**Files:**
- Modify: `cmd/client/model.go`
- Modify: `cmd/client/http.go`
- Modify: `cmd/client/model_test.go`
- Modify: `cmd/client/view_search_test.go`
- Modify: `cmd/client/view_library_test.go`
- Modify: `cmd/client/view_menu_test.go`

- [ ] **Step 1: Write failing tests for notification history**

Replace the two existing notif tests in `cmd/client/model_test.go` and add a cap test. Add `"fmt"` to imports.

```go
func TestTCPNotifAppendsToHistory(t *testing.T) {
	m := New("http://localhost:8080")
	next, _ := m.Update(tcpNotifMsg{text: "update1"})
	m2 := next.(Model)
	assert.Equal(t, []string{"update1"}, m2.notifications)
}

func TestNotifHistoryCappedAt20(t *testing.T) {
	m := New("http://localhost:8080")
	for i := range 20 {
		m.notifications = append([]string{fmt.Sprintf("msg%d", i)}, m.notifications...)
	}
	next, _ := m.Update(tcpNotifMsg{text: "new"})
	m2 := next.(Model)
	assert.Len(t, m2.notifications, 20)
	assert.Equal(t, "new", m2.notifications[0])
}

func TestUDPNotifAppendsToHistory(t *testing.T) {
	m := New("http://localhost:8080")
	next, _ := m.Update(udpNotifMsg{text: "chapter"})
	m2 := next.(Model)
	assert.Equal(t, []string{"chapter"}, m2.notifications)
}
```

- [ ] **Step 2: Run tests to confirm failures**

```
cd cmd/client && go test -run "TestTCPNotif|TestUDPNotif|TestNotifHistory" -v
```

Expected: compile errors — `m.notification` field no longer exists after Step 3.

- [ ] **Step 3: Update model.go — enums, fields, notification history, spinner**

**Add** new enums after the `view` enum block:

```go
type modalType int

const (
	modalNone modalType = iota
	modalJoinChat
	modalUpdateProgress
	modalConfirmAction
	modalError
	modalHelp
	modalNotifications
)

type confirmAction int

const (
	confirmLogout confirmAction = iota
	confirmRemoveManga
)
```

**Remove** the `searchState` enum and its three constants entirely:

```go
// DELETE:
type searchState int
const (
	searchStateForm searchState = iota
	searchStateResults
	searchStateDetail
)
```

**Add** `removeLibraryMsg` to the msg types block:

```go
type removeLibraryMsg struct{ err string }
```

**Replace** the entire `Model` struct with:

```go
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

	// notification history (newest first, max 20)
	notifications []string

	// terminal dimensions
	width, height int

	// auth forms
	authInputs []textinput.Model
	authFocus  int
	authErr    string

	// search view (searchState enum removed)
	searchInputFocused bool
	searchFocusPane    int
	detailPending      string
	searchInputs       []textinput.Model
	searchFocus        int
	searchResults      []mangaItem
	searchCursor       int
	searchPage         int
	searchTotal        int
	detailManga        mangaItem
	detailEntry        *libraryItem
	detailFocus        int

	// loading states
	searchLoading  bool
	detailLoading  bool
	libraryLoading bool
	spinner        spinner.Model

	// library view
	libraryGroups map[string][]libraryItem
	libraryFlat   []libraryItem
	libraryCursor int

	// dashboard (top 3 reading items, populated at login)
	dashboardReading []libraryItem

	// modal / overlay system
	activeModal       modalType
	modalInput        textinput.Model
	modalInputFocused bool
	modalCursor       int
	modalMessage      string
	modalConfirmAct   confirmAction
	modalIsAdding     bool

	// chat view
	chatMangaID     string
	chatMessages    []chatMessage
	chatInput       textinput.Model
	chatViewport    viewport.Model
	chatConn        *websocket.Conn
	chatPrompting   bool
	chatPromptInput textinput.Model
}
```

**Add** `"github.com/charmbracelet/bubbles/spinner"` to imports.

**Update** `New()`:

```go
func New(baseURL string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{
		baseURL:     baseURL,
		currentView: viewMenu,
		sidebarIdx:  0,
		spinner:     s,
	}
}
```

**Replace** the `tcpNotifMsg` and `udpNotifMsg` cases in `Update()`:

```go
case tcpNotifMsg:
	if msg.text != "" {
		m.notifications = append([]string{msg.text}, m.notifications...)
		if len(m.notifications) > 20 {
			m.notifications = m.notifications[:20]
		}
	}
	if m.tcpConn != nil {
		return m, waitForTCP(m.tcpConn)
	}
	return m, nil

case udpNotifMsg:
	if msg.text != "" {
		m.notifications = append([]string{msg.text}, m.notifications...)
		if len(m.notifications) > 20 {
			m.notifications = m.notifications[:20]
		}
	}
	if m.udpConn != nil {
		return m, waitForUDP(m.udpConn)
	}
	return m, nil
```

**Add** spinner tick handling in `Update()` before the view switch:

```go
case spinner.TickMsg:
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
```

**Update** `renderFooter` to use `notifications[0]`:

```go
func renderFooter(m Model) string {
	text := ""
	if len(m.notifications) > 0 {
		text = "  " + m.notifications[0]
	}
	return styleNotif.Width(m.width).Render(text)
}
```

- [ ] **Step 4: Add deleteJSON to http.go**

```go
// deleteJSON sends a DELETE request to url with optional Bearer token and decodes the response.
func deleteJSON(url, token string, dest interface{}) (int, error) {
	req, err := http.NewRequest(http.MethodDelete, url, nil)
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
```

- [ ] **Step 5: Fix broken test references**

In `cmd/client/view_search_test.go`:
- Remove all `searchState` field references (`m.searchState`, `searchStateForm`, `searchStateResults`, `searchStateDetail`)
- Update `newSearchModel()`:

```go
func newSearchModel() Model {
	m := New("http://localhost:8080")
	m.currentView = viewSearch
	m.searchInputs = initSearchInputs()
	m.width, m.height = 120, 40
	return m
}
```

- Delete `TestSearchResultsMsgSwitchesToResults`, `TestSearchDetailMsgSwitchesToDetail` — these will be rewritten in Task 3.
- Keep `TestTotalPages` and `TestSearchPaginationNext`, `TestSearchPaginationPrevOnFirstPage` — update them to not reference `searchState`:

```go
func TestSearchPaginationNext(t *testing.T) {
	m := newSearchModel()
	m.searchResults = make([]mangaItem, 20)
	m.searchPage = 1
	m.searchTotal = 50

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m2 := next.(Model)
	assert.Equal(t, 2, m2.searchPage)
	assert.NotNil(t, cmd)
}

func TestSearchPaginationPrevOnFirstPage(t *testing.T) {
	m := newSearchModel()
	m.searchPage = 1
	m.searchTotal = 50

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m2 := next.(Model)
	assert.Equal(t, 1, m2.searchPage)
	assert.Nil(t, cmd)
}

func TestTotalPages(t *testing.T) {
	assert.Equal(t, 1, totalPages(0, 20))
	assert.Equal(t, 1, totalPages(20, 20))
	assert.Equal(t, 2, totalPages(21, 20))
	assert.Equal(t, 3, totalPages(50, 20))
}
```

In `cmd/client/view_library_test.go`, update `TestLibraryNavDown` to use arrow key:

```go
func TestLibraryNavDown(t *testing.T) {
	m := New("http://localhost:8080")
	m.currentView = viewLibrary
	m.libraryFlat = []libraryItem{
		{MangaID: "one-piece"},
		{MangaID: "naruto"},
	}
	m.libraryCursor = 0

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m2 := next.(Model)
	assert.Equal(t, 1, m2.libraryCursor)
}
```

In `cmd/client/view_menu_test.go`, update `TestMenuNavDown` to use arrow key:

```go
func TestMenuNavDown(t *testing.T) {
	m := New("http://localhost:8080")
	m.sidebarIdx = 0
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m2 := next.(Model)
	assert.Equal(t, 1, m2.sidebarIdx)
}
```

Also update the `updateMenu` function in `cmd/client/view_menu.go` to use arrow keys (so the test passes). Replace `"up", "k"` and `"down", "j"` with arrow-only cases, and remove the `"1", "2", "3", "4"` number shortcuts:

```go
func updateMenu(m Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		items := sidebarItems(m)
		switch msg.String() {
		case "up":
			if m.sidebarIdx > 0 {
				m.sidebarIdx--
			}
		case "down":
			if m.sidebarIdx < len(items)-1 {
				m.sidebarIdx++
			}
		case "enter":
			return activateSidebarItem(m)
		}
	}
	return m, nil
}
```

- [ ] **Step 6: Run all tests**

```
cd cmd/client && go test ./... -v
```

Expected: all tests pass. Fix any remaining compile errors (e.g. `m.notification` references in view files — replace with `len(m.notifications) > 0` checks).

- [ ] **Step 7: Commit**

```bash
git add cmd/client/model.go cmd/client/http.go cmd/client/model_test.go cmd/client/view_search_test.go cmd/client/view_library_test.go cmd/client/view_menu_test.go
git commit -m "refactor(client): add modal/spinner fields, notification history, deleteJSON, remove searchState"
```

---

## Task 2: Modal Infrastructure

Create `view_modal.go` with all modal types. Wire global key routing and modal interception into `model.go`. Update sidebar Logout to open confirm modal.

**Files:**
- Create: `cmd/client/view_modal.go`
- Create: `cmd/client/view_modal_test.go`
- Modify: `cmd/client/model.go`
- Modify: `cmd/client/view_menu.go`

- [ ] **Step 1: Write failing tests for modal open/close**

Create `cmd/client/view_modal_test.go`:

```go
package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestHelpKeyOpensHelpModal(t *testing.T) {
	m := New("http://localhost:8080")
	m.width, m.height = 120, 40
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m2 := next.(Model)
	assert.Equal(t, modalHelp, m2.activeModal)
}

func TestNotifKeyOpensNotifModal(t *testing.T) {
	m := New("http://localhost:8080")
	m.width, m.height = 120, 40
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m2 := next.(Model)
	assert.Equal(t, modalNotifications, m2.activeModal)
}

func TestEscClosesModal(t *testing.T) {
	m := New("http://localhost:8080")
	m.width, m.height = 120, 40
	m.activeModal = modalHelp
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m2 := next.(Model)
	assert.Equal(t, modalNone, m2.activeModal)
}

func TestCKeyOpensJoinChatModalWhenLoggedIn(t *testing.T) {
	m := New("http://localhost:8080")
	m.width, m.height = 120, 40
	m.token = "tok"
	m.username = "alice"
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m2 := next.(Model)
	assert.Equal(t, modalJoinChat, m2.activeModal)
}

func TestCKeyDoesNotOpenChatWhenGuest(t *testing.T) {
	m := New("http://localhost:8080")
	m.width, m.height = 120, 40
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m2 := next.(Model)
	assert.Equal(t, modalNone, m2.activeModal)
}

func TestModalInterceptsEscWithoutChangingView(t *testing.T) {
	m := New("http://localhost:8080")
	m.width, m.height = 120, 40
	m.currentView = viewSearch
	m.activeModal = modalHelp
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m2 := next.(Model)
	assert.Equal(t, modalNone, m2.activeModal)
	assert.Equal(t, viewSearch, m2.currentView) // view unchanged
}

func TestConfirmYConfirmsLogout(t *testing.T) {
	m := New("http://localhost:8080")
	m.width, m.height = 120, 40
	m.token = "tok"
	m.username = "alice"
	m.activeModal = modalConfirmAction
	m.modalConfirmAct = confirmLogout
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m2 := next.(Model)
	assert.Equal(t, modalNone, m2.activeModal)
	assert.Empty(t, m2.token)
	assert.Equal(t, viewMenu, m2.currentView)
}

func TestConfirmNKeepModal(t *testing.T) {
	m := New("http://localhost:8080")
	m.width, m.height = 120, 40
	m.token = "tok"
	m.activeModal = modalConfirmAction
	m.modalConfirmAct = confirmLogout
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m2 := next.(Model)
	assert.Equal(t, modalNone, m2.activeModal)
	assert.Equal(t, "tok", m2.token) // not logged out
}
```

- [ ] **Step 2: Run to verify failures**

```
cd cmd/client && go test -run "TestHelpKey|TestNotifKey|TestEscCloses|TestCKey|TestModal|TestConfirm" -v
```

Expected: compile errors — `updateModal`, `renderModal`, `openModalJoinChat` not defined yet.

- [ ] **Step 3: Create view_modal.go**

Create `cmd/client/view_modal.go`:

```go
package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var modalStatusOptions = []string{"reading", "plan_to_read", "completed", "on_hold", "dropped"}

// --- Open helpers ---

func openModalJoinChat(m Model) Model {
	inp := textinput.New()
	inp.Placeholder = "manga ID or name (blank = general)"
	inp.Focus()
	inp.Width = 38
	m.activeModal = modalJoinChat
	m.modalInput = inp
	return m
}

func openModalUpdateProgress(m Model, isAdding bool, currentChapter int, currentStatus string) Model {
	inp := textinput.New()
	inp.Placeholder = "chapter number"
	if currentChapter > 0 {
		inp.SetValue(fmt.Sprintf("%d", currentChapter))
	}
	inp.Focus()
	inp.Width = 20

	cursor := 0
	for i, s := range modalStatusOptions {
		if s == currentStatus {
			cursor = i
			break
		}
	}

	m.activeModal = modalUpdateProgress
	m.modalInput = inp
	m.modalInputFocused = true
	m.modalCursor = cursor
	m.modalIsAdding = isAdding
	return m
}

func openModalConfirm(m Model, act confirmAction, message string) Model {
	m.activeModal = modalConfirmAction
	m.modalConfirmAct = act
	m.modalMessage = message
	return m
}

// --- Commands ---

func cmdRemoveFromLibrary(baseURL, token, mangaID string) tea.Cmd {
	return func() tea.Msg {
		var resp apiError
		code, err := deleteJSON(baseURL+"/users/library/"+mangaID, token, &resp)
		if err != nil {
			return removeLibraryMsg{err: err.Error()}
		}
		if code != 200 {
			return removeLibraryMsg{err: resp.Error}
		}
		return removeLibraryMsg{}
	}
}

// --- Update ---

func updateModal(m Model, msg tea.Msg) (Model, tea.Cmd) {
	keyMsg, isKey := msg.(tea.KeyMsg)
	if !isKey {
		// Propagate non-key msgs to text input when focused
		if m.activeModal == modalJoinChat || (m.activeModal == modalUpdateProgress && m.modalInputFocused) {
			var cmd tea.Cmd
			m.modalInput, cmd = m.modalInput.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	switch m.activeModal {
	case modalHelp, modalNotifications:
		if keyMsg.String() == "esc" {
			m.activeModal = modalNone
		}
		return m, nil

	case modalError:
		switch keyMsg.String() {
		case "esc", "r":
			m.activeModal = modalNone
			m.modalMessage = ""
		}
		return m, nil

	case modalConfirmAction:
		switch keyMsg.String() {
		case "y", "Y":
			return confirmModalAction(m)
		case "n", "N", "esc":
			m.activeModal = modalNone
		}
		return m, nil

	case modalJoinChat:
		switch keyMsg.String() {
		case "esc":
			m.activeModal = modalNone
			return m, nil
		case "enter":
			mangaID := strings.TrimSpace(m.modalInput.Value())
			if mangaID == "" {
				mangaID = "general"
			}
			m.activeModal = modalNone
			m.chatMangaID = mangaID
			m.chatPrompting = false
			m.chatMessages = nil
			m.chatInput = newChatInput()
			m.chatViewport = newChatViewport(m.width, m.height-4)
			m.currentView = viewChat
			return m, cmdConnectWS(m.baseURL, m.token, mangaID)
		default:
			var cmd tea.Cmd
			m.modalInput, cmd = m.modalInput.Update(msg)
			return m, cmd
		}

	case modalUpdateProgress:
		return updateModalProgress(m, keyMsg)
	}
	return m, nil
}

func updateModalProgress(m Model, msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.activeModal = modalNone
		return m, nil
	case "tab":
		m.modalInputFocused = !m.modalInputFocused
		if m.modalInputFocused {
			m.modalInput.Focus()
		} else {
			m.modalInput.Blur()
		}
		return m, textinput.Blink
	case "up":
		if !m.modalInputFocused && m.modalCursor > 0 {
			m.modalCursor--
		}
		return m, nil
	case "down":
		if !m.modalInputFocused && m.modalCursor < len(modalStatusOptions)-1 {
			m.modalCursor++
		}
		return m, nil
	case "enter":
		return submitModalProgress(m)
	default:
		if m.modalInputFocused {
			var cmd tea.Cmd
			m.modalInput, cmd = m.modalInput.Update(msg)
			return m, cmd
		}
		return m, nil
	}
}

func submitModalProgress(m Model) (Model, tea.Cmd) {
	chapter := 0
	fmt.Sscanf(strings.TrimSpace(m.modalInput.Value()), "%d", &chapter)
	status := modalStatusOptions[m.modalCursor]
	m.activeModal = modalNone
	if m.modalIsAdding {
		return m, cmdAddToLibrary(m.baseURL, m.token, m.detailManga.ID, status, chapter)
	}
	mangaID := m.detailManga.ID
	if len(m.libraryFlat) > m.libraryCursor {
		mangaID = m.libraryFlat[m.libraryCursor].MangaID
	}
	return m, cmdUpdateProgress(m.baseURL, m.token, mangaID, status, chapter)
}

func confirmModalAction(m Model) (Model, tea.Cmd) {
	m.activeModal = modalNone
	switch m.modalConfirmAct {
	case confirmLogout:
		if m.tcpConn != nil {
			m.tcpConn.Close()
			m.tcpConn = nil
		}
		if m.udpConn != nil {
			m.udpConn.Close()
			m.udpConn = nil
		}
		if m.chatConn != nil {
			m.chatConn.Close()
			m.chatConn = nil
		}
		m.token = ""
		m.userID = ""
		m.username = ""
		m.sidebarIdx = 0
		m.currentView = viewMenu
		m.dashboardReading = nil
		m.notifications = nil
	case confirmRemoveManga:
		mangaID := m.modalMessage
		m.modalMessage = ""
		return m, cmdRemoveFromLibrary(m.baseURL, m.token, mangaID)
	}
	return m, nil
}

// --- Render ---

func renderModal(m Model) string {
	switch m.activeModal {
	case modalHelp:
		return renderModalHelp()
	case modalNotifications:
		return renderModalNotifications(m)
	case modalError:
		return renderModalError(m)
	case modalConfirmAction:
		return renderModalConfirm(m)
	case modalJoinChat:
		return renderModalJoinChat(m)
	case modalUpdateProgress:
		return renderModalUpdateProgress(m)
	}
	return ""
}

func modalBox(title, content string) string {
	inner := lipgloss.NewStyle().Width(42).Padding(0, 1).Render(content)
	return styleBorderBox.Width(44).Render(
		styleTitle.Render("  "+title) + "\n\n" + inner,
	)
}

func renderModalHelp() string {
	lines := []string{
		styleMutedText.Render("Global"),
		"  ?    This help screen",
		"  n    Notification center",
		"  c    Join chat room",
		"  Esc  Back / close",
		"  q    Quit",
		"",
		styleMutedText.Render("Search"),
		"  /    Focus search input",
		"  ↑↓   Navigate results",
		"  ←→   Previous / next page",
		"  a    Add to library / update progress",
		"",
		styleMutedText.Render("Library"),
		"  ↑↓   Navigate",
		"  a    Update progress",
		"  d    Remove from library",
		"",
		styleMutedText.Render("Chat"),
		"  Enter   Send message",
		"  /exit   Leave room",
	}
	return modalBox("Keybindings", strings.Join(lines, "\n"))
}

func renderModalNotifications(m Model) string {
	if len(m.notifications) == 0 {
		return modalBox("Notifications", styleMutedText.Render("No notifications yet."))
	}
	var lines []string
	for _, n := range m.notifications {
		lines = append(lines, "• "+truncate(n, 38))
	}
	return modalBox("Notifications", strings.Join(lines, "\n"))
}

func renderModalError(m Model) string {
	content := styleError.Render(truncate(m.modalMessage, 40)) + "\n\n" +
		styleMutedText.Render("Esc  Dismiss")
	return modalBox("Error", content)
}

func renderModalConfirm(m Model) string {
	action := "logout"
	if m.modalConfirmAct == confirmRemoveManga {
		action = "remove this manga from your library"
	}
	content := fmt.Sprintf("Are you sure you want to %s?\n\n", action) +
		styleMutedText.Render("y  Yes    n / Esc  No")
	return modalBox("Confirm", content)
}

func renderModalJoinChat(m Model) string {
	content := styleMutedText.Render("Manga ID (blank = general):") + "\n" +
		m.modalInput.View() + "\n\n" +
		styleMutedText.Render("Enter to join    Esc to cancel")
	return modalBox("Join Chat Room", content)
}

func renderModalUpdateProgress(m Model) string {
	title := "Update Progress"
	if m.modalIsAdding {
		title = "Add to Library"
	}

	chapterLabel := styleMutedText.Render("Chapter: ")
	if m.modalInputFocused {
		chapterLabel = styleTitle.Render("Chapter: ")
	}
	chapterLine := chapterLabel + m.modalInput.View()

	var statusLines []string
	for i, s := range modalStatusOptions {
		label := strings.ReplaceAll(s, "_", " ")
		switch {
		case i == m.modalCursor && !m.modalInputFocused:
			statusLines = append(statusLines, styleSidebarSelected.Render("> "+label))
		case i == m.modalCursor:
			statusLines = append(statusLines, styleNormal.Render("  "+label)+" ◀")
		default:
			statusLines = append(statusLines, styleNormal.Render("  "+label))
		}
	}

	content := chapterLine + "\n\n" +
		styleMutedText.Render("Status:") + "\n" +
		strings.Join(statusLines, "\n") + "\n\n" +
		styleMutedText.Render("Tab switch    Enter save    Esc cancel")
	return modalBox(title, content)
}
```

- [ ] **Step 4: Wire modal routing into model.go Update()**

In `Update()`, replace the `tea.KeyMsg` handling. The new structure intercepts global keys first, then routes to modal if one is open, otherwise falls through to the view switch:

```go
case tea.KeyMsg:
	if msg.String() == "ctrl+c" {
		closeConns(m)
		return m, tea.Quit
	}

	// Global keys — intercept before modal and view routing
	if m.activeModal == modalNone {
		switch msg.String() {
		case "q":
			closeConns(m)
			return m, tea.Quit
		case "?":
			m.activeModal = modalHelp
			return m, nil
		case "n":
			m.activeModal = modalNotifications
			return m, nil
		case "c":
			if m.token != "" && m.currentView != viewChat {
				m = openModalJoinChat(m)
				return m, textinput.Blink
			}
		}
	}

	// Modal intercepts all remaining keys when open
	if m.activeModal != modalNone {
		var cmd tea.Cmd
		m, cmd = updateModal(m, msg)
		return m, cmd
	}
```

Also add `removeLibraryMsg` handling in the global switch (before the view switch), and move `libraryResultMsg` handling to global scope:

```go
case removeLibraryMsg:
	if msg.err != "" {
		m.notifications = append([]string{"Remove failed: " + msg.err}, m.notifications...)
		return m, nil
	}
	m.notifications = append([]string{"Removed from library."}, m.notifications...)
	if m.currentView == viewLibrary {
		m.libraryLoading = true
		return m, tea.Batch(cmdFetchLibrary(m.baseURL, m.token), m.spinner.Tick)
	}
	return m, nil

case libraryResultMsg:
	if msg.err != "" {
		m.notifications = append([]string{"Library error: " + msg.err}, m.notifications...)
		m.libraryLoading = false
		return m, nil
	}
	// Always update dashboard reading list
	reading := msg.groups["reading"]
	if len(reading) > 3 {
		reading = reading[:3]
	}
	m.dashboardReading = reading
	// If on library view, also populate full library state
	if m.currentView == viewLibrary {
		m.libraryGroups = msg.groups
		m.libraryFlat = flattenLibrary(msg.groups)
		m.libraryCursor = 0
		m.libraryLoading = false
	}
	return m, nil
```

- [ ] **Step 5: Wire modal rendering into renderContent() and renderChatScreen()**

In `model.go`, update `renderContent()`:

```go
func renderContent(m Model, width, height int) string {
	if m.activeModal != modalNone {
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, renderModal(m))
	}
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
```

Update `renderChatScreen()` in `view_chat.go`:

```go
func renderChatScreen(m Model) string {
	if m.activeModal != modalNone {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, renderModal(m))
	}
	// ... existing chat rendering unchanged
```

- [ ] **Step 6: Update sidebar Logout in view_menu.go to open confirm modal**

In `activateSidebarItem`, replace the `"Logout"` case:

```go
case "Logout":
	m = openModalConfirm(m, confirmLogout, "")
	return m, nil
```

- [ ] **Step 7: Run tests**

```
cd cmd/client && go test ./... -v
```

Expected: all tests pass including the new modal tests.

- [ ] **Step 8: Commit**

```bash
git add cmd/client/view_modal.go cmd/client/view_modal_test.go cmd/client/model.go cmd/client/view_menu.go cmd/client/view_chat.go
git commit -m "feat(client): modal/overlay infrastructure — help, notifications, join-chat, confirm, update-progress"
```

---

## Task 3: Split-pane Search

Rewrite `view_search.go` to remove the 3-state flow and implement a split-pane layout with auto detail-fetch on cursor move.

**Files:**
- Modify: `cmd/client/view_search.go`
- Modify: `cmd/client/view_search_test.go`

- [ ] **Step 1: Write failing tests**

Replace the contents of `cmd/client/view_search_test.go` with:

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
	m.searchInputs = initSearchInputs()
	m.width, m.height = 120, 40
	return m
}

func TestSearchResultsMsgSetsResults(t *testing.T) {
	m := newSearchModel()
	results := []mangaItem{
		{ID: "one-piece", Title: "One Piece", Author: "Oda"},
		{ID: "naruto", Title: "Naruto", Author: "Kishimoto"},
	}
	next, _ := m.Update(searchResultMsg{results: results, total: 2, page: 1})
	m2 := next.(Model)
	assert.Len(t, m2.searchResults, 2)
	assert.Equal(t, 0, m2.searchCursor)
	assert.Equal(t, 1, m2.searchPage)
	assert.Equal(t, 2, m2.searchTotal)
	assert.False(t, m2.searchLoading)
}

func TestSearchCursorMovesDown(t *testing.T) {
	m := newSearchModel()
	m.searchResults = []mangaItem{
		{ID: "one-piece", Title: "One Piece"},
		{ID: "naruto", Title: "Naruto"},
	}
	m.searchCursor = 0

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m2 := next.(Model)
	assert.Equal(t, 1, m2.searchCursor)
	assert.NotNil(t, cmd) // fires cmdFetchDetail
	assert.Equal(t, "naruto", m2.detailPending)
}

func TestSearchCursorDoesNotGoAboveZero(t *testing.T) {
	m := newSearchModel()
	m.searchResults = []mangaItem{{ID: "one-piece"}}
	m.searchCursor = 0

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m2 := next.(Model)
	assert.Equal(t, 0, m2.searchCursor)
}

func TestDetailResultIgnoredWhenStale(t *testing.T) {
	m := newSearchModel()
	m.detailPending = "naruto"
	m.detailManga = mangaItem{ID: "old"}

	next, _ := m.Update(detailResultMsg{manga: mangaItem{ID: "one-piece"}})
	m2 := next.(Model)
	assert.Equal(t, "old", m2.detailManga.ID)
}

func TestDetailResultAcceptedWhenCurrent(t *testing.T) {
	m := newSearchModel()
	m.detailPending = "one-piece"

	next, _ := m.Update(detailResultMsg{manga: mangaItem{ID: "one-piece", Title: "One Piece"}})
	m2 := next.(Model)
	assert.Equal(t, "one-piece", m2.detailManga.ID)
	assert.False(t, m2.detailLoading)
}

func TestSearchSlashFocusesInput(t *testing.T) {
	m := newSearchModel()
	m.searchInputFocused = false

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m2 := next.(Model)
	assert.True(t, m2.searchInputFocused)
}

func TestSearchAKeyOpensAddModalWhenNotInLibrary(t *testing.T) {
	m := newSearchModel()
	m.token = "tok"
	m.detailManga = mangaItem{ID: "one-piece"}
	m.detailEntry = nil

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m2 := next.(Model)
	assert.Equal(t, modalUpdateProgress, m2.activeModal)
	assert.True(t, m2.modalIsAdding)
}

func TestSearchAKeyOpensUpdateModalWhenInLibrary(t *testing.T) {
	m := newSearchModel()
	m.token = "tok"
	m.detailManga = mangaItem{ID: "one-piece"}
	m.detailEntry = &libraryItem{CurrentChapter: 100, Status: "reading"}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m2 := next.(Model)
	assert.Equal(t, modalUpdateProgress, m2.activeModal)
	assert.False(t, m2.modalIsAdding)
}

func TestSearchPaginationNext(t *testing.T) {
	m := newSearchModel()
	m.searchResults = make([]mangaItem, 20)
	m.searchPage = 1
	m.searchTotal = 50

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m2 := next.(Model)
	assert.Equal(t, 2, m2.searchPage)
	assert.NotNil(t, cmd)
}

func TestSearchPaginationPrevOnFirstPage(t *testing.T) {
	m := newSearchModel()
	m.searchPage = 1
	m.searchTotal = 50

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m2 := next.(Model)
	assert.Equal(t, 1, m2.searchPage)
	assert.Nil(t, cmd)
}

func TestTotalPages(t *testing.T) {
	assert.Equal(t, 1, totalPages(0, 20))
	assert.Equal(t, 1, totalPages(20, 20))
	assert.Equal(t, 2, totalPages(21, 20))
	assert.Equal(t, 3, totalPages(50, 20))
}
```

- [ ] **Step 2: Run to verify failures**

```
cd cmd/client && go test -run "TestSearch|TestDetail" -v
```

Expected: FAIL — cursor navigation, stale-response check, `a` key behavior not yet implemented.

- [ ] **Step 3: Rewrite view_search.go**

Replace the entire file (keep package declaration and imports, rewrite all functions):

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
			m.notifications = append([]string{"Search error: " + msg.err}, m.notifications...)
			if len(m.notifications) > 20 {
				m.notifications = m.notifications[:20]
			}
			m.searchLoading = false
			return m, nil
		}
		m.searchResults = msg.results
		m.searchTotal = msg.total
		m.searchPage = msg.page
		m.searchCursor = 0
		m.searchLoading = false
		if len(msg.results) > 0 {
			id := msg.results[0].ID
			m.detailPending = id
			m.detailLoading = true
			return m, tea.Batch(cmdFetchDetail(m.baseURL, m.token, id), m.spinner.Tick)
		}
		return m, nil

	case detailResultMsg:
		if msg.manga.ID != m.detailPending {
			return m, nil // stale response — ignore
		}
		if msg.err != "" {
			m.detailLoading = false
			return m, nil
		}
		m.detailManga = msg.manga
		m.detailEntry = msg.entry
		m.detailLoading = false
		return m, nil

	case addLibraryMsg:
		if msg.err != "" {
			m.notifications = append([]string{"Add failed: " + msg.err}, m.notifications...)
		} else {
			m.notifications = append([]string{fmt.Sprintf("Added %q to library.", m.detailManga.Title)}, m.notifications...)
			return m, cmdFetchDetail(m.baseURL, m.token, m.detailManga.ID)
		}
		if len(m.notifications) > 20 {
			m.notifications = m.notifications[:20]
		}
		return m, nil

	case updateProgressMsg:
		if msg.err != "" {
			m.notifications = append([]string{"Update failed: " + msg.err}, m.notifications...)
		} else {
			m.notifications = append([]string{fmt.Sprintf("Progress updated for %q.", m.detailManga.Title)}, m.notifications...)
			return m, cmdFetchDetail(m.baseURL, m.token, m.detailManga.ID)
		}
		if len(m.notifications) > 20 {
			m.notifications = m.notifications[:20]
		}
		return m, nil

	case tea.KeyMsg:
		return updateSearchKeys(m, msg)
	}

	if m.searchInputFocused && len(m.searchInputs) > 0 {
		var cmd tea.Cmd
		m.searchInputs[m.searchFocus], cmd = m.searchInputs[m.searchFocus].Update(msg)
		return m, cmd
	}
	return m, nil
}

func updateSearchKeys(m Model, msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.searchInputFocused {
		switch msg.String() {
		case "esc":
			m.searchInputs[m.searchFocus].Blur()
			m.searchInputFocused = false
			return m, nil
		case "enter":
			m.searchInputFocused = false
			m.searchInputs[m.searchFocus].Blur()
			q := strings.TrimSpace(m.searchInputs[0].Value())
			genre := strings.TrimSpace(m.searchInputs[1].Value())
			status := strings.TrimSpace(m.searchInputs[2].Value())
			m.searchPage = 1
			m.searchLoading = true
			return m, tea.Batch(cmdSearch(m.baseURL, m.token, q, genre, status, 1), m.spinner.Tick)
		default:
			var cmd tea.Cmd
			m.searchInputs[m.searchFocus], cmd = m.searchInputs[m.searchFocus].Update(msg)
			return m, cmd
		}
	}

	switch msg.String() {
	case "esc":
		m.currentView = viewMenu
	case "/":
		m.searchInputFocused = true
		m.searchInputs[m.searchFocus].Focus()
		return m, textinput.Blink
	case "up":
		if m.searchCursor > 0 {
			m.searchCursor--
			id := m.searchResults[m.searchCursor].ID
			m.detailPending = id
			m.detailLoading = true
			return m, tea.Batch(cmdFetchDetail(m.baseURL, m.token, id), m.spinner.Tick)
		}
	case "down":
		if m.searchCursor < len(m.searchResults)-1 {
			m.searchCursor++
			id := m.searchResults[m.searchCursor].ID
			m.detailPending = id
			m.detailLoading = true
			return m, tea.Batch(cmdFetchDetail(m.baseURL, m.token, id), m.spinner.Tick)
		}
	case "right":
		pages := totalPages(m.searchTotal, searchPageSize)
		if m.searchPage < pages {
			m.searchPage++
			q := strings.TrimSpace(m.searchInputs[0].Value())
			genre := strings.TrimSpace(m.searchInputs[1].Value())
			status := strings.TrimSpace(m.searchInputs[2].Value())
			m.searchLoading = true
			return m, tea.Batch(cmdSearch(m.baseURL, m.token, q, genre, status, m.searchPage), m.spinner.Tick)
		}
	case "left":
		if m.searchPage > 1 {
			m.searchPage--
			q := strings.TrimSpace(m.searchInputs[0].Value())
			genre := strings.TrimSpace(m.searchInputs[1].Value())
			status := strings.TrimSpace(m.searchInputs[2].Value())
			m.searchLoading = true
			return m, tea.Batch(cmdSearch(m.baseURL, m.token, q, genre, status, m.searchPage), m.spinner.Tick)
		}
	case "a":
		if m.token != "" && m.detailManga.ID != "" {
			isAdding := m.detailEntry == nil
			chapter := 0
			status := "reading"
			if !isAdding && m.detailEntry != nil {
				chapter = m.detailEntry.CurrentChapter
				status = m.detailEntry.Status
			}
			m = openModalUpdateProgress(m, isAdding, chapter, status)
			return m, textinput.Blink
		}
	}
	return m, nil
}

func renderSearch(m Model, width, height int) string {
	leftWidth := width * 38 / 100
	rightWidth := width - leftWidth - 1

	left := lipgloss.NewStyle().Width(leftWidth).Height(height).Render(
		renderSearchLeft(m, leftWidth, height),
	)
	divider := lipgloss.NewStyle().
		Width(1).Height(height).
		BorderLeft(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(colorMuted).
		Render("")
	right := lipgloss.NewStyle().Width(rightWidth).Height(height).Render(
		renderSearchRight(m, rightWidth, height),
	)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, divider, right)
}

func renderSearchLeft(m Model, width, height int) string {
	var sb strings.Builder
	inputPrefix := "  / "
	if m.searchInputFocused {
		inputPrefix = styleTitle.Render("  / ")
	}
	sb.WriteString(inputPrefix + m.searchInputs[0].View() + "\n")
	sb.WriteString(styleMutedText.Render(strings.Repeat("─", width)) + "\n")

	if m.searchLoading {
		sb.WriteString("\n  " + m.spinner.View() + " Searching...\n")
		return sb.String()
	}
	if len(m.searchResults) == 0 {
		sb.WriteString("\n" + styleMutedText.Render("  Search for manga using /") + "\n")
		return sb.String()
	}

	pages := totalPages(m.searchTotal, searchPageSize)
	sb.WriteString(styleMutedText.Render(fmt.Sprintf(
		"  %d result(s) — %d/%d", m.searchTotal, m.searchPage, pages)) + "\n")

	for i, item := range m.searchResults {
		line := "  " + truncate(item.Title, width-4)
		if i == m.searchCursor {
			sb.WriteString(styleSidebarSelected.Width(width).Render(line) + "\n")
		} else {
			sb.WriteString(styleNormal.Render(line) + "\n")
		}
	}
	sb.WriteString("\n" + styleMutedText.Render("  ↑↓ navigate  ←→ page  / search  a add") + "\n")
	return sb.String()
}

func renderSearchRight(m Model, width, height int) string {
	if m.detailLoading {
		return "\n  " + m.spinner.View() + " Loading...\n"
	}
	if m.detailManga.ID == "" {
		return "\n" + styleMutedText.Render("  Select a result to see details")
	}

	d := m.detailManga
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(styleTitle.Render("  "+truncate(d.Title, width-4)) + "\n\n")
	sb.WriteString(styleNormal.Render(fmt.Sprintf("  Author:   %s", d.Author)) + "\n")
	sb.WriteString(styleNormal.Render(fmt.Sprintf("  Status:   %s", d.Status)) + "\n")
	sb.WriteString(styleNormal.Render(fmt.Sprintf("  Genres:   %s", strings.Join(d.Genres, ", "))) + "\n")
	sb.WriteString(styleNormal.Render(fmt.Sprintf("  Chapters: %d", d.TotalChapters)) + "\n")
	if d.Description != "" {
		sb.WriteString("\n" + styleNormal.Render("  "+truncate(d.Description, width-4)) + "\n")
	}
	sb.WriteString("\n")
	if m.token != "" {
		if m.detailEntry != nil {
			sb.WriteString(styleMutedText.Render(fmt.Sprintf(
				"  In library: ch.%d · %s", m.detailEntry.CurrentChapter, m.detailEntry.Status)) + "\n")
			sb.WriteString(styleNormal.Render("  [a] Update Progress") + "\n")
		} else {
			sb.WriteString(styleNormal.Render("  [a] Add to Library") + "\n")
		}
		sb.WriteString(styleNormal.Render("  [c] Join Chat") + "\n")
	}
	return sb.String()
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}
```

- [ ] **Step 4: Run tests**

```
cd cmd/client && go test ./... -v
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add cmd/client/view_search.go cmd/client/view_search_test.go
git commit -m "feat(client): split-pane search with auto detail-fetch and stale-response guard"
```

---

## Task 4: Split-pane Library

Rewrite `view_library.go` to split-pane layout. Remove `libraryResultMsg` case from `updateLibrary` (now handled globally in Task 2). Wire `modalUpdateProgress` and `modalConfirmAction`.

**Files:**
- Modify: `cmd/client/view_library.go`
- Modify: `cmd/client/view_library_test.go`

- [ ] **Step 1: Write failing tests**

Replace the contents of `cmd/client/view_library_test.go`:

```go
package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func newLibraryModel() Model {
	m := New("http://localhost:8080")
	m.currentView = viewLibrary
	m.token = "tok"
	m.width, m.height = 120, 40
	return m
}

func TestLibraryNavDown(t *testing.T) {
	m := newLibraryModel()
	m.libraryFlat = []libraryItem{{MangaID: "a"}, {MangaID: "b"}}
	m.libraryCursor = 0
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m2 := next.(Model)
	assert.Equal(t, 1, m2.libraryCursor)
}

func TestLibraryNavDoesNotGoBelowZero(t *testing.T) {
	m := newLibraryModel()
	m.libraryFlat = []libraryItem{{MangaID: "a"}}
	m.libraryCursor = 0
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m2 := next.(Model)
	assert.Equal(t, 0, m2.libraryCursor)
}

func TestLibraryNavDoesNotExceedLen(t *testing.T) {
	m := newLibraryModel()
	m.libraryFlat = []libraryItem{{MangaID: "a"}}
	m.libraryCursor = 0
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m2 := next.(Model)
	assert.Equal(t, 0, m2.libraryCursor)
}

func TestLibraryAKeyOpensUpdateModal(t *testing.T) {
	m := newLibraryModel()
	m.libraryFlat = []libraryItem{
		{MangaID: "one-piece", CurrentChapter: 100, Status: "reading"},
	}
	m.libraryCursor = 0
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m2 := next.(Model)
	assert.Equal(t, modalUpdateProgress, m2.activeModal)
	assert.False(t, m2.modalIsAdding)
	assert.Equal(t, 0, m2.modalCursor) // "reading" = index 0 in modalStatusOptions
}

func TestLibraryDKeyOpensConfirmModal(t *testing.T) {
	m := newLibraryModel()
	m.libraryFlat = []libraryItem{{MangaID: "one-piece", Title: "One Piece"}}
	m.libraryCursor = 0
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m2 := next.(Model)
	assert.Equal(t, modalConfirmAction, m2.activeModal)
	assert.Equal(t, confirmRemoveManga, m2.modalConfirmAct)
	assert.Equal(t, "one-piece", m2.modalMessage)
}

func TestFlattenLibrary(t *testing.T) {
	groups := map[string][]libraryItem{
		"reading":   {{MangaID: "a"}},
		"completed": {{MangaID: "b"}},
		"on_hold":   {{MangaID: "c"}},
	}
	flat := flattenLibrary(groups)
	assert.Equal(t, "a", flat[0].MangaID)
	assert.Equal(t, "b", flat[1].MangaID)
}
```

- [ ] **Step 2: Run to verify failures**

```
cd cmd/client && go test -run "TestLibrary|TestFlatten" -v
```

Expected: FAIL — `a` and `d` key behaviors not yet implemented.

- [ ] **Step 3: Rewrite view_library.go**

Replace the entire file:

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
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.currentView = viewMenu
		case "up":
			if m.libraryCursor > 0 {
				m.libraryCursor--
			}
		case "down":
			if m.libraryCursor < len(m.libraryFlat)-1 {
				m.libraryCursor++
			}
		case "a":
			if m.libraryCursor < len(m.libraryFlat) {
				item := m.libraryFlat[m.libraryCursor]
				m = openModalUpdateProgress(m, false, item.CurrentChapter, item.Status)
			}
		case "d":
			if m.libraryCursor < len(m.libraryFlat) {
				item := m.libraryFlat[m.libraryCursor]
				m = openModalConfirm(m, confirmRemoveManga, item.MangaID)
			}
		}
	}
	return m, nil
}

func renderLibrary(m Model, width, height int) string {
	leftWidth := width * 38 / 100
	rightWidth := width - leftWidth - 1

	left := lipgloss.NewStyle().Width(leftWidth).Height(height).Render(
		renderLibraryLeft(m, leftWidth, height),
	)
	divider := lipgloss.NewStyle().
		Width(1).Height(height).
		BorderLeft(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(colorMuted).
		Render("")
	right := lipgloss.NewStyle().Width(rightWidth).Height(height).Render(
		renderLibraryRight(m, rightWidth, height),
	)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, divider, right)
}

func renderLibraryLeft(m Model, width, height int) string {
	if m.libraryLoading {
		return "\n  " + m.spinner.View() + " Loading library...\n"
	}
	if m.libraryFlat == nil {
		return "\n" + styleMutedText.Render("  Loading library...")
	}
	if len(m.libraryFlat) == 0 {
		return "\n" + styleNormal.Render("  Your library is empty.\n  Search for manga to add.")
	}

	var sb strings.Builder
	flatIdx := 0
	for _, status := range libraryStatusOrder {
		items := m.libraryGroups[status]
		if len(items) == 0 {
			continue
		}
		label := strings.ToUpper(strings.ReplaceAll(status, "_", " "))
		sb.WriteString("\n  " + styleMutedText.Render(
			fmt.Sprintf("%s (%d)", label, len(items))) + "\n")
		for _, item := range items {
			line := "  " + truncate(item.Title, width-4)
			if flatIdx == m.libraryCursor {
				sb.WriteString(styleSidebarSelected.Width(width).Render(line) + "\n")
			} else {
				sb.WriteString(styleNormal.Render(line) + "\n")
			}
			flatIdx++
		}
	}
	sb.WriteString("\n" + styleMutedText.Render("  ↑↓ navigate  a update  d remove") + "\n")
	return sb.String()
}

func renderLibraryRight(m Model, width, height int) string {
	if len(m.libraryFlat) == 0 || m.libraryCursor >= len(m.libraryFlat) {
		return "\n" + styleMutedText.Render("  Select an item to see details")
	}
	item := m.libraryFlat[m.libraryCursor]
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(styleTitle.Render("  "+truncate(item.Title, width-4)) + "\n\n")
	sb.WriteString(styleNormal.Render(fmt.Sprintf("  Progress:  ch.%d", item.CurrentChapter)) + "\n")
	sb.WriteString(styleNormal.Render(fmt.Sprintf("  Status:    %s",
		strings.ReplaceAll(item.Status, "_", " "))) + "\n")
	if item.UpdatedAt != "" {
		sb.WriteString(styleMutedText.Render("  Updated:   "+item.UpdatedAt) + "\n")
	}
	sb.WriteString("\n")
	sb.WriteString(styleNormal.Render("  [a] Update Progress") + "\n")
	sb.WriteString(styleNormal.Render("  [d] Remove from Library") + "\n")
	return sb.String()
}
```

- [ ] **Step 4: Run tests**

```
cd cmd/client && go test ./... -v
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add cmd/client/view_library.go cmd/client/view_library_test.go
git commit -m "feat(client): split-pane library with update-progress and remove modals"
```

---

## Task 5: Home Dashboard

Fetch library on login to populate `dashboardReading`. Render the dashboard in the menu content panel when logged in.

**Files:**
- Modify: `cmd/client/view_auth.go`
- Modify: `cmd/client/view_menu.go`
- Modify: `cmd/client/view_menu_test.go`

- [ ] **Step 1: Write failing tests**

Add to `cmd/client/view_menu_test.go`:

```go
func TestLoginSuccessFiresLibraryFetch(t *testing.T) {
	m := New("http://localhost:8080")
	m.width, m.height = 120, 40
	_, cmd := m.Update(loginSuccessMsg{token: "tok", userID: "u1", username: "alice"})
	assert.NotNil(t, cmd) // batch: TCP + UDP + library fetch
}

func TestDashboardReadingPopulatedFromLibraryResult(t *testing.T) {
	m := New("http://localhost:8080")
	m.currentView = viewMenu
	m.token = "tok"
	m.width, m.height = 120, 40
	groups := map[string][]libraryItem{
		"reading": {
			{MangaID: "a", Title: "One Piece", CurrentChapter: 1142},
			{MangaID: "b", Title: "Naruto", CurrentChapter: 700},
			{MangaID: "c", Title: "Bleach", CurrentChapter: 686},
			{MangaID: "d", Title: "HxH", CurrentChapter: 400},
		},
	}
	next, _ := m.Update(libraryResultMsg{groups: groups, total: 4})
	m2 := next.(Model)
	assert.Len(t, m2.dashboardReading, 3) // capped at 3
	assert.Equal(t, "One Piece", m2.dashboardReading[0].Title)
}
```

- [ ] **Step 2: Run to verify failures**

```
cd cmd/client && go test -run "TestLoginSuccess|TestDashboard" -v
```

Expected: FAIL — `dashboardReading` not populated from `libraryResultMsg`.

- [ ] **Step 3: Add cmdFetchLibrary to loginSuccessMsg in view_auth.go**

In `updateAuth`, update the `loginSuccessMsg` case to include `cmdFetchLibrary`:

```go
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
		cmdFetchLibrary(m.baseURL, m.token),
	)
```

- [ ] **Step 4: Rewrite renderMenu in view_menu.go**

Replace `renderMenu` with two functions. Also add `"fmt"` and `"strings"` imports if not already present:

```go
func renderMenu(m Model, width, height int) string {
	if m.token == "" {
		return renderMenuGuest(width)
	}
	return renderDashboard(m, width)
}

func renderMenuGuest(width int) string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(styleTitle.Render("  Welcome to MangaHub") + "\n\n")
	sb.WriteString(styleNormal.Render("  Select an option from the menu.") + "\n\n")
	sb.WriteString(styleMutedText.Render("  Search manga · Register · Login") + "\n")
	return lipgloss.NewStyle().Width(width).Render(sb.String())
}

func renderDashboard(m Model, width int) string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(styleTitle.Render("  Welcome back, "+m.username) + "\n\n")

	// Continue Reading
	sb.WriteString(styleNormal.Render("  Continue Reading") + "\n")
	sb.WriteString(styleMutedText.Render("  "+strings.Repeat("─", width-4)) + "\n")
	if len(m.dashboardReading) == 0 {
		sb.WriteString(styleMutedText.Render("  No manga in reading list.") + "\n")
	} else {
		for _, item := range m.dashboardReading {
			chap := fmt.Sprintf("ch.%d", item.CurrentChapter)
			title := truncate(item.Title, width-10-len(chap))
			gap := width - 4 - lipgloss.Width(title) - lipgloss.Width(chap)
			if gap < 1 {
				gap = 1
			}
			sb.WriteString(styleNormal.Render(
				"  "+title+strings.Repeat(" ", gap)+chap) + "\n")
		}
	}
	sb.WriteString("\n")

	// Recent Notifications
	sb.WriteString(styleNormal.Render("  Recent Notifications") + "\n")
	sb.WriteString(styleMutedText.Render("  "+strings.Repeat("─", width-4)) + "\n")
	notifs := m.notifications
	if len(notifs) > 5 {
		notifs = notifs[:5]
	}
	if len(notifs) == 0 {
		sb.WriteString(styleMutedText.Render("  No notifications yet. Press n for history.") + "\n")
	} else {
		for _, n := range notifs {
			sb.WriteString(styleNotif.Render("  • "+truncate(n, width-6)) + "\n")
		}
	}

	return lipgloss.NewStyle().Width(width).Render(sb.String())
}
```

- [ ] **Step 5: Run tests**

```
cd cmd/client && go test ./... -v
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add cmd/client/view_auth.go cmd/client/view_menu.go cmd/client/view_menu_test.go
git commit -m "feat(client): home dashboard with continue-reading and recent notifications"
```

---

## Task 6: Loading States

Wire `libraryLoading = true` and `spinner.Tick` when navigating to the Library view. Verify spinner rendering in search is already wired (from Task 3).

**Files:**
- Modify: `cmd/client/view_menu.go` (activateSidebarItem Library case)
- Modify: `cmd/client/view_library_test.go`

- [ ] **Step 1: Write failing test**

Add to `cmd/client/view_library_test.go`:

```go
func TestLibraryLoadingSetOnSidebarNavigation(t *testing.T) {
	m := New("http://localhost:8080")
	m.token = "tok"
	m.username = "alice"
	m.width, m.height = 120, 40

	items := sidebarItems(m)
	for i, item := range items {
		if item == "Library" {
			m.sidebarIdx = i
			break
		}
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := next.(Model)
	assert.Equal(t, viewLibrary, m2.currentView)
	assert.True(t, m2.libraryLoading)
	assert.NotNil(t, cmd)
}
```

- [ ] **Step 2: Run to verify failure**

```
cd cmd/client && go test -run "TestLibraryLoadingSet" -v
```

Expected: FAIL — `libraryLoading` is false after navigation.

- [ ] **Step 3: Update Library case in activateSidebarItem**

In `view_menu.go`, `activateSidebarItem`, replace the Library case:

```go
case "Library":
	m.currentView = viewLibrary
	m.libraryCursor = 0
	m.libraryLoading = true
	return m, tea.Batch(cmdFetchLibrary(m.baseURL, m.token), m.spinner.Tick)
```

- [ ] **Step 4: Run all tests**

```
cd cmd/client && go test ./... -v
```

Expected: all tests pass.

- [ ] **Step 5: Build binary to confirm no compile errors**

```
cd cmd/client && go build ./...
```

Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add cmd/client/view_menu.go cmd/client/view_library_test.go
git commit -m "feat(client): loading spinner on library navigation"
```

---

## Final Verification

- [ ] **Run full test suite**

```
cd cmd/client && go test ./... -v -count=1
```

Expected: all tests pass, no skipped tests.

- [ ] **Check for uncommitted files**

```bash
git status
```

Expected: clean working tree.
