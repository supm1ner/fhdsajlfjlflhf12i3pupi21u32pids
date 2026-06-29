package social

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"cotton-id/internal/auth"
)

// fakeUserStore is an in-memory userStore for resolver unit tests.
type fakeUserStore struct {
	byID       map[uuid.UUID]*auth.User
	byEmail    map[string]*auth.User
	byUsername map[string]bool // taken usernames (to exercise collision-suffixing)
	created    []*auth.User
}

func newFakeUserStore() *fakeUserStore {
	return &fakeUserStore{
		byID:       map[uuid.UUID]*auth.User{},
		byEmail:    map[string]*auth.User{},
		byUsername: map[string]bool{},
	}
}

func (f *fakeUserStore) addExisting(email, username string) *auth.User {
	u := &auth.User{ID: uuid.New(), Email: email, Username: username, Status: auth.StatusActive, EmailVerified: true}
	f.byID[u.ID] = u
	if email != "" {
		f.byEmail[normalizeEmail(email)] = u
	}
	if username != "" {
		f.byUsername[username] = true
	}
	return u
}

func (f *fakeUserStore) GetByID(_ context.Context, id uuid.UUID) (*auth.User, error) {
	if u, ok := f.byID[id]; ok {
		return u, nil
	}
	return nil, auth.ErrUserNotFound
}

func (f *fakeUserStore) GetByEmail(_ context.Context, email string) (*auth.User, error) {
	if u, ok := f.byEmail[normalizeEmail(email)]; ok {
		return u, nil
	}
	return nil, auth.ErrUserNotFound
}

func (f *fakeUserStore) CreateSocial(_ context.Context, p auth.CreateSocialUserParams) (*auth.User, error) {
	if p.Email != "" {
		if _, ok := f.byEmail[normalizeEmail(p.Email)]; ok {
			return nil, auth.ErrEmailTaken
		}
	}
	if f.byUsername[p.Username] {
		return nil, auth.ErrUsernameTaken
	}
	u := &auth.User{
		ID:            uuid.New(),
		Email:         p.Email,
		EmailVerified: p.EmailVerified,
		Username:      p.Username,
		DisplayName:   p.DisplayName,
		AvatarURL:     p.AvatarURL,
		Status:        auth.StatusActive,
	}
	f.byID[u.ID] = u
	if p.Email != "" {
		f.byEmail[normalizeEmail(p.Email)] = u
	}
	f.byUsername[p.Username] = true
	f.created = append(f.created, u)
	return u, nil
}

// fakeIdentityStore is an in-memory identityStore.
type fakeIdentityStore struct {
	byKey  map[string]*auth.SocialIdentity // "provider|subject"
	linked []*auth.SocialIdentity
}

func newFakeIdentityStore() *fakeIdentityStore {
	return &fakeIdentityStore{byKey: map[string]*auth.SocialIdentity{}}
}

func key(provider, subject string) string { return provider + "|" + subject }

func (f *fakeIdentityStore) GetByProviderSubject(_ context.Context, provider, subject string) (*auth.SocialIdentity, error) {
	if si, ok := f.byKey[key(provider, subject)]; ok {
		return si, nil
	}
	return nil, auth.ErrSocialIdentityNotFound
}

func (f *fakeIdentityStore) Link(_ context.Context, userID uuid.UUID, provider, subject string, email *string) (*auth.SocialIdentity, error) {
	si := &auth.SocialIdentity{ID: uuid.New(), UserID: userID, Provider: provider, ProviderSubject: subject, Email: email}
	f.byKey[key(provider, subject)] = si
	f.linked = append(f.linked, si)
	return si, nil
}

func newTestResolver() (*resolver, *fakeUserStore, *fakeIdentityStore) {
	us := newFakeUserStore()
	is := newFakeIdentityStore()
	return newResolver(us, is), us, is
}

// TestResolveExistingIdentity: a known (provider,subject) returns its user.
func TestResolveExistingIdentity(t *testing.T) {
	r, us, is := newTestResolver()
	u := us.addExisting("known@x.com", "known")
	_, _ = is.Link(context.Background(), u.ID, ProviderGoogle, "sub-1", nil)

	res, err := r.Resolve(context.Background(), ProviderGoogle, &Identity{Subject: "sub-1", Email: "known@x.com", EmailVerified: true})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Outcome != outcomeExisting || res.User.ID != u.ID {
		t.Errorf("got outcome=%s user=%v, want existing user %v", res.Outcome, res.User.ID, u.ID)
	}
}

