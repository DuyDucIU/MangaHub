package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestSidebarItemsGuest(t *testing.T) {
	m := New("http://localhost:8080")
	items := sidebarItems(m)
	assert.Equal(t, []string{"Search", "Register", "Login"}, items)
}

func TestSidebarItemsAuth(t *testing.T) {
	m := New("http://localhost:8080")
	m.token = "tok"
	m.username = "alice"
	items := sidebarItems(m)
	assert.Equal(t, []string{"Search", "Library", "Chat", "Logout"}, items)
}

func TestMenuNavDown(t *testing.T) {
	m := New("http://localhost:8080")
	m.sidebarIdx = 0
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m2 := next.(Model)
	assert.Equal(t, 1, m2.sidebarIdx)
}

func TestMenuNavSelectSearch(t *testing.T) {
	m := New("http://localhost:8080")
	m.width, m.height = 100, 40
	m.sidebarIdx = 0 // Search
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := next.(Model)
	assert.Equal(t, viewSearch, m2.currentView)
}

func TestMenuSelectLoginSetsInputs(t *testing.T) {
	m := New("http://localhost:8080")
	m.width, m.height = 100, 40
	// guest sidebar: 0=Search,1=Register,2=Login
	m.sidebarIdx = 2
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := next.(Model)
	assert.Equal(t, viewLogin, m2.currentView)
	assert.Len(t, m2.authInputs, 2) // username + password
}
