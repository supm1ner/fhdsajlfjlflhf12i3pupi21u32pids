package passkey

// signcount.go — the cloned-authenticator decision (design D5).
//
// Per the WebAuthn spec, on each authentication the RP compares the authenticator's
// returned signature counter to the stored value. If the authenticator maintains a
// counter (i.e. either value is non-zero) and the new value does NOT strictly
// exceed the stored one, the credential may have been cloned and authentication
// MUST be refused. Authenticators that do not maintain a counter report 0/0 on
// every assertion; that case is permitted (it is not a regression signal).

// signCountRegressed reports whether newCount represents a sign-count regression
// against stored, i.e. a possible cloned authenticator. It returns false when both
// counters are zero (the authenticator does not use a counter — always allowed).
//
// This mirrors the library's own webauthn.Authenticator.UpdateCounter clone rule
// (authDataCount <= stored && (authDataCount != 0 || stored != 0)) but as a pure,
// independently-testable decision the login handler can act on explicitly before
// persisting the new counter.
func signCountRegressed(stored, newCount uint32) bool {
	if stored == 0 && newCount == 0 {
		return false
	}
	return newCount <= stored
}
