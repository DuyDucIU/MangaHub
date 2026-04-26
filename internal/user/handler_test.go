package user_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"mangahub/internal/auth"
	"mangahub/internal/user"
	"mangahub/pkg/database"
)

func setupUserRouter(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := database.Connect(":memory:")
	if err != nil {
		t.Fatalf("db connect: %v", err)
	}
	_, err = db.Exec(`INSERT INTO manga (id, title, author, genres, status, total_chapters, description)
		VALUES ('one-piece', 'One Piece', 'Oda Eiichiro', '["Shounen"]', 'ongoing', 1100, 'Pirates!')`)
	if err != nil {
		t.Fatalf("seed manga: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	authH := &auth.Handler{DB: db, JWTSecret: "test-secret"}
	userH := &user.Handler{DB: db}

	r := gin.New()
	r.POST("/auth/register", authH.Register)
	r.POST("/auth/login", authH.Login)

	protected := r.Group("/")
	protected.Use(authH.JWTMiddleware())
	protected.POST("/users/library", userH.AddToLibrary)
	protected.GET("/users/library", userH.GetLibrary)
	protected.PUT("/users/progress", userH.UpdateProgress)

	doPost(r, "/auth/register", `{"username":"testuser","email":"test@test.com","password":"password123"}`)
	loginResp := doPost(r, "/auth/login", `{"username":"testuser","password":"password123"}`)
	var loginBody map[string]interface{}
	json.NewDecoder(loginResp.Body).Decode(&loginBody)
	token := loginBody["token"].(string)

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

// POST /users/library

func TestAddToLibrary_Success(t *testing.T) {
	r, token := setupUserRouter(t)
	w := authReq(r, http.MethodPost, "/users/library", `{"manga_id":"one-piece","status":"reading","current_chapter":10}`, token)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAddToLibrary_MangaNotFound(t *testing.T) {
	r, token := setupUserRouter(t)
	w := authReq(r, http.MethodPost, "/users/library", `{"manga_id":"nonexistent","status":"reading"}`, token)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestAddToLibrary_InvalidStatus(t *testing.T) {
	r, token := setupUserRouter(t)
	w := authReq(r, http.MethodPost, "/users/library", `{"manga_id":"one-piece","status":"invalid"}`, token)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestAddToLibrary_Duplicate(t *testing.T) {
	r, token := setupUserRouter(t)
	authReq(r, http.MethodPost, "/users/library", `{"manga_id":"one-piece","status":"reading"}`, token)
	w := authReq(r, http.MethodPost, "/users/library", `{"manga_id":"one-piece","status":"reading"}`, token)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

func TestAddToLibrary_NoAuth(t *testing.T) {
	r, _ := setupUserRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/users/library", bytes.NewBufferString(`{"manga_id":"one-piece","status":"reading"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// GET /users/library

func TestGetLibrary_Empty(t *testing.T) {
	r, token := setupUserRouter(t)
	w := authReq(r, http.MethodGet, "/users/library", "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["total"].(float64) != 0 {
		t.Errorf("expected total=0, got %v", resp["total"])
	}
}

func TestGetLibrary_WithEntries(t *testing.T) {
	r, token := setupUserRouter(t)
	authReq(r, http.MethodPost, "/users/library", `{"manga_id":"one-piece","status":"reading","current_chapter":50}`, token)
	w := authReq(r, http.MethodGet, "/users/library", "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["total"].(float64) != 1 {
		t.Errorf("expected total=1, got %v", resp["total"])
	}
}

// PUT /users/progress

func TestUpdateProgress_Success(t *testing.T) {
	r, token := setupUserRouter(t)
	authReq(r, http.MethodPost, "/users/library", `{"manga_id":"one-piece","status":"reading","current_chapter":50}`, token)
	w := authReq(r, http.MethodPut, "/users/progress", `{"manga_id":"one-piece","current_chapter":100}`, token)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateProgress_ExceedsTotal(t *testing.T) {
	r, token := setupUserRouter(t)
	authReq(r, http.MethodPost, "/users/library", `{"manga_id":"one-piece","status":"reading"}`, token)
	w := authReq(r, http.MethodPut, "/users/progress", `{"manga_id":"one-piece","current_chapter":9999}`, token)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateProgress_NotInLibrary(t *testing.T) {
	r, token := setupUserRouter(t)
	w := authReq(r, http.MethodPut, "/users/progress", `{"manga_id":"one-piece","current_chapter":100}`, token)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateProgress_MangaNotFound(t *testing.T) {
	r, token := setupUserRouter(t)
	w := authReq(r, http.MethodPut, "/users/progress", `{"manga_id":"nonexistent","current_chapter":1}`, token)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}
