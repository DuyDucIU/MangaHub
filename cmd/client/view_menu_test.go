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
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
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

func TestLoginSuccessFiresLibraryFetch(t *testing.T) {
	m := New("http://localhost:8080")
	m.currentView = viewLogin
	m.authInputs = initLoginInputs()
	m.width, m.height = 120, 40
	_, cmd := m.Update(loginSuccessMsg{token: "tok", userID: "u1", username: "alice"})
	assert.NotNil(t, cmd) // batch: TCP + UDP + library fetch
}

func TestDashboardReadingPopulatedFromLibraryResult(t *testing.T) {
	m := New("http://localhost:8080")
	m.currentView = viewMenu
	m.token = "tok"
	m.width, m.height = 120, 40
	groups := map[string][]libraryItem{
		"reading": {
			{MangaID: "a", Title: "One Piece", CurrentChapter: 1142},
			{MangaID: "b", Title: "Naruto", CurrentChapter: 700},
			{MangaID: "c", Title: "Bleach", CurrentChapter: 686},
			{MangaID: "d", Title: "HxH", CurrentChapter: 400},
		},
	}
	next, _ := m.Update(libraryResultMsg{groups: groups, total: 4})
	m2 := next.(Model)
	assert.Len(t, m2.dashboardReading, 3) // capped at 3
	assert.Equal(t, "One Piece", m2.dashboardReading[0].Title)
}
