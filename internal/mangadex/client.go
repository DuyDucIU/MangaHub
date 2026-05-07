package mangadex

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"mangahub/pkg/models"
)

const baseURL = "https://api.mangadex.org"

// Client is a minimal HTTP client for the MangaDx public API.
type Client struct {
	base string
	http *http.Client
}

func NewClient() *Client {
	return &Client{base: baseURL, http: &http.Client{Timeout: 15 * time.Second}}
}

// NewClientWithBaseURL creates a client pointing at a custom base URL.
// Intended for unit tests that spin up a local mock server.
func NewClientWithBaseURL(base string) *Client {
	return &Client{base: base, http: &http.Client{Timeout: 15 * time.Second}}
}

// MangaDx API response structures — only the fields we need.

type apiResponse struct {
	Result string      `json:"result"`
	Data   []mangaData `json:"data"`
}

type mangaData struct {
	ID            string          `json:"id"`
	Attributes    mangaAttributes `json:"attributes"`
	Relationships []relationship  `json:"relationships"`
}

type mangaAttributes struct {
	Title          map[string]string `json:"title"`
	Description    map[string]string `json:"description"`
	Status         string            `json:"status"`
	LastChapter    *string           `json:"lastChapter"`
	Tags           []tag             `json:"tags"`
}

type tag struct {
	Attributes tagAttributes `json:"attributes"`
}

type tagAttributes struct {
	Name  map[string]string `json:"name"`
	Group string            `json:"group"`
}

type relationship struct {
	ID         string                   `json:"id"`
	Type       string                   `json:"type"`
	Attributes *relationshipAttributes  `json:"attributes"`
}

type relationshipAttributes struct {
	Name     string `json:"name"`     // author
	FileName string `json:"fileName"` // cover_art
}

// SearchPopular fetches the most-followed manga from MangaDx.
func (c *Client) SearchPopular(limit int) ([]models.Manga, error) {
	url := fmt.Sprintf(
		"%s/manga?limit=%d&order[followedCount]=desc&includes[]=author&includes[]=cover_art&contentRating[]=safe&contentRating[]=suggestive",
		c.base, limit,
	)

	resp, err := c.http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("mangadex request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mangadex returned status %d", resp.StatusCode)
	}

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return mapToModels(apiResp.Data), nil
}

// FetchByID fetches a single manga by its MangaDx UUID.
func (c *Client) FetchByID(mangadexID string) (*models.Manga, error) {
	url := fmt.Sprintf("%s/manga/%s?includes[]=author&includes[]=cover_art", c.base, mangadexID)

	resp, err := c.http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("mangadex request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("manga %q not found on MangaDx", mangadexID)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mangadex returned status %d", resp.StatusCode)
	}

	// Single-item response wraps in "data" directly, not an array.
	var singleResp struct {
		Result string    `json:"result"`
		Data   mangaData `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&singleResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	results := mapToModels([]mangaData{singleResp.Data})
	if len(results) == 0 {
		return nil, fmt.Errorf("no usable data returned for %q", mangadexID)
	}
	return &results[0], nil
}

// mapToModels converts MangaDx API records to our internal Manga model.
func mapToModels(data []mangaData) []models.Manga {
	out := make([]models.Manga, 0, len(data))
	for _, d := range data {
		m := models.Manga{
			ID:       titleToSlug(englishTitle(d.Attributes.Title)),
			Title:    englishTitle(d.Attributes.Title),
			Author:   extractAuthor(d.Relationships),
			Genres:   extractGenres(d.Attributes.Tags),
			Status:   normalizeStatus(d.Attributes.Status),
			CoverURL: extractCoverURL(d.ID, d.Relationships),
		}
		if m.Title == "" || m.ID == "" {
			continue
		}
		if desc := d.Attributes.Description["en"]; desc != "" {
			// Trim to 300 chars to keep descriptions concise.
			if len(desc) > 300 {
				desc = desc[:297] + "..."
			}
			m.Description = desc
		}
		if d.Attributes.LastChapter != nil && *d.Attributes.LastChapter != "" {
			fmt.Sscanf(*d.Attributes.LastChapter, "%d", &m.TotalChapters)
		}
		out = append(out, m)
	}
	return out
}

func englishTitle(titles map[string]string) string {
	if t := titles["en"]; t != "" {
		return t
	}
	// Fall back to first available title.
	for _, v := range titles {
		return v
	}
	return ""
}

var nonAlphanumRe = regexp.MustCompile(`[^a-z0-9]+`)

func titleToSlug(title string) string {
	slug := strings.ToLower(title)
	slug = nonAlphanumRe.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	return slug
}

func extractAuthor(rels []relationship) string {
	for _, r := range rels {
		if r.Type == "author" && r.Attributes != nil {
			return r.Attributes.Name
		}
	}
	return "Unknown"
}

func extractCoverURL(mangaUUID string, rels []relationship) string {
	for _, r := range rels {
		if r.Type == "cover_art" && r.Attributes != nil && r.Attributes.FileName != "" {
			return "https://uploads.mangadex.org/covers/" + mangaUUID + "/" + r.Attributes.FileName
		}
	}
	return ""
}

func extractGenres(tags []tag) []string {
	var genres []string
	for _, t := range tags {
		if t.Attributes.Group == "genre" {
			if name := t.Attributes.Name["en"]; name != "" {
				genres = append(genres, name)
			}
		}
	}
	return genres
}

func normalizeStatus(s string) string {
	switch strings.ToLower(s) {
	case "completed":
		return "completed"
	case "hiatus":
		return "ongoing"
	default:
		return "ongoing"
	}
}
