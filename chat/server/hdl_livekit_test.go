package main

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// parseLiveKit verifies the HMAC signature and returns the claims.
func parseLiveKit(t *testing.T, token, secret string) jwt.MapClaims {
	t.Helper()
	claims := jwt.MapClaims{}
	_, err := jwt.ParseWithClaims(token, claims, func(tok *jwt.Token) (any, error) {
		if tok.Method.Alg() != "HS256" {
			t.Fatalf("unexpected alg %s", tok.Method.Alg())
		}
		return []byte(secret), nil
	})
	if err != nil {
		t.Fatalf("token failed to parse/verify: %v", err)
	}
	return claims
}

func TestMintLiveKitToken_Publisher(t *testing.T) {
	now := time.Now()
	tok, err := mintLiveKitToken("devkey", "s3cr3t-secret", "usrABC", "room42", true, 6*time.Hour, now)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	c := parseLiveKit(t, tok, "s3cr3t-secret")

	if c["iss"] != "devkey" {
		t.Errorf("iss = %v, want devkey", c["iss"])
	}
	if c["sub"] != "usrABC" {
		t.Errorf("sub = %v, want usrABC", c["sub"])
	}
	if got := int64(c["exp"].(float64)); got != now.Add(6*time.Hour).Unix() {
		t.Errorf("exp = %d, want %d", got, now.Add(6*time.Hour).Unix())
	}
	video, ok := c["video"].(map[string]any)
	if !ok {
		t.Fatalf("video grant missing/wrong type: %T", c["video"])
	}
	if video["room"] != "room42" {
		t.Errorf("video.room = %v, want room42", video["room"])
	}
	if video["roomJoin"] != true {
		t.Errorf("video.roomJoin = %v, want true", video["roomJoin"])
	}
	if video["canPublish"] != true {
		t.Errorf("video.canPublish = %v, want true", video["canPublish"])
	}
	if video["canSubscribe"] != true {
		t.Errorf("video.canSubscribe = %v, want true", video["canSubscribe"])
	}
}

func TestMintLiveKitToken_ViewOnly(t *testing.T) {
	now := time.Now()
	tok, err := mintLiveKitToken("devkey", "s3cr3t-secret", "usrXYZ", "room1", false, time.Hour, now)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	c := parseLiveKit(t, tok, "s3cr3t-secret")
	video := c["video"].(map[string]any)
	if video["canPublish"] != false {
		t.Errorf("view-only canPublish = %v, want false", video["canPublish"])
	}
	if video["canPublishData"] != false {
		t.Errorf("view-only canPublishData = %v, want false", video["canPublishData"])
	}
	if video["canSubscribe"] != true {
		t.Errorf("view-only canSubscribe = %v, want true", video["canSubscribe"])
	}
}

func TestMintLiveKitToken_WrongSecretRejected(t *testing.T) {
	now := time.Now()
	tok, _ := mintLiveKitToken("devkey", "right-secret", "u", "r", true, time.Hour, now)
	claims := jwt.MapClaims{}
	if _, err := jwt.ParseWithClaims(tok, claims, func(*jwt.Token) (any, error) {
		return []byte("wrong-secret"), nil
	}); err == nil {
		t.Fatal("token verified with the wrong secret")
	}
}

func TestLivekitTokenTTL_Default(t *testing.T) {
	t.Setenv("LIVEKIT_TOKEN_TTL_MIN", "")
	if got := livekitTokenTTL(); got != 6*time.Hour {
		t.Errorf("default TTL = %v, want 6h", got)
	}
}

func TestLivekitTokenTTL_FromEnv(t *testing.T) {
	t.Setenv("LIVEKIT_TOKEN_TTL_MIN", "45")
	if got := livekitTokenTTL(); got != 45*time.Minute {
		t.Errorf("env TTL = %v, want 45m", got)
	}
}
