# TUI Redesign — Design Spec

**Date:** 2026-05-10  
**Branch:** feat/bubbletea-tui-client  
**Approach:** Option B — Modal infrastructure first, then views

---

## 1. Scope

Full UX redesign of the BubbleTea TUI client (`cmd/client/`) based on GPT redesign recommendations. Covers:

- Modal / overlay infrastructure (built first, used by all features)
- Split-pane Search (replaces 3-state sequential flow)
- Split-pane Library (replaces flat grouped list)
- Home Dashboard (replaces static welcome content)
- Missing states: loading spinners, empty states, help screen, notification center

Out of scope: backend changes, new API endpoints, chat room user counts.

---

## 2. Architecture

### 2.1 Model Changes

Keep the flat `Model` struct. Add the following fields:

```go
// Modal / overlay system
activeModal     modalType
modalInput      textinput.Model   // join chat, update progress chapter field
modalCursor     int               // option list cursor
modalMessage    string            // error modal text
modalConfirmAct confirmAction

// Notification history (replaces single notification string)
notifications   []string          // ring buffer, max 20 entries

// Loading states
searchLoading  bool
detailLoading  bool
libraryLoading bool

// Modal context
modalIsAdding  bool   // true = POST /users/library, false = PUT /users/progress

// Dashboard data (populated at login)
dashboardReading []libraryItem    // top 3 reading items

// Search pane state (replaces searchState enum)
searchInputFocused bool
searchFocusPane    int            // 0=list, 1=detail
detailPending      string         // manga ID of in-flight detail fetch
```

Remove: `searchState searchState` enum and its three constants (`searchStateForm`, `searchStateResults`, `searchStateDetail`).

### 2.2 New Enums

```go
type modalType int
const (
    modalNone modalType = iota
    modalJoinChat
    modalUpdateProgress   // chapter input + status selector combined
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

### 2.3 Update Flow

```
Update(msg) {
    1. Handle global TCP/UDP/WS messages (unchanged)
    2. Handle global keys: ctrl+c, ?, n, c (logged-in only)
    3. If activeModal != none → updateModal(m, msg); return
    4. Route to view update: menu / auth / search / library / chat
}
```

### 2.4 Render Flow

```
View() {
    if currentView == viewChat → return renderChatScreen(m)
    base = renderLayout(m)
    if activeModal == none → return base
    modal = renderModal(m)
    return overlayCenter(base, modal, m.width, m.height)
}
```

### 2.5 Overlay Helper

`overlayCenter(bg, fg string, w, h int) string` merges two ANSI strings line-by-line, painting the modal box over the center of the background. Background content remains visible around modal edges.

Modal box: fixed 44 cols wide, height varies by content, centered on both axes.

---

## 3. Modal / Overlay System

### 3.1 Modal Types

| Modal | Trigger | Content |
|---|---|---|
| `modalJoinChat` | `c` (logged in, any view) | Text input for manga ID, Enter to connect |
| `modalUpdateProgress` | `a` in library detail pane | Chapter number input + status option list |
| `modalConfirmAction` | Logout sidebar item, `d` in library | "Are you sure?" Y/N |
| `modalError` | Network/API failures | Error message, `r` retry, Esc dismiss |
| `modalHelp` | `?` anywhere | Keybindings table |
| `modalNotifications` | `n` anywhere | Scrollable last-20 notifications |

### 3.2 modalUpdateProgress Layout

```
┌─ Update Progress ──────────────┐
│ Chapter: [1142        ]        │
│                                │
│ Status:                        │
│ > Reading                      │
│   Plan to Read                 │
│   Completed                    │
│   On Hold                      │
│   Dropped                      │
│                                │
│ Tab to switch · Enter save     │
└────────────────────────────────┘
```

Tab switches focus between chapter input and status list. Enter saves:
- If `modalIsAdding == true`: calls POST `/users/library` (add new entry, chapter defaults to 0)
- If `modalIsAdding == false`: calls PUT `/users/progress` (update existing entry)

Modal title shown as **"Add to Library"** when adding, **"Update Progress"** when updating.

### 3.3 Global Keybindings (always active, outside modal)

```
?       Open help modal
n       Open notification center modal
c       Open join chat modal (logged-in only)
q       Quit (confirm modal if in chat)
ctrl+c  Force quit
```

---

## 4. Split-pane Search

### 4.1 Layout

Pane split: 38% left / 62% right of content width.

```
┌─ Search ──────────────────────────────────────────────────┐
│ / one piece                    │ One Piece                 │
│ ────────────────────────────── │ Author: Oda               │
│ > One Piece                    │ Status: Ongoing           │
│   One Punch Man                │ Genres: Action, Adventure │
│   One Outs                     │ Chapters: 1142            │
│                                │                           │
│                                │ Synopsis                  │
│                                │ Pirates search for the... │
│                                │                           │
│                                │ [a] Add to Library        │
│                                │ [c] Join Chat             │
└────────────────────────────────┴───────────────────────────┘
```

### 4.2 State

- `searchInputFocused bool`: text input at top of left pane is active
- `searchFocusPane int`: 0=left list, 1=right detail (for action keys)
- `detailPending string`: ID of in-flight detail fetch; ignores stale responses
- Detail auto-fetches when cursor moves (only if result ID differs from current `detailManga.ID`)

Removes `searchState` enum entirely.

### 4.3 Keybindings

```
/           Focus search input
Esc         Blur input (back to list navigation)
Enter       Submit search (when input focused)
↑ / ↓       Move cursor in results list
← / →       Previous / next page
Tab         Switch focus pane (list ↔ detail)
a           Add to library → modalUpdateProgress with modalIsAdding=true (if detailEntry==nil)
            Update progress → modalUpdateProgress with modalIsAdding=false (if detailEntry!=nil)
