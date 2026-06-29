package auth

import (
	"errors"
	"regexp"
	"unicode/utf8"
)

// PasswordStrength is the 0..3 strength score mirroring the design prototype's
// strength() meter (see _design_ref/screen-auth.jsx). The client meter is
// advisory; the server enforces MinAcceptableStrength.
type PasswordStrength int

const (
	StrengthWeak   PasswordStrength = 0 // 0 and 1 both render as "weak" in the UI
	StrengthOK     PasswordStrength = 2
	StrengthStrong PasswordStrength = 3
)

// Password policy bounds.
const (
	// MinPasswordLength is the hard minimum length (also the first strength bit).
	MinPasswordLength = 8
	// MaxPasswordLength caps length to bound argon2 work / DoS.
	MaxPasswordLength = 256
	// MinAcceptableStrength is the minimum server-enforced strength score. A
	// password must score at least this on Strength() to be accepted. Level 2
	// ("ok") requires length>=8 plus at least one more class of complexity.
	MinAcceptableStrength = StrengthOK
)

var (
	reUpper  = regexp.MustCompile(`[A-Z]`)
	reLower  = regexp.MustCompile(`[a-z]`)
	reDigit  = regexp.MustCompile(`\d`)
	reSymbol = regexp.MustCompile(`[^A-Za-z0-9]`)
)

// Password policy errors.
var (
	ErrPasswordTooShort = errors.New("password must be at least 8 characters")
	ErrPasswordTooLong  = errors.New("password is too long")
	ErrPasswordTooWeak  = errors.New("password is too weak")
)

// Strength scores a password 0..3, exactly mirroring the prototype:
//
//	+1 if len >= 8
//	+1 if it has BOTH an uppercase and a lowercase letter
//	+1 if it has a digit
//	+1 if it has a symbol
//	capped at 3
func Strength(pw string) PasswordStrength {
	s := 0
	if utf8.RuneCountInString(pw) >= 8 {
		s++
	}
	if reUpper.MatchString(pw) && reLower.MatchString(pw) {
		s++
	}
	if reDigit.MatchString(pw) {
		s++
	}
	if reSymbol.MatchString(pw) {
		s++
	}
	if s > 3 {
		s = 3
	}
	return PasswordStrength(s)
}

// ValidatePassword enforces the server-side password policy: length bounds and
// minimum strength. It returns a typed error so handlers can map it to a field
// validation problem.
func ValidatePassword(pw string) error {
	n := utf8.RuneCountInString(pw)
	if n < MinPasswordLength {
		return ErrPasswordTooShort
	}
	if n > MaxPasswordLength {
		return ErrPasswordTooLong
	}
	if Strength(pw) < MinAcceptableStrength {
		return ErrPasswordTooWeak
	}
	return nil
}
