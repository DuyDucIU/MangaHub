package main

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func sidebarItems(m Model) []string {
	if m.token == "" {
		return []string{"Search", "Register", "Login"}
	}
	return []string{"Search", "Library", "Chat", "Logout"}
}

func updateMenu(m Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		items := sidebarItems(m)
		switch msg.String() {
		case "up":
			if m.sidebarIdx > 0 {
				m.sidebarIdx--
			}
		case "down":
			if m.sidebarIdx < len(items)-1 {
				m.sidebarIdx++
			}
		case "enter":
			return activateSidebarItem(m)
		}
	}
	return m, nil
}

func activateSidebarItem(m Model) (Model, tea.Cmd) {
	items := sidebarItems(m)
	if m.sidebarIdx >= len(items) {
		return m, nil
	}
	switch items[m.sidebarIdx] {
	case "Search":
		m.currentView = viewSearch
		m.searchInputs = initSearchInputs()
		m.searchFocus = 0
		m.searchResults = nil
		m.searchPage = 1
		m.searchTotal = 0
	case "Register":
		m.currentView = viewRegister
		m.authInputs = initRegisterInputs()
		m.authFocus = 0
		m.authErr = ""
	case "Login":
		m.currentView = viewLogin
		m.authInputs = initLoginInputs()
		m.authFocus = 0
		m.authErr = ""
	case "Library":
		m.currentView = viewLibrary
		m.libraryCursor = 0
		return m, cmdFetchLibrary(m.baseURL, m.token)
	case "Chat":
		m.currentView = viewChat
		m.chatPrompting = true
		m.chatMessages = nil
		inp := newChatPromptInput()
		m.chatPromptInput = inp
		return m, textinput.Blink
	case "Logout":
		m = openModalConfirm(m, confirmLogout, "")
		return m, nil
	}
	return m, nil
}

func renderSidebar(m Model, width, height int) string {
	items := sidebarItems(m)
	var sb strings.Builder
	for i, item := range items {
		label := "  " + item
		if i == m.sidebarIdx {
			sb.WriteString(styleSidebarSelected.Width(width).Render(label) + "\n")
		} else {
			sb.WriteString(styleSidebarItem.Width(width).Render(label) + "\n")
		}
	}
	// pad to fill height
	written := len(items)
	for i := written; i < height; i++ {
		sb.WriteString(styleSidebarItem.Width(width).Render("") + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

func renderMenu(m Model, width, height int) string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(styleTitle.Render("  Welcome to MangaHub") + "\n\n")
	sb.WriteString(styleMutedText.Render("  Server: "+m.baseURL) + "\n\n")
	if m.token == "" {
		sb.WriteString(styleNormal.Render("  Select an option from the menu.") + "\n")
	} else {
		sb.WriteString(styleNormal.Render("  Logged in as: "+m.username) + "\n")
	}
	return lipgloss.NewStyle().Width(width).Render(sb.String())
}
