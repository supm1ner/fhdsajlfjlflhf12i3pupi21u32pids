// Package oidc implements authentication by validating an OpenID Connect ID token
// issued by an external identity provider (e.g. Ory Hydra fronted by cotton-id).
//
// The client obtains an ID token from the IdP (authorization_code + PKCE) and passes
// the compact JWT to the server as the auth secret: login("oidc", "<id_token>").
// This handler verifies the token's signature against the IdP's JWKS, checks the
// issuer/audience/expiration, maps the token's stable "sub" claim to a local user
// (creating the account on first login when allow_new_accounts is set), and lets the
// existing "token" handler mint the Sunrise session token.
//
// The handler never sees passwords and never mints OIDC tokens — the IdP owns the
// protocol, this server owns the messaging identity.
package oidc

import (
	"crypto"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/golang-jwt/jwt/v5"

	"sunrise/chat/server/auth"
	"sunrise/chat/server/logs"
	"sunrise/chat/server/store"
	"sunrise/chat/server/store/types"
)

// Signing algorithms accepted from the IdP. Symmetric (HS*) and "none" are deliberately excluded.
var validSigningMethods = []string{"RS256", "RS384", "RS512", "ES256", "ES384", "ES512", "PS256", "PS384", "PS512"}

// joseSignatureAlgorithms mirrors validSigningMethods for go-jose JWKS parsing.
var joseSignatureAlgorithms = []jose.SignatureAlgorithm{
	jose.RS256, jose.RS384, jose.RS512,
	jose.ES256, jose.ES384, jose.ES512,
	jose.PS256, jose.PS384, jose.PS512,
}

// authenticator validates external OIDC ID tokens.
type authenticator struct {
	// Logical name of this authenticator.
	name string
	// Expected token issuer, e.g. "http://localhost:4444/". Must match the "iss" claim exactly.
	issuer string
	// Acceptable audiences. The token's "aud" must contain at least one of these.
	audiences []string
	// Allow creating a local account on first successful login.
	allowNewAccounts bool
	// Add the user's email as a searchable tag on account creation.
	addToTags bool
	// Default access modes assigned to auto-provisioned accounts.
	defaultAccessAuth string
	defaultAccessAnon string
	// How often to refresh the cached JWKS.
	jwksRefresh time.Duration

	// Pre-built JWT parser with issuer/expiration/method validation.
	parser *jwt.Parser

	// JWKS cache.
	mu          sync.RWMutex
	keys        map[string]crypto.PublicKey
	keysFetched time.Time
	jwksURI     string
	httpClient  *http.Client
}

// claims is the subset of ID-token claims this handler reads.
type claims struct {
	jwt.RegisteredClaims
	Email             string `json:"email"`
	EmailVerified     bool   `json:"email_verified"`
	Name              string `json:"name"`
	PreferredUsername string `json:"preferred_username"`
}

// discoveryDoc is the subset of the OIDC discovery document this handler reads.
type discoveryDoc struct {
	Issuer  string `json:"issuer"`
	JwksURI string `json:"jwks_uri"`
}

// Init initializes the handler.
func (a *authenticator) Init(jsonconf json.RawMessage, name string) error {
	if name == "" {
		return errors.New("auth_oidc: authenticator name cannot be blank")
	}
	if a.name != "" {
		return errors.New("auth_oidc: already initialized as " + a.name + "; " + name)
	}

	type configType struct {
		// Issuer is the expected "iss" claim, e.g. "http://localhost:4444/".
		Issuer string `json:"issuer"`
		// ClientID is the OAuth client id; used as the default expected audience.
		ClientID string `json:"client_id"`
		// Audiences is an optional explicit list of acceptable audiences. Defaults to [ClientID].
		Audiences []string `json:"audiences"`
		// AllowNewAccounts permits creating a local account on first login.
		AllowNewAccounts bool `json:"allow_new_accounts"`
		// AddToTags adds the user's email as a searchable tag on account creation.
		AddToTags bool `json:"add_to_tags"`
		// JwksRefreshSec is how often (seconds) to refresh the cached JWKS. Default 3600.
		JwksRefreshSec int `json:"jwks_refresh"`
		// DefaultAccess are the access modes for auto-provisioned accounts.
		DefaultAccess struct {
			Auth string `json:"auth"`
			Anon string `json:"anon"`
		} `json:"default_access"`
	}

	var config configType
	if err := json.Unmarshal(jsonconf, &config); err != nil {
		return errors.New("auth_oidc: failed to parse config: " + err.Error() + "(" + string(jsonconf) + ")")
	}

	if config.Issuer == "" {
		return errors.New("auth_oidc: 'issuer' is required")
	}

	audiences := config.Audiences
	if len(audiences) == 0 {
		if config.ClientID == "" {
			return errors.New("auth_oidc: either 'client_id' or 'audiences' must be set")
		}
		audiences = []string{config.ClientID}
	}

	refresh := time.Duration(config.JwksRefreshSec) * time.Second
	if refresh <= 0 {
		refresh = time.Hour
	}

	a.name = name
	a.issuer = config.Issuer
	a.audiences = audiences
	a.allowNewAccounts = config.AllowNewAccounts
	a.addToTags = config.AddToTags
	a.defaultAccessAuth = config.DefaultAccess.Auth
	a.defaultAccessAnon = config.DefaultAccess.Anon
	a.jwksRefresh = refresh
	a.httpClient = &http.Client{Timeout: 10 * time.Second}
	a.parser = jwt.NewParser(
		jwt.WithValidMethods(validSigningMethods),
		jwt.WithIssuer(a.issuer),
		jwt.WithExpirationRequired(),
	)

	return nil
}

