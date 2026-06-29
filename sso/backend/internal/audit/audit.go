// Package audit implements cotton-id's persistent, append-only audit log: the
// durable trail backing the admin console's Journal. The whole backend appends
// security-relevant and administrative events here (login ok/fail, signup,
// password reset, OIDC consent, OAuth client registration, admin lifecycle
// actions) in addition to the existing structured slog lines.
//
// The Writer performs a SYNCHRONOUS insert in the request goroutine. At this
// scale that adds negligible latency and keeps the trail simple and reliable.
// Crucially, an audit write must NEVER block or fail the user action: on insert
// failure the Writer logs at error via slog and returns nil. A nil *Writer is a
// valid no-op so handlers and tests can pass an absent writer harmlessly.
//
// The Reader serves the Journal: filtered, paginated, reverse-chronological
// queries over the log.
package audit

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"cotton-id/internal/httpx"
	"cotton-id/internal/observability"
)

// Action constants name the audited events. Centralizing them keeps the action
// vocabulary consistent across the call sites the Journal filters on.
const (
	ActionLoginOK           = "auth.login.ok"
	ActionLoginFail         = "auth.login.fail"
	ActionSignup            = "auth.signup"
	ActionPasswordResetReq  = "auth.password.reset.requested"
	ActionPasswordResetDone = "auth.password.reset.done"
	ActionConsentGrant      = "oidc.consent.grant"
	ActionConsentDeny       = "oidc.consent.deny"
	ActionClientCreate      = "admin.client.create"
	ActionClientDelete      = "admin.client.delete"
)

// Target-type constants name the kinds of entity an entry can target.
const (
	TargetUser    = "user"
	TargetClient  = "client"
	TargetSession = "session"
)

// Entry is one audit-log record. The zero Timestamp is filled by the database
// default (now()) on insert; callers normally leave it unset.
type Entry struct {
	ID         uuid.UUID      `json:"id"`
	Timestamp  time.Time      `json:"ts"`
	ActorID    *uuid.UUID     `json:"actorId,omitempty"`
	ActorLabel string         `json:"actorLabel,omitempty"`
	Action     string         `json:"action"`
	TargetType string         `json:"targetType,omitempty"`
	TargetID   string         `json:"targetId,omitempty"`
	IP         string         `json:"ip,omitempty"`
	RequestID  string         `json:"requestId,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// PgxPool is the minimal pgx pool surface the writer/reader use. *pgxpool.Pool
// satisfies it; declaring the interface keeps audit unit-testable with a fake.
type PgxPool interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Writer appends entries to the audit log. A nil *Writer is a valid no-op.
type Writer struct {
	db  PgxPool
	log *slog.Logger
}

// NewWriter builds a Writer over the pool. log records insert failures; if nil,
// slog.Default() is used.
func NewWriter(db PgxPool, log *slog.Logger) *Writer {
	if log == nil {
		log = slog.Default()
	}
	return &Writer{db: db, log: log}
}

// Append writes one audit entry synchronously. It NEVER blocks or fails the
// user's action: a nil Writer is a no-op, and an insert failure is logged at
// error and swallowed (returns nil). The bool result is always nil-error; the
// signature returns error only so future buffered implementations can surface
// backpressure without changing call sites.
func (w *Writer) Append(ctx context.Context, e Entry) error {
	if w == nil || w.db == nil {
		return nil
	}

	var meta []byte
	if len(e.Metadata) > 0 {
		b, err := json.Marshal(e.Metadata)
		if err != nil {
			// Bad metadata must not lose the event; record it without metadata.
			w.log.Error("audit metadata marshal failed",
				slog.String("action", e.Action), slog.Any("error", err))
		} else {
			meta = b
		}
	}

	// Detach the insert from the request context: the user's action has already
	// committed by the time we audit it, so a client disconnect / proxy timeout
	// (which cancels r.Context()) must NOT drop the audit record for exactly the
	// most security-critical actions (delete, role-elevation, suspend). Bound it
	// with a short timeout so a stuck DB can't leak goroutines.
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	const q = `INSERT INTO audit_log
		(actor_id, actor_label, action, target_type, target_id, ip, request_id, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	_, err := w.db.Exec(writeCtx, q,
		e.ActorID,
		nullString(e.ActorLabel),
		e.Action,
		nullString(e.TargetType),
		nullString(e.TargetID),
		nullString(e.IP),
		nullString(e.RequestID),
		meta,
	)
	if err != nil {
		// Deliberately swallowed: auditing must never block the user action.
		w.log.Error("audit append failed",
			slog.String("action", e.Action),
			slog.String("target_id", e.TargetID),
			slog.Any("error", err),
		)
		return nil
	}
	return nil
}

// FromRequest builds an Entry pre-populated with the request context: the
// trusted-proxy-aware client IP and the correlation request id. Callers set the
// action, actor, and target on the returned entry. It is the canonical way to
// construct an entry from an *http.Request so IP/request-id are always captured.
func FromRequest(r *http.Request, action string) Entry {
	e := Entry{Action: action}
	if r != nil {
		e.IP = httpx.ClientIP(r)
		e.RequestID = observability.RequestID(r.Context())
	}
	return e
}

// WithActor sets the acting user on the entry (id + a human label) and returns
// it, for fluent construction at call sites.
func (e Entry) WithActor(id uuid.UUID, label string) Entry {
	idCopy := id
	e.ActorID = &idCopy
	e.ActorLabel = label
	return e
}

// WithTarget sets the affected entity on the entry and returns it.
func (e Entry) WithTarget(targetType, targetID string) Entry {
	e.TargetType = targetType
	e.TargetID = targetID
	return e
}

// WithMetadata attaches action-specific context to the entry and returns it.
func (e Entry) WithMetadata(m map[string]any) Entry {
	e.Metadata = m
	return e
}

// nullString maps the empty string to nil so blank optional columns are stored
// as SQL NULL rather than empty strings (cleaner for the Journal + filters).
func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
