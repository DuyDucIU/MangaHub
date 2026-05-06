package websocket

import (
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

// TestHandler_UpgradesWithoutToken verifies the server accepts the WebSocket upgrade
// even without a token in the URL — auth now happens via the first message.
func TestHandler_UpgradesWithoutToken(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	h := &Handler{Hub: hub, JWTSecret: handlerTestSecret, AuthTimeout: 500 * time.Millisecond}

	srv := httptest.NewServer(setupHandlerRouter(h))
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/chat?manga_id=one-piece"
	conn, resp, err := gorillaws.DefaultDialer.Dial(url, nil)
	if !assert.NoError(t, err) {
		return
	}
	defer conn.Close()

	assert.Equal(t, 101, resp.StatusCode)
}

// TestHandler_MissingAuthMessage_ClosesConnection verifies that a client who never
// sends the auth message is disconnected after the auth timeout.
func TestHandler_MissingAuthMessage_ClosesConnection(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	h := &Handler{Hub: hub, JWTSecret: handlerTestSecret, AuthTimeout: 100 * time.Millisecond}

	srv := httptest.NewServer(setupHandlerRouter(h))
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/chat?manga_id=one-piece"
	conn, _, err := gorillaws.DefaultDialer.Dial(url, nil)
	if !assert.NoError(t, err) {
		return
	}
	defer conn.Close()

	// Don't send anything — auth deadline (100ms) should fire and close the connection.
	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, _, err = conn.ReadMessage()
	assert.Error(t, err)
}

// TestHandler_InvalidToken_InAuthMessage_ClosesWithCode4001 verifies that a client
// sending an invalid token as the first message is rejected with WebSocket close code 4001.
func TestHandler_InvalidToken_InAuthMessage_ClosesWithCode4001(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	h := &Handler{Hub: hub, JWTSecret: handlerTestSecret, AuthTimeout: 500 * time.Millisecond}

	srv := httptest.NewServer(setupHandlerRouter(h))
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/chat?manga_id=one-piece"
	conn, _, err := gorillaws.DefaultDialer.Dial(url, nil)
	if !assert.NoError(t, err) {
		return
	}
	defer conn.Close()

	conn.WriteJSON(map[string]string{"token": "bad.token.value"})

	_, _, err = conn.ReadMessage()
	closeErr, ok := err.(*gorillaws.CloseError)
	assert.True(t, ok, "expected a WebSocket close error")
	assert.Equal(t, 4001, closeErr.Code)
}

// TestHandler_ValidToken_InAuthMessage_RegistersUser verifies that a client sending
// a valid token as the first message is registered and a join event is broadcast.
func TestHandler_ValidToken_InAuthMessage_RegistersUser(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	h := &Handler{Hub: hub, JWTSecret: handlerTestSecret, AuthTimeout: 500 * time.Millisecond}

	watcher := newTestClient(hub, "one-piece", "watcher")
	hub.register <- watcher

	srv := httptest.NewServer(setupHandlerRouter(h))
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/chat?manga_id=one-piece"
	conn, _, err := gorillaws.DefaultDialer.Dial(url, nil)
	if !assert.NoError(t, err) {
		return
	}
	defer conn.Close()

	tok := signHandlerToken("usr_abc", "alice")
	conn.WriteJSON(map[string]string{"token": tok})

	msg := recv(t, watcher)
	assert.Equal(t, "join", msg.Type)
	assert.Equal(t, "usr_abc", msg.UserID)
	assert.Equal(t, "one-piece", msg.RoomID)
}

// TestHandler_NoMangaID_JoinsGeneralRoom verifies that omitting manga_id places
// the client in the "general" room.
func TestHandler_NoMangaID_JoinsGeneralRoom(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	h := &Handler{Hub: hub, JWTSecret: handlerTestSecret, AuthTimeout: 500 * time.Millisecond}

	watcher := newTestClient(hub, "general", "watcher")
	hub.register <- watcher

	srv := httptest.NewServer(setupHandlerRouter(h))
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/chat" // no manga_id
	conn, _, err := gorillaws.DefaultDialer.Dial(url, nil)
	if !assert.NoError(t, err) {
		return
	}
	defer conn.Close()

	tok := signHandlerToken("usr_xyz", "bob")
	conn.WriteJSON(map[string]string{"token": tok})

	msg := recv(t, watcher)
	assert.Equal(t, "join", msg.Type)
	assert.Equal(t, "usr_xyz", msg.UserID)
	assert.Equal(t, "general", msg.RoomID)
}
