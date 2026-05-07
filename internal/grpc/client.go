package grpc

import (
	"context"
	"fmt"

	grpclib "google.golang.org/grpc"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	grpcstatus "google.golang.org/grpc/status"
	"mangahub/pkg/models"
	pb "mangahub/proto/manga"
)

// Client is the thin adapter over the generated MangaServiceClient.
// It converts pb.* types to models.* and gRPC status codes to sentinel errors.
type Client struct {
	conn *grpclib.ClientConn
	svc  pb.MangaServiceClient
}

// NewClient dials addr with insecure credentials (no TLS).
func NewClient(addr string) (*Client, error) {
	conn, err := grpclib.NewClient(addr, grpclib.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("grpc dial %s: %w", addr, err)
	}
	return &Client{conn: conn, svc: pb.NewMangaServiceClient(conn)}, nil
}

// Close releases the underlying connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// GetManga fetches one manga by ID. Returns models.ErrNotFound if not found.
func (c *Client) GetManga(ctx context.Context, id string) (*models.Manga, error) {
	resp, err := c.svc.GetManga(ctx, &pb.GetMangaRequest{Id: id})
	if err != nil {
		if grpcstatus.Code(err) == grpccodes.NotFound {
			return nil, fmt.Errorf("%w: %s", models.ErrNotFound, grpcstatus.Convert(err).Message())
		}
		return nil, err
	}
	return &models.Manga{
		ID:            resp.Id,
		Title:         resp.Title,
		Author:        resp.Author,
		Genres:        resp.Genres,
		Status:        resp.Status,
		TotalChapters: int(resp.TotalChapters),
		Description:   resp.Description,
		CoverURL:      resp.CoverUrl,
	}, nil
}

// SearchManga searches manga with optional filters and pagination.
// Returns (results, total, error) where total is the full match count before paging.
func (c *Client) SearchManga(ctx context.Context, q, genre, statusFilter string, page, pageSize int) ([]models.Manga, int, error) {
	resp, err := c.svc.SearchManga(ctx, &pb.SearchRequest{
		Q: q, Genre: genre, Status: statusFilter, Page: int32(page), PageSize: int32(pageSize),
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]models.Manga, 0, len(resp.Results))
	for _, r := range resp.Results {
		out = append(out, models.Manga{
			ID:            r.Id,
			Title:         r.Title,
			Author:        r.Author,
			Genres:        r.Genres,
			Status:        r.Status,
			TotalChapters: int(r.TotalChapters),
			Description:   r.Description,
			CoverURL:      r.CoverUrl,
		})
	}
	return out, int(resp.Total), nil
}

// UpdateProgress updates reading progress.
// Returns models.ErrNotFound if manga or progress record is missing.
// Returns models.ErrInvalidArgument (wrapped) if chapter exceeds total.
func (c *Client) UpdateProgress(ctx context.Context, userID, mangaID string, chapter int32, newStatus string) (*models.UserProgress, error) {
	resp, err := c.svc.UpdateProgress(ctx, &pb.ProgressRequest{
		UserId: userID, MangaId: mangaID, CurrentChapter: chapter, Status: newStatus,
	})
	if err != nil {
		switch grpcstatus.Code(err) {
		case grpccodes.NotFound:
			return nil, fmt.Errorf("%w: %s", models.ErrNotFound, grpcstatus.Convert(err).Message())
		case grpccodes.InvalidArgument:
			return nil, fmt.Errorf("%w: %s", models.ErrInvalidArgument, grpcstatus.Convert(err).Message())
		}
		return nil, err
	}
	return &models.UserProgress{
		UserID:         userID,
		MangaID:        resp.MangaId,
		CurrentChapter: int(resp.CurrentChapter),
		Status:         resp.Status,
	}, nil
}
