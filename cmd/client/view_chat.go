package main

import (
	tea "github.com/charmbracelet/bubbletea"
)

func formatChatMsg(msg chatMessage, myUserID string) string { return "" }

func updateChat(m Model, msg tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func renderChatScreen(m Model) string                      { return "" }
