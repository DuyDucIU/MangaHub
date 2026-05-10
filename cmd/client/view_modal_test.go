package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestHelpKeyOpensHelpModal(t *testing.T) {
	m := New("http://localhost:8080")
	m.width, m.height = 120, 40
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m2 := next.(Model)
	assert.Equal(t, modalHelp, m2.activeModal)
}

func TestNotifKeyOpensNotifModal(t *testing.T) {
	m := New("http://localhost:8080")
	m.width, m.height = 120, 40
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m2 := next.(Model)
	assert.Equal(t, modalNotifications, m2.activeModal)
}

func TestEscClosesModal(t *testing.T) {
	m := New("http://localhost:8080")
	m.width, m.height = 120, 40
	m.activeModal = modalHelp
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m2 := next.(Model)
	assert.Equal(t, modalNone, m2.activeModal)
}

func TestCKeyOpensJoinChatModalWhenLoggedIn(t *testing.T) {
	m := New("http://localhost:8080")
	m.width, m.height = 120, 40
	m.token = "tok"
	m.username = "alice"
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m2 := next.(Model)
	assert.Equal(t, modalJoinChat, m2.activeModal)
}

func TestCKeyDoesNotOpenChatWhenGuest(t *testing.T) {
	m := New("http://localhost:8080")
	m.width, m.height = 120, 40
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m2 := next.(Model)
	assert.Equal(t, modalNone, m2.activeModal)
}

func TestModalInterceptsEscWithoutChangingView(t *testing.T) {
	m := New("http://localhost:8080")
	m.width, m.height = 120, 40
	m.currentView = viewSearch
	m.activeModal = modalHelp
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m2 := next.(Model)
	assert.Equal(t, modalNone, m2.activeModal)
	assert.Equal(t, viewSearch, m2.currentView) // view unchanged
}

func TestConfirmYConfirmsLogout(t *testing.T) {
	m := New("http://localhost:8080")
	m.width, m.height = 120, 40
	m.token = "tok"
	m.username = "alice"
	m.activeModal = modalConfirmAction
	m.modalConfirmAct = confirmLogout
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m2 := next.(Model)
	assert.Equal(t, modalNone, m2.activeModal)
	assert.Empty(t, m2.token)
	assert.Equal(t, viewMenu, m2.currentView)
}

func TestConfirmNKeepModal(t *testing.T) {
	m := New("http://localhost:8080")
	m.width, m.height = 120, 40
	m.token = "tok"
	m.activeModal = modalConfirmAction
	m.modalConfirmAct = confirmLogout
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m2 := next.(Model)
	assert.Equal(t, modalNone, m2.activeModal)
	assert.Equal(t, "tok", m2.token) // not logged out
}
