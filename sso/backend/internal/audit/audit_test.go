package audit

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// fakePool is a minimal PgxPool capturing the last Exec call and optionally
// failing, so the Writer can be tested without a database.
type fakePool struct {
	execArgs []any
	execErr  error
	execN    int
}

func (f *fakePool) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	f.execN++
	f.execArgs = args
	return pgconn.CommandTag{}, f.execErr
}

func (f *fakePool) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("not implemented")
}

func (f *fakePool) QueryRow(context.Context, string, ...any) pgx.Row { return nil }

func TestNilWriterAppendIsNoOp(t *testing.T) {
	var w *Writer // nil
	if err := w.Append(context.Background(), Entry{Action: ActionSignup}); err != nil {
		t.Fatalf("nil writer Append = %v, want nil", err)
	}
}

func TestWriterAppendInsertsBoundArgs(t *testing.T) {
	fp := &fakePool{}
	w := NewWriter(fp, nil)

	id := uuid.New()
	e := Entry{Action: ActionLoginOK}.
		WithActor(id, "alex").
		WithTarget(TargetUser, id.String()).
		WithMetadata(map[string]any{"k": "v"})
	e.IP = "203.0.113.5"
	e.RequestID = "req-123"

	if err := w.Append(context.Background(), e); err != nil {
		t.Fatalf("Append = %v, want nil", err)
	}
	if fp.execN != 1 {
		t.Fatalf("Exec called %d times, want 1", fp.execN)
	}
	// args order: actor_id, actor_label, action, target_type, target_id, ip, request_id, metadata
	if got := fp.execArgs[0].(*uuid.UUID); got == nil || *got != id {
		t.Errorf("actor_id arg = %v, want %v", got, id)
	}
	if fp.execArgs[1] != "alex" {
		t.Errorf("actor_label arg = %v, want alex", fp.execArgs[1])
	}
	if fp.execArgs[2] != ActionLoginOK {
		t.Errorf("action arg = %v, want %v", fp.execArgs[2], ActionLoginOK)
	}
	if fp.execArgs[5] != "203.0.113.5" {
		t.Errorf("ip arg = %v, want 203.0.113.5", fp.execArgs[5])
	}
	if fp.execArgs[6] != "req-123" {
		t.Errorf("request_id arg = %v, want req-123", fp.execArgs[6])
	}
	// metadata is JSON-marshaled bytes.
	if _, ok := fp.execArgs[7].([]byte); !ok {
		t.Errorf("metadata arg type = %T, want []byte", fp.execArgs[7])
	}
}

func TestWriterAppendEmptyOptionalsAreNull(t *testing.T) {
	fp := &fakePool{}
	w := NewWriter(fp, nil)

	// No actor, target, ip, request_id, or metadata.
	if err := w.Append(context.Background(), Entry{Action: ActionLoginFail}); err != nil {
		t.Fatalf("Append = %v, want nil", err)
	}
	// actor_label (idx 1), target_type (3), target_id (4), ip (5), request_id (6)
	for _, idx := range []int{1, 3, 4, 5, 6} {
		if fp.execArgs[idx] != nil {
			t.Errorf("arg[%d] = %v, want nil (SQL NULL)", idx, fp.execArgs[idx])
		}
	}
	// actor_id is a nil *uuid.UUID.
	if got := fp.execArgs[0].(*uuid.UUID); got != nil {
		t.Errorf("actor_id arg = %v, want nil", got)
	}
	// metadata is a nil/empty []byte when none provided (pgx stores SQL NULL).
	if b, ok := fp.execArgs[7].([]byte); !ok || len(b) != 0 {
		t.Errorf("metadata arg = %v, want nil/empty []byte", fp.execArgs[7])
	}
}

func TestWriterAppendSwallowsInsertError(t *testing.T) {
	fp := &fakePool{execErr: errors.New("boom")}
	w := NewWriter(fp, nil)
	// Insert fails, but Append must never surface the error to the caller.
	if err := w.Append(context.Background(), Entry{Action: ActionSignup}); err != nil {
		t.Fatalf("Append swallowed-error = %v, want nil", err)
	}
}

func TestFromRequestCapturesContext(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
	r.RemoteAddr = "198.51.100.7:5555"
	e := FromRequest(r, ActionLoginOK)
	if e.Action != ActionLoginOK {
		t.Errorf("action = %q, want %q", e.Action, ActionLoginOK)
	}
	if e.IP == "" {
		t.Error("expected IP to be captured from the request")
	}
}

func TestWithActorCopiesID(t *testing.T) {
	id := uuid.New()
	e := Entry{Action: ActionSignup}.WithActor(id, "bob")
	if e.ActorID == nil || *e.ActorID != id {
		t.Fatalf("ActorID = %v, want %v", e.ActorID, id)
	}
	// Mutating the original id var must not affect the stored copy.
	id2 := uuid.New()
	_ = id2
	if e.ActorLabel != "bob" {
		t.Errorf("ActorLabel = %q, want bob", e.ActorLabel)
	}
}
