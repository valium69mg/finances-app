// Package httpjson holds the small JSON request and response helpers shared by
// the module HTTP adapters.
package httpjson

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// MaxBodyBytes caps the size of a request body.
const MaxBodyBytes = 1 << 20

// Decode reads a JSON body into dst, rejecting unknown fields. On failure it
// writes 400 invalid_request and returns false.
func Decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_request")
		return false
	}
	return true
}

// PathID parses the {id} path value. A malformed or non-positive id writes 404
// not_found and returns false.
func PathID(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		WriteError(w, http.StatusNotFound, "not_found")
		return 0, false
	}
	return id, true
}

// ListLimit parses the optional ?limit= query value: 0 when absent, otherwise
// an integer in 1..max. Anything else writes 400 invalid_request and returns false.
func ListLimit(w http.ResponseWriter, r *http.Request, max int) (int, bool) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return 0, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 || n > max {
		WriteError(w, http.StatusBadRequest, "invalid_request")
		return 0, false
	}
	return n, true
}

// WriteError writes {"error": code}.
func WriteError(w http.ResponseWriter, status int, code string) {
	WriteJSON(w, status, map[string]string{"error": code})
}

// WriteJSON writes v as an uncacheable JSON response.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
