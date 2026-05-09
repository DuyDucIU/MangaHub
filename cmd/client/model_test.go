package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	m := New("http://localhost:8080")
	assert.Equal(t, viewMenu, m.currentView)
	assert.Equal(t, 0, m.sidebarIdx)
	assert.Empty(t, m.token)
	assert.Equal(t, "http://localhost:8080", m.baseURL)
}

func TestWindowResize(t *testing.T) {
	m := New("http://localhost:8080")
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m2 := next.(Model)
	assert.Equal(t, 120, m2.width)
	assert.Equal(t, 40, m2.height)
}

func TestTCPNotifNoConn(t *testing.T) {
	m := New("http://localhost:8080")
	next, cmd := m.Update(tcpNotifMsg{text: "progress update"})
	m2 := next.(Model)
	assert.Equal(t, "progress update", m2.notification)
	assert.Nil(t, cmd) // no conn → no re-subscribe
}

func TestUDPNotifNoConn(t *testing.T) {
	m := New("http://localhost:8080")
	next, cmd := m.Update(udpNotifMsg{text: "chapter released"})
	m2 := next.(Model)
	assert.Equal(t, "chapter released", m2.notification)
	assert.Nil(t, cmd)
}
