// Package passkey implements cotton-id's WebAuthn (FIDO2) passkey support: the
// relying-party-configured registration and passwordless-login ceremonies, the
// pgx-backed credential store, a signed short-lived ceremony-state cookie, and
// the HTTP surface mounted under /api/v1.
//
// cotton-id is the relying party (RP). The protocol/crypto are delegated to
// github.com/go-webauthn/webauthn; this package owns the user/credential storage
// and the HTTP handlers. A successful passkey authentication establishes a normal
// cotton-id session (and can continue a Hydra login_challenge) exactly like
// password and social login.
package passkey

import (
	"context"
	"errors"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrCredentialNotFound is returned when no credential matches a lookup (by
// credential id, or by (user, id) for a scoped delete).
var ErrCredentialNotFound = errors.New("passkey credential not found")

// ErrCredentialAlreadyRegistered is returned by Create when the credential id is
// already registered (the UNIQUE(credential_id) constraint), so the handler can
// reply with a clean 409 instead of a 500. The message stays generic and never
// reveals which account owns it.
var ErrCredentialAlreadyRegistered = errors.New("passkey credential already registered")

// PgxPool is the minimal pgx surface the store uses; *pgxpool.Pool satisfies it.
// Declaring the interface keeps the store unit-testable with a fake pool.
type PgxPool interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// StoredCredential is one row of webauthn_credentials: a registered passkey bound
// to a user. CredentialID, PublicKey and AAGUID are raw bytes; Transports are the
// authenticator transport hints; Name is the user-chosen nickname.
type StoredCredential struct {
	ID              uuid.UUID
	UserID          uuid.UUID
	CredentialID    []byte
	PublicKey       []byte
	AttestationType string
	AAGUID          []byte
	SignCount       int64
	Transports      []string
	Name            string
	CreatedAt       time.Time
	LastUsedAt      *time.Time
}

// CredentialStore persists and retrieves WebAuthn credentials.
type CredentialStore struct {
	db PgxPool
}

// NewCredentialStore builds a CredentialStore over the pool.
func NewCredentialStore(db PgxPool) *CredentialStore {
	return &CredentialStore{db: db}
}

const credentialColumns = `id, user_id, credential_id, public_key, attestation_type,
	aaguid, sign_count, transports, name, created_at, last_used_at`

// CreateParams holds the fields needed to persist a freshly-registered credential.
type CreateParams struct {
	UserID          uuid.UUID
	CredentialID    []byte
	PublicKey       []byte
	AttestationType string
	AAGUID          []byte
	SignCount       int64
	Transports      []string
	Name            string
}

// Create inserts a new credential and returns the stored row.
func (s *CredentialStore) Create(ctx context.Context, p CreateParams) (*StoredCredential, error) {
	const q = `INSERT INTO webauthn_credentials
		(user_id, credential_id, public_key, attestation_type, aaguid, sign_count, transports, name)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING ` + credentialColumns
	row := s.db.QueryRow(ctx, q,
		p.UserID, p.CredentialID, p.PublicKey, p.AttestationType, p.AAGUID,
		p.SignCount, p.Transports, p.Name,
	)
	c, err := scanCredential(row)
	if err != nil {
		// 23505 = unique_violation on the credential_id index → the authenticator
		// is already registered (possibly to a different account).
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrCredentialAlreadyRegistered
		}
		return nil, err
	}
	return c, nil
}

// ListByUser returns all credentials owned by userID, newest first.
func (s *CredentialStore) ListByUser(ctx context.Context, userID uuid.UUID) ([]StoredCredential, error) {
	const q = `SELECT ` + credentialColumns + `
		FROM webauthn_credentials WHERE user_id = $1 ORDER BY created_at DESC`
	rows, err := s.db.Query(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []StoredCredential
	for rows.Next() {
		c, err := scanCredential(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// GetByCredentialID returns the credential with the given raw credential id, or
// ErrCredentialNotFound. Used at login to resolve the asserting credential.
func (s *CredentialStore) GetByCredentialID(ctx context.Context, credentialID []byte) (*StoredCredential, error) {
	const q = `SELECT ` + credentialColumns + `
		FROM webauthn_credentials WHERE credential_id = $1`
	row := s.db.QueryRow(ctx, q, credentialID)
	c, err := scanCredential(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCredentialNotFound
		}
		return nil, err
	}
	return c, nil
}

// DeleteForUser removes the credential identified by id, but only when it belongs
// to userID — the cross-user scoping guard. It returns ErrCredentialNotFound when
// no such credential exists for that user (including when it belongs to someone
// else), so a caller can never delete or even probe another account's credential.
func (s *CredentialStore) DeleteForUser(ctx context.Context, userID, id uuid.UUID) error {
	const q = `DELETE FROM webauthn_credentials WHERE id = $1 AND user_id = $2`
	tag, err := s.db.Exec(ctx, q, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrCredentialNotFound
	}
	return nil
}

// UpdateSignCount writes back the new signature counter and stamps last_used_at,
// for the credential identified by its raw credential id. Called after every
// successful assertion (the value having already passed the clone-detection
// check in the login handler).
func (s *CredentialStore) UpdateSignCount(ctx context.Context, credentialID []byte, signCount int64) error {
	const q = `UPDATE webauthn_credentials
		SET sign_count = $1, last_used_at = now() WHERE credential_id = $2`
	tag, err := s.db.Exec(ctx, q, signCount, credentialID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrCredentialNotFound
	}
	return nil
}

// scanCredential scans a row in credentialColumns order.
func scanCredential(row pgx.Row) (*StoredCredential, error) {
	var c StoredCredential
	if err := row.Scan(
		&c.ID, &c.UserID, &c.CredentialID, &c.PublicKey, &c.AttestationType,
		&c.AAGUID, &c.SignCount, &c.Transports, &c.Name, &c.CreatedAt, &c.LastUsedAt,
	); err != nil {
		return nil, err
	}
	return &c, nil
}

// toLibraryCredential converts a stored row to the library's webauthn.Credential
// shape used by the User adapter and the ceremony validators. SignCount is stored
// as bigint but the library uses uint32 (the WebAuthn counter width).
func (c StoredCredential) toLibraryCredential() webauthn.Credential {
	transports := make([]protocol.AuthenticatorTransport, 0, len(c.Transports))
	for _, t := range c.Transports {
		transports = append(transports, protocol.AuthenticatorTransport(t))
	}
	return webauthn.Credential{
		ID:              c.CredentialID,
		PublicKey:       c.PublicKey,
		AttestationType: c.AttestationType,
		Transport:       transports,
		Authenticator: webauthn.Authenticator{
			AAGUID:    c.AAGUID,
			SignCount: uint32(c.SignCount),
		},
	}
}

// transportsToStrings converts library transports to the []string stored in the
// transports text[] column.
func transportsToStrings(ts []protocol.AuthenticatorTransport) []string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, string(t))
	}
	return out
}

// ensure *pgxpool.Pool-compatible types satisfy PgxPool at compile time is left
// to main.go (which passes the real pool); the interface mirrors auth.PgxPool.
