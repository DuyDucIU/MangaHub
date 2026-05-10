package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var libraryStatusOrder = []string{"reading", "completed", "plan_to_read", "on_hold", "dropped"}

func cmdFetchLibrary(baseURL, token string) tea.Cmd {
	return func() tea.Msg {
		var resp libraryResponse
		code, err := getJSON(baseURL+"/users/library", token, &resp)
		if err != nil {
			return libraryResultMsg{err: err.Error()}
		}
		if code != 200 {
			return libraryResultMsg{err: resp.Error}
		}
		return libraryResultMsg{groups: resp.ReadingLists, total: resp.Total}
	}
}

func flattenLibrary(groups map[string][]libraryItem) []libraryItem {
	var flat []libraryItem
	for _, s := range libraryStatusOrder {
		flat = append(flat, groups[s]...)
	}
	return flat
}

func updateLibrary(m Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.currentView = viewMenu
		case "up":
			if m.libraryCursor > 0 {
				m.libraryCursor--
			}
		case "down":
			if m.libraryCursor < len(m.libraryFlat)-1 {
				m.libraryCursor++
			}
		case "enter":
			if m.libraryCursor < len(m.libraryFlat) {
				id := m.libraryFlat[m.libraryCursor].MangaID
				m.currentView = viewSearch
				m.searchFocusPane = searchPaneDetail
				return m, cmdFetchDetail(m.baseURL, m.token, id)
			}
		}
	}
	return m, nil
}

func renderLibrary(m Model, width, height int) string {
	if m.libraryFlat == nil {
		return lipgloss.NewStyle().Width(width).Render(
			"\n" + styleMutedText.Render("  Loading library..."))
	}
	if len(m.libraryFlat) == 0 {
		return lipgloss.NewStyle().Width(width).Render(
			"\n" + styleNormal.Render("  Your library is empty. Add manga via Search."))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n  %s (%d total)\n",
		styleTitle.Render("My Library"), len(m.libraryFlat)))

	flatIdx := 0
	for _, status := range libraryStatusOrder {
		items := m.libraryGroups[status]
		if len(items) == 0 {
			continue
		}
		label := strings.ToUpper(strings.ReplaceAll(status, "_", " "))
		sb.WriteString("\n  " + styleMutedText.Render("["+label+"]") + "\n")
		for _, item := range items {
			line := fmt.Sprintf("  %-30s  ch.%-4d", truncate(item.Title, 30), item.CurrentChapter)
			if flatIdx == m.libraryCursor {
				sb.WriteString(styleSidebarSelected.Width(width).Render(line) + "\n")
			} else {
				sb.WriteString(styleNormal.Render(line) + "\n")
			}
			flatIdx++
		}
	}
	sb.WriteString("\n" + styleMutedText.Render("  ↑↓ navigate · Enter view detail · Esc back") + "\n")
	return lipgloss.NewStyle().Width(width).Render(sb.String())
}
