package main

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gorilla/websocket"
)

type wsInMsg struct {
	Type      string `json:"type"`
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
}

// cmdConnectWS dials the WebSocket server, sends the JWT auth message, and
// returns wsConnectedMsg on success or errMsg on failure.
func cmdConnectWS(baseURL, token, mangaID string) tea.Cmd {
	return func() tea.Msg {
		wsURL := strings.Replace(baseURL, "http://", "ws://", 1) +
			"/ws/chat?manga_id=" + mangaID
		dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
		conn, _, err := dialer.Dial(wsURL, nil)
		if err != nil {
			return errMsg{text: "Chat connect failed: " + err.Error()}
		}
		if err := conn.WriteJSON(map[string]string{"token": token}); err != nil {
			conn.Close()
			return errMsg{text: "Chat auth failed: " + err.Error()}
		}
		return wsConnectedMsg{conn: conn}
	}
}

// waitForWS blocks until one WebSocket message arrives, then returns it as
// wsMsgReceived / wsJoined / wsLeft. The caller must re-issue this Cmd.
func waitForWS(conn *websocket.Conn) tea.Cmd {
	return func() tea.Msg {
		var msg wsInMsg
		if err := conn.ReadJSON(&msg); err != nil {
			return errMsg{text: "Chat disconnected"}
		}
		switch msg.Type {
		case "message":
			return wsMsgReceived{
				userID:   msg.UserID,
				username: msg.Username,
				text:     msg.Message,
			}
		case "join":
			return wsJoined{username: msg.Username}
		case "leave":
			return wsLeft{username: msg.Username}
		default:
			return wsMsgReceived{}
		}
	}
}

// cmdSendWSMessage sends one message over the WebSocket.
func cmdSendWSMessage(conn *websocket.Conn, text string) tea.Cmd {
	return func() tea.Msg {
		err := conn.WriteJSON(map[string]interface{}{
			"message":   text,
			"timestamp": time.Now().Unix(),
		})
		if err != nil {
			return errMsg{text: "Send failed: " + err.Error()}
		}
		return waitForWS(conn)
	}
}
