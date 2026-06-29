package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// maxBodyBytes caps request bodies to defend against memory-exhaustion.
const maxBodyBytes = 1 << 20 // 1 MiB

// WriteJSON renders v as JSON with the given status. A nil v with status 204
// writes no body.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	if v == nil {
		w.WriteHeader(status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// DecodeJSON reads and strictly decodes the request body into dst. It enforces a
// body-size limit and rejects unknown fields and trailing data, returning a
// user-safe error message suitable for a 400 problem response.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		switch {
		case errors.Is(err, io.EOF):
			return errors.New("request body is empty")
		case errors.As(err, &maxErr):
			return errors.New("request body too large")
		default:
			return errors.New("request body is not valid JSON or contains unexpected fields")
		}
	}

	// Reject a second JSON value (e.g. "{}{}").
	if dec.More() {
		return errors.New("request body must contain a single JSON object")
	}
	return nil
}
