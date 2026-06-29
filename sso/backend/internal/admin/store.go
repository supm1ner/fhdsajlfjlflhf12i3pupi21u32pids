package admin

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"cotton-id/internal/audit"
	"cotton-id/internal/auth"
)

// pageDefaults bound a user-listing page so an unbounded query can never be
// requested (mirrors the audit Reader's clamping).
const (
	defaultPageSize = 20
	maxPageSize     = 100
)

// PgxPool is the minimal pgx pool surface the admin store uses. *pgxpool.Pool
// satisfies it; declaring the interface keeps the store unit-testable with a fake.
type PgxPool interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Store provides the admin console's read aggregates and user-listing queries
// over the cotton-id schema. Lifecycle mutations (set status / role / delete)
// reuse auth.UserStore; this store adds only the admin-specific reads.
type Store struct {
	db PgxPool
}

// NewStore builds a Store over the pool.
func NewStore(db PgxPool) *Store {
	return &Store{db: db}
}

// UserListItem is the row projection the users table needs: identity, status,
// role, joined date, and the per-user connected-services (social identity) count.
type UserListItem struct {
	ID            uuid.UUID
	DisplayName   string
	Username      string
	Email         string
	Status        string
	Role          string
	JoinedAt      time.Time
	ServicesCount int
}

// UserFilter selects and pages the user listing. Query is a case-insensitive
// substring match over username/displayName/email (citext + ILIKE); Status/Role
// are exact-match filters. All fields are optional; the zero filter returns the
// first page of all users newest-first.
type UserFilter struct {
	Query    string
	Status   string
	Role     string
	Page     int // 1-based; <=0 → 1
	PageSize int // clamped to [1, maxPageSize]; 0 → defaultPageSize
}

// normalize clamps the page/size and returns the SQL limit/offset.
func (f UserFilter) normalize() (limit, offset int) {
	size := f.PageSize
	if size <= 0 {
		size = defaultPageSize
	}
	if size > maxPageSize {
		size = maxPageSize
	}
	page := f.Page
	if page <= 0 {
		page = 1
	}
	return size, (page - 1) * size
}

