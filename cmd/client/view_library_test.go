package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func newLibraryModel() Model {
	m := New("http://localhost:8080")
	m.currentView = viewLibrary
	m.token = "tok"
	m.width, m.height = 120, 40
	return m
}

func TestLibraryNavDown(t *testing.T) {
	m := newLibraryModel()
	m.libraryFlat = []libraryItem{{MangaID: "a"}, {MangaID: "b"}}
	m.libraryCursor = 0
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m2 := next.(Model)
	assert.Equal(t, 1, m2.libraryCursor)
}

func TestLibraryNavDoesNotGoBelowZero(t *testing.T) {
	m := newLibraryModel()
	m.libraryFlat = []libraryItem{{MangaID: "a"}}
	m.libraryCursor = 0
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m2 := next.(Model)
	assert.Equal(t, 0, m2.libraryCursor)
}

func TestLibraryNavDoesNotExceedLen(t *testing.T) {
	m := newLibraryModel()
	m.libraryFlat = []libraryItem{{MangaID: "a"}}
	m.libraryCursor = 0
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m2 := next.(Model)
	assert.Equal(t, 0, m2.libraryCursor)
}

func TestLibraryAKeyOpensUpdateModal(t *testing.T) {
	m := newLibraryModel()
	m.libraryFlat = []libraryItem{
		{MangaID: "one-piece", CurrentChapter: 100, Status: "reading"},
	}
	m.libraryCursor = 0
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m2 := next.(Model)
	assert.Equal(t, modalUpdateProgress, m2.activeModal)
	assert.False(t, m2.modalIsAdding)
	assert.Equal(t, 0, m2.modalCursor) // "reading" = index 0 in modalStatusOptions
}

func TestLibraryDKeyOpensConfirmModal(t *testing.T) {
	m := newLibraryModel()
	m.libraryFlat = []libraryItem{{MangaID: "one-piece", Title: "One Piece"}}
	m.libraryCursor = 0
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m2 := next.(Model)
	assert.Equal(t, modalConfirmAction, m2.activeModal)
	assert.Equal(t, confirmRemoveManga, m2.modalConfirmAct)
	assert.Equal(t, "one-piece", m2.modalMessage)
}

func TestFlattenLibrary(t *testing.T) {
	groups := map[string][]libraryItem{
		"reading":   {{MangaID: "a"}},
		"completed": {{MangaID: "b"}},
		"on_hold":   {{MangaID: "c"}},
	}
	flat := flattenLibrary(groups)
	assert.Equal(t, "a", flat[0].MangaID)
	assert.Equal(t, "b", flat[1].MangaID)
}
