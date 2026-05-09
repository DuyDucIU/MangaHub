package main

import (
	"net"

	tea "github.com/charmbracelet/bubbletea"
)

func cmdConnectTCP(addr, token string) tea.Cmd { return nil }
func waitForTCP(conn net.Conn) tea.Cmd         { return nil }
