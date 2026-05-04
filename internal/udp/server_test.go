package udp

import (
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustResolve(t *testing.T, addr string) *net.UDPAddr {
	t.Helper()
	a, err := net.ResolveUDPAddr("udp", addr)
	require.NoError(t, err)
	return a
}

func TestNew(t *testing.T) {
	srv := New("9091")
	assert.Equal(t, "9091", srv.Port)
	assert.NotNil(t, srv.clients)
	assert.NotNil(t, srv.Notify)
	assert.NotNil(t, srv.done)
}

func TestRegister(t *testing.T) {
	srv := New("0")
	addr := mustResolve(t, "127.0.0.1:10001")
	srv.register(addr, []string{"one-piece"})
	assert.Equal(t, 1, srv.clientCount())
	srv.mu.RLock()
	e := srv.clients[addr.String()]
	srv.mu.RUnlock()
	assert.Equal(t, []string{"one-piece"}, e.Filter)
}

func TestRegister_Upsert(t *testing.T) {
	srv := New("0")
	addr := mustResolve(t, "127.0.0.1:10001")
	srv.register(addr, []string{"one-piece"})
	srv.register(addr, []string{"naruto"})
	assert.Equal(t, 1, srv.clientCount())
	srv.mu.RLock()
	e := srv.clients[addr.String()]
	srv.mu.RUnlock()
	assert.Equal(t, []string{"naruto"}, e.Filter)
}

func TestUnregister(t *testing.T) {
	srv := New("0")
	addr := mustResolve(t, "127.0.0.1:10001")
	srv.register(addr, []string{"one-piece"})
	srv.unregister(addr)
	assert.Equal(t, 0, srv.clientCount())
}

func TestUnregister_NoOp(t *testing.T) {
	srv := New("0")
	addr := mustResolve(t, "127.0.0.1:10001")
	srv.unregister(addr) // must not panic
}

// startTestServer spins up a real server on a random OS-assigned port.
// Registers t.Cleanup to call Shutdown() when the test ends.
func startTestServer(t *testing.T) (*NotificationServer, *net.UDPAddr) {
	t.Helper()
	srv := New("0")
	go srv.Run()
	time.Sleep(20 * time.Millisecond) // let Run() bind
	t.Cleanup(srv.Shutdown)
	srv.mu.RLock()
	addr := srv.conn.LocalAddr().(*net.UDPAddr)
	// Convert to 127.0.0.1 for client communication compatibility on all platforms
	if len(addr.IP) == 16 && addr.IP.To4() != nil {
		addr.IP = addr.IP.To4()
	}
	if addr.IP == nil || addr.IP.IsUnspecified() {
		addr.IP = net.ParseIP("127.0.0.1")
	}
	srv.mu.RUnlock()
	return srv, addr
}

// newTestClient creates a UDP socket and registers cleanup.
func newTestClient(t *testing.T) *net.UDPConn {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	return conn
}

// sendPkt marshals v as JSON and sends it to dst.
func sendPkt(t *testing.T, conn *net.UDPConn, dst *net.UDPAddr, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	_, err = conn.WriteToUDP(data, dst)
	require.NoError(t, err)
}

// recvPkt reads one UDP packet (500 ms deadline) and returns it as a map.
func recvPkt(t *testing.T, conn *net.UDPConn) map[string]any {
	t.Helper()
	conn.SetDeadline(time.Now().Add(500 * time.Millisecond))
	defer conn.SetDeadline(time.Time{})
	buf := make([]byte, 4096)
	n, _, err := conn.ReadFromUDP(buf)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(buf[:n], &m))
	return m
}

// noPacket asserts that no UDP packet arrives within 100 ms.
func noPacket(t *testing.T, conn *net.UDPConn) {
	t.Helper()
	conn.SetDeadline(time.Now().Add(100 * time.Millisecond))
	defer conn.SetDeadline(time.Time{})
	buf := make([]byte, 4096)
	_, _, err := conn.ReadFromUDP(buf)
	require.Error(t, err, "expected no packet but one arrived")
}

func TestRun_Register_SendsAck(t *testing.T) {
	srv, srvAddr := startTestServer(t)
	client := newTestClient(t)

	sendPkt(t, client, srvAddr, map[string]any{
		"type":      "register",
		"manga_ids": []string{"one-piece", "naruto"},
	})

	msg := recvPkt(t, client)
	assert.Equal(t, "ack", msg["type"])
	assert.Equal(t, "registered for 2 manga", msg["message"])
	assert.Equal(t, 1, srv.clientCount())
}

func TestRun_Register_EmptyFilter_SendsAck(t *testing.T) {
	_, srvAddr := startTestServer(t)
	client := newTestClient(t)

	sendPkt(t, client, srvAddr, map[string]any{"type": "register", "manga_ids": []string{}})

	msg := recvPkt(t, client)
	assert.Equal(t, "ack", msg["type"])
	assert.Equal(t, "registered for all manga", msg["message"])
}

func TestRun_Unregister_SendsAck(t *testing.T) {
	srv, srvAddr := startTestServer(t)
	client := newTestClient(t)

	sendPkt(t, client, srvAddr, map[string]any{"type": "register", "manga_ids": []string{"one-piece"}})
	recvPkt(t, client) // consume ack

	sendPkt(t, client, srvAddr, map[string]any{"type": "unregister"})
	msg := recvPkt(t, client)
	assert.Equal(t, "ack", msg["type"])
	assert.Equal(t, "unregistered", msg["message"])
	assert.Equal(t, 0, srv.clientCount())
}

func TestRun_UnknownType_Ignored(t *testing.T) {
	_, srvAddr := startTestServer(t)
	client := newTestClient(t)

	sendPkt(t, client, srvAddr, map[string]any{"type": "bogus"})
	noPacket(t, client)
}

func TestRun_MalformedJSON_Ignored(t *testing.T) {
	_, srvAddr := startTestServer(t)
	client := newTestClient(t)

	data := []byte("not json at all")
	_, err := client.WriteToUDP(data, srvAddr)
	require.NoError(t, err)
	noPacket(t, client)
}
