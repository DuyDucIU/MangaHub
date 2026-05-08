package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
)

type tcpServerMsg struct {
	Type    string `json:"type"`
	MangaID string `json:"manga_id"`
	Chapter int    `json:"chapter"`
	Message string `json:"message"`
}

func (a *App) connectTCP() {
	addr := getenv("TCP_ADDR", "localhost:9090")
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		fmt.Println("Warning: could not connect to TCP server:", err)
		fmt.Println("Real-time progress updates will not be received.")
		return
	}

	authMsg, err := json.Marshal(map[string]string{"type": "auth", "token": a.Token})
	if err != nil {
		conn.Close()
		fmt.Println("Warning: TCP auth marshal failed:", err)
		return
	}
	if _, err := fmt.Fprintf(conn, "%s\n", authMsg); err != nil {
		conn.Close()
		fmt.Println("Warning: TCP auth send failed:", err)
		return
	}

	a.TCPConn = conn
	go a.listenTCP(conn)
}

func (a *App) listenTCP(conn net.Conn) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		var msg tcpServerMsg
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}
		switch msg.Type {
		case "auth_ok":
			// silent confirmation
		case "progress_update":
			fmt.Printf("\nProgress updated: %s → chapter %d\n> ", msg.MangaID, msg.Chapter)
		case "error":
			fmt.Printf("\nTCP server error: %s\n> ", msg.Message)
		}
	}
}
