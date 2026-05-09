package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestLibraryResultMsg(t *testing.T) {
	m := New("http://localhost:8080")
	m.currentView = viewLibrary
	m.token = "tok"

	groups := map[string][]libraryItem{
		"reading":   {{MangaID: "one-piece", Title: "One Piece", CurrentChapter: 1096}},
		"completed": {{MangaID: "naruto", Title: "Naruto", CurrentChapter: 700}},
	}
	next, _ := m.Update(libraryResultMsg{groups: groups, total: 2})
	m2 := next.(Model)
	assert.Equal(t, 2, len(m2.libraryFlat))
	assert.Equal(t, 0, m2.libraryCursor)
}

func TestLibraryNavDown(t *testing.T) {
	m := New("http://localhost:8080")
	m.currentView = viewLibrary
	m.libraryFlat = []libraryItem{
		{MangaID: "one-piece"},
		{MangaID: "naruto"},
	}
	m.libraryCursor = 0

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m2 := next.(Model)
	assert.Equal(t, 1, m2.libraryCursor)
}

func TestFlattenLibrary(t *testing.T) {
	groups := map[string][]libraryItem{
		"reading":   {{MangaID: "a"}},
		"completed": {{MangaID: "b"}},
		"on_hold":   {{MangaID: "c"}},
	}
	flat := flattenLibrary(groups)
	// reading comes first, then completed
	assert.Equal(t, "a", flat[0].MangaID)
	assert.Equal(t, "b", flat[1].MangaID)
}
