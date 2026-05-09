package main

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type searchResponse struct {
	Results []mangaItem `json:"results"`
	Count   int         `json:"count"`
	Total   int         `json:"total"`
	Error   string      `json:"error"`
}

type mangaItem struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Author        string   `json:"author"`
	Genres        []string `json:"genres"`
	Status        string   `json:"status"`
	TotalChapters int      `json:"total_chapters"`
	Description   string   `json:"description"`
	CoverURL      string   `json:"cover_url,omitempty"`
}

type libraryResponse struct {
	ReadingLists map[string][]libraryItem `json:"reading_lists"`
	Total        int                      `json:"total"`
	Error        string                   `json:"error"`
}

type libraryItem struct {
	MangaID        string `json:"manga_id"`
	Title          string `json:"title"`
	CurrentChapter int    `json:"current_chapter"`
	Status         string `json:"status"`
	UpdatedAt      string `json:"updated_at"`
}

type apiError struct {
	Error string `json:"error"`
}

func (a *App) doSearch() {
	fmt.Println("\n--- Search Manga ---")
	q := a.prompt("Title / author (Enter to skip): ")
	genre := a.prompt("Genre (Enter to skip): ")
	status := a.prompt("Status — ongoing/completed (Enter to skip): ")

	params := url.Values{}
	if q != "" {
		params.Set("q", q)
	}
	if genre != "" {
		params.Set("genre", genre)
	}
	if status != "" {
		params.Set("status", status)
	}

	endpoint := a.BaseURL + "/manga"
	if len(params) > 0 {
		endpoint += "?" + params.Encode()
	}

	var resp searchResponse
	code, err := getJSON(endpoint, a.Token, &resp)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	if code != 200 {
		fmt.Println("Search failed:", resp.Error)
		return
	}
	if len(resp.Results) == 0 {
		fmt.Println("No results found.")
		return
	}

	fmt.Printf("\nFound %d result(s) (showing %d):\n\n", resp.Total, resp.Count)
	for i, m := range resp.Results {
		fmt.Printf("  %2d. %-35s  by %-20s  [%s]\n", i+1, m.Title, m.Author, m.ID)
	}

	choice := a.prompt("\nEnter number to view details (Enter to go back): ")
	if choice == "" {
		return
	}
	idx := 0
	fmt.Sscanf(choice, "%d", &idx)
	if idx < 1 || idx > len(resp.Results) {
		fmt.Println("Invalid selection.")
		return
	}
	a.doViewDetails(resp.Results[idx-1].ID)
}

func (a *App) doViewDetails(id string) {
	var m mangaItem
	code, err := getJSON(a.BaseURL+"/manga/"+id, a.Token, &m)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	if code != 200 {
		fmt.Println("Manga not found.")
		return
	}
	fmt.Printf("\n=== %s ===\n", m.Title)
	fmt.Printf("ID:       %s\n", m.ID)
	fmt.Printf("Author:   %s\n", m.Author)
	fmt.Printf("Genres:   %s\n", strings.Join(m.Genres, ", "))
	fmt.Printf("Status:   %s\n", m.Status)
	fmt.Printf("Chapters: %d\n", m.TotalChapters)
	if m.CoverURL != "" {
		fmt.Printf("Cover:    %s\n", m.CoverURL)
	}
	if m.Description != "" {
		fmt.Printf("\n%s\n", m.Description)
	}

	if a.Token == "" {
		return
	}

	entry := a.findInLibrary(id)
	if entry != nil {
		fmt.Printf("\n[In your library] Chapter: %d | Status: %s\n", entry.CurrentChapter, entry.Status)
		fmt.Println("\n1. Update progress")
		fmt.Println("0. Back")
		if a.prompt("> ") == "1" {
			a.doUpdateProgressFor(entry)
		}
	} else {
		fmt.Println("\n1. Add to library")
		fmt.Println("0. Back")
		if a.prompt("> ") == "1" {
			a.doAddToLibraryFor(id)
		}
	}
}

// findInLibrary returns the user's library entry for mangaID, or nil if not present.
func (a *App) findInLibrary(mangaID string) *libraryItem {
	var resp libraryResponse
	code, err := getJSON(a.BaseURL+"/users/library", a.Token, &resp)
	if err != nil || code != 200 {
		return nil
	}
	for _, items := range resp.ReadingLists {
		for i := range items {
			if items[i].MangaID == mangaID {
				return &items[i]
			}
		}
	}
	return nil
}

