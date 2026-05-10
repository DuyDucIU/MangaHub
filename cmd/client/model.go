package main

import (
	"fmt"
	"net"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/spinner"
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

// --- Modal / overlay enums ---

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

type tcpNotifMsg     struct{ text string }
type udpNotifMsg     struct{ text string }
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
type addLibraryMsg     struct{ err, title string }
type updateProgressMsg struct{ err string }
type removeLibraryMsg  struct{ err string }
type errMsg            struct{ text string }

// --- Lipgloss styles ---

var (
	styleHeader = lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1)

	styleSidebarItem = lipgloss.NewStyle().
				Padding(0, 1)

	styleSidebarSelected = lipgloss.NewStyle().
				Bold(true).
				Padding(0, 1)

	styleTitle = lipgloss.NewStyle().
			Bold(true)

	styleMutedText = lipgloss.NewStyle()

	styleNotif = lipgloss.NewStyle()

	styleNormal = lipgloss.NewStyle()

	styleBorderBox = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder())

	styleActiveBorderBox = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder())

	styleError = lipgloss.NewStyle().
			Bold(true)
)

const sidebarWidth = 22

var mangaHubArt = [6]string{
	`███╗   ███╗ █████╗ ███╗   ██╗ ██████╗  █████╗ ██╗  ██╗██╗   ██╗██████╗ `,
	`████╗ ████║██╔══██╗████╗  ██║██╔════╝ ██╔══██╗██║  ██║██║   ██║██╔══██╗`,
	`██╔████╔██║███████║██╔██╗ ██║██║  ███╗███████║███████║██║   ██║██████╔╝ `,
	`██║╚██╔╝██║██╔══██║██║╚██╗██║██║   ██║██╔══██║██╔══██║██║   ██║██╔══██╗`,
	`██║ ╚═╝ ██║██║  ██║██║ ╚████║╚██████╔╝██║  ██║██║  ██║╚██████╔╝██████╔╝`,
	`╚═╝     ╚═╝╚═╝  ╚═╝╚═╝  ╚═══╝ ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝ ╚═════╝ ╚═════╝`,
}

