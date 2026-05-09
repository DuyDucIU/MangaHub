package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"

	tea "github.com/charmbracelet/bubbletea"
)

type tcpServerMsg struct {
	Type    string `json:"type"`
	MangaID string `json:"manga_id"`
	Chapter int    `json:"chapter"`
	Message string `json:"message"`
}

// cmdConnectTCP dials the TCP server, sends the auth message, and returns
// tcpConnectedMsg on success or tcpNotifMsg with a warning on failure.
func cmdConnectTCP(addr, token string) tea.Cmd {
	return func() tea.Msg {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			return tcpNotifMsg{text: "Warning: TCP unavailable — progress updates disabled"}
		}
		auth, _ := json.Marshal(map[string]string{"type": "auth", "token": token})
		if _, err := fmt.Fprintf(conn, "%s\n", auth); err != nil {
			conn.Close()
			return tcpNotifMsg{text: "Warning: TCP auth failed — progress updates disabled"}
		}
		return tcpConnectedMsg{conn: conn}
	}
}

// waitForTCP blocks until one message arrives on conn, then returns it as a
// tcpNotifMsg. The caller must re-issue this Cmd after each message.
func waitForTCP(conn net.Conn) tea.Cmd {
	return func() tea.Msg {
		scanner := bufio.NewScanner(conn)
		if !scanner.Scan() {
			return tcpNotifMsg{text: "TCP connection closed"}
		}
		var msg tcpServerMsg
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			return tcpNotifMsg{text: "TCP: unreadable message"}
		}
		switch msg.Type {
		case "auth_ok":
			return tcpNotifMsg{text: ""}
		case "progress_update":
			return tcpNotifMsg{text: fmt.Sprintf("Progress updated: %s → chapter %d", msg.MangaID, msg.Chapter)}
		case "error":
			return tcpNotifMsg{text: "TCP: " + msg.Message}
		default:
			return tcpNotifMsg{text: ""}
		}
	}
}
