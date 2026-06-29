package account

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"cotton-id/internal/auth"
)

// ErrImageNotFound is returned when no profile image matches a (user, kind) lookup.
var ErrImageNotFound = errors.New("profile image not found")

// PgxPool is the minimal pgx surface the store uses; *pgxpool.Pool satisfies it.
// Declaring the interface keeps the store unit-testable with a fake pool (mirrors
// auth.PgxPool / passkey.PgxPool). Begin lets UpsertWithURL run the blob upsert +
// the user's URL update in ONE transaction so they commit together.
type PgxPool interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Begin(ctx context.Context) (pgx.Tx, error)
}

// ProfileImage is one row of profile_images: a user's avatar or banner blob.
type ProfileImage struct {
	UserID      uuid.UUID
	Kind        string // avatar|banner
	ContentType string // image/png|image/jpeg|image/webp
	Bytes       []byte
	UpdatedAt   time.Time
}

// ImageStore persists and retrieves a user's avatar/banner blobs in Postgres.
type ImageStore struct {
	db PgxPool
}

// NewImageStore builds an ImageStore over the pool.
func NewImageStore(db PgxPool) *ImageStore {
	return &ImageStore{db: db}
}

// Upsert stores (or replaces) the user's image of the given kind. There is at
// most one avatar and one banner per user (the (user_id, kind) primary key), so a
// new upload overwrites the previous one, keeping storage bounded.
func (s *ImageStore) Upsert(ctx context.Context, userID uuid.UUID, kind, contentType string, data []byte) error {
	const q = `INSERT INTO profile_images (user_id, kind, content_type, bytes, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (user_id, kind)
		DO UPDATE SET content_type = EXCLUDED.content_type, bytes = EXCLUDED.bytes, updated_at = now()`
	_, err := s.db.Exec(ctx, q, userID, kind, contentType, data)
	return err
}

// UpsertWithURL stores (or replaces) the user's image of the given kind AND points
// the user's avatar_url/banner_url at urlValue, atomically in ONE transaction so
// the blob and the URL commit together. Previously the two writes were separate,
// so a failure between them could leave a stored blob whose URL was never set (or
// a dangling URL): on any error here the transaction rolls back and neither change
// persists.
//
// kind MUST be "avatar" or "banner" (validated at the handler); it selects the
// fixed users column updated. urlValue is the served route URL the handler builds.
func (s *ImageStore) UpsertWithURL(ctx context.Context, userID uuid.UUID, kind, contentType string, data []byte, urlValue string) error {
	var col string
	switch kind {
	case "avatar":
		col = "avatar_url"
	case "banner":
		col = "banner_url"
	default:
		// Guard: never let an unexpected kind reach the column-name interpolation.
		return errors.New("unknown image kind")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	// Roll back on any early return; the commit below makes this a harmless no-op.
	defer func() { _ = tx.Rollback(ctx) }()

	const upsertQ = `INSERT INTO profile_images (user_id, kind, content_type, bytes, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (user_id, kind)
		DO UPDATE SET content_type = EXCLUDED.content_type, bytes = EXCLUDED.bytes, updated_at = now()`
	if _, err := tx.Exec(ctx, upsertQ, userID, kind, contentType, data); err != nil {
		return err
	}

	// col is from the fixed allow-list above (never user input), so this format is
	// not an injection vector; the value + id are still bound parameters.
	urlQ := `UPDATE users SET ` + col + ` = $1, updated_at = now() WHERE id = $2`
	tag, err := tx.Exec(ctx, urlQ, urlValue, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		// The user row vanished (e.g. concurrent delete) — fail and roll back the
		// blob upsert so we never persist an image for a non-existent user.
		return auth.ErrUserNotFound
	}

	return tx.Commit(ctx)
}

// Get returns the user's image of the given kind, or ErrImageNotFound.
func (s *ImageStore) Get(ctx context.Context, userID uuid.UUID, kind string) (*ProfileImage, error) {
	const q = `SELECT user_id, kind, content_type, bytes, updated_at
		FROM profile_images WHERE user_id = $1 AND kind = $2`
	var img ProfileImage
	err := s.db.QueryRow(ctx, q, userID, kind).Scan(
		&img.UserID, &img.Kind, &img.ContentType, &img.Bytes, &img.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrImageNotFound
		}
		return nil, err
	}
	return &img, nil
}
