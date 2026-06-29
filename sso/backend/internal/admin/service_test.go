package admin

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"cotton-id/internal/auth"
)

// fakeUsers is an in-memory userStore for the guard tests. It records the last
// SetStatus/SetRole/Delete so tests can assert the mutation (or its absence).
type fakeUsers struct {
	byID map[uuid.UUID]*auth.User

	statusCalls []statusCall
	roleCalls   []roleCall
	deleteCalls []uuid.UUID

	getErr    error
	statusErr error
	roleErr   error
	deleteErr error
}

type statusCall struct {
	id     uuid.UUID
	status string
}
type roleCall struct {
	id   uuid.UUID
	role string
}

func (f *fakeUsers) GetByID(_ context.Context, id uuid.UUID) (*auth.User, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	u, ok := f.byID[id]
	if !ok {
		return nil, auth.ErrUserNotFound
	}
	// Return a copy so the service mutating the returned user can't corrupt state.
	cp := *u
	return &cp, nil
}

func (f *fakeUsers) SetStatus(_ context.Context, id uuid.UUID, status string) error {
	if f.statusErr != nil {
		return f.statusErr
	}
	f.statusCalls = append(f.statusCalls, statusCall{id, status})
	if u, ok := f.byID[id]; ok {
		u.Status = status
	}
	return nil
}

func (f *fakeUsers) SetRole(_ context.Context, id uuid.UUID, role string) error {
	if f.roleErr != nil {
		return f.roleErr
	}
	f.roleCalls = append(f.roleCalls, roleCall{id, role})
	if u, ok := f.byID[id]; ok {
		u.Role = role
	}
	return nil
}

func (f *fakeUsers) Delete(_ context.Context, id uuid.UUID) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleteCalls = append(f.deleteCalls, id)
	delete(f.byID, id)
	return nil
}

// fakeRevoker records DeleteByUser calls.
type fakeRevoker struct {
	revoked []uuid.UUID
	err     error
}

func (f *fakeRevoker) DeleteByUser(_ context.Context, id uuid.UUID) error {
	if f.err != nil {
		return f.err
	}
	f.revoked = append(f.revoked, id)
	return nil
}

// fakeOwners returns a fixed owner count.
type fakeOwners struct {
	count int
	err   error
}

func (f fakeOwners) CountOwners(context.Context) (int, error) { return f.count, f.err }

// fakeResets records the user a reset was issued for.
type fakeResets struct {
	issued []uuid.UUID
	err    error
}

func (f *fakeResets) IssueResetForUser(_ context.Context, id uuid.UUID) error {
	if f.err != nil {
		return f.err
	}
	f.issued = append(f.issued, id)
	return nil
}

// fakeHydra records the subjects revoked and can fail each call.
type fakeHydra struct {
	consentSubjects []string
	loginSubjects   []string
	consentErr      error
	loginErr        error
}

func (f *fakeHydra) RevokeAllConsentSessions(_ context.Context, subject string) error {
	f.consentSubjects = append(f.consentSubjects, subject)
	return f.consentErr
}
func (f *fakeHydra) RevokeLoginSessions(_ context.Context, subject string) error {
	f.loginSubjects = append(f.loginSubjects, subject)
	return f.loginErr
}

func newUser(role, status string) *auth.User {
	return &auth.User{ID: uuid.New(), Role: role, Status: status, Username: "u-" + role}
}

func buildService(users *fakeUsers, owners fakeOwners, revoke *fakeRevoker, resets *fakeResets, hydra *fakeHydra) *Service {
	return NewService(ServiceDeps{
		Users:    users,
		Owners:   owners,
		Sessions: revoke,
		Resets:   resets,
		Hydra:    hydra,
	})
}

// --- Suspend ---

func TestSuspendRevokesSessions(t *testing.T) {
	admin := newUser(auth.RoleAdmin, auth.StatusActive)
	target := newUser(auth.RoleUser, auth.StatusActive)
	users := &fakeUsers{byID: map[uuid.UUID]*auth.User{admin.ID: admin, target.ID: target}}
	revoke := &fakeRevoker{}
	svc := buildService(users, fakeOwners{count: 1}, revoke, &fakeResets{}, &fakeHydra{})

	if _, err := svc.Suspend(context.Background(), admin, target.ID); err != nil {
		t.Fatalf("Suspend = %v", err)
	}
	if len(users.statusCalls) != 1 || users.statusCalls[0].status != auth.StatusSuspended {
		t.Fatalf("expected status set to suspended, got %+v", users.statusCalls)
	}
	if len(revoke.revoked) != 1 || revoke.revoked[0] != target.ID {
		t.Fatalf("expected sessions revoked for target, got %+v", revoke.revoked)
	}
}

