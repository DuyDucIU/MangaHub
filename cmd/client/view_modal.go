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
	return openModalJoinChatPrefilled(m, "")
}

func openModalJoinChatPrefilled(m Model, prefill string) Model {
	inp := textinput.New()
	inp.Placeholder = "manga ID or name (blank = general)"
	inp.Focus()
	inp.Width = 38
	if prefill != "" {
		inp.SetValue(prefill)
	}
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
	if m.currentView == viewLibrary && len(m.libraryFlat) > m.libraryCursor {
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
		"  [?]    This help screen",
		"  [n]    Notification center",
		"  [c]    Join chat room",
		"  [Esc]  Back / close",
		"  [q]    Quit",
		"",
		styleMutedText.Render("Search"),
		"  [/]    Focus search input",
		"  [↑↓]   Navigate results",
		"  [←→]   Previous / next page",
		"  [a]    Add to library / update progress",
		"",
		styleMutedText.Render("Library"),
		"  [↑↓]   Navigate",
		"  [a]    Update progress",
		"  [d]    Remove from library",
		"",
		styleMutedText.Render("Chat"),
		"  [Enter]   Send message",
		"  [/exit]   Leave room",
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
		styleMutedText.Render("[y] Yes    [n]/[Esc] No")
	return modalBox("Confirm", content)
}

func renderModalJoinChat(m Model) string {
	content := styleMutedText.Render("Manga ID (blank = general):") + "\n" +
		m.modalInput.View() + "\n\n" +
		styleMutedText.Render("[Enter] Join    [Esc] Cancel")
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
		styleMutedText.Render("[Tab] Switch    [Enter] Save    [Esc] Cancel")
	return modalBox(title, content)
}