// ListUsers returns a filtered, paginated page of users (newest-first) plus the
// total number of rows matching the filter (ignoring pagination) so callers can
// render page counts. The per-user servicesCount is the number of linked social
// identities. Every input is a bound parameter (never interpolated), so the
// search term is injection-safe even through ILIKE.
func (s *Store) ListUsers(ctx context.Context, f UserFilter) ([]UserListItem, int, error) {
	limit, offset := f.normalize()

	var conds []string
	var args []any
	add := func(cond string, val any) {
		args = append(args, val)
		conds = append(conds, strings.Replace(cond, "?", "$"+strconv.Itoa(len(args)), 1))
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		// citext columns compare case-insensitively; wrap in % for substring match.
		// The pattern is a bound parameter, so the term cannot break out of the LIKE.
		pattern := "%" + escapeLike(q) + "%"
		args = append(args, pattern)
		p := "$" + strconv.Itoa(len(args))
		conds = append(conds, "(u.username ILIKE "+p+" OR u.display_name ILIKE "+p+" OR u.email ILIKE "+p+")")
	}
	if f.Status != "" {
		add("u.status = ?", f.Status)
	}
	if f.Role != "" {
		add("u.role = ?", f.Role)
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	var total int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM users u`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limArg := "$" + strconv.Itoa(len(args)+1)
	offArg := "$" + strconv.Itoa(len(args)+2)
	pageArgs := append(append([]any{}, args...), limit, offset)
	q := `SELECT u.id, u.display_name, u.username, u.email, u.status, u.role, u.created_at,
			(SELECT count(*) FROM social_identities si WHERE si.user_id = u.id) AS services_count
		FROM users u` + where + `
		ORDER BY u.created_at DESC, u.id DESC
		LIMIT ` + limArg + ` OFFSET ` + offArg

	rows, err := s.db.Query(ctx, q, pageArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]UserListItem, 0, limit)
	for rows.Next() {
		var it UserListItem
		if err := rows.Scan(
			&it.ID, &it.DisplayName, &it.Username, &it.Email,
			&it.Status, &it.Role, &it.JoinedAt, &it.ServicesCount,
		); err != nil {
			return nil, 0, err
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// CountByStatus returns the number of users in each status (active/invited/
// suspended). Statuses with zero users are present with a 0 count is NOT
// guaranteed — callers should default-zero a missing key.
func (s *Store) CountByStatus(ctx context.Context) (map[string]int, error) {
	const q = `SELECT status, count(*) FROM users GROUP BY status`
	rows, err := s.db.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]int)
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return nil, err
		}
		out[status] = n
	}
	return out, rows.Err()
}

// CountOwners returns the number of accounts with the owner role. Used by the
// last-owner guards (cannot demote/delete the final owner).
func (s *Store) CountOwners(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRow(ctx, `SELECT count(*) FROM users WHERE role = $1`, auth.RoleOwner).Scan(&n)
	return n, err
}

// SignupPoint is one day of the 30-day daily sign-up series.
type SignupPoint struct {
	Date  time.Time
	Count int
}

// OverviewStats holds the scalar aggregates the overview card row needs.
type OverviewStats struct {
	TotalUsers  int
	ActiveToday int // distinct users with a successful login audited today
	NewThisWeek int // users created in the last 7 days
}

// OverviewStats returns the scalar overview aggregates in a single round trip.
// activeToday counts distinct actors with a successful-login audit entry since
// the start of today (UTC); newThisWeek counts users created within the last 7
// days; totalUsers is the full account count.
func (s *Store) OverviewStats(ctx context.Context) (OverviewStats, error) {
	var st OverviewStats
	const q = `SELECT
		(SELECT count(*) FROM users),
		(SELECT count(DISTINCT actor_id) FROM audit_log
			WHERE action = $1 AND ts >= date_trunc('day', now())),
		(SELECT count(*) FROM users WHERE created_at >= now() - interval '7 days')`
	err := s.db.QueryRow(ctx, q, audit.ActionLoginOK).Scan(&st.TotalUsers, &st.ActiveToday, &st.NewThisWeek)
	return st, err
}

// SignupSeries returns the daily count of new accounts for the last `days` days,
// oldest-first, with zero-filled gaps so the chart has one bar per day.
func (s *Store) SignupSeries(ctx context.Context, days int) ([]SignupPoint, error) {
	if days <= 0 {
		days = 30
	}
	// generate_series produces one row per day (including days with no signups);
	// the LEFT JOIN onto the per-day counts zero-fills the gaps.
	const q = `WITH days AS (
			SELECT generate_series(
				date_trunc('day', now()) - ($1::int - 1) * interval '1 day',
				date_trunc('day', now()),
				interval '1 day'
			) AS d
		),
		counts AS (
			SELECT date_trunc('day', created_at) AS d, count(*) AS n
			FROM users
			WHERE created_at >= date_trunc('day', now()) - ($1::int - 1) * interval '1 day'
			GROUP BY 1
		)
		SELECT days.d, COALESCE(counts.n, 0)
		FROM days LEFT JOIN counts ON counts.d = days.d
		ORDER BY days.d ASC`
	rows, err := s.db.Query(ctx, q, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]SignupPoint, 0, days)
	for rows.Next() {
		var p SignupPoint
		if err := rows.Scan(&p.Date, &p.Count); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// RecentSignups returns the latest n accounts (newest-first) for the overview's
// "recent sign-ups" list.
func (s *Store) RecentSignups(ctx context.Context, n int) ([]UserListItem, error) {
	if n <= 0 {
		n = 5
	}
	const q = `SELECT u.id, u.display_name, u.username, u.email, u.status, u.role, u.created_at,
			(SELECT count(*) FROM social_identities si WHERE si.user_id = u.id) AS services_count
		FROM users u
		ORDER BY u.created_at DESC, u.id DESC
		LIMIT $1`
	rows, err := s.db.Query(ctx, q, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]UserListItem, 0, n)
	for rows.Next() {
		var it UserListItem
		if err := rows.Scan(
			&it.ID, &it.DisplayName, &it.Username, &it.Email,
			&it.Status, &it.Role, &it.JoinedAt, &it.ServicesCount,
		); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// ConnectionsCount returns the number of linked social identities for a user —
// the per-user "connected services" count used in user detail.
func (s *Store) ConnectionsCount(ctx context.Context, userID uuid.UUID) (int, error) {
	var n int
	err := s.db.QueryRow(ctx, `SELECT count(*) FROM social_identities WHERE user_id = $1`, userID).Scan(&n)
	return n, err
}

// ListSubjectIDs returns up to limit user ids (as Hydra subjects, newest-first)
// and a bool that is true when the full set was returned (i.e. the user count did
// not exceed the cap). cotton-id is the only IdP, so these ids are exactly the
// subjects Hydra can hold consent grants for. Used for the BEST-EFFORT per-client
// consent count/revoke: Hydra v2.2.0 exposes no per-client consent query, so the
// console iterates subjects (design D3, and the capability note in oidc/hydra.go).
// A non-positive limit is treated as the maxSubjectScan default applied by the
// caller; the query always binds limit+1 to detect overflow without a second
// round trip.
func (s *Store) ListSubjectIDs(ctx context.Context, limit int) ([]uuid.UUID, bool, error) {
	if limit <= 0 {
		limit = 1
	}
	// Fetch one extra row so we can tell whether the result was truncated.
	const q = `SELECT id FROM users ORDER BY created_at DESC, id DESC LIMIT $1`
	rows, err := s.db.Query(ctx, q, limit+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := make([]uuid.UUID, 0, limit)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, false, err
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	complete := len(out) <= limit
	if !complete {
		out = out[:limit]
	}
	return out, complete, nil
}

// escapeLike escapes the LIKE wildcards (% and _) and the default escape
// character (\) in a user-supplied search term so they are matched literally
// rather than acting as wildcards. The term is still bound as a parameter; this
// only prevents a user from injecting LIKE metacharacters into their own search.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
