package main

import (
	"fmt"
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

func TestTCPNotifAppendsToHistory(t *testing.T) {
	m := New("http://localhost:8080")
	next, _ := m.Update(tcpNotifMsg{text: "update1"})
	m2 := next.(Model)
	assert.Equal(t, []string{"update1"}, m2.notifications)
}

func TestNotifHistoryCappedAt20(t *testing.T) {
	m := New("http://localhost:8080")
	for i := range 20 {
		m.notifications = append([]string{fmt.Sprintf("msg%d", i)}, m.notifications...)
	}
	next, _ := m.Update(tcpNotifMsg{text: "new"})
	m2 := next.(Model)
	assert.Len(t, m2.notifications, 20)
	assert.Equal(t, "new", m2.notifications[0])
}

func TestUDPNotifAppendsToHistory(t *testing.T) {
	m := New("http://localhost:8080")
	next, _ := m.Update(udpNotifMsg{text: "chapter"})
	m2 := next.(Model)
	assert.Equal(t, []string{"chapter"}, m2.notifications)
}
