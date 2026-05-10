package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func newSearchModel() Model {
	m := New("http://localhost:8080")
	m.currentView = viewSearch
	m.searchInputs = initSearchInputs()
	m.width, m.height = 120, 40
	return m
}

func TestSearchResultsMsgSetsResults(t *testing.T) {
	m := newSearchModel()
	results := []mangaItem{
		{ID: "one-piece", Title: "One Piece", Author: "Oda"},
		{ID: "naruto", Title: "Naruto", Author: "Kishimoto"},
	}
	next, _ := m.Update(searchResultMsg{results: results, total: 2, page: 1})
	m2 := next.(Model)
	assert.Len(t, m2.searchResults, 2)
	assert.Equal(t, 0, m2.searchCursor)
	assert.Equal(t, 1, m2.searchPage)
	assert.Equal(t, 2, m2.searchTotal)
	assert.False(t, m2.searchLoading)
}

func TestSearchCursorMovesDown(t *testing.T) {
	m := newSearchModel()
	m.searchResults = []mangaItem{
		{ID: "one-piece", Title: "One Piece"},
		{ID: "naruto", Title: "Naruto"},
	}
	m.searchCursor = 0

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m2 := next.(Model)
	assert.Equal(t, 1, m2.searchCursor)
	assert.NotNil(t, cmd) // fires cmdFetchDetail
	assert.Equal(t, "naruto", m2.detailPending)
}

func TestSearchCursorDoesNotGoAboveZero(t *testing.T) {
	m := newSearchModel()
	m.searchResults = []mangaItem{{ID: "one-piece"}}
	m.searchCursor = 0

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m2 := next.(Model)
	assert.Equal(t, 0, m2.searchCursor)
}

func TestDetailResultIgnoredWhenStale(t *testing.T) {
	m := newSearchModel()
	m.detailPending = "naruto"
	m.detailManga = mangaItem{ID: "old"}

	next, _ := m.Update(detailResultMsg{manga: mangaItem{ID: "one-piece"}})
	m2 := next.(Model)
	assert.Equal(t, "old", m2.detailManga.ID)
}

func TestDetailResultAcceptedWhenCurrent(t *testing.T) {
	m := newSearchModel()
	m.detailPending = "one-piece"

	next, _ := m.Update(detailResultMsg{manga: mangaItem{ID: "one-piece", Title: "One Piece"}})
	m2 := next.(Model)
	assert.Equal(t, "one-piece", m2.detailManga.ID)
	assert.False(t, m2.detailLoading)
}

func TestSearchSlashFocusesInput(t *testing.T) {
	m := newSearchModel()
	m.searchInputFocused = false

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m2 := next.(Model)
	assert.True(t, m2.searchInputFocused)
}

func TestSearchAKeyOpensAddModalWhenNotInLibrary(t *testing.T) {
	m := newSearchModel()
	m.token = "tok"
	m.detailManga = mangaItem{ID: "one-piece"}
	m.detailEntry = nil

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m2 := next.(Model)
	assert.Equal(t, modalUpdateProgress, m2.activeModal)
	assert.True(t, m2.modalIsAdding)
}

func TestSearchAKeyOpensUpdateModalWhenInLibrary(t *testing.T) {
	m := newSearchModel()
	m.token = "tok"
	m.detailManga = mangaItem{ID: "one-piece"}
	m.detailEntry = &libraryItem{CurrentChapter: 100, Status: "reading"}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m2 := next.(Model)
	assert.Equal(t, modalUpdateProgress, m2.activeModal)
	assert.False(t, m2.modalIsAdding)
}

func TestSearchPaginationNext(t *testing.T) {
	m := newSearchModel()
	m.searchResults = make([]mangaItem, 20)
	m.searchPage = 1
	m.searchTotal = 50

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m2 := next.(Model)
	assert.Equal(t, 2, m2.searchPage)
	assert.NotNil(t, cmd)
}

func TestSearchPaginationPrevOnFirstPage(t *testing.T) {
	m := newSearchModel()
	m.searchPage = 1
	m.searchTotal = 50

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m2 := next.(Model)
	assert.Equal(t, 1, m2.searchPage)
	assert.Nil(t, cmd)
}

func TestTotalPages(t *testing.T) {
	assert.Equal(t, 1, totalPages(0, 20))
	assert.Equal(t, 1, totalPages(20, 20))
	assert.Equal(t, 2, totalPages(21, 20))
	assert.Equal(t, 3, totalPages(50, 20))
}
