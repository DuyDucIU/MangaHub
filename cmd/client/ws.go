package main

import (
	"github.com/gorilla/websocket"
	tea "github.com/charmbracelet/bubbletea"
)

func cmdConnectWS(baseURL, token, mangaID string) tea.Cmd { return nil }
func waitForWS(conn *websocket.Conn) tea.Cmd              { return nil }