c           Join chat → modalJoinChat pre-filled with manga ID
Esc         Back to menu (when list focused, input not focused)
```

### 4.4 Empty / Loading States

| State | Left pane | Right pane |
|---|---|---|
| Before first search | `Search for manga using /` | `Select a result to see details` |
| Fetching results | spinner + `Searching...` | — |
| No results | `No results for "query"` | — |
| Fetching detail | — | spinner |
| Result selected, no auth | detail fields shown | `[a] Login to add` (disabled) |

---

## 5. Split-pane Library

### 5.1 Layout

Same 38/62 pane split. Detail pane uses data already in `libraryItem` — no extra API fetch.

```
┌─ Library ─────────────────────────────────────────────────┐
│ READING (3)                    │ One Piece                 │
│ > One Piece                    │ Progress: ch.1142         │
│   Vagabond                     │ Status: Reading           │
│   Blue Lock                    │ Last updated: 2h ago      │
│                                │                           │
│ PLAN TO READ (5)               │ [a] Update Progress       │
│   Berserk                      │ [d] Remove                │
│                                │                           │
│ COMPLETED (2)                  │                           │
│   Slam Dunk                    │                           │
└────────────────────────────────┴───────────────────────────┘
```

### 5.2 State

- `libraryCursor int`: flat index (same as current)
- Section headers are non-selectable; cursor skips over them automatically
- Detail pane always shows `libraryFlat[libraryCursor]`

### 5.3 Keybindings

```
↑ / ↓       Move cursor (skips section headers)
a           Update progress → modalUpdateProgress (pre-filled with current values)
d           Remove from library → modalConfirmAction (confirmRemoveManga)
Esc         Back to menu
```

### 5.4 Remove flow

`confirmRemoveManga` stores the manga ID in `modalMessage` temporarily. On confirm: calls `DELETE /users/library/{mangaID}` then refreshes library.

---

## 6. Home Dashboard

### 6.1 Trigger

Populated on `loginSuccessMsg` by firing `cmdFetchLibrary`. Dashboard is shown as the default content panel when `currentView == viewMenu` and `token != ""`.

### 6.2 Layout

```
┌─ Content ─────────────────────────────────────────────────┐
│                                                           │
│  Welcome back, duyduc                                     │
│                                                           │
│  Continue Reading                                         │
│  ──────────────────────────────────────────────────────   │
│  One Piece                              ch.1142           │
│  Vagabond                               ch.221            │
│  Blue Lock                              ch.307            │
│                                                           │
│  Recent Notifications                                     │
│  ──────────────────────────────────────────────────────   │
│  • TCP: New chapter available for Solo Leveling           │
│  • UDP: Library sync completed                            │
│  • TCP: Server restarted                                  │
│                                                           │
└───────────────────────────────────────────────────────────┘
```

### 6.3 Data Sources

- **Continue Reading:** top 3 items from `libraryGroups["reading"]` (populated at login)
- **Recent Notifications:** last 5 entries from `notifications []string`

### 6.4 Interactions

Dashboard is read-only. No cursor. Use sidebar to navigate. Press `n` for full notification history.

---

## 7. Missing States

### 7.1 Loading Spinner

Uses `bubbles/spinner`. `searchLoading`, `detailLoading`, `libraryLoading` booleans control per-pane spinners. Spinner `Tick` command is fired alongside fetch commands and stopped on result arrival.

### 7.2 Empty States

| Situation | Message |
|---|---|
| Search — before first query | `Search for manga using /` |
| Search — 0 results | `No results for "query"` |
| Library — empty | `Your library is empty. Search for manga to add.` |
| Detail pane — nothing selected | `Select a result to see details` |

### 7.3 Help Modal (`?`)

```
┌─ Keybindings ──────────────────────────┐
│ Global                                 │
│  ?       This help screen              │
│  n       Notification center           │
│  c       Join chat room                │
│  Esc     Back / close                  │
│  q       Quit                          │
│                                        │
│ Search                                 │
│  /       Focus search input            │
│  ↑↓      Navigate results             │
│  ←→      Previous / next page         │
│  a       Add to library                │
│                                        │
│ Library                                │
│  ↑↓      Navigate                      │
│  a       Update progress               │
│  d       Remove from library           │
└────────────────────────────────────────┘
```

### 7.4 Notification Center Modal (`n`)

Displays `notifications []string` in reverse-chronological order (newest first). `↑↓` to scroll, Esc to close. Max 20 entries stored; oldest dropped when full.

---

## 8. Notification History

Replace `notification string` in Model with `notifications []string`. On every `tcpNotifMsg` or `udpNotifMsg`:

```go
m.notifications = append([]string{msg.text}, m.notifications...)
if len(m.notifications) > 20 {
    m.notifications = m.notifications[:20]
}
```

Footer still shows `notifications[0]` (most recent) as before.

---

## 9. Build Order

1. **Modal infrastructure** — enums, `overlayCenter`, `updateModal`, `renderModal`, global key routing
2. **Split-pane Search** — remove `searchState`, implement split layout and auto-detail-fetch
3. **Split-pane Library** — implement split layout, wire `modalUpdateProgress` and `modalConfirmAction`
4. **Home Dashboard** — fetch on login, render dashboard content panel
5. **Missing states** — spinner, empty states, help modal content, notification center content

Each step is independently buildable and testable.
