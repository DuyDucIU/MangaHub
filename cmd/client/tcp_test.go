package main

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTCPNotifWithConn(t *testing.T) {
	// net.Pipe gives a synchronous in-memory connection pair
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	m := New("http://localhost:8080")
	m.tcpConn = client

	next, cmd := m.Update(tcpNotifMsg{text: "One Piece → chapter 1096"})
	m2 := next.(Model)
	assert.Equal(t, "One Piece → chapter 1096", m2.notification)
	assert.NotNil(t, cmd) // has conn → re-subscribes
}
