package audit

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// defaultLimit / maxLimit bound a Journal page so an unbounded query can never
// be requested.
const (
	defaultLimit = 50
	maxLimit     = 200
)

// Filter selects and pages audit entries for the Journal. All fields are
// optional; the zero Filter returns the most recent page across all actors and
// actions. From/To bound the timestamp range (inclusive lower, exclusive upper).
type Filter struct {
	Actor      *uuid.UUID // exact actor_id match
	ActorLabel string     // case-insensitive substring match on actor_label (ILIKE)
	Action     string     // exact action match (e.g. "auth.login.ok")
	TargetType string     // exact target_type match (e.g. "user", "client")
	TargetID   string     // exact target_id match (the affected entity's id)
	From       *time.Time // ts >= From
	To         *time.Time // ts < To
	Limit      int        // page size (clamped to [1, maxLimit]; 0 → defaultLimit)
	Offset     int        // rows to skip (clamped to >= 0)
}

// Reader serves the Journal: filtered, paginated, reverse-chronological reads
// over the audit log.
type Reader struct {
	db PgxPool
}

// NewReader builds a Reader over the pool.
func NewReader(db PgxPool) *Reader {
	return &Reader{db: db}
}

// Query returns the matching page of audit entries (newest first) and the total
// number of rows that match the filter (ignoring pagination) so callers can
// render page counts. Parameters are bound (never interpolated) so the actor /
// action / time inputs are injection-safe.
func (rd *Reader) Query(ctx context.Context, f Filter) ([]Entry, int, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	// Build the shared WHERE clause + args once for both the count and the page.
	var conds []string
	var args []any
	add := func(cond string, val any) {
		args = append(args, val)
		conds = append(conds, strings.Replace(cond, "?", "$"+strconv.Itoa(len(args)), 1))
	}
	if f.Actor != nil {
		add("actor_id = ?", *f.Actor)
	}
	if f.ActorLabel != "" {
		// Case-insensitive substring match for the Journal's free-text actor input
		// when the value is not a UUID. The % wildcards are part of the BOUND value
		// (not interpolated), so this stays injection-safe; literal % / _ in the
		// input are escaped so they match themselves rather than acting as wildcards.
		add("actor_label ILIKE ?", "%"+escapeLike(f.ActorLabel)+"%")
	}
	if f.Action != "" {
		add("action = ?", f.Action)
	}
	if f.TargetType != "" {
		add("target_type = ?", f.TargetType)
	}
	if f.TargetID != "" {
		add("target_id = ?", f.TargetID)
	}
	if f.From != nil {
		add("ts >= ?", *f.From)
	}
	if f.To != nil {
		add("ts < ?", *f.To)
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	// Total matching rows (for pagination), with the same filter args.
	var total int
	if err := rd.db.QueryRow(ctx, `SELECT count(*) FROM audit_log`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// The page: append limit/offset as the final two bound params.
	limArg := "$" + strconv.Itoa(len(args)+1)
	offArg := "$" + strconv.Itoa(len(args)+2)
	pageArgs := append(append([]any{}, args...), limit, offset)
	q := `SELECT id, ts, actor_id, actor_label, action, target_type, target_id, ip, request_id, metadata
		FROM audit_log` + where + `
		ORDER BY ts DESC, id DESC
		LIMIT ` + limArg + ` OFFSET ` + offArg

	rows, err := rd.db.Query(ctx, q, pageArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]Entry, 0, limit)
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// rowScanner is the subset of pgx.Row/pgx.Rows used to scan an entry.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanEntry scans one audit_log row into an Entry, decoding the nullable text /
// jsonb columns into their Go-friendly forms.
func scanEntry(row rowScanner) (Entry, error) {
	var (
		e          Entry
		actorID    *uuid.UUID
		actorLabel *string
		targetType *string
		targetID   *string
		ip         *string
		requestID  *string
		meta       []byte
	)
	if err := row.Scan(
		&e.ID, &e.Timestamp, &actorID, &actorLabel, &e.Action,
		&targetType, &targetID, &ip, &requestID, &meta,
	); err != nil {
		return Entry{}, err
	}
	e.ActorID = actorID
	e.ActorLabel = deref(actorLabel)
	e.TargetType = deref(targetType)
	e.TargetID = deref(targetID)
	e.IP = deref(ip)
	e.RequestID = deref(requestID)
	if len(meta) > 0 {
		// Best-effort: a malformed metadata blob does not fail the whole query.
		_ = json.Unmarshal(meta, &e.Metadata)
	}
	return e, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// escapeLike escapes the LIKE/ILIKE wildcard metacharacters (backslash, percent,
// underscore) so a free-text actor-label search treats them as literals rather
// than wildcards. Postgres LIKE/ILIKE uses backslash as the default escape
// character, so backslash itself is escaped first.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
