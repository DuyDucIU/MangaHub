package tcp

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"mangahub/pkg/jwtutil"
)

// ProgressUpdate is the message sent to TCP clients when a user's reading progress changes.
type ProgressUpdate struct {
	UserID     string `json:"user_id"`
	MangaID    string `json:"manga_id"`
	MangaTitle string `json:"manga_title"`
	Chapter    int    `json:"chapter"`
	Timestamp  int64  `json:"timestamp"`
}

// ProgressSyncServer manages persistent TCP client connections and broadcasts progress updates.
type ProgressSyncServer struct {
	Port           string
	Connections    map[string]net.Conn // user_id → active connection
	Broadcast      chan ProgressUpdate
	MaxConnections int
	mu             sync.RWMutex
	listener       net.Listener
	done           chan struct{}
	closeOnce      sync.Once
}

// New creates a ProgressSyncServer with sensible defaults.
func New(port string) *ProgressSyncServer {
	return &ProgressSyncServer{
		Port:           port,
		Connections:    make(map[string]net.Conn),
		Broadcast:      make(chan ProgressUpdate, 100),
		MaxConnections: 30,
		done:           make(chan struct{}),
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

// Unregister removes the connection for userID only if it is still the same conn.
// This prevents a reconnecting user's old goroutine from evicting the new registration.
func (s *ProgressSyncServer) Unregister(userID string, conn net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.Connections[userID]; ok && c == conn {
		delete(s.Connections, userID)
	}
}

// connFor returns the connection registered for userID, or nil if not connected.
func (s *ProgressSyncServer) connFor(userID string) net.Conn {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Connections[userID]
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
		Type:       "progress_update",
		UserID:     update.UserID,
		MangaID:    update.MangaID,
		MangaTitle: update.MangaTitle,
		Chapter:    update.Chapter,
		Timestamp:  update.Timestamp,
	}
	if err := writeMsg(conn, msg); err != nil {
		log.Printf("tcp: write failed for user %s: %v", update.UserID, err)
		delete(s.Connections, update.UserID)
	}
}

// Run starts the TCP listener and the broadcast consumer goroutine. Blocks until Shutdown is called.
func (s *ProgressSyncServer) Run() {
	ln, err := net.Listen("tcp", ":"+s.Port)
	if err != nil {
		log.Fatalf("tcp: listen: %v", err)
	}
	s.mu.Lock()
	s.listener = ln
	s.mu.Unlock()
	defer ln.Close()
	log.Printf("tcp: listening on :%s", s.Port)

	go func() {
		for {
			select {
			case update, ok := <-s.Broadcast:
				if !ok {
					return
				}
				s.BroadcastToUser(update)
			case <-s.done:
				return
			}
		}
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return // listener closed by Shutdown
			}
			log.Printf("tcp: accept: %v", err)
			continue
		}
		go s.handleConn(conn)
	}
}

// Shutdown closes the listener and all active client connections gracefully.
func (s *ProgressSyncServer) Shutdown() {
	s.closeOnce.Do(func() {
		log.Println("tcp: shutting down...")
		s.mu.Lock()
		if s.listener != nil {
			s.listener.Close()
		}
		for userID, conn := range s.Connections {
			conn.Close()
			delete(s.Connections, userID)
		}
		s.mu.Unlock()
		close(s.done)
		log.Println("tcp: all connections closed")
	})
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

	claims, err := jwtutil.ValidateToken(msg.Token, jwtutil.DefaultSecret())
	if err != nil {
		writeMsg(conn, serverMsg{Type: "auth_error", Message: "invalid token"}) //nolint:errcheck
		return
	}
	userID := claims.UserID

	s.Register(userID, conn)
	defer s.Unregister(userID, conn)

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
	Type       string `json:"type"`
	UserID     string `json:"user_id,omitempty"`
	MangaID    string `json:"manga_id,omitempty"`
	MangaTitle string `json:"manga_title,omitempty"`
	Chapter    int    `json:"chapter"`
	Timestamp  int64  `json:"timestamp,omitempty"`
	Message    string `json:"message,omitempty"`
}

func writeMsg(conn net.Conn, msg serverMsg) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = conn.Write(append(data, '\n'))
	return err
}

