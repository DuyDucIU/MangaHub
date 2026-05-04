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
	rawAddr := srv.conn.LocalAddr().(*net.UDPAddr)
	addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: rawAddr.Port}
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

func TestBroadcast_MatchingSubscriber(t *testing.T) {
	srv, srvAddr := startTestServer(t)
	client := newTestClient(t)

	sendPkt(t, client, srvAddr, map[string]any{"type": "register", "manga_ids": []string{"one-piece"}})
	recvPkt(t, client) // consume ack

	srv.Notify <- NotifyRequest{MangaID: "one-piece", Message: "Chapter 1101 released!"}

	msg := recvPkt(t, client)
	assert.Equal(t, "notification", msg["type"])
	assert.Equal(t, "one-piece", msg["manga_id"])
	assert.Equal(t, "Chapter 1101 released!", msg["message"])
	assert.NotZero(t, msg["timestamp"])
}

func TestBroadcast_NonMatchingSubscriber(t *testing.T) {
	srv, srvAddr := startTestServer(t)
	client := newTestClient(t)

	sendPkt(t, client, srvAddr, map[string]any{"type": "register", "manga_ids": []string{"naruto"}})
	recvPkt(t, client)

	srv.Notify <- NotifyRequest{MangaID: "one-piece", Message: "Chapter 1101 released!"}
	noPacket(t, client)
}

func TestBroadcast_EmptyFilter_ReceivesAll(t *testing.T) {
	srv, srvAddr := startTestServer(t)
	client := newTestClient(t)

	sendPkt(t, client, srvAddr, map[string]any{"type": "register", "manga_ids": []string{}})
	recvPkt(t, client)

	srv.Notify <- NotifyRequest{MangaID: "one-piece", Message: "new chapter"}

	msg := recvPkt(t, client)
	assert.Equal(t, "notification", msg["type"])
	assert.Equal(t, "one-piece", msg["manga_id"])
}

func TestBroadcast_MultipleClients_OnlyMatchingNotified(t *testing.T) {
	srv, srvAddr := startTestServer(t)
	c1 := newTestClient(t) // subscribes to one-piece
	c2 := newTestClient(t) // subscribes to naruto

	sendPkt(t, c1, srvAddr, map[string]any{"type": "register", "manga_ids": []string{"one-piece"}})
	recvPkt(t, c1)
	sendPkt(t, c2, srvAddr, map[string]any{"type": "register", "manga_ids": []string{"naruto"}})
	recvPkt(t, c2)

	srv.Notify <- NotifyRequest{MangaID: "one-piece", Message: "new chapter"}

	msg := recvPkt(t, c1)
	assert.Equal(t, "notification", msg["type"])
	assert.Equal(t, "one-piece", msg["manga_id"])

	noPacket(t, c2)
}
