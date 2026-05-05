package websocket

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	gorillaws "github.com/gorilla/websocket"
	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
)

const handlerTestSecret = "handler-test-secret"

func signHandlerToken(userID, username string) string {
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":  userID,
		"username": username,
		"exp":      time.Now().Add(time.Hour).Unix(),
	})
	s, _ := tok.SignedString([]byte(handlerTestSecret))
	return s
}

func setupHandlerRouter(h *Handler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/ws/chat", h.ServeWS)
	return r
}

func TestHandler_MissingToken_Returns401(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	h := &Handler{Hub: hub, JWTSecret: handlerTestSecret}

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/ws/chat", nil)
	setupHandlerRouter(h).ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHandler_InvalidToken_Returns401(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	h := &Handler{Hub: hub, JWTSecret: handlerTestSecret}

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/ws/chat?token=bad.token.value", nil)
	setupHandlerRouter(h).ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHandler_ValidToken_UpgradesConnection(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	h := &Handler{Hub: hub, JWTSecret: handlerTestSecret}

	srv := httptest.NewServer(setupHandlerRouter(h))
	defer srv.Close()

	tok := signHandlerToken("usr_abc", "alice")
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/chat?token=" + tok + "&manga_id=one-piece"

	conn, resp, err := gorillaws.DefaultDialer.Dial(url, nil)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	conn.Close()
}

func TestHandler_NoMangaID_JoinsGeneralRoom(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	h := &Handler{Hub: hub, JWTSecret: handlerTestSecret}

	// Pre-register a client in "general" to receive the join notification.
	watcher := newTestClient(hub, "general", "watcher")
	hub.register <- watcher

	srv := httptest.NewServer(setupHandlerRouter(h))
	defer srv.Close()

	tok := signHandlerToken("usr_xyz", "bob")
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/chat?token=" + tok // no manga_id

	conn, _, err := gorillaws.DefaultDialer.Dial(url, nil)
	assert.NoError(t, err)
	defer conn.Close()

	// Watcher should receive a join notification for bob in "general".
	msg := recv(t, watcher)
	assert.Equal(t, "join", msg.Type)
	assert.Equal(t, "usr_xyz", msg.UserID)
	assert.Equal(t, "general", msg.RoomID)
}
