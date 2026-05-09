package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func newSearchModel() Model {
	m := New("http://localhost:8080")
	m.currentView = viewSearch
	m.searchState = searchStateForm
	m.searchInputs = initSearchInputs()
	m.width, m.height = 120, 40
	return m
}

func TestSearchResultsMsgSwitchesToResults(t *testing.T) {
	m := newSearchModel()
	results := []mangaItem{
		{ID: "one-piece", Title: "One Piece", Author: "Oda"},
		{ID: "naruto", Title: "Naruto", Author: "Kishimoto"},
	}
	next, _ := m.Update(searchResultMsg{results: results, total: 2, page: 1})
	m2 := next.(Model)
	assert.Equal(t, searchStateResults, m2.searchState)
	assert.Len(t, m2.searchResults, 2)
	assert.Equal(t, 0, m2.searchCursor)
	assert.Equal(t, 1, m2.searchPage)
	assert.Equal(t, 2, m2.searchTotal)
}

func TestSearchPaginationNext(t *testing.T) {
	m := newSearchModel()
	m.searchState = searchStateResults
	m.searchResults = make([]mangaItem, 20)
	m.searchPage = 1
	m.searchTotal = 50 // 3 pages of 20

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m2 := next.(Model)
	assert.Equal(t, 2, m2.searchPage)
	assert.NotNil(t, cmd) // fetches next page
}

func TestSearchPaginationPrevOnFirstPage(t *testing.T) {
	m := newSearchModel()
	m.searchState = searchStateResults
	m.searchPage = 1
	m.searchTotal = 50

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m2 := next.(Model)
	assert.Equal(t, 1, m2.searchPage) // stays on page 1
	assert.Nil(t, cmd)
}

func TestSearchDetailMsgSwitchesToDetail(t *testing.T) {
	m := newSearchModel()
	m.searchState = searchStateResults
	manga := mangaItem{ID: "one-piece", Title: "One Piece", Author: "Oda"}
	next, _ := m.Update(detailResultMsg{manga: manga, entry: nil})
	m2 := next.(Model)
	assert.Equal(t, searchStateDetail, m2.searchState)
	assert.Equal(t, "one-piece", m2.detailManga.ID)
	assert.Nil(t, m2.detailEntry)
}

func TestTotalPages(t *testing.T) {
	assert.Equal(t, 1, totalPages(0, 20))
	assert.Equal(t, 1, totalPages(20, 20))
	assert.Equal(t, 2, totalPages(21, 20))
	assert.Equal(t, 3, totalPages(50, 20))
}
