package main

import (
	"net/url"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
)

func initSearchInputs() []textinput.Model { return nil }

func cmdSearch(baseURL, token, q, genre, status string, page int) tea.Cmd { return nil }
func cmdFetchDetail(baseURL, token, id string) tea.Cmd                    { return nil }
func cmdAddToLibrary(baseURL, token, mangaID, status string, chapter int) tea.Cmd { return nil }
func cmdUpdateProgress(baseURL, token, mangaID, status string, chapter int) tea.Cmd { return nil }

func totalPages(total, pageSize int) int {
	if pageSize == 0 {
		return 1
	}
	p := total / pageSize
	if total%pageSize > 0 {
		p++
	}
	return p
}

var _ = url.Values{} // suppress unused import until Task 8

func updateSearch(m Model, msg tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func renderSearch(m Model, width, height int) string         { return "" }
