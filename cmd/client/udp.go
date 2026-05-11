package main

import (
	"encoding/json"
	"fmt"
	"net"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type udpInPkt struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	MangaID string `json:"manga_id"`
}

// cmdConnectUDP opens a local UDP socket, sends a register packet to serverAddr,
// and returns udpConnectedMsg on success or udpNotifMsg with a warning on failure.
func cmdConnectUDP(serverAddr string) tea.Cmd {
	return func() tea.Msg {
		srv, err := net.ResolveUDPAddr("udp", serverAddr)
		if err != nil {
			return udpNotifMsg{text: "Warning: UDP unavailable — chapter notifications disabled"}
		}
		conn, err := net.ListenUDP("udp", &net.UDPAddr{})
		if err != nil {
			return udpNotifMsg{text: "Warning: UDP unavailable — chapter notifications disabled"}
		}
		reg, _ := json.Marshal(map[string]interface{}{"type": "register", "manga_ids": []string{}})
		if _, err := conn.WriteToUDP(reg, srv); err != nil {
			conn.Close()
			return udpNotifMsg{text: "Warning: UDP registration failed — chapter notifications disabled"}
		}
		return udpConnectedMsg{conn: conn}
	}
}

// cmdReregisterUDP waits 60 seconds then re-sends the register packet so the
// client stays on the server's subscriber list even after a silent eviction.
func cmdReregisterUDP(conn *net.UDPConn, serverAddr string) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(60 * time.Second)
		srv, err := net.ResolveUDPAddr("udp", serverAddr)
		if err != nil {
			return udpReregisteredMsg{}
		}
		reg, _ := json.Marshal(map[string]interface{}{"type": "register", "manga_ids": []string{}})
		conn.WriteToUDP(reg, srv)
		return udpReregisteredMsg{}
	}
}

// waitForUDP blocks until one UDP packet arrives, then returns it as a
// udpNotifMsg. The caller must re-issue this Cmd after each message.
func waitForUDP(conn *net.UDPConn) tea.Cmd {
	return func() tea.Msg {
		buf := make([]byte, 65535)
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			return udpNotifMsg{text: "UDP connection closed"}
		}
		var pkt udpInPkt
		if err := json.Unmarshal(buf[:n], &pkt); err != nil {
			return udpNotifMsg{text: ""}
		}
		switch pkt.Type {
		case "ack":
			return udpNotifMsg{text: fmt.Sprintf("Notifications active: %s", pkt.Message)}
		case "notification":
			return udpNotifMsg{text: fmt.Sprintf("Notification: %s", pkt.Message)}
		default:
			return udpNotifMsg{text: ""}
		}
	}
}
