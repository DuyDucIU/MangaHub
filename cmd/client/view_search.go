package main

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

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

func cmdAddToLibrary(baseURL, token, mangaID, title, status string, chapter int) tea.Cmd {
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
		return addLibraryMsg{title: title}
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
			m.searchErr = "Service unavailable — could not reach server"
			m.searchLoading = false
			return m, nil
		}
		m.searchErr = ""
		m.searchResults = msg.results
		m.searchTotal = msg.total
		m.searchPage = msg.page
		m.searchCursor = 0
		m.searchLoading = false
		if len(msg.results) > 0 {
			id := msg.results[0].ID
			m.detailPending = id
			m.detailLoading = true
			return m, tea.Batch(cmdFetchDetail(m.baseURL, m.token, id), m.spinner.Tick)
		}
		return m, nil

	case detailResultMsg:
		if msg.manga.ID != m.detailPending {
			return m, nil // stale response — ignore
		}
		if msg.err != "" {
			m.detailLoading = false
			return m, nil
		}
		m.detailManga = msg.manga
		m.detailEntry = msg.entry
		m.detailLoading = false
		return m, nil

	case tea.KeyMsg:
		return updateSearchKeys(m, msg)
	}

	if m.searchInputFocused && len(m.searchInputs) > 0 {
		var cmd tea.Cmd
		m.searchInputs[m.searchFocus], cmd = m.searchInputs[m.searchFocus].Update(msg)
		return m, cmd
	}
	return m, nil
}

func updateSearchKeys(m Model, msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.searchInputFocused {
		switch msg.String() {
		case "esc":
			m.searchInputs[m.searchFocus].Blur()
			m.searchInputFocused = false
			return m, nil
		case "enter":
			m.searchInputFocused = false
			m.searchInputs[m.searchFocus].Blur()
			q := strings.TrimSpace(m.searchInputs[0].Value())
			genre := strings.TrimSpace(m.searchInputs[1].Value())
			status := strings.TrimSpace(m.searchInputs[2].Value())
			m.searchPage = 1
			m.searchLoading = true
			m.searchPerformed = true
			m.searchLastQuery = q
			m.detailManga = mangaItem{}
			m.detailEntry = nil
			m.detailPending = ""
			return m, tea.Batch(cmdSearch(m.baseURL, m.token, q, genre, status, 1), m.spinner.Tick)
		default:
			var cmd tea.Cmd
			m.searchInputs[m.searchFocus], cmd = m.searchInputs[m.searchFocus].Update(msg)
			return m, cmd
		}
	}

	switch msg.String() {
	case "esc":
		m.currentView = viewMenu
	case "/":
		m.searchInputFocused = true
		m.searchInputs[m.searchFocus].Focus()
		return m, textinput.Blink
	case "up":
		if m.searchCursor > 0 {
			m.searchCursor--
			id := m.searchResults[m.searchCursor].ID
			m.detailPending = id
			m.detailLoading = true
			return m, tea.Batch(cmdFetchDetail(m.baseURL, m.token, id), m.spinner.Tick)
		}
	case "down":
		if m.searchCursor < len(m.searchResults)-1 {
			m.searchCursor++
			id := m.searchResults[m.searchCursor].ID
			m.detailPending = id
			m.detailLoading = true
			return m, tea.Batch(cmdFetchDetail(m.baseURL, m.token, id), m.spinner.Tick)
		}
	case "right":
		pages := totalPages(m.searchTotal, searchPageSize)
		if m.searchPage < pages {
			m.searchPage++
			q := strings.TrimSpace(m.searchInputs[0].Value())
			genre := strings.TrimSpace(m.searchInputs[1].Value())
			status := strings.TrimSpace(m.searchInputs[2].Value())
			m.searchLoading = true
			return m, tea.Batch(cmdSearch(m.baseURL, m.token, q, genre, status, m.searchPage), m.spinner.Tick)
		}
	case "left":
		if m.searchPage > 1 {
			m.searchPage--
			q := strings.TrimSpace(m.searchInputs[0].Value())
			genre := strings.TrimSpace(m.searchInputs[1].Value())
			status := strings.TrimSpace(m.searchInputs[2].Value())
			m.searchLoading = true
			return m, tea.Batch(cmdSearch(m.baseURL, m.token, q, genre, status, m.searchPage), m.spinner.Tick)
		}
	case "a":
		if m.token != "" && m.detailManga.ID != "" {
			isAdding := m.detailEntry == nil
			chapter := 0
			status := "reading"
			if !isAdding && m.detailEntry != nil {
				chapter = m.detailEntry.CurrentChapter
				status = m.detailEntry.Status
			}
			m = openModalUpdateProgress(m, isAdding, chapter, status)
			return m, textinput.Blink
		}
	}
	return m, nil
}