// IsInitialized returns true if the handler is initialized.
func (a *authenticator) IsInitialized() bool {
	return a.name != ""
}

// discoveryURL returns the OIDC discovery document URL for the configured issuer.
func (a *authenticator) discoveryURL() string {
	return strings.TrimRight(a.issuer, "/") + "/.well-known/openid-configuration"
}

// refreshKeys fetches the JWKS from the IdP and rebuilds the key cache.
func (a *authenticator) refreshKeys() error {
	// Resolve jwks_uri from discovery if not yet known.
	jwksURI := a.jwksURI
	if jwksURI == "" {
		resp, err := a.httpClient.Get(a.discoveryURL())
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return errors.New("auth_oidc: discovery returned " + resp.Status)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			return err
		}
		var disc discoveryDoc
		if err := json.Unmarshal(body, &disc); err != nil {
			return err
		}
		if disc.JwksURI == "" {
			return errors.New("auth_oidc: discovery document has no jwks_uri")
		}
		jwksURI = disc.JwksURI
	}

	resp, err := a.httpClient.Get(jwksURI)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("auth_oidc: jwks fetch returned " + resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}

	var set jose.JSONWebKeySet
	if err := json.Unmarshal(body, &set); err != nil {
		return err
	}

	keys := make(map[string]crypto.PublicKey, len(set.Keys))
	for i := range set.Keys {
		jwk := set.Keys[i]
		// Only accept signature keys with an accepted algorithm.
		if jwk.Use != "" && jwk.Use != "sig" {
			continue
		}
		keys[jwk.KeyID] = jwk.Public().Key
	}
	if len(keys) == 0 {
		return errors.New("auth_oidc: jwks contains no usable signing keys")
	}

	a.mu.Lock()
	a.jwksURI = jwksURI
	a.keys = keys
	a.keysFetched = time.Now()
	a.mu.Unlock()

	return nil
}

// keyFunc resolves the verification key for a token, refreshing the JWKS on a cache miss.
func (a *authenticator) keyFunc(token *jwt.Token) (any, error) {
	kid, _ := token.Header["kid"].(string)

	a.mu.RLock()
	key, ok := a.keys[kid]
	stale := time.Since(a.keysFetched) > a.jwksRefresh
	a.mu.RUnlock()

	if ok && !stale {
		return key, nil
	}

	// Cache miss or stale cache: refresh and retry once.
	if err := a.refreshKeys(); err != nil {
		// If the refresh failed but we still hold a usable key, use it rather than fail the login.
		if ok {
			return key, nil
		}
		return nil, err
	}

	a.mu.RLock()
	key, ok = a.keys[kid]
	a.mu.RUnlock()
	if !ok {
		return nil, errors.New("auth_oidc: no signing key for kid '" + kid + "'")
	}
	return key, nil
}

// audienceOK reports whether the token's audience is acceptable.
func (a *authenticator) audienceOK(aud jwt.ClaimStrings) bool {
	for _, want := range a.audiences {
		for _, got := range aud {
			if got == want {
				return true
			}
		}
	}
	return false
}

// validate parses and validates the ID token, returning its claims.
func (a *authenticator) validate(secret []byte) (*claims, error) {
	tokenStr := strings.TrimSpace(string(secret))
	if tokenStr == "" {
		return nil, types.ErrMalformed
	}

	cl := &claims{}
	if _, err := a.parser.ParseWithClaims(tokenStr, cl, a.keyFunc); err != nil {
		logs.Warn.Println("auth_oidc: token validation failed:", err)
		return nil, types.ErrFailed
	}
	if !a.audienceOK(cl.Audience) {
		logs.Warn.Println("auth_oidc: token audience rejected:", cl.Audience)
		return nil, types.ErrFailed
	}
	if cl.Subject == "" {
		return nil, types.ErrMalformed
	}
	return cl, nil
}

