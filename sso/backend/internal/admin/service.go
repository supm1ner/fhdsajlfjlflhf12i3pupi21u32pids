package admin

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"cotton-id/internal/auth"
)

// Admin audit actions. These name the admin-lifecycle events the Journal filters
// on. They live in this package (not internal/audit) because they are owned by
// the admin console; internal/audit owns only the cross-cutting auth/oidc events.
const (
	ActionUserSuspend    = "admin.user.suspend"
	ActionUserReactivate = "admin.user.reactivate"
	ActionUserRole       = "admin.user.role"
	ActionUserReset      = "admin.user.reset_password"
	ActionUserDelete     = "admin.user.delete"
	ActionUserMessage    = "admin.user.message"

	// Service (OAuth relying-party client) lifecycle actions, performed from the
	// console's Services tab. These reuse audit.TargetClient + the created/edited/
	// deleted client id as the target.
	ActionServiceCreate         = "admin.service.create"
	ActionServiceUpdate         = "admin.service.update"
	ActionServiceDelete         = "admin.service.delete"
	ActionServiceConsentsRevoke = "admin.service.consents.revoke"
)

// Guard errors. Handlers map these to 4xx problem responses; they are the
// privilege-escalation and last-owner invariants enforced server-side on every
// lifecycle action (design D5 / spec "User lifecycle actions"). They are distinct
// values so handlers can choose the precise status + message.
var (
	// ErrSelfAction is returned when an actor targets their own account for an
	// action that must not be self-applied (suspend, role change, delete).
	ErrSelfAction = errors.New("cannot perform this action on your own account")
	// ErrOwnerOnly is returned when a non-owner attempts an owner-only action
	// (grant/revoke admin or owner, delete a user, or act on an owner).
	ErrOwnerOnly = errors.New("only an owner may perform this action")
	// ErrLastOwner is returned when an action would remove the final owner
	// (demoting or deleting the last owner).
	ErrLastOwner = errors.New("cannot remove the last owner")
	// ErrInvalidRole is returned for a role value outside user|admin|owner.
	ErrInvalidRole = errors.New("role must be one of user, admin, owner")
	// ErrInvalidStatusTransition is returned when a status action does not apply
	// (e.g. reactivating an already-active user, suspending a suspended one).
	ErrInvalidStatusTransition = errors.New("the account is already in the requested state")
)

// owners is the subset of auth.UserStore + admin.Store the service needs.
// Declaring it as an interface keeps the service unit-testable with fakes and
// documents exactly which store methods the guards depend on.
type userStore interface {
	GetByID(ctx context.Context, id uuid.UUID) (*auth.User, error)
	SetStatus(ctx context.Context, id uuid.UUID, status string) error
	SetRole(ctx context.Context, id uuid.UUID, role string) error
	Delete(ctx context.Context, id uuid.UUID) error
}

// sessionRevoker revokes a user's sessions (auth.SessionStore.DeleteByUser).
type sessionRevoker interface {
	DeleteByUser(ctx context.Context, userID uuid.UUID) error
}

// ownerCounter reports the number of owner accounts (admin.Store.CountOwners).
type ownerCounter interface {
	CountOwners(ctx context.Context) (int, error)
}

// resetIssuer issues a single-use password-reset token + link for a user. The
// auth.Service satisfies this via IssueResetForUser (added in this change as an
// exported admin seam over the existing reset-token flow).
type resetIssuer interface {
	IssueResetForUser(ctx context.Context, userID uuid.UUID) error
}

// hydraRevoker is the best-effort Hydra cleanup on account delete: revoke the
// subject's consent + login sessions. *oidc.HydraClient satisfies it.
type hydraRevoker interface {
	RevokeAllConsentSessions(ctx context.Context, subject string) error
	RevokeLoginSessions(ctx context.Context, subject string) error
}

// Service implements the admin lifecycle actions with the privilege-escalation
// guards. It is HTTP-agnostic (no http types) so the guard logic is directly
// unit-testable. Handlers translate its typed errors into problem responses and
// write the audit entries.
type Service struct {
	users  userStore
	owners ownerCounter
	revoke sessionRevoker
	resets resetIssuer
	hydra  hydraRevoker
}

// ServiceDeps wires the Service.
type ServiceDeps struct {
	Users    userStore
	Owners   ownerCounter
	Sessions sessionRevoker
	Resets   resetIssuer
	Hydra    hydraRevoker
}

// NewService builds the admin lifecycle service.
func NewService(d ServiceDeps) *Service {
	return &Service{
		users:  d.Users,
		owners: d.Owners,
		revoke: d.Sessions,
		resets: d.Resets,
		hydra:  d.Hydra,
	}
}

// Suspend sets the target's status to suspended and revokes all their sessions,
// enforcing: a user cannot suspend themselves; an admin cannot suspend an owner
// (only an owner may act on an owner). Returns the target (pre-mutation) for the
// caller's audit entry, or a guard error / auth.ErrUserNotFound.
func (s *Service) Suspend(ctx context.Context, actor *auth.User, targetID uuid.UUID) (*auth.User, error) {
	target, err := s.users.GetByID(ctx, targetID)
	if err != nil {
		return nil, err
	}
	if actor.ID == target.ID {
		return nil, ErrSelfAction
	}
	// Acting on an owner requires owner privilege (admins cannot touch owners).
	if target.Role == auth.RoleOwner && actor.Role != auth.RoleOwner {
		return nil, ErrOwnerOnly
	}
	if target.Status == auth.StatusSuspended {
		return nil, ErrInvalidStatusTransition
	}
	if err := s.users.SetStatus(ctx, targetID, auth.StatusSuspended); err != nil {
		return nil, err
	}
	// Suspension must immediately log the user out everywhere.
	if err := s.revoke.DeleteByUser(ctx, targetID); err != nil {
		return nil, err
	}
	return target, nil
}

