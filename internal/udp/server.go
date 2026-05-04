package udp

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strconv"
	"sync"
	"time"
)

type clientEntry struct {
	Addr   *net.UDPAddr
	Filter []string // manga IDs; empty = all manga
}

// NotificationServer manages registered UDP subscribers and fans out notifications.
type NotificationServer struct {
	Port      string
	clients   map[string]clientEntry // key: addr.String()
	Notify    chan NotifyRequest
	mu        sync.RWMutex
	conn      *net.UDPConn
	done      chan struct{}
	closeOnce sync.Once
}

// NotifyRequest is the payload the HTTP handler sends into the Notify channel.
type NotifyRequest struct {
	MangaID string `json:"manga_id"`
	Message string `json:"message"`
}

// New creates a NotificationServer with a buffered Notify channel (capacity 100).
func New(port string) *NotificationServer {
	return &NotificationServer{
		Port:    port,
		clients: make(map[string]clientEntry),
		Notify:  make(chan NotifyRequest, 100),
		done:    make(chan struct{}),
	}
}

func (s *NotificationServer) register(addr *net.UDPAddr, mangaIDs []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients[addr.String()] = clientEntry{Addr: addr, Filter: mangaIDs}
}

func (s *NotificationServer) unregister(addr *net.UDPAddr) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.clients, addr.String())
}

func (s *NotificationServer) clientCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.clients)
}

// --- packet types, Run, Shutdown, broadcast defined in later steps ---
var (
	_ = json.Marshal
	_ = fmt.Sprintf
	_ = log.Printf
	_ = net.ListenUDP
	_ = strconv.Atoi
	_ = time.Now
)
