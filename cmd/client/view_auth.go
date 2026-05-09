package main

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func initLoginInputs() []textinput.Model    { return nil }
func initRegisterInputs() []textinput.Model { return nil }

func cmdLogin(baseURL, username, password string) tea.Cmd                    { return nil }
func cmdRegister(baseURL, username, email, password string) tea.Cmd { return nil }

func parseJWTClaims(token string) (userID, username string, ok bool) { return }

func updateAuth(m Model, msg tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func renderAuth(m Model, width, height int) string         { return "" }
