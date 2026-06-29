package oidc

import "cotton-id/internal/auth"

// claims.go — maps an authenticated *auth.User to ID-token claims when accepting
// consent, honoring the granted scopes (spec: "ID token claims"). The subject is
// always the stable, non-reassignable account id; the standard profile/email
// claims are populated only when the corresponding scope was granted.
//
//	sub                = user.ID.String()   // always (stable identifier)
//	email              = user.Email         // scope: email
//	email_verified     = user.EmailVerified // scope: email
//	name               = user.DisplayName   // scope: profile
//	preferred_username = user.Username      // scope: profile

// Scope constants for the OIDC standard scopes cotton-id maps claims for.
const (
	ScopeOpenID  = "openid"
	ScopeProfile = "profile"
	ScopeEmail   = "email"
)

// IDTokenClaims is the claim set placed in Hydra's consent session id_token. Only
// populated fields are serialized (omitempty), so a token never carries a claim
// for a scope the user did not grant.
type IDTokenClaims struct {
	Subject           string `json:"sub"`
	Email             string `json:"email,omitempty"`
	EmailVerified     bool   `json:"email_verified,omitempty"`
	Name              string `json:"name,omitempty"`
	PreferredUsername string `json:"preferred_username,omitempty"`
}

// ClaimsForUser builds the ID-token claims for a user given the granted scopes.
// `sub` is always set to the stable account id. The email/profile claims are
// gated on the matching granted scope so the relying party only receives what
// the user consented to.
func ClaimsForUser(u *auth.User, grantedScopes []string) IDTokenClaims {
	c := IDTokenClaims{Subject: u.ID.String()}
	for _, s := range grantedScopes {
		switch s {
		case ScopeEmail:
			c.Email = u.Email
			c.EmailVerified = u.EmailVerified
		case ScopeProfile:
			c.Name = u.DisplayName
			c.PreferredUsername = u.Username
		}
	}
	return c
}

// scopeIntersection returns the elements of requested that are also present in
// allowed, preserving the order of `requested`. It is used to grant only the
// scopes the user actually consents to (here: all requested scopes that cotton-id
// recognizes / the client requested), never widening beyond the request.
func scopeIntersection(requested, allowed []string) []string {
	if len(requested) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(allowed))
	for _, s := range allowed {
		set[s] = struct{}{}
	}
	out := make([]string, 0, len(requested))
	for _, s := range requested {
		if _, ok := set[s]; ok {
			out = append(out, s)
		}
	}
	return out
}
