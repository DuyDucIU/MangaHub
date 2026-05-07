package grpc_test

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	grpclib "google.golang.org/grpc"
	mangagrpc "mangahub/internal/grpc"
	"mangahub/pkg/models"
	pb "mangahub/proto/manga"
)

// setupClientTest starts an in-process gRPC server backed by a real in-memory DB
// and returns a connected Client adapter for testing.
func setupClientTest(t *testing.T) (*mangagrpc.Client, func()) {
	t.Helper()
	db := setupDB(t)
	svc := &mangagrpc.Service{DB: db}

	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	s := grpclib.NewServer()
	pb.RegisterMangaServiceServer(s, svc)
	go s.Serve(lis) //nolint:errcheck

	client, err := mangagrpc.NewClient(lis.Addr().String())
	require.NoError(t, err)

	seedManga(t, db)
	seedUser(t, db)
	seedProgress(t, db)

	return client, func() {
		client.Close()  //nolint:errcheck
		s.Stop()
	}
}

func TestClient_GetManga_Found(t *testing.T) {
	client, cleanup := setupClientTest(t)
	defer cleanup()

	m, err := client.GetManga(context.Background(), "one-piece")
	require.NoError(t, err)
	assert.Equal(t, "one-piece", m.ID)
	assert.Equal(t, "One Piece", m.Title)
	assert.Equal(t, []string{"Action", "Shounen"}, m.Genres)
	assert.Equal(t, 1100, m.TotalChapters)
}

func TestClient_GetManga_NotFound(t *testing.T) {
	client, cleanup := setupClientTest(t)
	defer cleanup()

	_, err := client.GetManga(context.Background(), "nonexistent")
	require.Error(t, err)
	assert.True(t, errors.Is(err, models.ErrNotFound))
}

func TestClient_SearchManga_ReturnsResults(t *testing.T) {
	client, cleanup := setupClientTest(t)
	defer cleanup()

	results, total, err := client.SearchManga(context.Background(), "one", "", "", 1, 20)
	require.NoError(t, err)
	assert.Equal(t, int32(1), total)
	assert.Len(t, results, 1)
	assert.Equal(t, "one-piece", results[0].ID)
}

func TestClient_SearchManga_EmptyResults(t *testing.T) {
	client, cleanup := setupClientTest(t)
	defer cleanup()

	results, total, err := client.SearchManga(context.Background(), "zzznomatch", "", "", 1, 20)
	require.NoError(t, err)
	assert.Equal(t, int32(0), total)
	assert.Empty(t, results)
}

func TestClient_UpdateProgress_Success(t *testing.T) {
	client, cleanup := setupClientTest(t)
	defer cleanup()

	up, err := client.UpdateProgress(context.Background(), "user1", "one-piece", 50, "reading")
	require.NoError(t, err)
	assert.Equal(t, 50, up.CurrentChapter)
	assert.Equal(t, "one-piece", up.MangaID)
	assert.Equal(t, "reading", up.Status)
}

func TestClient_UpdateProgress_NotFound(t *testing.T) {
	client, cleanup := setupClientTest(t)
	defer cleanup()

	_, err := client.UpdateProgress(context.Background(), "user1", "nonexistent", 1, "reading")
	require.Error(t, err)
	assert.True(t, errors.Is(err, models.ErrNotFound))
}

func TestClient_UpdateProgress_InvalidArgument(t *testing.T) {
	client, cleanup := setupClientTest(t)
	defer cleanup()

	_, err := client.UpdateProgress(context.Background(), "user1", "one-piece", 9999, "reading")
	require.Error(t, err)
	assert.True(t, errors.Is(err, models.ErrInvalidArgument))
}
