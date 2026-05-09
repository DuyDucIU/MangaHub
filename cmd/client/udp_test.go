package main

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUDPNotifWithConn(t *testing.T) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Skip("cannot open UDP socket:", err)
	}
	defer conn.Close()

	m := New("http://localhost:8080")
	m.udpConn = conn

	next, cmd := m.Update(udpNotifMsg{text: "Bleach chapter 700 released!"})
	m2 := next.(Model)
	assert.Equal(t, "Bleach chapter 700 released!", m2.notification)
	assert.NotNil(t, cmd) // has conn → re-subscribes
}
