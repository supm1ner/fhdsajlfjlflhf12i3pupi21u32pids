package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// social_store.go adds the persistence helpers the social-login slice
// (internal/social) needs, without touching the existing password-auth flows.
// It provides:
//   - SocialIdentityStore over the social_identities table (find/link).
//   - UserStore.CreateSocial — create a user from a verified-or-unverified social
//     profile (sets email_verified + avatar_url in one insert).
//   - UserStore.GetByUsernameLike helper is not needed; uniqueness is handled by
//     the resolver via collision-suffixing.

// ErrSocialIdentityNotFound is returned when no social_identities row matches the
// (provider, subject) lookup.
var ErrSocialIdentityNotFound = errors.New("social identity not found")

// SocialIdentity is one external-provider identity linked to a cotton-id user.
type SocialIdentity struct {
	ID              uuid.UUID
	UserID          uuid.UUID
	Provider        string
	ProviderSubject string
	Email           *string
	CreatedAt       time.Time
}

// SocialIdentityStore persists and retrieves linked social identities.
type SocialIdentityStore struct {
	db PgxPool
}

// NewSocialIdentityStore builds a SocialIdentityStore over the pool.
func NewSocialIdentityStore(db PgxPool) *SocialIdentityStore {
	return &SocialIdentityStore{db: db}
}

const socialIdentityColumns = `id, user_id, provider, provider_subject, email, created_at`

// GetByProviderSubject returns the identity for a (provider, subject) pair, or
// ErrSocialIdentityNotFound when none is linked yet.
func (s *SocialIdentityStore) GetByProviderSubject(ctx context.Context, provider, subject string) (*SocialIdentity, error) {
	const q = `SELECT ` + socialIdentityColumns + `
		FROM social_identities WHERE provider = $1 AND provider_subject = $2`
	row := s.db.QueryRow(ctx, q, provider, subject)
	id, err := scanSocialIdentity(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSocialIdentityNotFound
		}
		return nil, err
	}
	return id, nil
}

// Link inserts a new social_identities row binding (provider, subject) to userID.
// The email is the address the provider asserted (which may be unverified); it is
// stored for audit/account-self-service but is not itself a trust signal.
func (s *SocialIdentityStore) Link(ctx context.Context, userID uuid.UUID, provider, subject string, email *string) (*SocialIdentity, error) {
	const q = `INSERT INTO social_identities (user_id, provider, provider_subject, email)
		VALUES ($1, $2, $3, $4)
		RETURNING ` + socialIdentityColumns
	row := s.db.QueryRow(ctx, q, userID, provider, subject, email)
	id, err := scanSocialIdentity(row)
	if err != nil {
		return nil, err
	}
	return id, nil
}

func scanSocialIdentity(row pgx.Row) (*SocialIdentity, error) {
	var si SocialIdentity
	if err := row.Scan(&si.ID, &si.UserID, &si.Provider, &si.ProviderSubject, &si.Email, &si.CreatedAt); err != nil {
		return nil, err
	}
	return &si, nil
}

// CreateSocialUserParams holds the fields needed to create an account from a
// social profile. PasswordHash is intentionally absent: social-created accounts
// have no password (the column is nullable).
type CreateSocialUserParams struct {
	Email         string
	EmailVerified bool
	Username      string
	DisplayName   string
	AvatarURL     *string
}

// CreateSocial inserts a new active, password-less user from a social profile,
// setting email_verified and avatar_url at creation time. Unique-violation errors
// map to ErrEmailTaken / ErrUsernameTaken so the resolver can retry with a
// suffixed username on a username collision.
func (s *UserStore) CreateSocial(ctx context.Context, p CreateSocialUserParams) (*User, error) {
	const q = `INSERT INTO users (email, email_verified, username, display_name, avatar_url, password_hash)
		VALUES ($1, $2, $3, $4, $5, NULL)
		RETURNING ` + userColumns
	row := s.db.QueryRow(ctx, q, p.Email, p.EmailVerified, p.Username, p.DisplayName, p.AvatarURL)
	u, err := scanUser(row)
	if err != nil {
		return nil, mapUserWriteError(err)
	}
	return u, nil
}
