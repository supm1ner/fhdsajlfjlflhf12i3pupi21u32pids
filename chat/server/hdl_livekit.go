// LiveKit access-token endpoint for SFU-based group calls.
//
// The browser/app cannot mint its own LiveKit token (the API secret must stay server-side),
// so an authenticated Sunrise user requests a short-lived token here. The token is a standard
// LiveKit JWT (HS256 signed with the API secret) carrying a video grant scoped to one room.
//
// Configuration via environment variables:
//
//	LIVEKIT_URL         e.g. wss://livekit.example.com   (returned to the client)
//	LIVEKIT_API_KEY     LiveKit API key (becomes the JWT issuer)
//	LIVEKIT_API_SECRET  LiveKit API secret (HMAC signing key)
//
// If unset, the endpoint reports 501 Not Implemented so calls fall back to the mesh path.
package main

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"sunrise/chat/server/logs"
	"sunrise/chat/server/store/types"
)

type livekitTokenResponse struct {
	URL      string `json:"url"`
	Token    string `json:"token"`
	Room     string `json:"room"`
	Identity string `json:"identity"`
}

// livekitTokenHandler issues a LiveKit access token for an authenticated user and room.
func livekitTokenHandler(wrt http.ResponseWriter, req *http.Request) {
	now := types.TimeNow()
	wrt.Header().Set("Content-Type", "application/json; charset=utf-8")

	writeErr := func(code int, text string) {
		wrt.WriteHeader(code)
		json.NewEncoder(wrt).Encode(map[string]any{"ctrl": map[string]any{"code": code, "text": text, "ts": now}})
	}

	if req.Method != http.MethodGet && req.Method != http.MethodPost {
		writeErr(http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	url := os.Getenv("LIVEKIT_URL")
	apiKey := os.Getenv("LIVEKIT_API_KEY")
	apiSecret := os.Getenv("LIVEKIT_API_SECRET")
	if url == "" || apiKey == "" || apiSecret == "" {
		writeErr(http.StatusNotImplemented, "LiveKit is not configured")
		return
	}

	// API key check (same as other HTTP endpoints).
	if isValid, _ := checkAPIKey(getAPIKey(req)); !isValid {
		writeErr(http.StatusForbidden, "valid API key required")
		return
	}

	// Authenticate the Sunrise user.
	authMethod, secret := getHttpAuth(req)
	uid, challenge, err := authFileRequest(authMethod, secret, req.FormValue("sid"), getRemoteAddr(req))
	if err != nil || challenge != nil || uid.IsZero() {
		writeErr(http.StatusUnauthorized, "authentication required")
		return
	}

	room := req.FormValue("room")
	if room == "" {
		writeErr(http.StatusBadRequest, "'room' is required")
		return
	}

	identity := uid.UserId()
	claims := jwt.MapClaims{
		"iss":  apiKey,
		"sub":  identity,
		"name": identity,
		"nbf":  now.Unix(),
		"exp":  now.Add(6 * time.Hour).Unix(),
		"video": map[string]any{
			"roomJoin":       true,
			"room":           room,
			"canPublish":     true,
			"canSubscribe":   true,
			"canPublishData": true,
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(apiSecret))
	if err != nil {
		logs.Warn.Println("livekit: failed to sign token:", err)
		writeErr(http.StatusInternalServerError, "failed to mint token")
		return
	}

	wrt.WriteHeader(http.StatusOK)
	json.NewEncoder(wrt).Encode(livekitTokenResponse{URL: url, Token: token, Room: room, Identity: identity})
}
