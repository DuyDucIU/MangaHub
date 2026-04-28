package mangadex_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mangahub/internal/mangadex"
)

// sampleResponse mimics a real MangaDx /manga list response.
const sampleResponse = `{
  "result": "ok",
  "data": [
    {
      "id": "some-uuid-1234",
      "attributes": {
        "title": {"en": "Test Manga"},
        "description": {"en": "A test description."},
        "status": "ongoing",
        "lastChapter": "42",
        "tags": [
          {"attributes": {"name": {"en": "Action"}, "group": "genre"}},
          {"attributes": {"name": {"en": "Comedy"}, "group": "genre"}},
          {"attributes": {"name": {"en": "Full Color"}, "group": "format"}}
        ]
      },
      "relationships": [
        {"type": "author", "attributes": {"name": "Test Author"}}
      ]
    },
    {
      "id": "no-title-uuid",
      "attributes": {
        "title": {},
        "description": {},
        "status": "completed",
        "tags": []
      },
      "relationships": []
    }
  ]
}`

func newMockServer(t *testing.T, body string, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
}

func TestSearchPopular_ParsesFields(t *testing.T) {
	srv := newMockServer(t, sampleResponse, http.StatusOK)
	defer srv.Close()

	client := mangadex.NewClientWithBaseURL(srv.URL)
	mangas, err := client.SearchPopular(10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The second item has no English title and should be skipped.
	if len(mangas) != 1 {
		t.Fatalf("expected 1 manga (no-title skipped), got %d", len(mangas))
	}

	m := mangas[0]
	if m.Title != "Test Manga" {
		t.Errorf("title: want 'Test Manga', got %q", m.Title)
	}
	if m.ID != "test-manga" {
		t.Errorf("id slug: want 'test-manga', got %q", m.ID)
	}
	if m.Author != "Test Author" {
		t.Errorf("author: want 'Test Author', got %q", m.Author)
	}
	if m.Status != "ongoing" {
		t.Errorf("status: want 'ongoing', got %q", m.Status)
	}
	if m.TotalChapters != 42 {
		t.Errorf("chapters: want 42, got %d", m.TotalChapters)
	}

	// format tags (non-genre) must be excluded; only Action and Comedy survive.
	if len(m.Genres) != 2 {
		t.Errorf("genres: want 2, got %d: %v", len(m.Genres), m.Genres)
	}
}

func TestSearchPopular_ServerError(t *testing.T) {
	srv := newMockServer(t, `{"result":"error"}`, http.StatusInternalServerError)
	defer srv.Close()

	client := mangadex.NewClientWithBaseURL(srv.URL)
	_, err := client.SearchPopular(5)
	if err == nil {
		t.Fatal("expected error on non-200 status")
	}
}

func TestSearchPopular_MalformedJSON(t *testing.T) {
	srv := newMockServer(t, `not-json`, http.StatusOK)
	defer srv.Close()

	client := mangadex.NewClientWithBaseURL(srv.URL)
	_, err := client.SearchPopular(5)
	if err == nil {
		t.Fatal("expected error on malformed JSON")
	}
}

// Ensure our slug generation works on edge cases.
func TestSlugGeneration(t *testing.T) {
	cases := []struct {
		title string
		want  string
	}{
		{"One Piece", "one-piece"},
		{"Attack on Titan", "attack-on-titan"},
		{"Fullmetal Alchemist: Brotherhood", "fullmetal-alchemist-brotherhood"},
		{"Tokyo Ghoul:re", "tokyo-ghoul-re"},
	}

	// Test indirectly by creating a mock with a custom title.
	for _, tc := range cases {
		payload := map[string]interface{}{
			"result": "ok",
			"data": []map[string]interface{}{
				{
					"id": "uuid",
					"attributes": map[string]interface{}{
						"title":       map[string]string{"en": tc.title},
						"description": map[string]string{},
						"status":      "ongoing",
						"tags":        []interface{}{},
					},
					"relationships": []interface{}{},
				},
			},
		}
		body, _ := json.Marshal(payload)
		srv := newMockServer(t, string(body), http.StatusOK)

		client := mangadex.NewClientWithBaseURL(srv.URL)
		mangas, err := client.SearchPopular(1)
		srv.Close()

		if err != nil {
			t.Errorf("%q: unexpected error: %v", tc.title, err)
			continue
		}
		if len(mangas) == 0 {
			t.Errorf("%q: no mangas returned", tc.title)
			continue
		}
		if mangas[0].ID != tc.want {
			t.Errorf("%q: want slug %q, got %q", tc.title, tc.want, mangas[0].ID)
		}
	}
}
