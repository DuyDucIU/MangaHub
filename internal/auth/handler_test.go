package auth_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"mangahub/internal/auth"
	"mangahub/pkg/database"
)

func setupAuthRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := database.Connect(":memory:")
	if err != nil {
		t.Fatalf("db connect: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	h := &auth.Handler{DB: db, JWTSecret: "test-secret"}
	r := gin.New()
	r.POST("/auth/register", h.Register)
	r.POST("/auth/login", h.Login)
	return r
}

func postJSON(r *gin.Engine, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestRegister_Success(t *testing.T) {
	r := setupAuthRouter(t)
	w := postJSON(r, "/auth/register", `{"username":"testuser","email":"test@test.com","password":"password123"}`)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["user_id"] == "" {
		t.Error("expected user_id in response")
	}
}

func TestRegister_DuplicateUsername(t *testing.T) {
	r := setupAuthRouter(t)
	body := `{"username":"testuser","email":"test@test.com","password":"password123"}`
	postJSON(r, "/auth/register", body)
	w := postJSON(r, "/auth/register", body)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestRegister_InvalidEmail(t *testing.T) {
	r := setupAuthRouter(t)
	w := postJSON(r, "/auth/register", `{"username":"testuser","email":"notanemail","password":"password123"}`)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}
}

func TestLogin_Success(t *testing.T) {
	r := setupAuthRouter(t)
	postJSON(r, "/auth/register", `{"username":"testuser","email":"test@test.com","password":"password123"}`)
	w := postJSON(r, "/auth/login", `{"username":"testuser","password":"password123"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["token"] == nil {
		t.Error("expected token in response")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	r := setupAuthRouter(t)
	postJSON(r, "/auth/register", `{"username":"testuser","email":"test@test.com","password":"password123"}`)
	w := postJSON(r, "/auth/login", `{"username":"testuser","password":"wrongpassword"}`)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestLogin_UserNotFound(t *testing.T) {
	r := setupAuthRouter(t)
	w := postJSON(r, "/auth/login", `{"username":"nobody","password":"password123"}`)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}
