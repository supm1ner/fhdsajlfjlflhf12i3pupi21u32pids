package audit

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// queryRecorder is a PgxPool that records the SQL+args of the count QueryRow and
// the page Query, returns 0 for the count, and an empty result for the page. It
// lets us assert the Reader builds a parameterized, correctly-ordered query.
type queryRecorder struct {
	countSQL  string
	countArgs []any
	pageSQL   string
	pageArgs  []any
}

func (q *queryRecorder) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (q *queryRecorder) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	q.pageSQL = sql
	q.pageArgs = args
	return &emptyRows{}, nil
}

func (q *queryRecorder) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	q.countSQL = sql
	q.countArgs = args
	return zeroRow{}
}

// zeroRow scans 0 into the count target.
type zeroRow struct{}

func (zeroRow) Scan(dest ...any) error {
	if len(dest) == 1 {
		if p, ok := dest[0].(*int); ok {
			*p = 0
		}
	}
	return nil
}

// emptyRows is a pgx.Rows that yields no rows.
type emptyRows struct{}

func (*emptyRows) Close()                        {}
func (*emptyRows) Err() error                    { return nil }
func (*emptyRows) CommandTag() pgconn.CommandTag { return pgconn.CommandTag{} }
func (*emptyRows) FieldDescriptions() []pgconn.FieldDescription {
	return nil
}
func (*emptyRows) Next() bool             { return false }
func (*emptyRows) Scan(...any) error      { return nil }
func (*emptyRows) Values() ([]any, error) { return nil, nil }
func (*emptyRows) RawValues() [][]byte    { return nil }
func (*emptyRows) Conn() *pgx.Conn        { return nil }

func TestQueryDefaultsAndOrder(t *testing.T) {
	qr := &queryRecorder{}
	rd := NewReader(qr)

	_, total, err := rd.Query(context.Background(), Filter{})
	if err != nil {
		t.Fatalf("Query = %v", err)
	}
	if total != 0 {
		t.Fatalf("total = %d, want 0", total)
	}
	// No filters → no WHERE clause; ordered newest-first.
	if strings.Contains(qr.pageSQL, "WHERE") {
		t.Errorf("expected no WHERE for empty filter, got: %s", qr.pageSQL)
	}
	if !strings.Contains(qr.pageSQL, "ORDER BY ts DESC") {
		t.Errorf("expected reverse-chronological order, got: %s", qr.pageSQL)
	}
	// limit/offset are the only bound args; default limit applied.
	if len(qr.pageArgs) != 2 {
		t.Fatalf("page args = %d, want 2 (limit, offset)", len(qr.pageArgs))
	}
	if qr.pageArgs[0] != defaultLimit {
		t.Errorf("limit arg = %v, want default %d", qr.pageArgs[0], defaultLimit)
	}
	if qr.pageArgs[1] != 0 {
		t.Errorf("offset arg = %v, want 0", qr.pageArgs[1])
	}
}

func TestQueryFiltersAreBound(t *testing.T) {
	qr := &queryRecorder{}
	rd := NewReader(qr)

	actor := uuid.New()
	from := time.Now().Add(-24 * time.Hour)
	to := time.Now()

	_, _, err := rd.Query(context.Background(), Filter{
		Actor:  &actor,
		Action: ActionLoginOK,
		From:   &from,
		To:     &to,
		Limit:  10,
		Offset: 20,
	})
	if err != nil {
		t.Fatalf("Query = %v", err)
	}

	if !strings.Contains(qr.pageSQL, "WHERE") {
		t.Fatalf("expected a WHERE clause, got: %s", qr.pageSQL)
	}
	for _, frag := range []string{"actor_id = $1", "action = $2", "ts >= $3", "ts < $4"} {
		if !strings.Contains(qr.pageSQL, frag) {
			t.Errorf("expected %q in query, got: %s", frag, qr.pageSQL)
		}
	}
	// Filter args (4) + limit + offset = 6 on the page query.
	if len(qr.pageArgs) != 6 {
		t.Fatalf("page args = %d, want 6", len(qr.pageArgs))
	}
	if qr.pageArgs[0] != actor {
		t.Errorf("actor arg = %v, want %v", qr.pageArgs[0], actor)
	}
	if qr.pageArgs[1] != ActionLoginOK {
		t.Errorf("action arg = %v, want %v", qr.pageArgs[1], ActionLoginOK)
	}
	if qr.pageArgs[4] != 10 {
		t.Errorf("limit arg = %v, want 10", qr.pageArgs[4])
	}
	if qr.pageArgs[5] != 20 {
		t.Errorf("offset arg = %v, want 20", qr.pageArgs[5])
	}
	// The count query shares the 4 filter args (no limit/offset).
	if len(qr.countArgs) != 4 {
		t.Fatalf("count args = %d, want 4", len(qr.countArgs))
	}
}

