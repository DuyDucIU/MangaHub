package manga_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"mangahub/internal/auth"
	"mangahub/internal/manga"
	"mangahub/pkg/database"
)

func setupMangaRouter(t *testing.T) (*gin.Engine, string) {
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

	authH := &auth.Handler{DB: db, JWTSecret: "test-secret"}
	h := &manga.Handler{DB: db}

	r := gin.New()
	r.POST("/auth/register", authH.Register)
	r.POST("/auth/login", authH.Login)
	r.GET("/manga", h.Search)
	r.GET("/manga/:id", h.GetByID)

	doPost(r, "/auth/register", `{"username":"tester","email":"t@t.com","password":"password123"}`)
	loginResp := doPost(r, "/auth/login", `{"username":"tester","password":"password123"}`)
	var loginBody map[string]interface{}
	json.NewDecoder(loginResp.Body).Decode(&loginBody)
	token := loginBody["token"].(string)

	protected := r.Group("/")
	protected.Use(authH.JWTMiddleware())
	protected.POST("/manga", h.Create)

	return r, token
}

func doPost(r *gin.Engine, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func authReq(r *gin.Engine, method, path, body, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// GET /manga

func TestSearch_ReturnsResults(t *testing.T) {
	r, _ := setupMangaRouter(t)
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
	r, _ := setupMangaRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/manga", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSearch_GenreFilter(t *testing.T) {
	r, _ := setupMangaRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/manga?genre=Shounen", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if count, _ := resp["count"].(float64); count == 0 {
		t.Error("expected at least 1 shounen result")
	}
}

func TestGetByID_Found(t *testing.T) {
	r, _ := setupMangaRouter(t)
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
	r, _ := setupMangaRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/manga/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// POST /manga

func TestCreateManga_Success(t *testing.T) {
	r, token := setupMangaRouter(t)
	body := `{"id":"naruto","title":"Naruto","author":"Kishimoto Masashi","genres":["Action","Shounen"],"status":"completed","total_chapters":700,"description":"A ninja story."}`
	w := authReq(r, http.MethodPost, "/manga", body, token)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateManga_InvalidStatus(t *testing.T) {
	r, token := setupMangaRouter(t)
	body := `{"id":"test-manga","title":"Test","author":"Author","genres":["Action"],"status":"invalid"}`
	w := authReq(r, http.MethodPost, "/manga", body, token)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateManga_InvalidGenre(t *testing.T) {
	r, token := setupMangaRouter(t)
	body := `{"id":"test-manga","title":"Test","author":"Author","genres":["NotAGenre"],"status":"ongoing"}`
	w := authReq(r, http.MethodPost, "/manga", body, token)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateManga_Duplicate(t *testing.T) {
	r, token := setupMangaRouter(t)
	body := `{"id":"one-piece","title":"One Piece","author":"Oda","genres":["Action"],"status":"ongoing"}`
	w := authReq(r, http.MethodPost, "/manga", body, token)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateManga_NoAuth(t *testing.T) {
	r, _ := setupMangaRouter(t)
	body := `{"id":"new-manga","title":"New","author":"Author","genres":["Action"],"status":"ongoing"}`
	req := httptest.NewRequest(http.MethodPost, "/manga", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestCreateManga_MissingFields(t *testing.T) {
	r, token := setupMangaRouter(t)
	body := `{"id":"partial-manga","title":"Partial"}`
	w := authReq(r, http.MethodPost, "/manga", body, token)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
}
