package auth

import (
	"errors"
	"testing"
)

func TestStrengthMirrorsPrototype(t *testing.T) {
	t.Parallel()
	cases := []struct {
		pw   string
		want PasswordStrength
	}{
		{"", 0},           // no bits
		{"abc", 0},        // <8, lowercase only
		{"abcdefgh", 1},   // len>=8 only
		{"abcdefgH", 2},   // len + case mix
		{"abcdefg1", 2},   // len + digit
		{"abcdefgH1", 3},  // len + case + digit
		{"abcdefgH1!", 3}, // all four bits, capped at 3
		{"Aa1!", 3},       // <8 but case+digit+symbol => 3 bits
		{"Short1!", 3},    // len 7 <8: case (S/h) + digit + symbol => 3 bits
	}
	for _, c := range cases {
		if got := Strength(c.pw); got != c.want {
			t.Errorf("Strength(%q) = %d, want %d", c.pw, got, c.want)
		}
	}
}

func TestStrengthCappedAtThree(t *testing.T) {
	t.Parallel()
	// Has all four bonuses; must cap at 3 like the prototype's Math.min(s,3).
	if got := Strength("LongEnough1!"); got != 3 {
		t.Errorf("Strength = %d, want 3 (capped)", got)
	}
}

func TestValidatePassword(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		pw      string
		wantErr error
	}{
		{"too short", "Ab1!", ErrPasswordTooShort},
		{"len only too weak", "abcdefgh", ErrPasswordTooWeak},
		{"ok: len + digit", "abcdefg1", nil},
		{"ok: len + case", "abcdefgH", nil},
		{"strong", "abcdefgH1!", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidatePassword(c.pw)
			if c.wantErr == nil && err != nil {
				t.Fatalf("ValidatePassword(%q) = %v, want nil", c.pw, err)
			}
			if c.wantErr != nil && !errors.Is(err, c.wantErr) {
				t.Fatalf("ValidatePassword(%q) = %v, want %v", c.pw, err, c.wantErr)
			}
		})
	}
}

func TestValidatePasswordTooLong(t *testing.T) {
	t.Parallel()
	long := make([]byte, MaxPasswordLength+1)
	for i := range long {
		long[i] = 'a'
	}
	if err := ValidatePassword(string(long)); !errors.Is(err, ErrPasswordTooLong) {
		t.Fatalf("want ErrPasswordTooLong, got %v", err)
	}
}
