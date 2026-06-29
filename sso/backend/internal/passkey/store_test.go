package passkey

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// capturePool is a fake PgxPool that records the last SQL + args and returns a
// configurable command tag, so scoping-guard tests can assert the query always
// filters by user_id without a real database.
type capturePool struct {
	lastSQL  string
	lastArgs []any
	tag      pgconn.CommandTag
}

func (p *capturePool) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	p.lastSQL = sql
	p.lastArgs = args
	return p.tag, nil
}

func (p *capturePool) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	p.lastSQL = sql
	p.lastArgs = args
	return nil, errors.New("not used in this test")
}

func (p *capturePool) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	p.lastSQL = sql
	p.lastArgs = args
	return nil
}

// TestDeleteForUserIsScoped asserts the delete is parameterized by BOTH the
// credential id and the owning user id — the cross-user scoping guard. A delete
// query that filtered on id alone would let a user remove another's credential.
func TestDeleteForUserIsScoped(t *testing.T) {
	pool := &capturePool{tag: pgconn.NewCommandTag("DELETE 1")}
	store := NewCredentialStore(pool)

	userID := uuid.New()
	credID := uuid.New()
	if err := store.DeleteForUser(context.Background(), userID, credID); err != nil {
		t.Fatalf("DeleteForUser: %v", err)
	}

	if !strings.Contains(pool.lastSQL, "user_id") {
		t.Errorf("delete SQL must be scoped by user_id, got: %s", pool.lastSQL)
	}
	if len(pool.lastArgs) != 2 {
		t.Fatalf("expected 2 args (id, user_id), got %d: %v", len(pool.lastArgs), pool.lastArgs)
	}
	if pool.lastArgs[0] != credID || pool.lastArgs[1] != userID {
		t.Errorf("delete args = %v, want [%v %v]", pool.lastArgs, credID, userID)
	}
}

// TestDeleteForUserNotFound: zero rows affected (e.g. another user's credential or
// a non-existent id) maps to ErrCredentialNotFound — never a silent success.
func TestDeleteForUserNotFound(t *testing.T) {
	pool := &capturePool{tag: pgconn.NewCommandTag("DELETE 0")}
	store := NewCredentialStore(pool)

	err := store.DeleteForUser(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("err = %v, want ErrCredentialNotFound", err)
	}
}

// TestListByUserIsScoped asserts the list query filters by the owning user id, so
// a user can only ever see their own credentials.
func TestListByUserIsScoped(t *testing.T) {
	pool := &capturePool{}
	store := NewCredentialStore(pool)

	userID := uuid.New()
	_, _ = store.ListByUser(context.Background(), userID) // returns error (no rows); we only inspect the query

	if !strings.Contains(pool.lastSQL, "WHERE user_id = $1") {
		t.Errorf("list SQL must be scoped by user_id, got: %s", pool.lastSQL)
	}
	if len(pool.lastArgs) != 1 || pool.lastArgs[0] != userID {
		t.Errorf("list args = %v, want [%v]", pool.lastArgs, userID)
	}
}
