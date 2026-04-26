package manga_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"mangahub/internal/manga"
	"mangahub/pkg/database"
)

func setupMangaRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := database.Connect(":memory:")
	if err != nil {
		t.Fatalf("db connect: %v", err)
	}
	_, err = db.Exec(`INSERT INTO manga (id, title, author, genres, status, total_chapters, description)
		VALUES ('one-piece', 'One Piece', 'Oda Eiichiro', '["Action","Shounen"]', 'ongoing', 1100, 'Pirates!')`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	h := &manga.Handler{DB: db}
	r := gin.New()
	r.GET("/manga", h.Search)
	r.GET("/manga/:id", h.GetByID)
	return r
}

func TestSearch_ReturnsResults(t *testing.T) {
	r := setupMangaRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/manga?q=one", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if count, ok := resp["count"].(float64); !ok || count == 0 {
		t.Error("expected at least 1 result")
	}
}

func TestSearch_EmptyQuery_ReturnsAll(t *testing.T) {
	r := setupMangaRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/manga", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestGetByID_Found(t *testing.T) {
	r := setupMangaRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/manga/one-piece", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var m map[string]interface{}
	json.NewDecoder(w.Body).Decode(&m)
	if m["id"] != "one-piece" {
		t.Errorf("expected id 'one-piece', got %v", m["id"])
	}
}

func TestGetByID_NotFound(t *testing.T) {
	r := setupMangaRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/manga/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}
