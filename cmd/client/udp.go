package main

import (
	"encoding/json"
	"fmt"
	"net"
)

type udpInPkt struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	MangaID string `json:"manga_id"`
}

func (a *App) connectUDP() {
	serverAddr, err := net.ResolveUDPAddr("udp", getenv("UDP_ADDR", "localhost:9091"))
	if err != nil {
		fmt.Println("Warning: could not resolve UDP server:", err)
		return
	}

	conn, err := net.ListenUDP("udp", &net.UDPAddr{})
	if err != nil {
		fmt.Println("Warning: could not start UDP listener:", err)
		fmt.Println("Chapter notifications will not be received.")
		return
	}

	reg, _ := json.Marshal(map[string]interface{}{"type": "register", "manga_ids": []string{}})
	if _, err := conn.WriteToUDP(reg, serverAddr); err != nil {
		conn.Close()
		fmt.Println("Warning: UDP registration failed:", err)
		return
	}

	a.UDPConn = conn
	go a.listenUDP(conn)
}

func (a *App) listenUDP(conn *net.UDPConn) {
	defer conn.Close()
	buf := make([]byte, 65535)
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			return // connection closed on logout
		}
		var pkt udpInPkt
		if err := json.Unmarshal(buf[:n], &pkt); err != nil {
			continue
		}
		switch pkt.Type {
		case "ack":
			fmt.Printf("\nNotifications active: %s\n> ", pkt.Message)
		case "notification":
			fmt.Printf("\nNotification: %s\n> ", pkt.Message)
		}
	}
}
