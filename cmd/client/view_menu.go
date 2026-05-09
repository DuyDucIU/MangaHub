package main

import tea "github.com/charmbracelet/bubbletea"

func sidebarItems(m Model) []string {
	if m.token == "" {
		return []string{"Search", "Register", "Login"}
	}
	return []string{"Search", "Library", "Chat", "Logout"}
}

func updateMenu(m Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	return m, nil
}

func renderSidebar(m Model, width, height int) string {
	return ""
}

func renderMenu(m Model, width, height int) string {
	return ""
}
