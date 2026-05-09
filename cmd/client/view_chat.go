package main

import (
	"github.com/gorilla/websocket"
	tea "github.com/charmbracelet/bubbletea"
)

func cmdSendWSMessage(conn *websocket.Conn, text string) tea.Cmd { return nil }

func formatChatMsg(msg chatMessage, myUserID string) string { return "" }

func updateChat(m Model, msg tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func renderChatScreen(m Model) string                      { return "" }
