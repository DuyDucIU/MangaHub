package main

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const searchPageSize = 20

type searchResponse struct {
	Results  []mangaItem `json:"results"`
	Count    int         `json:"count"`
	Total    int         `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
	Error    string      `json:"error"`
}

type libraryResponse struct {
	ReadingLists map[string][]libraryItem `json:"reading_lists"`
	Total        int                      `json:"total"`
	Error        string                   `json:"error"`
}

type apiError struct {
	Error string `json:"error"`
}

func initSearchInputs() []textinput.Model {
	title := textinput.New()
	title.Placeholder = "title or author"
	title.Focus()
	title.Width = 30

	genre := textinput.New()
	genre.Placeholder = "e.g. Action, Romance"
	genre.Width = 30

	status := textinput.New()
	status.Placeholder = "ongoing / completed"
	status.Width = 30

	return []textinput.Model{title, genre, status}
}

func totalPages(total, pageSize int) int {
	if total == 0 || pageSize == 0 {
		return 1
	}
	p := total / pageSize
	if total%pageSize > 0 {
		p++
	}
	return p
}

func cmdSearch(baseURL, token, q, genre, status string, page int) tea.Cmd {
	return func() tea.Msg {
		params := url.Values{}
		if q != "" {
			params.Set("q", q)
		}
		if genre != "" {
			params.Set("genre", genre)
		}
		if status != "" {
			params.Set("status", status)
		}
		params.Set("page", strconv.Itoa(page))
		params.Set("page_size", strconv.Itoa(searchPageSize))

		endpoint := baseURL + "/manga?" + params.Encode()
		var resp searchResponse
		code, err := getJSON(endpoint, token, &resp)
		if err != nil {
			return searchResultMsg{err: err.Error()}
		}
		if code != 200 {
			return searchResultMsg{err: resp.Error}
		}
		return searchResultMsg{results: resp.Results, total: resp.Total, page: resp.Page}
	}
}

func cmdFetchDetail(baseURL, token, id string) tea.Cmd {
	return func() tea.Msg {
		var manga mangaItem
		code, err := getJSON(baseURL+"/manga/"+id, token, &manga)
		if err != nil {
			return detailResultMsg{err: err.Error()}
		}
		if code != 200 {
			return detailResultMsg{err: "manga not found"}
		}
		var entry *libraryItem
		if token != "" {
			entry = fetchLibraryEntry(baseURL, token, id)
		}
		return detailResultMsg{manga: manga, entry: entry}
	}
}

// fetchLibraryEntry checks the user's library for a specific manga ID.
func fetchLibraryEntry(baseURL, token, mangaID string) *libraryItem {
	var resp libraryResponse
	code, err := getJSON(baseURL+"/users/library", token, &resp)
	if err != nil || code != 200 {
		return nil
	}
	for _, items := range resp.ReadingLists {
		for i := range items {
			if items[i].MangaID == mangaID {
				return &items[i]
			}
		}
	}
	return nil
}

func cmdAddToLibrary(baseURL, token, mangaID, status string, chapter int) tea.Cmd {
	return func() tea.Msg {
		var resp apiError
		code, err := postJSON(baseURL+"/users/library", token, map[string]interface{}{
			"manga_id":        mangaID,
			"status":          status,
			"current_chapter": chapter,
		}, &resp)
		if err != nil {
			return addLibraryMsg{err: err.Error()}
		}
		if code != 201 {
			return addLibraryMsg{err: resp.Error}
		}
		return addLibraryMsg{}
	}
}

func cmdUpdateProgress(baseURL, token, mangaID, status string, chapter int) tea.Cmd {
	return func() tea.Msg {
		body := map[string]interface{}{
			"manga_id":        mangaID,
			"current_chapter": chapter,
		}
		if status != "" {
			body["status"] = status
		}
		var resp apiError
		code, err := putJSON(baseURL+"/users/progress", token, body, &resp)
		if err != nil {
			return updateProgressMsg{err: err.Error()}
		}
		if code != 200 {
			return updateProgressMsg{err: resp.Error}
		}
		return updateProgressMsg{}
	}
}

func updateSearch(m Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case searchResultMsg:
		if msg.err != "" {
			m.notification = "Search error: " + msg.err
			return m, nil
		}
		m.searchResults = msg.results
		m.searchTotal = msg.total
		m.searchPage = msg.page
		m.searchCursor = 0
		m.searchState = searchStateResults
		return m, nil

	case detailResultMsg:
		if msg.err != "" {
			m.notification = "Error: " + msg.err
			return m, nil
		}
		m.detailManga = msg.manga
		m.detailEntry = msg.entry
		m.searchState = searchStateDetail
		m.detailFocus = 0
		return m, nil

	case addLibraryMsg:
		if msg.err != "" {
			m.notification = "Add failed: " + msg.err
		} else {
			m.notification = fmt.Sprintf("Added %q to library.", m.detailManga.Title)
			// refresh entry
			return m, cmdFetchDetail(m.baseURL, m.token, m.detailManga.ID)
		}
		return m, nil

	case updateProgressMsg:
		if msg.err != "" {
			m.notification = "Update failed: " + msg.err
		} else {
			m.notification = fmt.Sprintf("Progress updated for %q.", m.detailManga.Title)
			return m, cmdFetchDetail(m.baseURL, m.token, m.detailManga.ID)
		}
		return m, nil

	case tea.KeyMsg:
		switch m.searchState {
		case searchStateForm:
			return updateSearchForm(m, msg)
		case searchStateResults:
			return updateSearchResults(m, msg)
		case searchStateDetail:
			return updateSearchDetail(m, msg)
		}
	}
	// propagate to focused input when in form state
	if m.searchState == searchStateForm && len(m.searchInputs) > 0 {
		var cmd tea.Cmd
		m.searchInputs[m.searchFocus], cmd = m.searchInputs[m.searchFocus].Update(msg)
		return m, cmd
	}
	return m, nil
}

func updateSearchForm(m Model, msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.currentView = viewMenu
		return m, nil
	case "tab", "down":
		m.searchInputs[m.searchFocus].Blur()
		m.searchFocus = (m.searchFocus + 1) % len(m.searchInputs)
		m.searchInputs[m.searchFocus].Focus()
		return m, textinput.Blink
	case "shift+tab", "up":
		m.searchInputs[m.searchFocus].Blur()
		m.searchFocus = (m.searchFocus - 1 + len(m.searchInputs)) % len(m.searchInputs)
		m.searchInputs[m.searchFocus].Focus()
		return m, textinput.Blink
	case "enter":
		q := strings.TrimSpace(m.searchInputs[0].Value())
		genre := strings.TrimSpace(m.searchInputs[1].Value())
		status := strings.TrimSpace(m.searchInputs[2].Value())
		m.searchPage = 1
		return m, cmdSearch(m.baseURL, m.token, q, genre, status, 1)
	default:
		var cmd tea.Cmd
		m.searchInputs[m.searchFocus], cmd = m.searchInputs[m.searchFocus].Update(msg)
		return m, cmd
	}
}

func updateSearchResults(m Model, msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.searchState = searchStateForm
	case "up", "k":
		if m.searchCursor > 0 {
			m.searchCursor--
		}
	case "down", "j":
		if m.searchCursor < len(m.searchResults)-1 {
			m.searchCursor++
		}
	case "right", "l":
		pages := totalPages(m.searchTotal, searchPageSize)
		if m.searchPage < pages {
			m.searchPage++
			q := strings.TrimSpace(m.searchInputs[0].Value())
			genre := strings.TrimSpace(m.searchInputs[1].Value())
			status := strings.TrimSpace(m.searchInputs[2].Value())
			return m, cmdSearch(m.baseURL, m.token, q, genre, status, m.searchPage)
		}
	case "left", "h":
		if m.searchPage > 1 {
			m.searchPage--
			q := strings.TrimSpace(m.searchInputs[0].Value())
			genre := strings.TrimSpace(m.searchInputs[1].Value())
			status := strings.TrimSpace(m.searchInputs[2].Value())
			return m, cmdSearch(m.baseURL, m.token, q, genre, status, m.searchPage)
		}
	case "enter":
		if m.searchCursor < len(m.searchResults) {
			id := m.searchResults[m.searchCursor].ID
			return m, cmdFetchDetail(m.baseURL, m.token, id)
		}
	}
	return m, nil
}

func updateSearchDetail(m Model, msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.searchState = searchStateResults
	case "1":
		// action 1: Add to library (if not in library) or Update progress (if in library)
		if m.token == "" {
			return m, nil
		}
		if m.detailEntry == nil {
			// add with defaults
			return m, cmdAddToLibrary(m.baseURL, m.token, m.detailManga.ID, "reading", 0)
		}
		// increment chapter by 1 as a quick update
		return m, cmdUpdateProgress(m.baseURL, m.token, m.detailManga.ID, "", m.detailEntry.CurrentChapter+1)
	}
	return m, nil
}

func renderSearch(m Model, width, height int) string {
	switch m.searchState {
	case searchStateForm:
		return renderSearchForm(m, width, height)
	case searchStateResults:
		return renderSearchResults(m, width, height)
	case searchStateDetail:
		return renderSearchDetail(m, width, height)
	}
	return ""
}

func renderSearchForm(m Model, width, height int) string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(styleTitle.Render("  Search Manga") + "\n\n")
	labels := []string{"Title / Author", "Genre", "Status"}
	for i, inp := range m.searchInputs {
		sb.WriteString(styleMutedText.Render("  "+labels[i]+":") + "\n")
		sb.WriteString("  " + inp.View() + "\n\n")
	}
	sb.WriteString(styleMutedText.Render("  Tab/↑↓ move · Enter search · Esc back") + "\n")
	return lipgloss.NewStyle().Width(width).Render(sb.String())
}

func renderSearchResults(m Model, width, height int) string {
	var sb strings.Builder
	pages := totalPages(m.searchTotal, searchPageSize)
	sb.WriteString(fmt.Sprintf("\n  Found %d result(s) — Page %d of %d\n\n",
		m.searchTotal, m.searchPage, pages))

	for i, item := range m.searchResults {
		line := fmt.Sprintf("  %-35s  %s", truncate(item.Title, 35), styleMutedText.Render(item.Author))
		if i == m.searchCursor {
			sb.WriteString(styleSidebarSelected.Width(width).Render(line) + "\n")
		} else {
			sb.WriteString(styleNormal.Render(line) + "\n")
		}
	}
	sb.WriteString("\n")
	sb.WriteString(styleMutedText.Render("  ↑↓ navigate · ←→ page · Enter detail · Esc back") + "\n")
	return lipgloss.NewStyle().Width(width).Render(sb.String())
}

func renderSearchDetail(m Model, width, height int) string {
	m2 := m.detailManga
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(styleTitle.Render("  "+m2.Title) + "\n\n")
	sb.WriteString(styleNormal.Render(fmt.Sprintf("  Author:   %s", m2.Author)) + "\n")
	sb.WriteString(styleNormal.Render(fmt.Sprintf("  Genres:   %s", strings.Join(m2.Genres, ", "))) + "\n")
	sb.WriteString(styleNormal.Render(fmt.Sprintf("  Status:   %s", m2.Status)) + "\n")
	sb.WriteString(styleNormal.Render(fmt.Sprintf("  Chapters: %d", m2.TotalChapters)) + "\n")
	if m2.CoverURL != "" {
		sb.WriteString(styleMutedText.Render("  Cover:    "+m2.CoverURL) + "\n")
	}
	if m2.Description != "" {
		sb.WriteString("\n" + styleNormal.Render("  "+truncate(m2.Description, width-4)) + "\n")
	}
	sb.WriteString("\n")
	if m.token != "" {
		if m.detailEntry != nil {
			sb.WriteString(styleNormal.Render(fmt.Sprintf(
				"  [In library] ch.%d · %s", m.detailEntry.CurrentChapter, m.detailEntry.Status)) + "\n\n")
			sb.WriteString(styleSidebarSelected.Render("  1. Quick +1 chapter") + "\n")
		} else {
			sb.WriteString(styleSidebarSelected.Render("  1. Add to library (reading, ch.0)") + "\n")
		}
	}
	sb.WriteString("\n" + styleMutedText.Render("  Esc back to results") + "\n")
	return lipgloss.NewStyle().Width(width).Render(sb.String())
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
