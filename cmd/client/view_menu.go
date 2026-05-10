package main

import (
	"fmt"
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
		m.libraryLoading = true
		return m, tea.Batch(cmdFetchLibrary(m.baseURL, m.token), m.spinner.Tick)
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
		if i == m.sidebarIdx {
			sb.WriteString(styleSidebarSelected.Width(width).Render("> " + item) + "\n")
		} else {
			sb.WriteString(styleSidebarItem.Width(width).Render("  " + item) + "\n")
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
	if m.token == "" {
		return renderMenuGuest(width)
	}
	return renderDashboard(m, width)
}

func renderMenuGuest(width int) string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(styleTitle.Render("  Welcome to MangaHub") + "\n\n")
	sb.WriteString(styleNormal.Render("  Select an option from the menu.") + "\n\n")
	sb.WriteString(styleMutedText.Render("  Search manga · Register · Login") + "\n")
	return lipgloss.NewStyle().Width(width).Render(sb.String())
}

func renderDashboard(m Model, width int) string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(styleTitle.Render("  Welcome back, "+m.username) + "\n\n")

	// Continue Reading
	sb.WriteString(styleNormal.Render("  Continue Reading") + "\n")
	sb.WriteString(styleMutedText.Render("  "+strings.Repeat("─", width-4)) + "\n")
	if len(m.dashboardReading) == 0 {
		sb.WriteString(styleMutedText.Render("  No manga in reading list.") + "\n")
	} else {
		for _, item := range m.dashboardReading {
			chap := fmt.Sprintf("ch.%d", item.CurrentChapter)
			title := truncate(item.Title, width-10-len(chap))
			gap := width - 4 - lipgloss.Width(title) - lipgloss.Width(chap)
			if gap < 1 {
				gap = 1
			}
			sb.WriteString(styleNormal.Render(
				"  "+title+strings.Repeat(" ", gap)+chap) + "\n")
		}
	}
	sb.WriteString("\n")

	// Recent Notifications
	sb.WriteString(styleNormal.Render("  Recent Notifications") + "\n")
	sb.WriteString(styleMutedText.Render("  "+strings.Repeat("─", width-4)) + "\n")
	notifs := m.notifications
	if len(notifs) > 5 {
		notifs = notifs[:5]
	}
	if len(notifs) == 0 {
		sb.WriteString(styleMutedText.Render("  No notifications yet. Press n for history.") + "\n")
	} else {
		for _, n := range notifs {
			sb.WriteString(styleNotif.Render("  • "+truncate(n, width-6)) + "\n")
		}
	}

	return lipgloss.NewStyle().Width(width).Render(sb.String())
}
