package main

import (
	"net"
	"strings"

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
type addLibraryMsg     struct{ err string }
type updateProgressMsg struct{ err string }
type removeLibraryMsg  struct{ err string }
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
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}
	return styleHeader.Width(m.width).Render(left + strings.Repeat(" ", gap) + right)
}

const maxNotifications = 20

func pushNotif(notifs []string, text string) []string {
	n := append([]string{text}, notifs...)
	if len(n) > maxNotifications {
		n = n[:maxNotifications]
	}
	return n
}

func renderFooter(m Model) string {
	text := ""
	if len(m.notifications) > 0 {
		text = "  " + m.notifications[0]
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
