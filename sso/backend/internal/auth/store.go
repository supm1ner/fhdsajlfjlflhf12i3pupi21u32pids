package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// resetTokenBytes is the entropy of a password-reset token (256 bits).
const resetTokenBytes = 32

// ErrResetTokenInvalid is returned for a missing, expired, or already-used reset
// token (the three cases are deliberately indistinguishable to the caller).
var ErrResetTokenInvalid = errors.New("reset token is invalid or expired")

// ResetToken is a single-use password-reset token record. Only the sha256 hex of
// the token is stored.
type ResetToken struct {
	TokenHash string
	UserID    uuid.UUID
	CreatedAt time.Time
	ExpiresAt time.Time
	UsedAt    *time.Time
}

// ResetTokenStore persists password-reset tokens.
type ResetTokenStore struct {
	db PgxPool
}

// NewResetTokenStore builds a ResetTokenStore over the pool.
func NewResetTokenStore(db PgxPool) *ResetTokenStore {
	return &ResetTokenStore{db: db}
}

// GenerateResetToken returns a fresh opaque reset token and its sha256 hex hash.
func GenerateResetToken() (token, hash string, err error) {
	b := make([]byte, resetTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashToken(token), nil
}

// Create stores a reset token hash for a user with the given expiry.
func (s *ResetTokenStore) Create(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error {
	const q = `INSERT INTO password_reset_tokens (token_hash, user_id, expires_at)
		VALUES ($1, $2, $3)`
	_, err := s.db.Exec(ctx, q, tokenHash, userID, expiresAt)
	return err
}

// Consume atomically validates and marks a reset token used, returning the owning
// user id. It enforces single-use by updating used_at only when the token is
// unexpired and unused; if zero rows update, the token is invalid/expired/used.
func (s *ResetTokenStore) Consume(ctx context.Context, token string) (uuid.UUID, error) {
	hash := HashToken(token)
	const q = `UPDATE password_reset_tokens
		SET used_at = now()
		WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()
		RETURNING user_id`
	var userID uuid.UUID
	err := s.db.QueryRow(ctx, q, hash).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrResetTokenInvalid
		}
		return uuid.Nil, err
	}
	return userID, nil
}

// DeleteByUser removes all reset tokens for a user (e.g. after a successful
// reset or password change), invalidating any outstanding links.
func (s *ResetTokenStore) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	const q = `DELETE FROM password_reset_tokens WHERE user_id = $1`
	_, err := s.db.Exec(ctx, q, userID)
	return err
}
