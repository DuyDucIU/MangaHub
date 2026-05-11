package main

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func initLoginInputs() []textinput.Model {
	username := textinput.New()
	username.Placeholder = "username"
	username.Focus()
	username.Width = 30

	password := textinput.New()
	password.Placeholder = "password"
	password.EchoMode = textinput.EchoPassword
	password.Width = 30

	return []textinput.Model{username, password}
}

func initRegisterInputs() []textinput.Model {
	username := textinput.New()
	username.Placeholder = "username (min 3 chars)"
	username.Focus()
	username.Width = 30

	email := textinput.New()
	email.Placeholder = "email"
	email.Width = 30

	password := textinput.New()
	password.Placeholder = "password (min 8 chars)"
	password.EchoMode = textinput.EchoPassword
	password.Width = 30

	return []textinput.Model{username, email, password}
}

func cmdLogin(baseURL, username, password string) tea.Cmd {
	return func() tea.Msg {
		var resp struct {
			Token string `json:"token"`
			Error string `json:"error"`
		}
		code, err := postJSON(baseURL+"/auth/login", "", map[string]string{
			"username": username,
			"password": password,
		}, &resp)
		if err != nil {
			return errMsg{text: "Service unavailable — could not reach server"}
		}
		if code != 200 {
			return errMsg{text: resp.Error}
		}
		uid, uname, ok := parseJWTClaims(resp.Token)
		if !ok {
			return errMsg{text: "Could not parse server token"}
		}
		return loginSuccessMsg{token: resp.Token, userID: uid, username: uname}
	}
}

func cmdRegister(baseURL, username, email, password string) tea.Cmd {
	return func() tea.Msg {
		var resp struct {
			Message string `json:"message"`
			Error   string `json:"error"`
		}
		code, err := postJSON(baseURL+"/auth/register", "", map[string]string{
			"username": username,
			"email":    email,
			"password": password,
		}, &resp)
		if err != nil {
			return errMsg{text: "Service unavailable — could not reach server"}
		}
		if code != 201 {
			return errMsg{text: resp.Error}
		}
		return registerSuccessMsg{}
	}
}

func parseJWTClaims(token string) (userID, username string, ok bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return
	}
	var claims map[string]interface{}
	if err := json.Unmarshal(b, &claims); err != nil {
		return
	}
	userID, _ = claims["user_id"].(string)
	username, _ = claims["username"].(string)
	ok = true
	return
}

func updateAuth(m Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case loginSuccessMsg:
		m.token = msg.token
		m.userID = msg.userID
		m.username = msg.username
		m.currentView = viewMenu
		m.sidebarIdx = 0
		tcpAddr := getenv("TCP_ADDR", "localhost:9090")
		udpAddr := getenv("UDP_ADDR", "localhost:9091")
		m.tcpAddr = tcpAddr
		return m, tea.Batch(
			cmdConnectTCP(tcpAddr, m.token),
			cmdConnectUDP(udpAddr),
			cmdFetchLibrary(m.baseURL, m.token),
		)

	case registerSuccessMsg:
		m.currentView = viewLogin
		m.authInputs = initLoginInputs()
		m.authFocus = 0
		m.authErr = "Registered successfully! You can now login."
		return m, nil

	case errMsg:
		m.authErr = msg.text
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.currentView = viewMenu
			m.authErr = ""
			return m, nil

		case "tab", "down":
			m.authInputs[m.authFocus].Blur()
			m.authFocus = (m.authFocus + 1) % len(m.authInputs)
			m.authInputs[m.authFocus].Focus()
			return m, textinput.Blink

		case "shift+tab", "up":
			m.authInputs[m.authFocus].Blur()
			m.authFocus = (m.authFocus - 1 + len(m.authInputs)) % len(m.authInputs)
			m.authInputs[m.authFocus].Focus()
			return m, textinput.Blink

		case "enter":
			if m.authFocus < len(m.authInputs)-1 {
				// advance to next field
				m.authInputs[m.authFocus].Blur()
				m.authFocus++
				m.authInputs[m.authFocus].Focus()
				return m, textinput.Blink
			}
			// last field — submit
			return submitAuth(m)
		}

		// pass keystrokes to focused input
		var cmd tea.Cmd
		m.authInputs[m.authFocus], cmd = m.authInputs[m.authFocus].Update(msg)
		return m, cmd
	}

	// propagate to all inputs (handles cursor blink etc.)
	var cmds []tea.Cmd
	for i := range m.authInputs {
		var c tea.Cmd
		m.authInputs[i], c = m.authInputs[i].Update(msg)
		cmds = append(cmds, c)
	}
	return m, tea.Batch(cmds...)
}

func submitAuth(m Model) (Model, tea.Cmd) {
	if m.currentView == viewLogin {
		username := strings.TrimSpace(m.authInputs[0].Value())
		password := strings.TrimSpace(m.authInputs[1].Value())
		if username == "" || password == "" {
			m.authErr = "Username and password are required."
			return m, nil
		}
		return m, cmdLogin(m.baseURL, username, password)
	}
	// register
	username := strings.TrimSpace(m.authInputs[0].Value())
	email := strings.TrimSpace(m.authInputs[1].Value())
	password := strings.TrimSpace(m.authInputs[2].Value())
	var errs []string
	if len(username) < 3 {
		errs = append(errs, "Username must be at least 3 chars")
	}
	if !strings.Contains(email, "@") {
		errs = append(errs, "Email is invalid")
	}
	if len(password) < 8 {
		errs = append(errs, "Password must be at least 8 chars")
	}
	if len(errs) > 0 {
		m.authErr = strings.Join(errs, " · ")
		return m, nil
	}
	return m, cmdRegister(m.baseURL, username, email, password)
}

func renderAuth(m Model, width, height int) string {
	title := "Login"
	if m.currentView == viewRegister {
		title = "Register"
	}
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(styleTitle.Render("  "+title) + "\n\n")

	for i, inp := range m.authInputs {
		label := ""
		switch {
		case m.currentView == viewLogin && i == 0:
			label = "Username"
		case m.currentView == viewLogin && i == 1:
			label = "Password"
		case m.currentView == viewRegister && i == 0:
			label = "Username"
		case m.currentView == viewRegister && i == 1:
			label = "Email"
		case m.currentView == viewRegister && i == 2:
			label = "Password"
		}
		sb.WriteString(styleMutedText.Render("  "+label+":") + "\n")
		sb.WriteString("  " + inp.View() + "\n\n")
	}

	if m.authErr != "" {
		sb.WriteString(styleError.Render("  "+m.authErr) + "\n\n")
	}
	sb.WriteString(styleMutedText.Render("  [Tab]/[↑↓] Move · [Enter] Submit · [Esc] Back") + "\n")
	return lipgloss.NewStyle().Width(width).Render(sb.String())
}
