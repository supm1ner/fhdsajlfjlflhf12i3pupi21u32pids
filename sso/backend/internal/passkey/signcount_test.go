package passkey

import "testing"

func TestSignCountRegressed(t *testing.T) {
	cases := []struct {
		name    string
		stored  uint32
		newC    uint32
		regress bool
	}{
		{"strict increase is fine", 5, 6, false},
		{"large jump is fine", 5, 100, false},
		{"equal counters is a regression", 5, 5, true},
		{"decrease is a regression", 5, 4, true},
		{"first use from zero stored", 0, 1, false},
		{"counterless authenticator (0/0) is allowed", 0, 0, false},
		{"new zero against non-zero stored is a regression", 7, 0, true},
		{"stored zero, new zero — allowed (no counter)", 0, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := signCountRegressed(c.stored, c.newC); got != c.regress {
				t.Errorf("signCountRegressed(%d, %d) = %v, want %v", c.stored, c.newC, got, c.regress)
			}
		})
	}
}