func padVisual(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

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
	searchPerformed    bool
	searchLastQuery    string
	detailPending      string
	searchInputs       []textinput.Model
	searchFocus        int
	searchResults      []mangaItem
	searchCursor       int
	searchPage         int
	searchTotal        int
	detailManga        mangaItem
	detailEntry        *libraryItem

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

		// Global keys — intercept before modal and view routing.
		// Skip when a text input in the current view has focus.
		textInputActive := m.searchInputFocused ||
			m.currentView == viewLogin || m.currentView == viewRegister ||
			m.currentView == viewChat
		if m.activeModal == modalNone && !textInputActive {
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
					prefill := ""
					if m.currentView == viewSearch && m.detailManga.ID != "" {
						prefill = m.detailManga.ID
					}
					m = openModalJoinChatPrefilled(m, prefill)
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

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tcpNotifMsg:
		if msg.text != "" {
			m.notifications = pushNotif(m.notifications, msg.text)
		}
		if m.tcpConn != nil {
			return m, waitForTCP(m.tcpConn)
		}
		return m, nil

	case udpNotifMsg:
		if msg.text != "" {
			m.notifications = pushNotif(m.notifications, msg.text)
		}
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

	case addLibraryMsg:
		if msg.err != "" {
			m.notifications = pushNotif(m.notifications, "Add failed: "+msg.err)
			return m, nil
		}
		m.notifications = pushNotif(m.notifications, fmt.Sprintf("Added %q to library.", msg.title))
		if m.currentView == viewSearch && m.detailManga.ID != "" {
			m.detailPending = m.detailManga.ID
			m.detailLoading = true
			return m, tea.Batch(cmdFetchDetail(m.baseURL, m.token, m.detailManga.ID), m.spinner.Tick)
		}
		return m, nil

	case updateProgressMsg:
		if msg.err != "" {
			m.notifications = pushNotif(m.notifications, "Update failed: "+msg.err)
			return m, nil
		}
		if m.currentView == viewLibrary {
			m.libraryLoading = true
			return m, tea.Batch(cmdFetchLibrary(m.baseURL, m.token), m.spinner.Tick)
		}
		if m.currentView == viewSearch && m.detailManga.ID != "" {
			m.detailPending = m.detailManga.ID
			m.detailLoading = true
			return m, tea.Batch(cmdFetchDetail(m.baseURL, m.token, m.detailManga.ID), m.spinner.Tick)
		}
		return m, nil

	case removeLibraryMsg:
		if msg.err != "" {
			m.notifications = pushNotif(m.notifications, "Remove failed: "+msg.err)
			return m, nil
		}
		m.notifications = pushNotif(m.notifications, "Removed from library.")
		if m.currentView == viewLibrary {
			m.libraryLoading = true
			return m, tea.Batch(cmdFetchLibrary(m.baseURL, m.token), m.spinner.Tick)
		}
		return m, nil

	case libraryResultMsg:
		if msg.err != "" {
			m.notifications = pushNotif(m.notifications, "Library error: "+msg.err)
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

// renderLayout draws the full framed layout using box-drawing characters.
func renderLayout(m Model) string {
	if m.width < 40 || m.height < 12 {
		return ""
	}

	innerW := m.width - 2
	sidebarW := sidebarWidth
	contentW := innerW - sidebarW - 1

	headerLines := buildHeaderLines(m, innerW)
	headerH := len(headerLines)
	footerLines := buildFooterLines(m, innerW)
	footerH := len(footerLines)

	bodyH := m.height - headerH - footerH - 4 // top + h-b sep + b-f sep + bottom
	if bodyH < 1 {
		bodyH = 1
	}

	sidebarBlock := lipgloss.NewStyle().Width(sidebarW).Height(bodyH).Render(
		renderSidebar(m, sidebarW, bodyH))
	contentBlock := lipgloss.NewStyle().Width(contentW).Height(bodyH).Render(
		renderContent(m, contentW-2, bodyH-1))

	splitLines := func(s string, w, count int) []string {
		lines := strings.Split(s, "\n")
		for len(lines) < count {
			lines = append(lines, strings.Repeat(" ", w))
		}
		return lines[:count]
	}
	sidebarLines := splitLines(sidebarBlock, sidebarW, bodyH)
	contentLines := splitLines(contentBlock, contentW, bodyH)

	var sb strings.Builder

	sb.WriteString("┌" + strings.Repeat("─", innerW) + "┐\n")

	for _, line := range headerLines {
		sb.WriteString("│" + padVisual(line, innerW) + "│\n")
	}

	sb.WriteString("├" + strings.Repeat("─", sidebarW) + "┬" + strings.Repeat("─", contentW) + "┤\n")

	for i := 0; i < bodyH; i++ {
		sb.WriteString("│" + sidebarLines[i] + "│" + contentLines[i] + "│\n")
	}

	sb.WriteString("├" + strings.Repeat("─", sidebarW) + "┴" + strings.Repeat("─", contentW) + "┤\n")

	for _, line := range footerLines {
		sb.WriteString("│" + padVisual(line, innerW) + "│\n")
	}

	sb.WriteString("└" + strings.Repeat("─", innerW) + "┘")

	return sb.String()
}

func buildHeaderLines(m Model, innerW int) []string {
	timeStr := time.Now().Format("3:04 PM")
	lines := make([]string, 0, len(mangaHubArt)+1)
	lines = append(lines, "") // blank top padding

	for i, art := range mangaHubArt {
		var right string
		switch i {
		case 2:
			if m.username != "" {
				right = "  user: " + m.username + "  "
			}
		case 3:
			right = "  [" + timeStr + "]  "
		}
		artW := lipgloss.Width(art)
		rightW := lipgloss.Width(right)
		gap := innerW - artW - rightW
		if gap < 2 {
			// terminal too narrow to show right info — omit it
			right = ""
			gap = innerW - artW
			if gap < 0 {
				gap = 0
			}
		}
		lines = append(lines, art+strings.Repeat(" ", gap)+right)
	}
	return lines
}

func buildFooterLines(m Model, innerW int) []string {
	keys := "  [?] Help   [n] Notif   [q] Quit"
	if len(m.notifications) > 0 {
		notif := "   •  " + m.notifications[0]
		if lipgloss.Width(keys)+lipgloss.Width(notif) <= innerW {
			keys += notif
		}
	}
	return []string{padVisual(keys, innerW)}
}

const maxNotifications = 20

func pushNotif(notifs []string, text string) []string {
	n := append([]string{text}, notifs...)
	if len(n) > maxNotifications {
		n = n[:maxNotifications]
	}
	return n
}

// renderContent dispatches to the active view's render function.
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
