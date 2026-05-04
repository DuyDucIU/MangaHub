package tcp

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestHandleBroadcast_ValidPayload(t *testing.T) {
	srv := New("9090")
	s, c := net.Pipe()
	defer s.Close()
	defer c.Close()
	srv.Register("user1", s)

	// drain broadcast channel so handler doesn't block
	stopDrain := make(chan struct{})
	t.Cleanup(func() { close(stopDrain) })
	go func() {
		for {
			select {
			case update, ok := <-srv.Broadcast:
				if !ok {
					return
				}
				srv.BroadcastToUser(update)
			case <-stopDrain:
				return
			}
		}
	}()

	body := `{"user_id":"user1","manga_id":"one-piece","chapter":95,"timestamp":1000}`
	req := httptest.NewRequest(http.MethodPost, "/internal/broadcast", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.InternalHandler().ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// verify the update arrived at the client connection
	msgCh := make(chan map[string]any, 1)
	go func() {
		c.SetDeadline(time.Now().Add(2 * time.Second))
		var msg map[string]any
		json.NewDecoder(c).Decode(&msg)
		msgCh <- msg
	}()
	msg := <-msgCh
	assert.Equal(t, "progress_update", msg["type"])
	assert.Equal(t, "one-piece", msg["manga_id"])
	assert.Equal(t, float64(95), msg["chapter"])
}

func TestHandleBroadcast_MalformedPayload(t *testing.T) {
	srv := New("9090")
	req := httptest.NewRequest(http.MethodPost, "/internal/broadcast", strings.NewReader("not json"))
	w := httptest.NewRecorder()

	srv.InternalHandler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleBroadcast_UnknownUser(t *testing.T) {
	srv := New("9090")

	// drain so handler doesn't block on channel send
	stopDrain := make(chan struct{})
	t.Cleanup(func() { close(stopDrain) })
	go func() {
		for {
			select {
			case _, ok := <-srv.Broadcast:
				if !ok {
					return
				}
			case <-stopDrain:
				return
			}
		}
	}()

	body := `{"user_id":"nobody","manga_id":"one-piece","chapter":1,"timestamp":1000}`
	req := httptest.NewRequest(http.MethodPost, "/internal/broadcast", strings.NewReader(body))
	w := httptest.NewRecorder()

	srv.InternalHandler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code) // silent 200 — user simply not connected
}

func TestHandleBroadcast_MethodNotAllowed(t *testing.T) {
	srv := New("9090")
	req := httptest.NewRequest(http.MethodGet, "/internal/broadcast", nil)
	w := httptest.NewRecorder()

	srv.InternalHandler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}
