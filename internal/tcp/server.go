package tcp

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

// ProgressUpdate is the message sent to TCP clients when a user's reading progress changes.
type ProgressUpdate struct {
	UserID    string `json:"user_id"`
	MangaID   string `json:"manga_id"`
	Chapter   int    `json:"chapter"`
	Timestamp int64  `json:"timestamp"`
}

// ProgressSyncServer manages persistent TCP client connections and broadcasts progress updates.
type ProgressSyncServer struct {
	Port           string
	Connections    map[string]net.Conn // user_id → active connection
	Broadcast      chan ProgressUpdate
	MaxConnections int
	mu             sync.RWMutex
}

// New creates a ProgressSyncServer with sensible defaults.
func New(port string) *ProgressSyncServer {
	return &ProgressSyncServer{
		Port:           port,
		Connections:    make(map[string]net.Conn),
		Broadcast:      make(chan ProgressUpdate, 100),
		MaxConnections: 30,
	}
}

// Register adds or replaces the connection for userID. Closes any existing connection.
func (s *ProgressSyncServer) Register(userID string, conn net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, ok := s.Connections[userID]; ok {
		old.Close()
	}
	s.Connections[userID] = conn
}

// Unregister removes the connection for userID.
func (s *ProgressSyncServer) Unregister(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.Connections, userID)
}

// count returns the current number of active connections.
func (s *ProgressSyncServer) count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.Connections)
}

// BroadcastToUser sends a progress_update message to the user's active connection.
// If the write fails, the connection is removed. No-op if user is not connected.
func (s *ProgressSyncServer) BroadcastToUser(update ProgressUpdate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	conn, ok := s.Connections[update.UserID]
	if !ok {
		return
	}
	msg := serverMsg{
		Type:      "progress_update",
		UserID:    update.UserID,
		MangaID:   update.MangaID,
		Chapter:   update.Chapter,
		Timestamp: update.Timestamp,
	}
	if err := writeMsg(conn, msg); err != nil {
		log.Printf("tcp: write failed for user %s: %v", update.UserID, err)
		delete(s.Connections, update.UserID)
	}
}

// Run starts the TCP listener and the broadcast consumer goroutine. Blocks until listener fails.
func (s *ProgressSyncServer) Run() {
	ln, err := net.Listen("tcp", ":"+s.Port)
	if err != nil {
		log.Fatalf("tcp: listen: %v", err)
	}
	defer ln.Close()
	log.Printf("tcp: listening on :%s", s.Port)

	go func() {
		for update := range s.Broadcast {
			s.BroadcastToUser(update)
		}
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("tcp: accept: %v", err)
			continue
		}
		go s.handleConn(conn)
	}
}

// handleConn runs in its own goroutine for each TCP client.
// It performs the auth handshake then keeps the connection open until the client disconnects.
func (s *ProgressSyncServer) handleConn(conn net.Conn) {
	defer conn.Close()

	// UC-007 A2: reject when at capacity
	if s.count() >= s.MaxConnections {
		writeMsg(conn, serverMsg{Type: "error", Message: "server at capacity"}) //nolint:errcheck
		return
	}

	// client must send auth message within 5 seconds
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	var msg authMsg
	if err := json.NewDecoder(conn).Decode(&msg); err != nil || msg.Type != "auth" {
		return
	}

	conn.SetDeadline(time.Time{}) // clear deadline after auth attempt

	userID, err := validateJWT(msg.Token)
	if err != nil {
		writeMsg(conn, serverMsg{Type: "auth_error", Message: "invalid token"}) //nolint:errcheck
		return
	}

	s.Register(userID, conn)
	defer s.Unregister(userID)

	writeMsg(conn, serverMsg{Type: "auth_ok", UserID: userID}) //nolint:errcheck

	// keep alive — read until client disconnects
	io.Copy(io.Discard, conn)
}

// --- private types and helpers ---

type authMsg struct {
	Type  string `json:"type"`
	Token string `json:"token"`
}

type serverMsg struct {
	Type      string `json:"type"`
	UserID    string `json:"user_id,omitempty"`
	MangaID   string `json:"manga_id,omitempty"`
	Chapter   int    `json:"chapter,omitempty"`
	Timestamp int64  `json:"timestamp,omitempty"`
	Message   string `json:"message,omitempty"`
}

func writeMsg(conn net.Conn, msg serverMsg) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = conn.Write(append(data, '\n'))
	return err
}

func validateJWT(tokenStr string) (string, error) {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "mangahub-secret-key"
	}
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		return "", fmt.Errorf("invalid token")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", fmt.Errorf("invalid claims")
	}
	userID, ok := claims["user_id"].(string)
	if !ok || userID == "" {
		return "", fmt.Errorf("missing user_id claim")
	}
	return userID, nil
}