// Reactivate sets a suspended target back to active. A user cannot reactivate
// themselves (a suspended user cannot reach the admin API anyway, but the guard
// keeps the invariant explicit and symmetric with Suspend).
func (s *Service) Reactivate(ctx context.Context, actor *auth.User, targetID uuid.UUID) (*auth.User, error) {
	target, err := s.users.GetByID(ctx, targetID)
	if err != nil {
		return nil, err
	}
	if actor.ID == target.ID {
		return nil, ErrSelfAction
	}
	if target.Role == auth.RoleOwner && actor.Role != auth.RoleOwner {
		return nil, ErrOwnerOnly
	}
	if target.Status == auth.StatusActive {
		return nil, ErrInvalidStatusTransition
	}
	if err := s.users.SetStatus(ctx, targetID, auth.StatusActive); err != nil {
		return nil, err
	}
	return target, nil
}

// ChangeRole sets the target's role. This is OWNER-ONLY: only an owner may grant
// or revoke admin/owner. Guards: the new role must be valid; a user cannot change
// their own role (no self-escalation/-demotion); the last owner cannot be demoted.
// Returns the target's previous role for the audit entry.
func (s *Service) ChangeRole(ctx context.Context, actor *auth.User, targetID uuid.UUID, newRole string) (prevRole string, target *auth.User, err error) {
	// Owner-only: this gate is enforced both here and at the route (RequireRole is
	// admin-level for the subtree; owner-only actions re-check in the handler).
	if actor.Role != auth.RoleOwner {
		return "", nil, ErrOwnerOnly
	}
	if !validRole(newRole) {
		return "", nil, ErrInvalidRole
	}
	target, err = s.users.GetByID(ctx, targetID)
	if err != nil {
		return "", nil, err
	}
	if actor.ID == target.ID {
		// An owner cannot change their own role (prevents self-demotion that could
		// strip the last owner via the self-path, and self-escalation generally).
		return "", nil, ErrSelfAction
	}
	if target.Role == newRole {
		// No-op change: nothing to audit, surface as an invalid transition.
		return "", nil, ErrInvalidStatusTransition
	}
	// Demoting an owner away from owner must not remove the final owner.
	if target.Role == auth.RoleOwner && newRole != auth.RoleOwner {
		if err := s.guardNotLastOwner(ctx); err != nil {
			return "", nil, err
		}
	}
	prevRole = target.Role
	if err := s.users.SetRole(ctx, targetID, newRole); err != nil {
		return "", nil, err
	}
	return prevRole, target, nil
}

// ResetPassword issues a single-use reset token + (stubbed) email for the target,
// reusing the auth reset-token flow. Any admin may force a reset. It returns the
// resolved target (for the audit entry) regardless of whether token issuance
// succeeded, so the handler can distinguish a lookup failure (target == nil) from
// a best-effort issuance failure (target != nil, issueErr != nil): the mailer is
// a dev stub, so a delivery failure must not fail the admin action — the token,
// when persisted, is still valid for the admin to re-share the link.
func (s *Service) ResetPassword(ctx context.Context, actor *auth.User, targetID uuid.UUID) (target *auth.User, issueErr error, err error) {
	target, err = s.users.GetByID(ctx, targetID)
	if err != nil {
		return nil, nil, err
	}
	if e := s.resets.IssueResetForUser(ctx, targetID); e != nil {
		return target, e, nil
	}
	return target, nil, nil
}

// Delete removes the target account (cascading sessions/passkeys/social via FK)
// and best-effort revokes their Hydra consent + login sessions. OWNER-ONLY.
// Guards: cannot delete yourself; cannot delete the last owner. The Hydra revoke
// is best-effort: a Hydra error is returned via hydraErr (the DB row is already
// gone) so the caller can log it without failing the delete. Returns the deleted
// target for the audit entry.
func (s *Service) Delete(ctx context.Context, actor *auth.User, targetID uuid.UUID) (target *auth.User, hydraErr error, err error) {
	if actor.Role != auth.RoleOwner {
		return nil, nil, ErrOwnerOnly
	}
	target, err = s.users.GetByID(ctx, targetID)
	if err != nil {
		return nil, nil, err
	}
	if actor.ID == target.ID {
		return nil, nil, ErrSelfAction
	}
	if target.Role == auth.RoleOwner {
		if err := s.guardNotLastOwner(ctx); err != nil {
			return nil, nil, err
		}
	}
	if err := s.users.Delete(ctx, targetID); err != nil {
		return nil, nil, err
	}
	// Best-effort Hydra cleanup AFTER the row is gone: failures do not undo the
	// delete; they are surfaced to the caller for logging only.
	if s.hydra != nil {
		subject := targetID.String()
		if e := s.hydra.RevokeAllConsentSessions(ctx, subject); e != nil {
			hydraErr = e
		}
		if e := s.hydra.RevokeLoginSessions(ctx, subject); e != nil {
			hydraErr = e
		}
	}
	return target, hydraErr, nil
}

// guardNotLastOwner returns ErrLastOwner when only one owner remains, so the
// caller refuses to demote/delete the final owner (the system must always have
// at least one owner).
func (s *Service) guardNotLastOwner(ctx context.Context) error {
	n, err := s.owners.CountOwners(ctx)
	if err != nil {
		return err
	}
	if n <= 1 {
		return ErrLastOwner
	}
	return nil
}

// validRole reports whether r is a recognized role string.
func validRole(r string) bool {
	switch r {
	case auth.RoleUser, auth.RoleAdmin, auth.RoleOwner:
		return true
	default:
		return false
	}
}