func renderSearch(m Model, width, height int) string {
	leftWidth := width * 38 / 100
	rightWidth := width - leftWidth - 1

	left := lipgloss.NewStyle().Width(leftWidth).Height(height).Render(
		renderSearchLeft(m, leftWidth, height),
	)
	divider := lipgloss.NewStyle().
		Width(1).Height(height).
		BorderLeft(true).
		BorderStyle(lipgloss.NormalBorder()).
		Render("")
	right := lipgloss.NewStyle().Width(rightWidth).Height(height).Render(
		renderSearchRight(m, rightWidth, height),
	)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, divider, right)
}

func renderSearchLeft(m Model, width, height int) string {
	var sb strings.Builder
	inputPrefix := "  / "
	if m.searchInputFocused {
		inputPrefix = styleTitle.Render("  / ")
	}
	sb.WriteString(inputPrefix + m.searchInputs[0].View() + "\n")
	sb.WriteString(styleMutedText.Render(strings.Repeat("─", width)) + "\n")

	if m.searchLoading {
		sb.WriteString("\n  " + m.spinner.View() + " Searching...\n")
		return sb.String()
	}
	if m.searchErr != "" {
		sb.WriteString("\n" + styleError.Render("  "+m.searchErr) + "\n")
		return sb.String()
	}
	if len(m.searchResults) == 0 {
		if m.searchPerformed {
			label := "  No results found"
			if m.searchLastQuery != "" {
				label = fmt.Sprintf("  No results found for %q", m.searchLastQuery)
			}
			sb.WriteString("\n" + styleMutedText.Render(label) + "\n")
		} else {
			sb.WriteString("\n" + styleMutedText.Render("  Search for manga using [/]") + "\n")
		}
		return sb.String()
	}

	pages := totalPages(m.searchTotal, searchPageSize)
	sb.WriteString(styleMutedText.Render(fmt.Sprintf(
		"  %d result(s) — %d/%d", m.searchTotal, m.searchPage, pages)) + "\n")

	for i, item := range m.searchResults {
		if i == m.searchCursor {
			sb.WriteString(styleSidebarSelected.Width(width).Render("> " + truncate(item.Title, width-4)) + "\n")
		} else {
			sb.WriteString(styleNormal.Render("  " + truncate(item.Title, width-4)) + "\n")
		}
	}
	hint := "  [↑↓] Navigate  [←→] Page  [/] Search"
	if m.token != "" {
		hint += "  [a] Add"
	}
	sb.WriteString("\n" + styleMutedText.Render(hint) + "\n")
	return sb.String()
}

func renderSearchRight(m Model, width, height int) string {
	if m.detailLoading {
		return "\n  " + m.spinner.View() + " Loading...\n"
	}
	if m.detailManga.ID == "" {
		return "\n" + styleMutedText.Render("  Select a result to see details")
	}

	d := m.detailManga
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(styleTitle.Render("  "+truncate(d.Title, width-4)) + "\n\n")
	sb.WriteString(styleNormal.Render(fmt.Sprintf("  Author:   %s", d.Author)) + "\n")
	sb.WriteString(styleNormal.Render(fmt.Sprintf("  Status:   %s", d.Status)) + "\n")
	sb.WriteString(styleNormal.Render(fmt.Sprintf("  Genres:   %s", strings.Join(d.Genres, ", "))) + "\n")
	sb.WriteString(styleNormal.Render(fmt.Sprintf("  Chapters: %d", d.TotalChapters)) + "\n")
	if d.Description != "" {
		sb.WriteString("\n" + styleNormal.Render("  "+truncate(d.Description, width-4)) + "\n")
	}
	sb.WriteString("\n")
	if m.token != "" {
		if m.detailEntry != nil {
			sb.WriteString(styleMutedText.Render(fmt.Sprintf(
				"  In library: Ch.%d · %s", m.detailEntry.CurrentChapter, capitalizeFirst(strings.ReplaceAll(m.detailEntry.Status, "_", " ")))) + "\n")
			sb.WriteString(styleNormal.Render("  [a] Update Progress") + "\n")
		} else {
			sb.WriteString(styleNormal.Render("  [a] Add to Library") + "\n")
		}
		sb.WriteString(styleNormal.Render("  [c] Join Chat") + "\n")
	}
	return sb.String()
}

func capitalizeFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}

func friendlyTime(raw string) string {
	if raw == "" {
		return ""
	}
	var t time.Time
	var err error
	for _, f := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
		t, err = time.Parse(f, raw)
		if err == nil {
			break
		}
	}
	if err != nil {
		return raw
	}
	diff := time.Since(t)
	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		return fmt.Sprintf("%dm ago", int(diff.Minutes()))
	case diff < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(diff.Hours()))
	case diff < 2*24*time.Hour:
		return "yesterday"
	case diff < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(diff.Hours()/24))
	case diff < 30*24*time.Hour:
		return fmt.Sprintf("%dw ago", int(diff.Hours()/24/7))
	default:
		if t.Year() == time.Now().Year() {
			return t.Format("Jan 2")
		}
		return t.Format("Jan 2, 2006")
	}
}
