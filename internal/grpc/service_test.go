package grpc_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	mangagrpc "mangahub/internal/grpc"
	"mangahub/pkg/database"
	pb "mangahub/proto/manga"
)

func setupDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := database.Connect(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func seedManga(t *testing.T, db *sql.DB) {
	t.Helper()
	genres, _ := json.Marshal([]string{"Action", "Shounen"})
	_, err := db.Exec(
		`INSERT INTO manga (id, title, author, genres, status, total_chapters, description) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"one-piece", "One Piece", "Oda Eiichiro", string(genres), "ongoing", 1100, "Pirates!",
	)
	require.NoError(t, err)
}

func seedUser(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO users (id, username, email, password_hash) VALUES (?, ?, ?, ?)`,
		"user1", "testuser", "test@test.com", "hash",
	)
	require.NoError(t, err)
}

func seedProgress(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO user_progress (user_id, manga_id, current_chapter, status) VALUES (?, ?, ?, ?)`,
		"user1", "one-piece", 10, "reading",
	)
	require.NoError(t, err)
}

func TestGetManga_Found(t *testing.T) {
	db := setupDB(t)
	seedManga(t, db)
	svc := &mangagrpc.Service{DB: db}

	resp, err := svc.GetManga(context.Background(), &pb.GetMangaRequest{Id: "one-piece"})
	require.NoError(t, err)
	assert.Equal(t, "one-piece", resp.Id)
	assert.Equal(t, "One Piece", resp.Title)
	assert.Equal(t, []string{"Action", "Shounen"}, resp.Genres)
	assert.Equal(t, int32(1100), resp.TotalChapters)
}

func TestGetManga_NotFound(t *testing.T) {
	db := setupDB(t)
	svc := &mangagrpc.Service{DB: db}

	_, err := svc.GetManga(context.Background(), &pb.GetMangaRequest{Id: "nonexistent"})
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestSearchManga_QueryFilter(t *testing.T) {
	db := setupDB(t)
	seedManga(t, db)
	svc := &mangagrpc.Service{DB: db}

	resp, err := svc.SearchManga(context.Background(), &pb.SearchRequest{Q: "one", Page: 1, PageSize: 20})
	require.NoError(t, err)
	assert.Equal(t, int32(1), resp.Total)
	assert.Len(t, resp.Results, 1)
	assert.Equal(t, int32(1), resp.Count)
}

func TestSearchManga_GenreFilter(t *testing.T) {
	db := setupDB(t)
	seedManga(t, db)
	svc := &mangagrpc.Service{DB: db}

	resp, err := svc.SearchManga(context.Background(), &pb.SearchRequest{Genre: "Shounen", Page: 1, PageSize: 20})
	require.NoError(t, err)
	assert.Equal(t, int32(1), resp.Total)

	resp2, err := svc.SearchManga(context.Background(), &pb.SearchRequest{Genre: "Romance", Page: 1, PageSize: 20})
	require.NoError(t, err)
	assert.Equal(t, int32(0), resp2.Total)
	assert.Empty(t, resp2.Results)
}

func TestSearchManga_Pagination(t *testing.T) {
	db := setupDB(t)
	seedManga(t, db)
	svc := &mangagrpc.Service{DB: db}

	resp, err := svc.SearchManga(context.Background(), &pb.SearchRequest{Page: 1, PageSize: 1})
	require.NoError(t, err)
	assert.Equal(t, int32(1), resp.Total)
	assert.Len(t, resp.Results, 1)

	resp2, err := svc.SearchManga(context.Background(), &pb.SearchRequest{Page: 2, PageSize: 1})
	require.NoError(t, err)
	assert.Equal(t, int32(1), resp2.Total)
	assert.Empty(t, resp2.Results)
}

func TestUpdateProgress_Success(t *testing.T) {
	db := setupDB(t)
	seedManga(t, db)
	seedUser(t, db)
	seedProgress(t, db)
	svc := &mangagrpc.Service{DB: db}

	resp, err := svc.UpdateProgress(context.Background(), &pb.ProgressRequest{
		UserId: "user1", MangaId: "one-piece", CurrentChapter: 50, Status: "reading",
	})
	require.NoError(t, err)
	assert.Equal(t, int32(50), resp.CurrentChapter)
	assert.Equal(t, "one-piece", resp.MangaId)
	assert.Equal(t, "reading", resp.Status)
}

func TestUpdateProgress_MangaNotFound(t *testing.T) {
	db := setupDB(t)
	seedUser(t, db)
	svc := &mangagrpc.Service{DB: db}

	_, err := svc.UpdateProgress(context.Background(), &pb.ProgressRequest{
		UserId: "user1", MangaId: "nonexistent", CurrentChapter: 1,
	})
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestUpdateProgress_ExceedsTotal(t *testing.T) {
	db := setupDB(t)
	seedManga(t, db)
	seedUser(t, db)
	seedProgress(t, db)
	svc := &mangagrpc.Service{DB: db}

	_, err := svc.UpdateProgress(context.Background(), &pb.ProgressRequest{
		UserId: "user1", MangaId: "one-piece", CurrentChapter: 9999,
	})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestUpdateProgress_NotInLibrary(t *testing.T) {
	db := setupDB(t)
	seedManga(t, db)
	seedUser(t, db)
	svc := &mangagrpc.Service{DB: db}

	_, err := svc.UpdateProgress(context.Background(), &pb.ProgressRequest{
		UserId: "user1", MangaId: "one-piece", CurrentChapter: 10,
	})
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}
