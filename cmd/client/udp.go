package main

import (
	"net"

	tea "github.com/charmbracelet/bubbletea"
)

func cmdConnectUDP(serverAddr string) tea.Cmd { return nil }
func waitForUDP(conn *net.UDPConn) tea.Cmd   { return nil }
