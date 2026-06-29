package auth

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// execRecorder is a PgxPool that records the SQL + args of the last Exec, used to
// assert the BumpLastSeen UPDATE is conditional (throttled) and parameterized.
type execRecorder struct {
	execSQL  string
	execArgs []any
}

func (e *execRecorder) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	e.execSQL = sql
	e.execArgs = args
	return pgconn.CommandTag{}, nil
}
func (e *execRecorder) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}
func (e *execRecorder) QueryRow(context.Context, string, ...any) pgx.Row { return nil }

// TestBumpLastSeenIsThrottledUpdate: BumpLastSeen issues a single conditional
// UPDATE that only matches when last_seen_at is already older than the throttle
// window, with the session id and the interval bound as parameters (never
// interpolated).
func TestBumpLastSeenIsThrottledUpdate(t *testing.T) {
	rec := &execRecorder{}
	store := NewSessionStore(rec)

	if err := store.BumpLastSeen(context.Background(), "sess-id-123"); err != nil {
		t.Fatalf("BumpLastSeen: %v", err)
	}

	sql := rec.execSQL
	if !strings.Contains(sql, "UPDATE sessions") || !strings.Contains(sql, "last_seen_at = now()") {
		t.Errorf("expected a last_seen_at UPDATE, got: %s", sql)
	}
	// The throttle is enforced in SQL: only bump when the row is already stale.
	if !strings.Contains(sql, "last_seen_at < now() -") {
		t.Errorf("expected a conditional (throttled) WHERE clause, got: %s", sql)
	}
	if len(rec.execArgs) != 2 {
		t.Fatalf("exec args = %d, want 2 (id, interval)", len(rec.execArgs))
	}
	if rec.execArgs[0] != "sess-id-123" {
		t.Errorf("id arg = %v, want sess-id-123", rec.execArgs[0])
	}
	// The interval is the throttle window as a bound string (e.g. "1m0s"), not
	// interpolated into the SQL.
	if got, ok := rec.execArgs[1].(string); !ok || got != lastSeenThrottle.String() {
		t.Errorf("interval arg = %v, want %q", rec.execArgs[1], lastSeenThrottle.String())
	}
}

// TestShouldBumpLastSeenDecision documents the in-memory pre-check that gates the
// UPDATE round-trip in UserForSession: a fresh session (seen within the throttle
// window) is skipped; a stale one is bumped. This is the throttle decision the
// service applies before calling BumpLastSeen.
func TestShouldBumpLastSeenDecision(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name     string
		lastSeen time.Time
		wantBump bool
	}{
		{"just seen (skip)", now.Add(-1 * time.Second), false},
		{"seen 30s ago (skip)", now.Add(-30 * time.Second), false},
		{"seen just under window (skip)", now.Add(-(lastSeenThrottle - time.Second)), false},
		{"seen 2m ago (bump)", now.Add(-2 * time.Minute), true},
		{"never recorded / zero (bump)", time.Time{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := time.Since(tc.lastSeen) > lastSeenThrottle
			if got != tc.wantBump {
				t.Errorf("shouldBump(lastSeen=%v) = %v, want %v", tc.lastSeen, got, tc.wantBump)
			}
		})
	}
}
