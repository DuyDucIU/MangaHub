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

func TestSearchPaginationNext(t *testing.T) {
	m := newSearchModel()
	m.searchFocusPane = searchPaneResults
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
	m.searchFocusPane = searchPaneResults
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