// TestResolveLinkByVerifiedEmail: a verified email matching an existing account
// links the identity to it.
func TestResolveLinkByVerifiedEmail(t *testing.T) {
	r, us, is := newTestResolver()
	u := us.addExisting("link@x.com", "linkuser")

	res, err := r.Resolve(context.Background(), ProviderGitHub, &Identity{Subject: "gh-9", Email: "link@x.com", EmailVerified: true})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Outcome != outcomeLinked || res.User.ID != u.ID {
		t.Errorf("got outcome=%s user=%v, want linked to %v", res.Outcome, res.User.ID, u.ID)
	}
	if len(is.linked) != 1 {
		t.Errorf("expected exactly one link, got %d", len(is.linked))
	}
}

// TestResolveCreateVerified: a verified email with no existing account creates a
// new verified account and links it.
func TestResolveCreateVerified(t *testing.T) {
	r, us, _ := newTestResolver()

	res, err := r.Resolve(context.Background(), ProviderYandex, &Identity{
		Subject: "y-1", Email: "new@x.com", EmailVerified: true, Username: "newbie", Name: "New Bie",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Outcome != outcomeCreated {
		t.Fatalf("outcome = %s, want created", res.Outcome)
	}
	if !res.User.EmailVerified {
		t.Error("created account should be email_verified")
	}
	if res.User.Username != "newbie" {
		t.Errorf("username = %q, want newbie", res.User.Username)
	}
	if len(us.created) != 1 {
		t.Errorf("expected one created user, got %d", len(us.created))
	}
}

// TestResolveNeverLinkOnUnverified is the account-takeover guard: an unverified
// email matching an existing account must NOT link; it creates a separate
// account with email_verified=false.
func TestResolveNeverLinkOnUnverified(t *testing.T) {
	r, us, is := newTestResolver()
	victim := us.addExisting("victim@x.com", "victim")

	res, err := r.Resolve(context.Background(), ProviderGitHub, &Identity{
		Subject: "attacker-sub", Email: "victim@x.com", EmailVerified: false, Username: "victim",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Outcome != outcomeUnverif {
		t.Fatalf("outcome = %s, want created_unverified", res.Outcome)
	}
	if res.User.ID == victim.ID {
		t.Fatal("SECURITY: unverified email must not resolve to the existing victim account")
	}
	if res.User.EmailVerified {
		t.Error("separate account from unverified email must have email_verified=false")
	}
	// The link must point at the new account, never the victim.
	if len(is.linked) != 1 || is.linked[0].UserID == victim.ID {
		t.Fatalf("SECURITY: identity linked to victim account: %+v", is.linked)
	}
	// Username collided with "victim" → must be suffixed.
	if res.User.Username == "victim" {
		t.Errorf("expected a suffixed username on collision, got %q", res.User.Username)
	}
}

// TestResolveUsernameCollisionSuffix verifies numeric-suffix uniquification.
func TestResolveUsernameCollisionSuffix(t *testing.T) {
	r, us, _ := newTestResolver()
	us.byUsername["taken"] = true
	us.byUsername["taken2"] = true

	res, err := r.Resolve(context.Background(), ProviderGoogle, &Identity{
		Subject: "g-x", Email: "fresh@x.com", EmailVerified: true, Username: "taken",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.User.Username != "taken3" {
		t.Errorf("username = %q, want taken3 (suffix past taken, taken2)", res.User.Username)
	}
}

func TestDeriveUsername(t *testing.T) {
	tests := []struct {
		name  string
		id    *Identity
		email string
		want  string
	}{
		{"provider username", &Identity{Username: "octocat"}, "", "octocat"},
		{"email local-part fallback", &Identity{}, "jane.doe@x.com", "jane.doe"},
		{"subject fallback", &Identity{Subject: "12345"}, "", "12345"},
		{"sanitized", &Identity{Username: "a b!c@d"}, "", "abcd"},
		{"padded to min length", &Identity{Username: "ab"}, "", "ab0"},
		{"empty becomes user", &Identity{}, "", "user"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveUsername(tt.id, normalizeEmail(tt.email)); got != tt.want {
				t.Errorf("deriveUsername = %q, want %q", got, tt.want)
			}
		})
	}
}
