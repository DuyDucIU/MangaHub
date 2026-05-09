package main

import tea "github.com/charmbracelet/bubbletea"

func cmdFetchLibrary(baseURL, token string) tea.Cmd { return nil }

func flattenLibrary(groups map[string][]libraryItem) []libraryItem {
	order := []string{"reading", "completed", "plan_to_read", "on_hold", "dropped"}
	var flat []libraryItem
	for _, s := range order {
		flat = append(flat, groups[s]...)
	}
	return flat
}

func updateLibrary(m Model, msg tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func renderLibrary(m Model, width, height int) string         { return "" }
