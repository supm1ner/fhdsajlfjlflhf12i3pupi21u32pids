package social

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"cotton-id/internal/auth"
)

// resolver.go — the account resolver (design D3), the social-login security crux.
//
// Resolution order for an authenticated provider Identity:
//  1. If social_identities(provider, subject) exists → that user (returning user).
//  2. Else if Identity.EmailVerified AND a user with that email exists → LINK the
//     identity to that user and sign in.
//  3. Else if Identity.EmailVerified (no existing user) → CREATE a new account
//     (username derived + uniquified), link, sign in.
//  4. Else (email UNVERIFIED) → NEVER link to an existing account. Create a
//     SEPARATE account keyed on provider+subject (email stored but
//     email_verified=false), link, sign in.
//
// Rule (4) is the account-takeover guard: linking on an unverified email lets an
// attacker who controls a provider account claiming victim@example.com hijack the
// victim's cotton-id account. We refuse to link on unverified emails, full stop.

// userStore is the subset of auth.UserStore the resolver needs (interface kept
// for unit-testing with fakes).
type userStore interface {
	GetByID(ctx context.Context, id uuid.UUID) (*auth.User, error)
	GetByEmail(ctx context.Context, email string) (*auth.User, error)
	CreateSocial(ctx context.Context, p auth.CreateSocialUserParams) (*auth.User, error)
}

// identityStore is the subset of auth.SocialIdentityStore the resolver needs.
type identityStore interface {
	GetByProviderSubject(ctx context.Context, provider, subject string) (*auth.SocialIdentity, error)
	Link(ctx context.Context, userID uuid.UUID, provider, subject string, email *string) (*auth.SocialIdentity, error)
}

// resolveOutcome describes how an Identity mapped to a cotton-id account, for
// security-event logging.
type resolveOutcome string

const (
	outcomeExisting resolveOutcome = "existing_identity"  // (1) returning user
	outcomeLinked   resolveOutcome = "linked_by_email"    // (2) verified email → existing user
	outcomeCreated  resolveOutcome = "created_verified"   // (3) verified email → new user
	outcomeUnverif  resolveOutcome = "created_unverified" // (4) unverified email → separate new user
)

// resolveResult bundles the resolved user and how it was resolved.
type resolveResult struct {
	User    *auth.User
	Outcome resolveOutcome
}

// resolver implements D3 over the user and identity stores.
type resolver struct {
	users      userStore
	identities identityStore
}

func newResolver(users userStore, identities identityStore) *resolver {
	return &resolver{users: users, identities: identities}
}

// usernameSanitizeRe strips characters not allowed in cotton-id usernames
// (mirrors auth.usernameRe charset: letters, digits, _ . -).
var usernameSanitizeRe = regexp.MustCompile(`[^a-zA-Z0-9_.-]+`)

// Resolve maps an authenticated provider Identity to a cotton-id account per D3.
func (r *resolver) Resolve(ctx context.Context, provider string, id *Identity) (*resolveResult, error) {
	// (1) Known (provider, subject): return the linked user.
	si, err := r.identities.GetByProviderSubject(ctx, provider, id.Subject)
	if err == nil {
		user, uErr := r.users.GetByID(ctx, si.UserID)
		if uErr != nil {
			return nil, uErr
		}
		return &resolveResult{User: user, Outcome: outcomeExisting}, nil
	}
	if !errors.Is(err, auth.ErrSocialIdentityNotFound) {
		return nil, err
	}

	email := normalizeEmail(id.Email)

	// (2) Verified email matching an existing account → link to it.
	if id.EmailVerified && email != "" {
		existing, gErr := r.users.GetByEmail(ctx, email)
		switch {
		case gErr == nil:
			if _, lErr := r.identities.Link(ctx, existing.ID, provider, id.Subject, ptrOrNil(email)); lErr != nil {
				return nil, lErr
			}
			return &resolveResult{User: existing, Outcome: outcomeLinked}, nil
		case errors.Is(gErr, auth.ErrUserNotFound):
			// fall through to create
		default:
			return nil, gErr
		}
	}

	// (3)/(4) Create a new account. The provider email is honored as verified only
	// when it ALSO passes the same validity bar the password path enforces — a
	// malformed/CR-LF address from a provider must never be persisted to
	// users.email (it lands only in social_identities.email via the synthetic path).
	verified := id.EmailVerified && email != "" && auth.ValidEmail(email)

	// The users.email column is UNIQUE NOT NULL, so the new account cannot reuse an
	// email that already belongs to another account. For the UNVERIFIED path
	// (outcome 4) we therefore key the user row on a synthetic, provider-scoped
	// placeholder email and store the *real* (untrusted) email only in
	// social_identities. This guarantees we never collide with — let alone link to
	// — the existing same-email account (the takeover guard). Verified accounts use
	// the real email on the user row.
	userEmail := email
	if !verified {
		userEmail = syntheticEmail(provider, id.Subject)
	}

	user, err := r.createUser(ctx, id, userEmail, email, verified)
	if err != nil {
		// Verified-create can race a concurrent linker that just created the
		// same-email account; in that case link to it rather than failing.
		if verified && errors.Is(err, auth.ErrEmailTaken) {
			existing, gErr := r.users.GetByEmail(ctx, email)
			if gErr != nil {
				return nil, err
			}
			if _, lErr := r.identities.Link(ctx, existing.ID, provider, id.Subject, ptrOrNil(email)); lErr != nil {
				return nil, lErr
			}
			return &resolveResult{User: existing, Outcome: outcomeLinked}, nil
		}
		return nil, err
	}
	if _, err := r.identities.Link(ctx, user.ID, provider, id.Subject, ptrOrNil(email)); err != nil {
		return nil, err
	}
	outcome := outcomeCreated
	if !verified {
		outcome = outcomeUnverif
	}
	return &resolveResult{User: user, Outcome: outcome}, nil
}

