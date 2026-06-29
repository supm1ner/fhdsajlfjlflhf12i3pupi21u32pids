package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Account status values.
const (
	StatusActive    = "active"
	StatusInvited   = "invited"
	StatusSuspended = "suspended"
)

// User is a cotton-id account. PasswordHash is never serialized to clients.
type User struct {
	ID            uuid.UUID
	Email         string
	EmailVerified bool
	Username      string
	DisplayName   string
	PasswordHash  *string // nil for social-only accounts
	Status        string
	Role          string
	About         string
	Location      string
	AvatarURL     *string // nil when no avatar is set (e.g. password-only accounts)
	BannerURL     *string // nil when no banner is set
	// Server-persisted preferences (self-service, change add-account-self-service).
	PrefTheme          string // dark|light|system
	PrefLang           string // ru|en
	LoginNotifications bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// PublicUser is the client-safe projection returned by the API (camelCase JSON).
type PublicUser struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"emailVerified"`
	Username      string `json:"username"`
	DisplayName   string `json:"displayName"`
	Role          string `json:"role"`
	About         string `json:"about"`
	Location      string `json:"location"`
}

// Public returns the client-safe projection of u.
func (u *User) Public() PublicUser {
	return PublicUser{
		ID:            u.ID.String(),
		Email:         u.Email,
		EmailVerified: u.EmailVerified,
		Username:      u.Username,
		DisplayName:   u.DisplayName,
		Role:          u.Role,
		About:         u.About,
		Location:      u.Location,
	}
}

// Store errors.
var (
	// ErrUserNotFound is returned when no user matches the lookup.
	ErrUserNotFound = errors.New("user not found")
	// ErrEmailTaken / ErrUsernameTaken signal unique-constraint violations.
	ErrEmailTaken    = errors.New("email already in use")
	ErrUsernameTaken = errors.New("username already in use")
)

// PgxPool is the minimal pgx pool surface the stores use. *pgxpool.Pool
// satisfies it; declaring the interface keeps the stores unit-testable.
type PgxPool interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// UserStore persists and retrieves users.
type UserStore struct {
	db PgxPool
}

// NewUserStore builds a UserStore over the pool.
func NewUserStore(db PgxPool) *UserStore {
	return &UserStore{db: db}
}

const userColumns = `id, email, email_verified, username, display_name, password_hash,
	status, role, about, location, avatar_url, banner_url,
	pref_theme, pref_lang, login_notifications, created_at, updated_at`

// CreateUserParams holds the fields needed to create an account.
type CreateUserParams struct {
	Email        string
	Username     string
	DisplayName  string
	PasswordHash string
}

// Create inserts a new active user. Unique-violation errors are mapped to
// ErrEmailTaken / ErrUsernameTaken so callers can respond without leaking which
// constraint failed beyond the field name.
func (s *UserStore) Create(ctx context.Context, p CreateUserParams) (*User, error) {
	const q = `INSERT INTO users (email, username, display_name, password_hash)
		VALUES ($1, $2, $3, $4)
		RETURNING ` + userColumns
	row := s.db.QueryRow(ctx, q, p.Email, p.Username, p.DisplayName, p.PasswordHash)
	u, err := scanUser(row)
	if err != nil {
		return nil, mapUserWriteError(err)
	}
	return u, nil
}

// GetByEmail returns the user with the given email (case-insensitive via citext).
func (s *UserStore) GetByEmail(ctx context.Context, email string) (*User, error) {
	const q = `SELECT ` + userColumns + ` FROM users WHERE email = $1`
	return s.getOne(ctx, q, email)
}

// GetByUsername returns the user with the given username.
func (s *UserStore) GetByUsername(ctx context.Context, username string) (*User, error) {
	const q = `SELECT ` + userColumns + ` FROM users WHERE username = $1`
	return s.getOne(ctx, q, username)
}

// GetByID returns the user with the given id.
func (s *UserStore) GetByID(ctx context.Context, id uuid.UUID) (*User, error) {
	const q = `SELECT ` + userColumns + ` FROM users WHERE id = $1`
	return s.getOne(ctx, q, id)
}

// UpdatePassword sets a new password hash and bumps updated_at.
func (s *UserStore) UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) error {
	const q = `UPDATE users SET password_hash = $1, updated_at = now() WHERE id = $2`
	tag, err := s.db.Exec(ctx, q, passwordHash, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// SetStatus updates a user's account status.
func (s *UserStore) SetStatus(ctx context.Context, id uuid.UUID, status string) error {
	const q = `UPDATE users SET status = $1, updated_at = now() WHERE id = $2`
	tag, err := s.db.Exec(ctx, q, status, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// SetRole updates a user's role (user|admin|owner). The admin console uses this
// for owner-gated role changes; the caller is responsible for validating the
// role value and enforcing the privilege-escalation guards (last-owner,
// owner-only, no self-change).
func (s *UserStore) SetRole(ctx context.Context, id uuid.UUID, role string) error {
	const q = `UPDATE users SET role = $1, updated_at = now() WHERE id = $2`
	tag, err := s.db.Exec(ctx, q, role, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// UpdateProfileParams holds the editable public-profile fields.
type UpdateProfileParams struct {
	DisplayName string
	About       string
	Location    string
}

// UpdateProfile saves the user's editable public profile (display name, about,
// location) and returns the refreshed row. Callers are expected to have validated
// and bounded the inputs.
func (s *UserStore) UpdateProfile(ctx context.Context, id uuid.UUID, p UpdateProfileParams) (*User, error) {
	const q = `UPDATE users
		SET display_name = $1, about = $2, location = $3, updated_at = now()
		WHERE id = $4
		RETURNING ` + userColumns
	row := s.db.QueryRow(ctx, q, p.DisplayName, p.About, p.Location, id)
	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return u, nil
}

// UpdatePreferencesParams holds the server-persisted user preferences.
type UpdatePreferencesParams struct {
	Theme              string // dark|light|system
	Lang               string // ru|en
	LoginNotifications bool
}

// UpdatePreferences saves the user's theme/language/login-notification
// preferences and returns the refreshed row.
func (s *UserStore) UpdatePreferences(ctx context.Context, id uuid.UUID, p UpdatePreferencesParams) (*User, error) {
	const q = `UPDATE users
		SET pref_theme = $1, pref_lang = $2, login_notifications = $3, updated_at = now()
		WHERE id = $4
		RETURNING ` + userColumns
	row := s.db.QueryRow(ctx, q, p.Theme, p.Lang, p.LoginNotifications, id)
	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return u, nil
}

// SetImageURL points the user's avatar_url or banner_url at the served URL for an
// uploaded image. kind must be "avatar" or "banner"; any other value is a no-op
// error so callers cannot write an arbitrary column.
func (s *UserStore) SetImageURL(ctx context.Context, id uuid.UUID, kind, urlValue string) error {
	var col string
	switch kind {
	case "avatar":
		col = "avatar_url"
	case "banner":
		col = "banner_url"
	default:
		return errors.New("unknown image kind")
	}
	// col is from a fixed allow-list above (never user input), so this format is
	// not an injection vector; the value is still a parameter.
	q := `UPDATE users SET ` + col + ` = $1, updated_at = now() WHERE id = $2`
	tag, err := s.db.Exec(ctx, q, urlValue, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// Delete removes the user row. FK ON DELETE CASCADE removes the user's sessions,
// passkeys, social identities, reset tokens, and profile images.
func (s *UserStore) Delete(ctx context.Context, id uuid.UUID) error {
	const q = `DELETE FROM users WHERE id = $1`
	tag, err := s.db.Exec(ctx, q, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (s *UserStore) getOne(ctx context.Context, q string, arg any) (*User, error) {
	row := s.db.QueryRow(ctx, q, arg)
	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return u, nil
}

// scanUser scans a row in userColumns order.
func scanUser(row pgx.Row) (*User, error) {
	var u User
	err := row.Scan(
		&u.ID, &u.Email, &u.EmailVerified, &u.Username, &u.DisplayName, &u.PasswordHash,
		&u.Status, &u.Role, &u.About, &u.Location, &u.AvatarURL, &u.BannerURL,
		&u.PrefTheme, &u.PrefLang, &u.LoginNotifications, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// mapUserWriteError maps Postgres unique-violation (SQLSTATE 23505) on the email
// or username constraint to the typed sentinel errors.
func mapUserWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		switch pgErr.ConstraintName {
		case "users_email_key":
			return ErrEmailTaken
		case "users_username_key":
			return ErrUsernameTaken
		}
		// Fall back to message inspection if constraint name differs.
		if contains(pgErr.ConstraintName, "email") {
			return ErrEmailTaken
		}
		if contains(pgErr.ConstraintName, "username") {
			return ErrUsernameTaken
		}
	}
	return err
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
