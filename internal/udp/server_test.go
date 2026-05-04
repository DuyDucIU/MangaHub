package udp

import (
	"net"
	"testing"

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