// TestQueryTargetFilterBound: the target-type/target-id filters are bound
// parameters and appear in the query (backs the admin user-detail activity feed +
// the Journal targetType/targetId params).
func TestQueryTargetFilterBound(t *testing.T) {
	qr := &queryRecorder{}
	rd := NewReader(qr)

	if _, _, err := rd.Query(context.Background(), Filter{
		TargetType: TargetUser,
		TargetID:   "abc-123",
		Limit:      10,
	}); err != nil {
		t.Fatalf("Query = %v", err)
	}

	for _, frag := range []string{"target_type = $1", "target_id = $2"} {
		if !strings.Contains(qr.pageSQL, frag) {
			t.Errorf("expected %q in query, got: %s", frag, qr.pageSQL)
		}
	}
	// 2 filter args + limit + offset.
	if len(qr.pageArgs) != 4 {
		t.Fatalf("page args = %d, want 4", len(qr.pageArgs))
	}
	if qr.pageArgs[0] != TargetUser {
		t.Errorf("target_type arg = %v, want %v", qr.pageArgs[0], TargetUser)
	}
	if qr.pageArgs[1] != "abc-123" {
		t.Errorf("target_id arg = %v, want abc-123", qr.pageArgs[1])
	}
	// The count query shares the 2 filter args (no limit/offset).
	if len(qr.countArgs) != 2 {
		t.Fatalf("count args = %d, want 2", len(qr.countArgs))
	}
}

// TestQueryActorLabelILIKE: a free-text actor-label filter becomes a bound ILIKE
// substring match with the wildcards in the VALUE (not interpolated), and
// LIKE-metacharacters in the input are escaped so they match literally.
func TestQueryActorLabelILIKE(t *testing.T) {
	qr := &queryRecorder{}
	rd := NewReader(qr)

	if _, _, err := rd.Query(context.Background(), Filter{ActorLabel: "al_ice%"}); err != nil {
		t.Fatalf("Query = %v", err)
	}

	if !strings.Contains(qr.pageSQL, "actor_label ILIKE $1") {
		t.Errorf("expected bound ILIKE clause, got: %s", qr.pageSQL)
	}
	// The bound value wraps the escaped input in % wildcards; the literal _ and %
	// in the input are backslash-escaped so they are not treated as wildcards.
	want := `%al\_ice\%%`
	if qr.pageArgs[0] != want {
		t.Errorf("actor_label arg = %q, want %q", qr.pageArgs[0], want)
	}
}

// TestQueryActorIDAndLabelCoexist: actor_id (exact) and actor_label (substring)
// are independent filters that can both apply.
func TestQueryActorIDAndLabelCoexist(t *testing.T) {
	qr := &queryRecorder{}
	rd := NewReader(qr)
	actor := uuid.New()

	if _, _, err := rd.Query(context.Background(), Filter{Actor: &actor, ActorLabel: "bob"}); err != nil {
		t.Fatalf("Query = %v", err)
	}
	if !strings.Contains(qr.pageSQL, "actor_id = $1") || !strings.Contains(qr.pageSQL, "actor_label ILIKE $2") {
		t.Errorf("expected both actor_id and actor_label clauses, got: %s", qr.pageSQL)
	}
}

func TestQueryClampsLimit(t *testing.T) {
	qr := &queryRecorder{}
	rd := NewReader(qr)
	if _, _, err := rd.Query(context.Background(), Filter{Limit: 99999, Offset: -5}); err != nil {
		t.Fatalf("Query = %v", err)
	}
	if qr.pageArgs[0] != maxLimit {
		t.Errorf("limit arg = %v, want clamped %d", qr.pageArgs[0], maxLimit)
	}
	if qr.pageArgs[1] != 0 {
		t.Errorf("offset arg = %v, want clamped 0", qr.pageArgs[1])
	}
}
