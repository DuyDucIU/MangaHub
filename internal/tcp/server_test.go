package tcp

import (
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
)

// readMsg reads one JSON line from conn with a 2s deadline.
func readMsg(t *testing.T, conn net.Conn) map[string]any {
	t.Helper()
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	defer conn.SetDeadline(time.Time{})
	var msg map[string]any
	if err := json.NewDecoder(conn).Decode(&msg); err != nil {
		t.Fatalf("readMsg: %v", err)
	}
	return msg
}

func TestRegister(t *testing.T) {
	srv := New("9090")
	s, c := net.Pipe()
	defer s.Close()
	defer c.Close()

	srv.Register("user1", s)

	assert.Equal(t, 1, srv.count())
	assert.Equal(t, s, srv.connFor("user1"))
}

func TestRegister_ReplacesExistingConnection(t *testing.T) {
	srv := New("9090")
	s1, c1 := net.Pipe()
	s2, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	srv.Register("user1", s1)
	srv.Register("user1", s2) // should close s1 and replace

	assert.Equal(t, 1, srv.count())
	assert.Equal(t, s2, srv.connFor("user1"))

	buf := make([]byte, 1)
	s1.SetDeadline(time.Now().Add(100 * time.Millisecond))
	_, err := s1.Read(buf)
	assert.Error(t, err, "old connection should be closed")
}

func TestUnregister(t *testing.T) {
	srv := New("9090")
	s, c := net.Pipe()
	defer s.Close()
	defer c.Close()

	srv.Register("user1", s)
	srv.Unregister("user1", s)

	assert.Equal(t, 0, srv.count())
}

func TestUnregister_NoOp(t *testing.T) {
	srv := New("9090")
	s2, c2 := net.Pipe()
	defer s2.Close()
	defer c2.Close()
	// should not panic on unknown user
	srv.Unregister("nobody", s2)
}

func TestBroadcastToUser(t *testing.T) {
	srv := New("9090")
	s, c := net.Pipe()
	defer s.Close()
	defer c.Close()

	srv.Register("user1", s)

	update := ProgressUpdate{UserID: "user1", MangaID: "one-piece", Chapter: 95, Timestamp: 1000}

	msgCh := make(chan map[string]any, 1)
	go func() {
		msgCh <- readMsg(t, c)
	}()

	srv.BroadcastToUser(update)

	msg := <-msgCh
	assert.Equal(t, "progress_update", msg["type"])
	assert.Equal(t, "user1", msg["user_id"])
	assert.Equal(t, "one-piece", msg["manga_id"])
	assert.Equal(t, float64(95), msg["chapter"])
	assert.Equal(t, float64(1000), msg["timestamp"])
}

func TestBroadcastToUser_UnknownUser(t *testing.T) {
	srv := New("9090")
	// should not panic
	srv.BroadcastToUser(ProgressUpdate{UserID: "nobody"})
}

func TestBroadcastToUser_DeadConnection(t *testing.T) {
	srv := New("9090")
	s, c := net.Pipe()
	c.Close() // close client side so write on server side fails

	srv.Register("user1", s)
	srv.BroadcastToUser(ProgressUpdate{UserID: "user1", MangaID: "one-piece", Chapter: 1, Timestamp: 1000})

	// failed write should remove the connection
	assert.Equal(t, 0, srv.count())
	s.Close()
}

func makeToken(t *testing.T, userID string) string {
	t.Helper()
	t.Setenv("JWT_SECRET", "test-secret")
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":  userID,
		"username": "tester",
		"exp":      time.Now().Add(time.Hour).Unix(),
	})
	signed, err := tok.SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatalf("makeToken: %v", err)
	}
	return signed
}

func TestHandleConn_CapacityLimit(t *testing.T) {
	srv := New("9090")
	srv.MaxConnections = 1

	existing, _ := net.Pipe()
	defer existing.Close()
	srv.Register("existing", existing)

	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.handleConn(serverSide)
	}()

	msg := readMsg(t, clientSide)
	assert.Equal(t, "error", msg["type"])
	assert.Equal(t, "server at capacity", msg["message"])
	clientSide.Close()
	<-done
}

func TestHandleConn_InvalidToken(t *testing.T) {
	srv := New("9090")
	t.Setenv("JWT_SECRET", "test-secret")

	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.handleConn(serverSide)
	}()

	payload, _ := json.Marshal(authMsg{Type: "auth", Token: "bad.token.here"})
	clientSide.Write(append(payload, '\n'))

	msg := readMsg(t, clientSide)
	assert.Equal(t, "auth_error", msg["type"])
	<-done
}

func TestHandleConn_ValidAuth(t *testing.T) {
	srv := New("9090")
	token := makeToken(t, "user1")

	serverSide, clientSide := net.Pipe()

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.handleConn(serverSide)
	}()

	payload, _ := json.Marshal(authMsg{Type: "auth", Token: token})
	clientSide.Write(append(payload, '\n'))

	msg := readMsg(t, clientSide)
	assert.Equal(t, "auth_ok", msg["type"])
	assert.Equal(t, "user1", msg["user_id"])

	assert.Equal(t, 1, srv.count())

	clientSide.Close() // trigger disconnect
	<-done             // wait for handleConn to clean up

	assert.Equal(t, 0, srv.count())
}