// AddRecord persists the sub->uid mapping for the given user.
func (a *authenticator) AddRecord(rec *auth.Rec, secret []byte, remoteAddr string) (*auth.Rec, error) {
	cl, err := a.validate(secret)
	if err != nil {
		return nil, err
	}

	authLevel := rec.AuthLevel
	if authLevel == auth.LevelNone {
		authLevel = auth.LevelAuth
	}

	if err := store.Users.AddAuthRecord(rec.Uid, authLevel, a.name, cl.Subject, nil, time.Time{}); err != nil {
		return nil, err
	}

	rec.AuthLevel = authLevel
	rec.Features |= auth.FeatureValidated
	if a.addToTags && cl.Email != "" {
		rec.Tags = append(rec.Tags, "email:"+strings.ToLower(cl.Email))
	}
	return rec, nil
}

// UpdateRecord is not supported: federated identities are managed by the IdP.
func (*authenticator) UpdateRecord(rec *auth.Rec, secret []byte, remoteAddr string) (*auth.Rec, error) {
	return nil, types.ErrUnsupported
}

// Authenticate validates the ID token and maps it to a local user, provisioning one on first login.
func (a *authenticator) Authenticate(secret []byte, remoteAddr string) (*auth.Rec, []byte, error) {
	cl, err := a.validate(secret)
	if err != nil {
		return nil, nil, err
	}

	uid, authLvl, _, expires, err := store.Users.GetAuthUniqueRecord(a.name, cl.Subject)
	if err != nil {
		return nil, nil, err
	}

	if !uid.IsZero() {
		if !expires.IsZero() && expires.Before(time.Now()) {
			return nil, nil, types.ErrExpired
		}
		return &auth.Rec{
			Uid:       uid,
			AuthLevel: authLvl,
			Lifetime:  0,
			Features:  auth.FeatureValidated,
			State:     types.StateUndefined,
		}, nil, nil
	}

	// Unknown subject: provision a new account if allowed.
	if !a.allowNewAccounts {
		return nil, nil, types.ErrFailed
	}

	user := types.User{State: types.StateOK}
	if a.defaultAccessAuth != "" {
		user.Access.Auth.UnmarshalText([]byte(a.defaultAccessAuth))
	}
	if a.defaultAccessAnon != "" {
		user.Access.Anon.UnmarshalText([]byte(a.defaultAccessAnon))
	}
	if cl.Name != "" {
		user.Public = map[string]any{"fn": cl.Name}
	}
	if a.addToTags && cl.Email != "" {
		user.Tags = append(user.Tags, "email:"+strings.ToLower(cl.Email))
	}

	if _, err := store.Users.Create(&user, nil); err != nil {
		logs.Warn.Println("auth_oidc: failed to create user:", err)
		return nil, nil, err
	}

	if err := store.Users.AddAuthRecord(user.Uid(), auth.LevelAuth, a.name, cl.Subject, nil, time.Time{}); err != nil {
		// Roll back the incomplete user record.
		if delErr := store.Users.Delete(user.Uid(), true); delErr != nil {
			logs.Warn.Println("auth_oidc: failed to delete incomplete user record:", delErr)
		}
		return nil, nil, err
	}

	return &auth.Rec{
		Uid:       user.Uid(),
		AuthLevel: auth.LevelAuth,
		Lifetime:  0,
		Features:  auth.FeatureValidated,
		Tags:      user.Tags,
		State:     types.StateUndefined,
	}, nil, nil
}

// AsTag does not produce searchable tags for this scheme.
func (*authenticator) AsTag(token string) string {
	return ""
}

// IsUnique reports whether the token's subject is not yet mapped to a local user.
func (a *authenticator) IsUnique(secret []byte, remoteAddr string) (bool, error) {
	cl, err := a.validate(secret)
	if err != nil {
		return false, err
	}
	uid, _, _, _, err := store.Users.GetAuthUniqueRecord(a.name, cl.Subject)
	if err != nil {
		return false, err
	}
	if uid.IsZero() {
		return true, nil
	}
	return false, types.ErrDuplicate
}

// GenSecret is not supported: session tokens are minted by the "token" handler.
func (*authenticator) GenSecret(rec *auth.Rec) ([]byte, time.Time, error) {
	return nil, time.Time{}, types.ErrUnsupported
}

// DelRecords deletes the OIDC auth records for the given user.
func (a *authenticator) DelRecords(uid types.Uid) error {
	return store.Users.DelAuthRecords(uid, a.name)
}

// RestrictedTags returns tag namespaces restricted by this handler.
func (a *authenticator) RestrictedTags() ([]string, error) {
	return nil, nil
}

// GetResetParams returns no reset parameters: federated identities cannot reset here.
func (*authenticator) GetResetParams(uid types.Uid) (map[string]any, error) {
	return nil, nil
}

const realName = "oidc"

// GetRealName returns the hardcoded name of the authenticator.
func (*authenticator) GetRealName() string {
	return realName
}

func init() {
	store.RegisterAuthScheme(realName, &authenticator{})
}