// createUser creates a new social account, deriving a username from the provider
// username (else the email local-part, else the provider+subject) and uniquifying
// it with a numeric suffix on collision. userEmail is the value written to the
// users.email column; profileEmail is the provider's real email used only to seed
// the derived username.
func (r *resolver) createUser(ctx context.Context, id *Identity, userEmail, profileEmail string, verified bool) (*auth.User, error) {
	base := deriveUsername(id, profileEmail)
	display := id.Name
	if display == "" {
		display = base
	}

	// Try the base username, then base2, base3, … on a username collision.
	candidate := base
	for attempt := 1; attempt <= 50; attempt++ {
		user, err := r.users.CreateSocial(ctx, auth.CreateSocialUserParams{
			Email:         userEmail,
			EmailVerified: verified,
			Username:      candidate,
			DisplayName:   display,
			AvatarURL:     ptrOrNil(id.AvatarURL),
		})
		if err == nil {
			return user, nil
		}
		if errors.Is(err, auth.ErrUsernameTaken) {
			candidate = base + strconv.Itoa(attempt+1)
			continue
		}
		return nil, err
	}
	return nil, fmt.Errorf("could not allocate a unique username for %q after retries", base)
}

// syntheticEmail builds a unique, provider-scoped placeholder email for an
// account created from an UNVERIFIED social identity, so the users.email unique
// constraint holds without ever colliding with a real account. The real
// (untrusted) email is preserved in social_identities.email. The .invalid TLD is
// reserved (RFC 2606) and can never be a deliverable address.
func syntheticEmail(provider, subject string) string {
	return provider + "_" + subject + "@social.invalid"
}

// deriveUsername builds a base username candidate: the provider username, else the
// email local-part, else "provider_subject". The result is sanitized to the
// allowed charset and padded to the 3-char minimum.
func deriveUsername(id *Identity, email string) string {
	raw := strings.TrimSpace(id.Username)
	if raw == "" && email != "" {
		if at := strings.IndexByte(email, '@'); at > 0 {
			raw = email[:at]
		}
	}
	if raw == "" {
		raw = id.Subject
	}
	raw = usernameSanitizeRe.ReplaceAllString(raw, "")
	if raw == "" {
		raw = "user"
	}
	if len(raw) > 28 {
		// Leave headroom for a numeric suffix (auth caps usernames at 32).
		raw = raw[:28]
	}
	for len(raw) < 3 {
		raw += "0"
	}
	return raw
}

// normalizeEmail lower-cases and trims an email and strips any control
// characters (CR/LF/etc.) a provider may return, matching auth's storage
// normalization (the users.email column is citext) and keeping the value safe
// for the audit log, admin views, and email headers.
func normalizeEmail(email string) string {
	email = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1 // drop control characters
		}
		return r
	}, email)
	return strings.ToLower(strings.TrimSpace(email))
}

// ptrOrNil returns a pointer to s, or nil when s is empty, so an absent value is
// stored as SQL NULL rather than an empty string.
func ptrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
