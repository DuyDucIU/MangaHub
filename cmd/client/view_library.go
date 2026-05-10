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
	flat := make([]libraryItem, 0)
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
		case "a":
			if m.libraryCursor < len(m.libraryFlat) {
				item := m.libraryFlat[m.libraryCursor]
				m = openModalUpdateProgress(m, false, item.CurrentChapter, item.Status)
			}
		case "d":
			if m.libraryCursor < len(m.libraryFlat) {
				item := m.libraryFlat[m.libraryCursor]
				m = openModalConfirm(m, confirmRemoveManga, item.MangaID)
			}
		}
	}
	return m, nil
}

func renderLibrary(m Model, width, height int) string {
	leftWidth := width * 38 / 100
	rightWidth := width - leftWidth - 1

	left := lipgloss.NewStyle().Width(leftWidth).Height(height).Render(
		renderLibraryLeft(m, leftWidth, height),
	)
	divider := lipgloss.NewStyle().
		Width(1).Height(height).
		BorderLeft(true).
		BorderStyle(lipgloss.NormalBorder()).
		Render("")
	right := lipgloss.NewStyle().Width(rightWidth).Height(height).Render(
		renderLibraryRight(m, rightWidth, height),
	)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, divider, right)
}

func renderLibraryLeft(m Model, width, height int) string {
	if m.libraryLoading {
		return "\n  " + m.spinner.View() + " Loading library...\n"
	}
	if m.libraryFlat == nil {
		return "\n" + styleMutedText.Render("  Loading library...")
	}
	if len(m.libraryFlat) == 0 {
		return "\n" + styleNormal.Render("  Your library is empty.\n  Search for manga to add.")
	}

	var sb strings.Builder
	flatIdx := 0
	for _, status := range libraryStatusOrder {
		items := m.libraryGroups[status]
		if len(items) == 0 {
			continue
		}
		label := strings.ToUpper(strings.ReplaceAll(status, "_", " "))
		sb.WriteString("\n  " + styleMutedText.Render(
			fmt.Sprintf("%s (%d)", label, len(items))) + "\n")
		for _, item := range items {
			if flatIdx == m.libraryCursor {
				sb.WriteString(styleSidebarSelected.Width(width).Render("> " + truncate(item.Title, width-4)) + "\n")
			} else {
				sb.WriteString(styleNormal.Render("  " + truncate(item.Title, width-4)) + "\n")
			}
			flatIdx++
		}
	}
	sb.WriteString("\n" + styleMutedText.Render("  [↑↓] Navigate  [a] Update  [d] Remove") + "\n")
	return sb.String()
}

func renderLibraryRight(m Model, width, height int) string {
	if len(m.libraryFlat) == 0 || m.libraryCursor >= len(m.libraryFlat) {
		return "\n" + styleMutedText.Render("  Select an item to see details")
	}
	item := m.libraryFlat[m.libraryCursor]
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(styleTitle.Render("  "+truncate(item.Title, width-4)) + "\n\n")
	sb.WriteString(styleNormal.Render(fmt.Sprintf("  Progress:  Ch.%d", item.CurrentChapter)) + "\n")
	sb.WriteString(styleNormal.Render(fmt.Sprintf("  Status:    %s",
		capitalizeFirst(strings.ReplaceAll(item.Status, "_", " ")))) + "\n")
	if item.UpdatedAt != "" {
		sb.WriteString(styleMutedText.Render("  Updated:   "+friendlyTime(item.UpdatedAt)) + "\n")
	}
	sb.WriteString("\n")
	sb.WriteString(styleNormal.Render("  [a] Update Progress") + "\n")
	sb.WriteString(styleNormal.Render("  [d] Remove from Library") + "\n")
	return sb.String()
}
