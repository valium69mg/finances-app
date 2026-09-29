// Package cors provides a minimal CORS middleware that allows a single origin.
package cors

import (
	"errors"
	"net/http"
	"net/url"
)

const (
	allowMethods = "GET, POST, OPTIONS"
	allowHeaders = "Authorization, Content-Type"
	maxAge       = "600"
)

// OriginFromURL derives the CORS origin (scheme://host[:port], no path and no
// trailing slash) from a base URL.
func OriginFromURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", errors.New("not a parsable URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New(`scheme must be "http" or "https"`)
	}
	if u.Host == "" {
		return "", errors.New("host is required")
	}
	return u.Scheme + "://" + u.Host, nil
}

// Middleware returns a middleware that allows exactly allowedOrigin. The origin
// is reflected only when the request Origin matches; other requests pass
// through untouched, without CORS headers. Credentials are never allowed.
func Middleware(allowedOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Add("Vary", "Origin")

			origin := r.Header.Get("Origin")
			if origin == "" || origin != allowedOrigin {
				next.ServeHTTP(w, r)
				return
			}

			h.Set("Access-Control-Allow-Origin", allowedOrigin)

			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				h.Add("Vary", "Access-Control-Request-Method")
				h.Add("Vary", "Access-Control-Request-Headers")
				h.Set("Access-Control-Allow-Methods", allowMethods)
				h.Set("Access-Control-Allow-Headers", allowHeaders)
				h.Set("Access-Control-Max-Age", maxAge)
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
