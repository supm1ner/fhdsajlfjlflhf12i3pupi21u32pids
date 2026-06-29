package admin

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"cotton-id/internal/auth"
)

// recorderPool is a PgxPool that records the SQL+args of the count QueryRow and
// the page Query, returning 0 for the count and no rows for the page. It lets us
// assert ListUsers builds a parameterized, correctly-ordered, filter-aware query.
type recorderPool struct {
	countSQL  string
	countArgs []any
	pageSQL   string
	pageArgs  []any
}

func (p *recorderPool) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (p *recorderPool) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	p.pageSQL = sql
	p.pageArgs = args
	return &noRows{}, nil
}
func (p *recorderPool) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	p.countSQL = sql
	p.countArgs = args
	return countRow{}
}

type countRow struct{}

func (countRow) Scan(dest ...any) error {
	if len(dest) >= 1 {
		if ip, ok := dest[0].(*int); ok {
			*ip = 0
		}
	}
	return nil
}

type noRows struct{}

func (*noRows) Close()                                       {}
func (*noRows) Err() error                                   { return nil }
func (*noRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (*noRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (*noRows) Next() bool                                   { return false }
func (*noRows) Scan(...any) error                            { return nil }
func (*noRows) Values() ([]any, error)                       { return nil, nil }
func (*noRows) RawValues() [][]byte                          { return nil }
func (*noRows) Conn() *pgx.Conn                              { return nil }

func TestListUsersNoFilter(t *testing.T) {
	p := &recorderPool{}
	s := NewStore(p)
	if _, _, err := s.ListUsers(context.Background(), UserFilter{}); err != nil {
		t.Fatalf("ListUsers = %v", err)
	}
	// The services_count subquery legitimately carries its own WHERE; assert there
	// is no top-level filter WHERE on the users table instead.
	if strings.Contains(p.pageSQL, "FROM users u WHERE") {
		t.Errorf("empty filter should produce no top-level WHERE, got: %s", p.pageSQL)
	}
	if strings.Contains(p.countSQL, "WHERE") {
		t.Errorf("empty-filter count should have no WHERE, got: %s", p.countSQL)
	}
	if !strings.Contains(p.pageSQL, "ORDER BY u.created_at DESC") {
		t.Errorf("expected newest-first ordering, got: %s", p.pageSQL)
	}
	// Only limit + offset are bound; default page size applied.
	if len(p.pageArgs) != 2 {
		t.Fatalf("page args = %d, want 2 (limit, offset)", len(p.pageArgs))
	}
	if p.pageArgs[0] != defaultPageSize {
		t.Errorf("limit = %v, want default %d", p.pageArgs[0], defaultPageSize)
	}
	if p.pageArgs[1] != 0 {
		t.Errorf("offset = %v, want 0", p.pageArgs[1])
	}
}

func TestListUsersAllFiltersBound(t *testing.T) {
	p := &recorderPool{}
	s := NewStore(p)
	_, _, err := s.ListUsers(context.Background(), UserFilter{
		Query:    "alex",
		Status:   auth.StatusActive,
		Role:     auth.RoleAdmin,
		Page:     3,
		PageSize: 25,
	})
	if err != nil {
		t.Fatalf("ListUsers = %v", err)
	}
	if !strings.Contains(p.pageSQL, "WHERE") {
		t.Fatalf("expected WHERE for filters, got: %s", p.pageSQL)
	}
	// Search uses ILIKE across the three identity columns, all bound to $1.
	for _, frag := range []string{"u.username ILIKE $1", "u.display_name ILIKE $1", "u.email ILIKE $1"} {
		if !strings.Contains(p.pageSQL, frag) {
			t.Errorf("expected %q in query, got: %s", frag, p.pageSQL)
		}
	}
	// status and role become subsequent bound params.
	if !strings.Contains(p.pageSQL, "u.status = $2") {
		t.Errorf("expected status bound param, got: %s", p.pageSQL)
	}
	if !strings.Contains(p.pageSQL, "u.role = $3") {
		t.Errorf("expected role bound param, got: %s", p.pageSQL)
	}
	// args: pattern, status, role, limit, offset = 5.
	if len(p.pageArgs) != 5 {
		t.Fatalf("page args = %d, want 5", len(p.pageArgs))
	}
	if got, ok := p.pageArgs[0].(string); !ok || got != "%alex%" {
		t.Errorf("search pattern = %v, want %%alex%%", p.pageArgs[0])
	}
	if p.pageArgs[1] != auth.StatusActive {
		t.Errorf("status arg = %v, want active", p.pageArgs[1])
	}
	if p.pageArgs[2] != auth.RoleAdmin {
		t.Errorf("role arg = %v, want admin", p.pageArgs[2])
	}
	// Page 3 of size 25 → offset 50.
	if p.pageArgs[3] != 25 {
		t.Errorf("limit = %v, want 25", p.pageArgs[3])
	}
	if p.pageArgs[4] != 50 {
		t.Errorf("offset = %v, want 50", p.pageArgs[4])
	}
	// The count query shares the 3 filter args (no limit/offset).
	if len(p.countArgs) != 3 {
		t.Fatalf("count args = %d, want 3", len(p.countArgs))
	}
}

func TestListUsersClampsPageSize(t *testing.T) {
	p := &recorderPool{}
	s := NewStore(p)
	if _, _, err := s.ListUsers(context.Background(), UserFilter{Page: -2, PageSize: 9999}); err != nil {
		t.Fatalf("ListUsers = %v", err)
	}
	if p.pageArgs[0] != maxPageSize {
		t.Errorf("limit = %v, want clamped %d", p.pageArgs[0], maxPageSize)
	}
	if p.pageArgs[1] != 0 {
		t.Errorf("offset = %v, want 0 (page clamped to 1)", p.pageArgs[1])
	}
}

func TestListUsersEscapesLikeWildcards(t *testing.T) {
	p := &recorderPool{}
	s := NewStore(p)
	// A search term containing LIKE metacharacters must be escaped so they match
	// literally rather than acting as wildcards.
	if _, _, err := s.ListUsers(context.Background(), UserFilter{Query: "50%_x"}); err != nil {
		t.Fatalf("ListUsers = %v", err)
	}
	pattern, _ := p.pageArgs[0].(string)
	if !strings.Contains(pattern, `\%`) || !strings.Contains(pattern, `\_`) {
		t.Errorf("expected escaped wildcards in pattern, got %q", pattern)
	}
}

func TestEscapeLike(t *testing.T) {
	cases := map[string]string{
		"plain":  "plain",
		"50%":    `50\%`,
		"a_b":    `a\_b`,
		`back\s`: `back\\s`,
	}
	for in, want := range cases {
		if got := escapeLike(in); got != want {
			t.Errorf("escapeLike(%q) = %q, want %q", in, got, want)
		}
	}
}
