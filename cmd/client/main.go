package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
)

// App holds all shared client state. All methods on App live in other files in this package.
type App struct {
	BaseURL  string
	Token    string
	UserID   string
	Username string
	TCPConn  net.Conn
	UDPConn  *net.UDPConn
	scanner  *bufio.Scanner
}

func main() {
	app := &App{
		BaseURL: getenv("API_URL", "http://localhost:8080"),
		scanner: bufio.NewScanner(os.Stdin),
	}
	fmt.Println("=== MangaHub Terminal Client ===")
	fmt.Println("Server:", app.BaseURL)
	app.run()
}

func (a *App) run() {
	for {
		if a.Token == "" {
			if !a.guestMenu() {
				break
			}
		} else {
			if !a.mainMenu() {
				break
			}
		}
	}
	a.cleanup()
	fmt.Println("Goodbye!")
}

func (a *App) guestMenu() bool {
	fmt.Println()
	fmt.Println("1. Search manga")
	fmt.Println("2. Register")
	fmt.Println("3. Login")
	fmt.Println("0. Exit")
	switch a.prompt("> ") {
	case "1":
		a.doSearch()
	case "2":
		a.doRegister()
	case "3":
		a.doLogin()
	case "0":
		return false
	default:
		fmt.Println("Invalid choice.")
	}
	return true
}

func (a *App) mainMenu() bool {
	fmt.Printf("\n=== MangaHub === [%s]\n", a.Username)
	fmt.Println("1. Search manga")
	fmt.Println("2. View my library")
	fmt.Println("3. Update reading progress")
	fmt.Println("4. Add manga to library")
	fmt.Println("5. Enter chat room")
	fmt.Println("0. Logout")
	switch a.prompt("> ") {
	case "1":
		a.doSearch()
	case "2":
		a.doLibrary()
	case "3":
		a.doUpdateProgress()
	case "4":
		a.doAddToLibrary()
	case "5":
		a.enterChatRoom()
	case "0":
		a.doLogout()
		return true
	default:
		fmt.Println("Invalid choice.")
	}
	return true
}

func (a *App) prompt(p string) string {
	fmt.Print(p)
	a.scanner.Scan()
	return strings.TrimSpace(a.scanner.Text())
}

func (a *App) cleanup() {
	if a.TCPConn != nil {
		a.TCPConn.Close()
		a.TCPConn = nil
	}
	if a.UDPConn != nil {
		a.UDPConn.Close()
		a.UDPConn = nil
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
