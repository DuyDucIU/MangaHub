package main

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func newChatPromptInput() textinput.Model {
	inp := textinput.New()
	inp.Placeholder = "manga ID (blank = general)"
	inp.Focus()
	return inp
}

func formatChatMsg(msg chatMessage, myUserID string) string { return "" }

func updateChat(m Model, msg tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func renderChatScreen(m Model) string                      { return "" }
