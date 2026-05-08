package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type wsInMsg struct {
	Type      string `json:"type"`
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
}

func (a *App) enterChatRoom() {
	mangaID := a.prompt("Enter manga ID (or press Enter for general): ")
	if mangaID == "" {
		mangaID = "general"
	}

	wsURL := strings.Replace(a.BaseURL, "http://", "ws://", 1) + "/ws/chat?manga_id=" + mangaID

	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		fmt.Println("Error connecting to chat:", err)
		return
	}
	defer conn.Close()

	// first message must be the JWT token
	if err := conn.WriteJSON(map[string]string{"token": a.Token}); err != nil {
		fmt.Println("Error sending auth:", err)
		return
	}

	done := make(chan struct{})

	// reader goroutine: receives messages from server and prints them
	go func() {
		defer close(done)
		for {
			var msg wsInMsg
			if err := conn.ReadJSON(&msg); err != nil {
				return
			}
			switch msg.Type {
			case "message":
				label := msg.Username
				if msg.UserID == a.UserID {
					label = "You"
				}
				fmt.Printf("\r[%-8s] %s\n> ", label, msg.Message)
			case "join":
				fmt.Printf("\r%s joined the room\n> ", msg.Username)
			case "leave":
				fmt.Printf("\r%s left the room\n> ", msg.Username)
			}
		}
	}()

	fmt.Printf("\n=== Chat Room: %s ===\n", mangaID)
	fmt.Println("(type a message and press Enter, /exit to leave)")
	fmt.Println()

	for {
		fmt.Print("> ")
		a.scanner.Scan()
		text := strings.TrimSpace(a.scanner.Text())
		if text == "/exit" {
			break
		}
		if text == "" {
			continue
		}
		if err := conn.WriteJSON(map[string]interface{}{
			"message":   text,
			"timestamp": time.Now().Unix(),
		}); err != nil {
			fmt.Println("Error sending message:", err)
			break
		}
	}

	conn.WriteMessage(websocket.CloseMessage, //nolint:errcheck
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	select {
	case <-done:
	case <-time.After(time.Second):
	}
}
