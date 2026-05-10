package grpc

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"strings"
	"time"

	grpccodes "google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"mangahub/internal/tcp"
	pb "mangahub/proto/manga"
)

// Service implements pb.MangaServiceServer.
// TCPBroadcast is optional: when set, progress updates are pushed directly onto
// the TCP server's buffered channel instead of going through the internal HTTP sidecar.
// If nil (e.g. standalone grpc-server binary), broadcasts are skipped silently.
type Service struct {
	pb.UnimplementedMangaServiceServer
	DB           *sql.DB
	TCPBroadcast chan tcp.ProgressUpdate
}

func (s *Service) GetManga(ctx context.Context, req *pb.GetMangaRequest) (*pb.MangaResponse, error) {
	var m pb.MangaResponse
	var genresStr string
	err := s.DB.QueryRowContext(ctx,
		"SELECT id, title, author, genres, status, total_chapters, description, cover_url FROM manga WHERE id = ?",
		req.Id,
	).Scan(&m.Id, &m.Title, &m.Author, &genresStr, &m.Status, &m.TotalChapters, &m.Description, &m.CoverUrl)
	if err == sql.ErrNoRows {
		return nil, grpcstatus.Errorf(grpccodes.NotFound, "manga %q not found", req.Id)
	}
	if err != nil {
		return nil, grpcstatus.Errorf(grpccodes.Internal, "db: %v", err)
	}
	json.Unmarshal([]byte(genresStr), &m.Genres) //nolint:errcheck
	return &m, nil
}

func (s *Service) SearchManga(ctx context.Context, req *pb.SearchRequest) (*pb.SearchResponse, error) {
	page, pageSize := req.Page, req.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	const maxPageSize = int32(100)
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	query := "SELECT id, title, author, genres, status, total_chapters, description, cover_url FROM manga WHERE 1=1"
	args := []interface{}{}
	if req.Q != "" {
		query += " AND (LOWER(title) LIKE LOWER(?) OR LOWER(author) LIKE LOWER(?))"
		like := "%" + req.Q + "%"
		args = append(args, like, like)
	}
	if req.Status != "" {
		query += " AND status = ?"
		args = append(args, req.Status)
	}

	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, grpcstatus.Errorf(grpccodes.Internal, "db: %v", err)
	}
	defer rows.Close()

	var all []*pb.MangaResponse
	for rows.Next() {
		var m pb.MangaResponse
		var genresStr string
		if err := rows.Scan(&m.Id, &m.Title, &m.Author, &genresStr, &m.Status, &m.TotalChapters, &m.Description, &m.CoverUrl); err != nil {
			log.Printf("grpc: SearchManga scan error: %v", err)
			continue
		}
		if err := json.Unmarshal([]byte(genresStr), &m.Genres); err != nil {
			log.Printf("grpc: SearchManga: failed to parse genres for %q: %v", m.Id, err)
		}
		if req.Genre != "" {
			match := false
			for _, g := range m.Genres {
				if strings.EqualFold(g, req.Genre) {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		all = append(all, &m)
	}
	if err := rows.Err(); err != nil {
		return nil, grpcstatus.Errorf(grpccodes.Internal, "db iteration: %v", err)
	}

	total := int32(len(all))
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	pageResults := all[start:end]
	return &pb.SearchResponse{Results: pageResults, Count: int32(len(pageResults)), Total: total}, nil
}

func (s *Service) UpdateProgress(ctx context.Context, req *pb.ProgressRequest) (*pb.ProgressResponse, error) {
	var totalChapters int32
	var mangaTitle string
	err := s.DB.QueryRowContext(ctx, "SELECT total_chapters, title FROM manga WHERE id = ?", req.MangaId).Scan(&totalChapters, &mangaTitle)
	if err == sql.ErrNoRows {
		return nil, grpcstatus.Errorf(grpccodes.NotFound, "manga %q not found", req.MangaId)
	}
	if err != nil {
		return nil, grpcstatus.Errorf(grpccodes.Internal, "db: %v", err)
	}
	if totalChapters > 0 && req.CurrentChapter > totalChapters {
		return nil, grpcstatus.Errorf(grpccodes.InvalidArgument, "chapter %d exceeds total (%d)", req.CurrentChapter, totalChapters)
	}

	var currentStatus string
	err = s.DB.QueryRowContext(ctx,
		"SELECT status FROM user_progress WHERE user_id = ? AND manga_id = ?",
		req.UserId, req.MangaId,
	).Scan(&currentStatus)
	if err == sql.ErrNoRows {
		return nil, grpcstatus.Errorf(grpccodes.NotFound, "manga not in library")
	}
	if err != nil {
		return nil, grpcstatus.Errorf(grpccodes.Internal, "db: %v", err)
	}

	newStatus := currentStatus
	if req.Status != "" {
		newStatus = req.Status
	}

	_, err = s.DB.ExecContext(ctx,
		`UPDATE user_progress SET current_chapter = ?, status = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE user_id = ? AND manga_id = ?`,
		req.CurrentChapter, newStatus, req.UserId, req.MangaId,
	)
	if err != nil {
		return nil, grpcstatus.Errorf(grpccodes.Internal, "db: %v", err)
	}

	s.pushTCPBroadcast(req.UserId, req.MangaId, mangaTitle, req.CurrentChapter)

	return &pb.ProgressResponse{MangaId: req.MangaId, CurrentChapter: req.CurrentChapter, Status: newStatus}, nil
}

// pushTCPBroadcast enqueues a progress update onto the TCP server's buffered channel.
// Non-blocking: logs and drops if the channel is full rather than stalling the gRPC call.
// No-op when TCPBroadcast is nil (standalone grpc-server binary).
func (s *Service) pushTCPBroadcast(userID, mangaID, mangaTitle string, chapter int32) {
	if s.TCPBroadcast == nil {
		return
	}
	update := tcp.ProgressUpdate{
		UserID:     userID,
		MangaID:    mangaID,
		MangaTitle: mangaTitle,
		Chapter:    int(chapter),
		Timestamp:  time.Now().Unix(),
	}
	select {
	case s.TCPBroadcast <- update:
	default:
		log.Printf("grpc: TCP broadcast queue full, dropping update for user %s manga %s", userID, mangaID)
	}
}
