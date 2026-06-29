package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// sessionTokenBytes is the entropy of the opaque session token (256 bits).
const sessionTokenBytes = 32

// ErrSessionNotFound is returned when no live session matches a token.
var ErrSessionNotFound = errors.New("session not found")

// lastSeenThrottle is the minimum interval between sessions.last_seen_at bumps for
// a single session. A session's last_seen_at is updated on use only when it is
// older than this, so an active session is touched at most once per minute,
// bounding the write amplification of per-request bumps (design D4).
const lastSeenThrottle = time.Minute

// Session is a server-side session record. ID is the sha256 hex of the opaque
// token; the raw token lives only in the user's cookie.
type Session struct {
	ID         string // sha256(token) hex — primary key, never sent to client
	UserID     uuid.UUID
	Remember   bool
	UserAgent  string
	IP         string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	LastSeenAt time.Time // when the session last authenticated a request (throttled bump)
}

// GenerateSessionToken returns a fresh opaque session token (URL-safe base64)
// together with its sha256 hex id. Only the token goes in the cookie; only the
// id is stored, so a database read cannot reconstruct a usable cookie.
func GenerateSessionToken() (token string, id string, err error) {
	b := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	id = HashToken(token)
	return token, id, nil
}

// HashToken returns the sha256 hex of a token. Used for both session and reset
// tokens so the raw secret is never persisted.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// SessionStore persists sessions.
type SessionStore struct {
	db PgxPool
}

// NewSessionStore builds a SessionStore over the pool.
func NewSessionStore(db PgxPool) *SessionStore {
	return &SessionStore{db: db}
}

// Create inserts a session row. id must be HashToken(token).
func (s *SessionStore) Create(ctx context.Context, sess *Session) error {
	const q = `INSERT INTO sessions (id, user_id, remember, user_agent, ip, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)`
	_, err := s.db.Exec(ctx, q, sess.ID, sess.UserID, sess.Remember, sess.UserAgent, sess.IP, sess.ExpiresAt)
	return err
}

// GetByToken resolves a raw cookie token to a non-expired session. Expired rows
// are treated as not found (and are not honored).
func (s *SessionStore) GetByToken(ctx context.Context, token string) (*Session, error) {
	id := HashToken(token)
	const q = `SELECT id, user_id, remember, user_agent, ip, created_at, expires_at, last_seen_at
		FROM sessions WHERE id = $1 AND expires_at > now()`
	var sess Session
	err := s.db.QueryRow(ctx, q, id).Scan(
		&sess.ID, &sess.UserID, &sess.Remember, &sess.UserAgent, &sess.IP, &sess.CreatedAt, &sess.ExpiresAt, &sess.LastSeenAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}
	return &sess, nil
}

// BumpLastSeen records that the session identified by id authenticated a request,
// THROTTLED in SQL to at most once per minute per session: the UPDATE's WHERE
// clause only matches when last_seen_at is already older than lastSeenThrottle, so
// a burst of requests for an active session performs a single no-op-bounded write
// (or none). This keeps the per-request bump from amplifying writes (design D4).
//
// It is best-effort and MUST be called for its side effect only: a caller ignores
// the error so a failed bump never fails the authenticated request. id is the
// session primary key (HashToken(cookie)); no token hashing happens here.
func (s *SessionStore) BumpLastSeen(ctx context.Context, id string) error {
	const q = `UPDATE sessions
		SET last_seen_at = now()
		WHERE id = $1 AND last_seen_at < now() - $2::interval`
	// $2 is bound as a Postgres interval literal (seconds) — parameterized, not
	// interpolated — so the throttle window is injection-safe.
	_, err := s.db.Exec(ctx, q, id, lastSeenThrottle.String())
	return err
}

// DeleteByToken revokes the session identified by a raw cookie token.
func (s *SessionStore) DeleteByToken(ctx context.Context, token string) error {
	id := HashToken(token)
	const q = `DELETE FROM sessions WHERE id = $1`
	_, err := s.db.Exec(ctx, q, id)
	return err
}

// DeleteByUser revokes every session for a user (used on password reset).
func (s *SessionStore) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	const q = `DELETE FROM sessions WHERE user_id = $1`
	_, err := s.db.Exec(ctx, q, userID)
	return err
}

// ListByUser returns a user's non-expired sessions, newest first. Used by the
// account self-service "active sessions" view.
func (s *SessionStore) ListByUser(ctx context.Context, userID uuid.UUID) ([]Session, error) {
	const q = `SELECT id, user_id, remember, user_agent, ip, created_at, expires_at, last_seen_at
		FROM sessions WHERE user_id = $1 AND expires_at > now() ORDER BY created_at DESC`
	rows, err := s.db.Query(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Session
	for rows.Next() {
		var sess Session
		if err := rows.Scan(
			&sess.ID, &sess.UserID, &sess.Remember, &sess.UserAgent, &sess.IP, &sess.CreatedAt, &sess.ExpiresAt, &sess.LastSeenAt,
		); err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

// DeleteForUser revokes the session identified by id, but only when it belongs to
// userID — the cross-user scoping guard for self-service session revocation. It
// returns ErrSessionNotFound when no such session exists for that user (including
// when it belongs to someone else), so a caller can never revoke or probe another
// account's session.
func (s *SessionStore) DeleteForUser(ctx context.Context, userID uuid.UUID, id string) error {
	const q = `DELETE FROM sessions WHERE id = $1 AND user_id = $2`
	tag, err := s.db.Exec(ctx, q, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrSessionNotFound
	}
	return nil
}

// DeleteByUserExcept revokes every session for a user except the one identified by
// keepID (the current request's session), returning the number revoked. Used by
// the password-change and "revoke other sessions" flows so the in-flight device
// stays signed in.
func (s *SessionStore) DeleteByUserExcept(ctx context.Context, userID uuid.UUID, keepID string) (int64, error) {
	const q = `DELETE FROM sessions WHERE user_id = $1 AND id <> $2`
	tag, err := s.db.Exec(ctx, q, userID, keepID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// PurgeExpired removes expired session rows; returns the number deleted.
func (s *SessionStore) PurgeExpired(ctx context.Context) (int64, error) {
	const q = `DELETE FROM sessions WHERE expires_at <= now()`
	tag, err := s.db.Exec(ctx, q)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