func TestSuspendCannotSuspendSelf(t *testing.T) {
	admin := newUser(auth.RoleAdmin, auth.StatusActive)
	users := &fakeUsers{byID: map[uuid.UUID]*auth.User{admin.ID: admin}}
	revoke := &fakeRevoker{}
	svc := buildService(users, fakeOwners{count: 1}, revoke, &fakeResets{}, &fakeHydra{})

	if _, err := svc.Suspend(context.Background(), admin, admin.ID); !errors.Is(err, ErrSelfAction) {
		t.Fatalf("self-suspend err = %v, want ErrSelfAction", err)
	}
	if len(users.statusCalls) != 0 || len(revoke.revoked) != 0 {
		t.Fatal("self-suspend must not mutate or revoke")
	}
}

func TestSuspendAdminCannotSuspendOwner(t *testing.T) {
	admin := newUser(auth.RoleAdmin, auth.StatusActive)
	owner := newUser(auth.RoleOwner, auth.StatusActive)
	users := &fakeUsers{byID: map[uuid.UUID]*auth.User{admin.ID: admin, owner.ID: owner}}
	svc := buildService(users, fakeOwners{count: 1}, &fakeRevoker{}, &fakeResets{}, &fakeHydra{})

	if _, err := svc.Suspend(context.Background(), admin, owner.ID); !errors.Is(err, ErrOwnerOnly) {
		t.Fatalf("admin suspends owner err = %v, want ErrOwnerOnly", err)
	}
}

func TestSuspendOwnerCanSuspendOwner(t *testing.T) {
	owner := newUser(auth.RoleOwner, auth.StatusActive)
	target := newUser(auth.RoleOwner, auth.StatusActive)
	users := &fakeUsers{byID: map[uuid.UUID]*auth.User{owner.ID: owner, target.ID: target}}
	svc := buildService(users, fakeOwners{count: 2}, &fakeRevoker{}, &fakeResets{}, &fakeHydra{})

	if _, err := svc.Suspend(context.Background(), owner, target.ID); err != nil {
		t.Fatalf("owner suspends owner = %v, want nil", err)
	}
}

func TestSuspendAlreadySuspended(t *testing.T) {
	admin := newUser(auth.RoleAdmin, auth.StatusActive)
	target := newUser(auth.RoleUser, auth.StatusSuspended)
	users := &fakeUsers{byID: map[uuid.UUID]*auth.User{admin.ID: admin, target.ID: target}}
	svc := buildService(users, fakeOwners{count: 1}, &fakeRevoker{}, &fakeResets{}, &fakeHydra{})

	if _, err := svc.Suspend(context.Background(), admin, target.ID); !errors.Is(err, ErrInvalidStatusTransition) {
		t.Fatalf("re-suspend err = %v, want ErrInvalidStatusTransition", err)
	}
}

func TestSuspendUnknownTarget(t *testing.T) {
	admin := newUser(auth.RoleAdmin, auth.StatusActive)
	users := &fakeUsers{byID: map[uuid.UUID]*auth.User{admin.ID: admin}}
	svc := buildService(users, fakeOwners{count: 1}, &fakeRevoker{}, &fakeResets{}, &fakeHydra{})

	if _, err := svc.Suspend(context.Background(), admin, uuid.New()); !errors.Is(err, auth.ErrUserNotFound) {
		t.Fatalf("suspend unknown err = %v, want ErrUserNotFound", err)
	}
}

// --- Reactivate ---

func TestReactivate(t *testing.T) {
	admin := newUser(auth.RoleAdmin, auth.StatusActive)
	target := newUser(auth.RoleUser, auth.StatusSuspended)
	users := &fakeUsers{byID: map[uuid.UUID]*auth.User{admin.ID: admin, target.ID: target}}
	svc := buildService(users, fakeOwners{count: 1}, &fakeRevoker{}, &fakeResets{}, &fakeHydra{})

	if _, err := svc.Reactivate(context.Background(), admin, target.ID); err != nil {
		t.Fatalf("Reactivate = %v", err)
	}
	if users.statusCalls[0].status != auth.StatusActive {
		t.Fatalf("expected status active, got %q", users.statusCalls[0].status)
	}
}

