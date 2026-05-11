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
	inp.Placeholder = "type a message..."
	inp.Focus()
	inp.Width = 80
	inp.CharLimit = chatMaxMsgLen
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
		if msg.reconnected {
			m.chatMessages = append(m.chatMessages, chatMessage{
				text:     "Reconnected to chat",
				isSystem: true,
			})
			m.chatViewport.SetContent(renderMessages(m))
			m.chatViewport.GotoBottom()
		}
		return m, waitForWS(msg.conn)

	case wsDisconnectedMsg:
		m.chatConn = nil
		m.chatMessages = append(m.chatMessages, chatMessage{
			text:     "Chat disconnected — reconnecting in 5s...",
			isSystem: true,
		})
		m.chatViewport.SetContent(renderMessages(m))
		m.chatViewport.GotoBottom()
		if m.currentView == viewChat && m.chatMangaID != "" && m.token != "" {
			return m, cmdReconnectWS(m.baseURL, m.token, m.chatMangaID)
		}
		return m, nil

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
			return m, tea.Batch(waitForWS(m.chatConn), textinput.Blink)
		}
		return m, textinput.Blink

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

	case wsErrMsg:
		m.chatMessages = append(m.chatMessages, chatMessage{
			text:     msg.text,
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
	if m.activeModal != modalNone {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, renderModal(m))
	}
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
