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

// udpPkt is an inbound UDP datagram with its sender address.
type udpPkt struct {
	data []byte
	addr *net.UDPAddr
}

// inPkt is the JSON shape of client→server packets.
type inPkt struct {
	Type     string   `json:"type"`
	MangaIDs []string `json:"manga_ids"`
}

// outPkt is the JSON shape of server→client packets.
type outPkt struct {
	Type      string `json:"type"`
	Message   string `json:"message,omitempty"`
	MangaID   string `json:"manga_id,omitempty"`
	Timestamp int64  `json:"timestamp,omitempty"`
}

// Run opens the UDP listener and processes packets and Notify requests until Shutdown.
func (s *NotificationServer) Run() {
	port, _ := strconv.Atoi(s.Port)
	conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: port})
	if err != nil {
		log.Fatalf("udp: listen: %v", err)
	}
	s.mu.Lock()
	s.conn = conn
	s.mu.Unlock()
	defer conn.Close()
	log.Printf("udp: listening on %s", conn.LocalAddr())

	pktCh := make(chan udpPkt, 256)
	go func() {
		buf := make([]byte, 65535)
		for {
			n, addr, err := conn.ReadFromUDP(buf)
			if err != nil {
				select {
				case <-s.done:
				default:
					log.Printf("udp: read error: %v", err)
				}
				return
			}
			data := make([]byte, n)
			copy(data, buf[:n])
			select {
			case pktCh <- udpPkt{data: data, addr: addr}:
			case <-s.done:
				return
			}
		}
	}()

	for {
		select {
		case pkt := <-pktCh:
			s.handlePacket(pkt.data, pkt.addr)
		case req := <-s.Notify:
			s.broadcast(req)
		case <-s.done:
			return
		}
	}
}

// Shutdown closes the UDP connection and signals the run loop to stop.
func (s *NotificationServer) Shutdown() {
	s.closeOnce.Do(func() {
		log.Println("udp: shutting down...")
		close(s.done)
		s.mu.Lock()
		if s.conn != nil {
			s.conn.Close()
		}
		s.mu.Unlock()
	})
}

func (s *NotificationServer) handlePacket(data []byte, addr *net.UDPAddr) {
	var pkt inPkt
	if err := json.Unmarshal(data, &pkt); err != nil {
		log.Printf("udp: decode error from %s: %v", addr, err)
		return
	}
	switch pkt.Type {
	case "register":
		s.register(addr, pkt.MangaIDs)
		var msg string
		if len(pkt.MangaIDs) == 0 {
			msg = "registered for all manga"
		} else {
			msg = fmt.Sprintf("registered for %d manga", len(pkt.MangaIDs))
		}
		s.sendAck(addr, msg)
	case "unregister":
		s.unregister(addr)
		s.sendAck(addr, "unregistered")
	default:
		log.Printf("udp: unknown type %q from %s", pkt.Type, addr)
	}
}

func (s *NotificationServer) sendAck(addr *net.UDPAddr, message string) {
	data, _ := json.Marshal(outPkt{Type: "ack", Message: message})
	s.mu.RLock()
	conn := s.conn
	s.mu.RUnlock()
	if _, err := conn.WriteToUDP(data, addr); err != nil {
		log.Printf("udp: ack to %s failed: %v", addr, err)
	}
}

// broadcast is a placeholder — implemented in Task 3.
func (s *NotificationServer) broadcast(_ NotifyRequest) {}

// matchesFilter is a placeholder — implemented in Task 3.
func matchesFilter(_ []string, _ string) bool { return false }

// keep time imported until Task 3 uses it
var _ = time.Now