func TestReactivateAlreadyActive(t *testing.T) {
	admin := newUser(auth.RoleAdmin, auth.StatusActive)
	target := newUser(auth.RoleUser, auth.StatusActive)
	users := &fakeUsers{byID: map[uuid.UUID]*auth.User{admin.ID: admin, target.ID: target}}
	svc := buildService(users, fakeOwners{count: 1}, &fakeRevoker{}, &fakeResets{}, &fakeHydra{})

	if _, err := svc.Reactivate(context.Background(), admin, target.ID); !errors.Is(err, ErrInvalidStatusTransition) {
		t.Fatalf("reactivate active err = %v, want ErrInvalidStatusTransition", err)
	}
}

// --- ChangeRole ---

func TestChangeRoleOwnerOnly(t *testing.T) {
	admin := newUser(auth.RoleAdmin, auth.StatusActive)
	target := newUser(auth.RoleUser, auth.StatusActive)
	users := &fakeUsers{byID: map[uuid.UUID]*auth.User{admin.ID: admin, target.ID: target}}
	svc := buildService(users, fakeOwners{count: 1}, &fakeRevoker{}, &fakeResets{}, &fakeHydra{})

	// An admin (not owner) cannot grant roles.
	if _, _, err := svc.ChangeRole(context.Background(), admin, target.ID, auth.RoleAdmin); !errors.Is(err, ErrOwnerOnly) {
		t.Fatalf("admin ChangeRole err = %v, want ErrOwnerOnly", err)
	}
	if len(users.roleCalls) != 0 {
		t.Fatal("rejected role change must not mutate")
	}
}

func TestChangeRoleGrantsAdmin(t *testing.T) {
	owner := newUser(auth.RoleOwner, auth.StatusActive)
	target := newUser(auth.RoleUser, auth.StatusActive)
	users := &fakeUsers{byID: map[uuid.UUID]*auth.User{owner.ID: owner, target.ID: target}}
	svc := buildService(users, fakeOwners{count: 1}, &fakeRevoker{}, &fakeResets{}, &fakeHydra{})

	prev, _, err := svc.ChangeRole(context.Background(), owner, target.ID, auth.RoleAdmin)
	if err != nil {
		t.Fatalf("ChangeRole = %v", err)
	}
	if prev != auth.RoleUser {
		t.Fatalf("prevRole = %q, want user", prev)
	}
	if len(users.roleCalls) != 1 || users.roleCalls[0].role != auth.RoleAdmin {
		t.Fatalf("expected role set to admin, got %+v", users.roleCalls)
	}
}

func TestChangeRoleCannotChangeOwnRole(t *testing.T) {
	owner := newUser(auth.RoleOwner, auth.StatusActive)
	users := &fakeUsers{byID: map[uuid.UUID]*auth.User{owner.ID: owner}}
	svc := buildService(users, fakeOwners{count: 2}, &fakeRevoker{}, &fakeResets{}, &fakeHydra{})

	if _, _, err := svc.ChangeRole(context.Background(), owner, owner.ID, auth.RoleAdmin); !errors.Is(err, ErrSelfAction) {
		t.Fatalf("self role change err = %v, want ErrSelfAction", err)
	}
}

func TestChangeRoleCannotDemoteLastOwner(t *testing.T) {
	owner := newUser(auth.RoleOwner, auth.StatusActive)
	target := newUser(auth.RoleOwner, auth.StatusActive)
	users := &fakeUsers{byID: map[uuid.UUID]*auth.User{owner.ID: owner, target.ID: target}}
	// Only ONE owner remains overall → demoting target to admin would leave zero.
	svc := buildService(users, fakeOwners{count: 1}, &fakeRevoker{}, &fakeResets{}, &fakeHydra{})

	if _, _, err := svc.ChangeRole(context.Background(), owner, target.ID, auth.RoleAdmin); !errors.Is(err, ErrLastOwner) {
		t.Fatalf("demote last owner err = %v, want ErrLastOwner", err)
	}
	if len(users.roleCalls) != 0 {
		t.Fatal("last-owner demotion must not mutate")
	}
}

func TestChangeRoleCanDemoteOwnerWhenOthersExist(t *testing.T) {
	owner := newUser(auth.RoleOwner, auth.StatusActive)
	target := newUser(auth.RoleOwner, auth.StatusActive)
	users := &fakeUsers{byID: map[uuid.UUID]*auth.User{owner.ID: owner, target.ID: target}}
	svc := buildService(users, fakeOwners{count: 2}, &fakeRevoker{}, &fakeResets{}, &fakeHydra{})

	if _, _, err := svc.ChangeRole(context.Background(), owner, target.ID, auth.RoleAdmin); err != nil {
		t.Fatalf("demote owner with 2 owners = %v, want nil", err)
	}
}

