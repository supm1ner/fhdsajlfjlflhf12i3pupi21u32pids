package auth

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"sync"
	"time"
)

const (
	codeCharset  = "0123456789"
	codeLength   = 6
	codeTTL      = 10 * time.Minute
	codeCleanInt = 5 * time.Minute
)

// codeEntry stores a verification code with its expiry.
type codeEntry struct {
	Code      string
	ExpiresAt time.Time
	Attempts  int
}

// CodeStore is an in-memory store for email verification codes.
type CodeStore struct {
	mu       sync.RWMutex
	codes    map[string]*codeEntry
	maxAtt   int
}

// NewCodeStore creates a verification code store. maxAttempts limits failed
// attempts per email before the code is invalidated.
func NewCodeStore(maxAttempts int) *CodeStore {
	cs := &CodeStore{
		codes:    make(map[string]*codeEntry),
		maxAtt:   maxAttempts,
	}
	go cs.cleanLoop()
	return cs
}

func (cs *CodeStore) cleanLoop() {
	t := time.NewTicker(codeCleanInt)
	defer t.Stop()
	for range t.C {
		cs.mu.Lock()
		now := time.Now()
		for k, v := range cs.codes {
			if now.After(v.ExpiresAt) {
				delete(cs.codes, k)
			}
		}
		cs.mu.Unlock()
	}
}

// Generate creates a new code for the given email, invalidating any previous one.
func (cs *CodeStore) Generate(ctx context.Context, email string) (string, error) {
	code, err := randomCode()
	if err != nil {
		return "", fmt.Errorf("generate code: %w", err)
	}
	cs.mu.Lock()
	cs.codes[email] = &codeEntry{
		Code:      code,
		ExpiresAt: time.Now().Add(codeTTL),
	}
	cs.mu.Unlock()
	return code, nil
}

// Verify checks a code for the given email. If valid it removes the entry (one-time use).
// Returns true when the code matches and has not expired.
func (cs *CodeStore) Verify(ctx context.Context, email, code string) bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	entry, ok := cs.codes[email]
	if !ok {
		return false
	}
	if time.Now().After(entry.ExpiresAt) {
		delete(cs.codes, email)
		return false
	}
	entry.Attempts++
	if entry.Attempts > cs.maxAtt {
		delete(cs.codes, email)
		return false
	}
	if entry.Code != code {
		return false
	}
	delete(cs.codes, email)
	return true
}

func randomCode() (string, error) {
	b := make([]byte, codeLength)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(codeCharset))))
		if err != nil {
			return "", err
		}
		b[i] = codeCharset[n.Int64()]
	}
	return string(b), nil
}
