package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type tcpServerMsg struct {
	Type       string `json:"type"`
	MangaID    string `json:"manga_id"`
	MangaTitle string `json:"manga_title"`
	Chapter    int    `json:"chapter"`
	Message    string `json:"message"`
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

// cmdReconnectTCP waits 5 seconds then attempts to re-dial and re-authenticate.
// Returns tcpConnectedMsg on success or tcpDisconnectedMsg to trigger another retry.
func cmdReconnectTCP(addr, token string) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(5 * time.Second)
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			return tcpDisconnectedMsg{}
		}
		auth, _ := json.Marshal(map[string]string{"type": "auth", "token": token})
		if _, err := fmt.Fprintf(conn, "%s\n", auth); err != nil {
			conn.Close()
			return tcpDisconnectedMsg{}
		}
		return tcpConnectedMsg{conn: conn, reconnected: true}
	}
}

// waitForTCP blocks until one message arrives on conn, then returns it as a
// tcpNotifMsg. The caller must re-issue this Cmd after each message.
func waitForTCP(conn net.Conn) tea.Cmd {
	return func() tea.Msg {
		scanner := bufio.NewScanner(conn)
		if !scanner.Scan() {
			return tcpDisconnectedMsg{}
		}
		var msg tcpServerMsg
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			return tcpNotifMsg{text: "TCP: unreadable message"}
		}
		switch msg.Type {
		case "auth_ok":
			return tcpNotifMsg{text: ""}
		case "progress_update":
			name := msg.MangaTitle
			if name == "" {
				name = msg.MangaID
			}
			return tcpNotifMsg{text: fmt.Sprintf("Progress updated: %s → Chapter %d", name, msg.Chapter)}
		case "error":
			return tcpNotifMsg{text: "TCP: " + msg.Message}
		default:
			return tcpNotifMsg{text: ""}
		}
	}
}