func TestChangeRoleInvalidRole(t *testing.T) {
	owner := newUser(auth.RoleOwner, auth.StatusActive)
	target := newUser(auth.RoleUser, auth.StatusActive)
	users := &fakeUsers{byID: map[uuid.UUID]*auth.User{owner.ID: owner, target.ID: target}}
	svc := buildService(users, fakeOwners{count: 1}, &fakeRevoker{}, &fakeResets{}, &fakeHydra{})

	if _, _, err := svc.ChangeRole(context.Background(), owner, target.ID, "superuser"); !errors.Is(err, ErrInvalidRole) {
		t.Fatalf("invalid role err = %v, want ErrInvalidRole", err)
	}
}

func TestChangeRoleNoOpRejected(t *testing.T) {
	owner := newUser(auth.RoleOwner, auth.StatusActive)
	target := newUser(auth.RoleAdmin, auth.StatusActive)
	users := &fakeUsers{byID: map[uuid.UUID]*auth.User{owner.ID: owner, target.ID: target}}
	svc := buildService(users, fakeOwners{count: 1}, &fakeRevoker{}, &fakeResets{}, &fakeHydra{})

	if _, _, err := svc.ChangeRole(context.Background(), owner, target.ID, auth.RoleAdmin); !errors.Is(err, ErrInvalidStatusTransition) {
		t.Fatalf("no-op role change err = %v, want ErrInvalidStatusTransition", err)
	}
}

// --- ResetPassword ---

func TestResetPasswordIssuesToken(t *testing.T) {
	admin := newUser(auth.RoleAdmin, auth.StatusActive)
	target := newUser(auth.RoleUser, auth.StatusActive)
	users := &fakeUsers{byID: map[uuid.UUID]*auth.User{admin.ID: admin, target.ID: target}}
	resets := &fakeResets{}
	svc := buildService(users, fakeOwners{count: 1}, &fakeRevoker{}, resets, &fakeHydra{})

	got, issueErr, err := svc.ResetPassword(context.Background(), admin, target.ID)
	if err != nil || issueErr != nil {
		t.Fatalf("ResetPassword = (err=%v, issueErr=%v)", err, issueErr)
	}
	if got.ID != target.ID {
		t.Fatalf("returned target = %v, want %v", got.ID, target.ID)
	}
	if len(resets.issued) != 1 || resets.issued[0] != target.ID {
		t.Fatalf("expected reset issued for target, got %+v", resets.issued)
	}
}

func TestResetPasswordIssuanceFailureIsBestEffort(t *testing.T) {
	admin := newUser(auth.RoleAdmin, auth.StatusActive)
	target := newUser(auth.RoleUser, auth.StatusActive)
	users := &fakeUsers{byID: map[uuid.UUID]*auth.User{admin.ID: admin, target.ID: target}}
	resets := &fakeResets{err: errors.New("mailer down")}
	svc := buildService(users, fakeOwners{count: 1}, &fakeRevoker{}, resets, &fakeHydra{})

	got, issueErr, err := svc.ResetPassword(context.Background(), admin, target.ID)
	if err != nil {
		t.Fatalf("ResetPassword returned fatal err = %v, want nil (best-effort)", err)
	}
	if issueErr == nil {
		t.Fatal("expected issueErr surfaced for logging")
	}
	if got == nil || got.ID != target.ID {
		t.Fatal("target must still be resolved for the audit entry")
	}
}

func TestResetPasswordUnknownTarget(t *testing.T) {
	admin := newUser(auth.RoleAdmin, auth.StatusActive)
	users := &fakeUsers{byID: map[uuid.UUID]*auth.User{admin.ID: admin}}
	svc := buildService(users, fakeOwners{count: 1}, &fakeRevoker{}, &fakeResets{}, &fakeHydra{})

	got, _, err := svc.ResetPassword(context.Background(), admin, uuid.New())
	if !errors.Is(err, auth.ErrUserNotFound) {
		t.Fatalf("reset unknown err = %v, want ErrUserNotFound", err)
	}
	if got != nil {
		t.Fatal("unknown target must yield nil user")
	}
}

// --- Delete ---