// doAddToLibraryFor adds a specific manga to the library with the manga ID pre-filled.
func (a *App) doAddToLibraryFor(mangaID string) {
	status := a.prompt("Status (reading / completed / plan_to_read / on_hold / dropped): ")
	if status == "" {
		return
	}
	chStr := a.prompt("Current chapter (default 0): ")
	chapter := 0
	if chStr != "" {
		if n, err := strconv.Atoi(chStr); err == nil {
			chapter = n
		} else {
			fmt.Println("Invalid chapter number, defaulting to 0.")
		}
	}

	var resp apiError
	code, err := postJSON(a.BaseURL+"/users/library", a.Token, map[string]interface{}{
		"manga_id":        mangaID,
		"status":          status,
		"current_chapter": chapter,
	}, &resp)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	if code != 201 {
		fmt.Println("Failed:", resp.Error)
		return
	}
	fmt.Printf("Added to library with status %q at chapter %d.\n", status, chapter)
}

// doUpdateProgressFor updates progress for a specific library entry with the manga ID pre-filled.
func (a *App) doUpdateProgressFor(entry *libraryItem) {
	fmt.Printf("Current: chapter %d | status: %s\n", entry.CurrentChapter, entry.Status)
	chStr := a.prompt(fmt.Sprintf("New chapter (Enter to keep %d): ", entry.CurrentChapter))
	chapter := entry.CurrentChapter
	if chStr != "" {
		if n, err := strconv.Atoi(chStr); err == nil {
			chapter = n
		} else {
			fmt.Println("Invalid chapter number, keeping current.")
		}
	}
	status := a.prompt(fmt.Sprintf("New status (Enter to keep %q): ", entry.Status))

	body := map[string]interface{}{
		"manga_id":        entry.MangaID,
		"current_chapter": chapter,
	}
	if status != "" {
		body["status"] = status
	}

	var resp apiError
	code, err := putJSON(a.BaseURL+"/users/progress", a.Token, body, &resp)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	if code != 200 {
		fmt.Println("Failed:", resp.Error)
		return
	}
	fmt.Printf("Progress updated: chapter %d | status: %s\n", chapter, func() string {
		if status != "" {
			return status
		}
		return entry.Status
	}())
}

func (a *App) doLibrary() {
	var resp libraryResponse
	code, err := getJSON(a.BaseURL+"/users/library", a.Token, &resp)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	if code != 200 {
		fmt.Println("Failed to fetch library:", resp.Error)
		return
	}
	if resp.Total == 0 {
		fmt.Println("\nYour library is empty. Add manga with option 4.")
		return
	}
	fmt.Printf("\n=== My Library (%d total) ===\n", resp.Total)
	for _, status := range []string{"reading", "completed", "plan_to_read", "on_hold", "dropped"} {
		items := resp.ReadingLists[status]
		if len(items) == 0 {
			continue
		}
		label := strings.ToUpper(strings.ReplaceAll(status, "_", " "))
		fmt.Printf("\n[%s]\n", label)
		for _, item := range items {
			fmt.Printf("  %-30s  ch.%-4d  (%s)\n", item.Title, item.CurrentChapter, item.MangaID)
		}
	}
}

func (a *App) doAddToLibrary() {
	fmt.Println("\n--- Add to Library ---")
	mangaID := a.prompt("Manga ID: ")
	status := a.prompt("Status (reading / completed / plan_to_read / on_hold / dropped): ")

	chStr := a.prompt("Current chapter (default 0): ")
	chapter := 0
	if chStr != "" {
		if n, err := strconv.Atoi(chStr); err == nil {
			chapter = n
		} else {
			fmt.Println("Invalid chapter number, defaulting to 0.")
		}
	}

	var resp apiError
	code, err := postJSON(a.BaseURL+"/users/library", a.Token, map[string]interface{}{
		"manga_id":        mangaID,
		"status":          status,
		"current_chapter": chapter,
	}, &resp)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	if code != 201 {
		fmt.Println("Failed:", resp.Error)
		return
	}
	fmt.Printf("Added %q to library with status %q at chapter %d.\n", mangaID, status, chapter)
}

func (a *App) doUpdateProgress() {
	fmt.Println("\n--- Update Progress ---")
	mangaID := a.prompt("Manga ID: ")
	chStr := a.prompt("Current chapter: ")
	chapter := 0
	if n, err := strconv.Atoi(chStr); err == nil {
		chapter = n
	} else {
		fmt.Println("Invalid chapter number, using 0.")
	}
	status := a.prompt("New status (Enter to keep current): ")

	body := map[string]interface{}{
		"manga_id":        mangaID,
		"current_chapter": chapter,
	}
	if status != "" {
		body["status"] = status
	}

	var resp apiError
	code, err := putJSON(a.BaseURL+"/users/progress", a.Token, body, &resp)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	if code != 200 {
		fmt.Println("Failed:", resp.Error)
		return
	}
	fmt.Printf("Progress updated: %s → chapter %d\n", mangaID, chapter)
}
