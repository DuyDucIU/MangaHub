package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

type registerResponse struct {
	Message string `json:"message"`
	UserID  string `json:"user_id"`
	Error   string `json:"error"`
}

type loginResponse struct {
	Token string `json:"token"`
	Error string `json:"error"`
}

func (a *App) doRegister() {
	fmt.Println("\n--- Register ---")
	username := a.prompt("Username (min 3 chars): ")
	email := a.prompt("Email: ")
	password := a.prompt("Password (min 8 chars): ")

	var errs []string
	if len(username) < 3 {
		errs = append(errs, "Username must be at least 3 characters.")
	}
	if !strings.Contains(email, "@") || !strings.Contains(email, ".") {
		errs = append(errs, "Email is invalid.")
	}
	if len(password) < 8 {
		errs = append(errs, "Password must be at least 8 characters.")
	}
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Println(" -", e)
		}
		return
	}

	var resp registerResponse
	status, err := postJSON(a.BaseURL+"/auth/register", "", map[string]string{
		"username": username,
		"email":    email,
		"password": password,
	}, &resp)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	if status != 201 {
		fmt.Println("Registration failed:")
		for _, e := range strings.Split(resp.Error, "; ") {
			fmt.Println(" -", e)
		}
		return
	}
	fmt.Println("Registered successfully! You can now log in.")
}

func (a *App) doLogin() {
	fmt.Println("\n--- Login ---")
	username := a.prompt("Username: ")
	password := a.prompt("Password: ")

	var errs []string
	if username == "" {
		errs = append(errs, "Username is required.")
	}
	if password == "" {
		errs = append(errs, "Password is required.")
	}
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Println(" -", e)
		}
		return
	}

	var resp loginResponse
	status, err := postJSON(a.BaseURL+"/auth/login", "", map[string]string{
		"username": username,
		"password": password,
	}, &resp)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	if status != 200 {
		fmt.Println("Login failed:")
		for _, e := range strings.Split(resp.Error, "; ") {
			fmt.Println(" -", e)
		}
		if status == 404 {
			fmt.Println("Tip: No account found. Use option 2 to register.")
		}
		return
	}

	a.Token = resp.Token
	var ok bool
	a.UserID, a.Username, ok = parseJWTClaims(resp.Token)
	if !ok || a.Username == "" {
		fmt.Println("Warning: could not parse token claims — logged in but identity unknown.")
	}
	fmt.Printf("Welcome, %s!\n", a.Username)

	a.connectTCP()
	a.connectUDP()
}

func (a *App) doLogout() {
	a.cleanup()
	a.Token = ""
	a.UserID = ""
	a.Username = ""
	a.TCPConn = nil
	a.UDPConn = nil
	fmt.Println("Logged out.")
}

// parseJWTClaims decodes the JWT payload (middle segment) and extracts user_id and username.
// Does not verify the signature — the client trusts the server-issued token.
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