func TestDeleteOwnerOnly(t *testing.T) {
	admin := newUser(auth.RoleAdmin, auth.StatusActive)
	target := newUser(auth.RoleUser, auth.StatusActive)
	users := &fakeUsers{byID: map[uuid.UUID]*auth.User{admin.ID: admin, target.ID: target}}
	svc := buildService(users, fakeOwners{count: 1}, &fakeRevoker{}, &fakeResets{}, &fakeHydra{})

	if _, _, err := svc.Delete(context.Background(), admin, target.ID); !errors.Is(err, ErrOwnerOnly) {
		t.Fatalf("admin Delete err = %v, want ErrOwnerOnly", err)
	}
	if len(users.deleteCalls) != 0 {
		t.Fatal("rejected delete must not mutate")
	}
}

func TestDeleteCannotDeleteSelf(t *testing.T) {
	owner := newUser(auth.RoleOwner, auth.StatusActive)
	users := &fakeUsers{byID: map[uuid.UUID]*auth.User{owner.ID: owner}}
	svc := buildService(users, fakeOwners{count: 2}, &fakeRevoker{}, &fakeResets{}, &fakeHydra{})

	if _, _, err := svc.Delete(context.Background(), owner, owner.ID); !errors.Is(err, ErrSelfAction) {
		t.Fatalf("self delete err = %v, want ErrSelfAction", err)
	}
}

func TestDeleteCannotDeleteLastOwner(t *testing.T) {
	owner := newUser(auth.RoleOwner, auth.StatusActive)
	target := newUser(auth.RoleOwner, auth.StatusActive)
	users := &fakeUsers{byID: map[uuid.UUID]*auth.User{owner.ID: owner, target.ID: target}}
	svc := buildService(users, fakeOwners{count: 1}, &fakeRevoker{}, &fakeResets{}, &fakeHydra{})

	if _, _, err := svc.Delete(context.Background(), owner, target.ID); !errors.Is(err, ErrLastOwner) {
		t.Fatalf("delete last owner err = %v, want ErrLastOwner", err)
	}
}

func TestDeleteCascadesAndRevokesHydra(t *testing.T) {
	owner := newUser(auth.RoleOwner, auth.StatusActive)
	target := newUser(auth.RoleUser, auth.StatusActive)
	users := &fakeUsers{byID: map[uuid.UUID]*auth.User{owner.ID: owner, target.ID: target}}
	hydra := &fakeHydra{}
	svc := buildService(users, fakeOwners{count: 1}, &fakeRevoker{}, &fakeResets{}, hydra)

	got, hydraErr, err := svc.Delete(context.Background(), owner, target.ID)
	if err != nil {
		t.Fatalf("Delete = %v", err)
	}
	if hydraErr != nil {
		t.Fatalf("hydraErr = %v, want nil", hydraErr)
	}
	if got.ID != target.ID {
		t.Fatalf("returned target id = %v, want %v", got.ID, target.ID)
	}
	if len(users.deleteCalls) != 1 || users.deleteCalls[0] != target.ID {
		t.Fatalf("expected delete of target, got %+v", users.deleteCalls)
	}
	subject := target.ID.String()
	if len(hydra.consentSubjects) != 1 || hydra.consentSubjects[0] != subject {
		t.Fatalf("expected consent revoke for %s, got %+v", subject, hydra.consentSubjects)
	}
	if len(hydra.loginSubjects) != 1 || hydra.loginSubjects[0] != subject {
		t.Fatalf("expected login revoke for %s, got %+v", subject, hydra.loginSubjects)
	}
}

func TestDeleteHydraFailureIsBestEffort(t *testing.T) {
	owner := newUser(auth.RoleOwner, auth.StatusActive)
	target := newUser(auth.RoleUser, auth.StatusActive)
	users := &fakeUsers{byID: map[uuid.UUID]*auth.User{owner.ID: owner, target.ID: target}}
	hydra := &fakeHydra{consentErr: errors.New("hydra down")}
	svc := buildService(users, fakeOwners{count: 1}, &fakeRevoker{}, &fakeResets{}, hydra)

	got, hydraErr, err := svc.Delete(context.Background(), owner, target.ID)
	if err != nil {
		t.Fatalf("Delete with hydra error returned err = %v, want nil (best-effort)", err)
	}
	if hydraErr == nil {
		t.Fatal("expected hydraErr surfaced for logging")
	}
	if got == nil || len(users.deleteCalls) != 1 {
		t.Fatal("the account must still be deleted despite the Hydra failure")
	}
}
