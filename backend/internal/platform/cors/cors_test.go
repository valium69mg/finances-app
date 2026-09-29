package cors_test

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/valium69mg/finances-app/backend/internal/platform/cors"
)

const allowed = "http://localhost:5173"

func serve(t *testing.T, method, origin, requestMethod string) (*httptest.ResponseRecorder, bool) {
	t.Helper()
	reached := false
	h := cors.Middleware(allowed)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(method, "/auth/login", nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if requestMethod != "" {
		req.Header.Set("Access-Control-Request-Method", requestMethod)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec, reached
}

func TestMiddleware(t *testing.T) {
	tests := []struct {
		name          string
		method        string
		origin        string
		requestMethod string
		wantStatus    int
		wantACAO      string
		wantReached   bool
		wantPreflight bool
	}{
		{"allowed origin", http.MethodPost, allowed, "", http.StatusOK, allowed, true, false},
		{"disallowed origin", http.MethodPost, "http://evil.example", "", http.StatusOK, "", true, false},
		{"origin with different port", http.MethodGet, "http://localhost:3000", "", http.StatusOK, "", true, false},
		{"no origin", http.MethodGet, "", "", http.StatusOK, "", true, false},
		{"preflight allowed", http.MethodOptions, allowed, http.MethodPost, http.StatusNoContent, allowed, false, true},
		{"preflight disallowed", http.MethodOptions, "http://evil.example", http.MethodPost, http.StatusOK, "", true, false},
		{"options without request method", http.MethodOptions, allowed, "", http.StatusOK, allowed, true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec, reached := serve(t, tc.method, tc.origin, tc.requestMethod)
			h := rec.Header()

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if got := h.Get("Access-Control-Allow-Origin"); got != tc.wantACAO {
				t.Errorf("Allow-Origin = %q, want %q", got, tc.wantACAO)
			}
			if reached != tc.wantReached {
				t.Errorf("handler reached = %v, want %v", reached, tc.wantReached)
			}
			if !slices.Contains(h.Values("Vary"), "Origin") {
				t.Errorf("Vary = %v, want it to contain Origin", h.Values("Vary"))
			}
			if got := h.Get("Access-Control-Allow-Credentials"); got != "" {
				t.Errorf("Allow-Credentials = %q, want empty", got)
			}
			if tc.wantPreflight {
				if got := h.Get("Access-Control-Allow-Methods"); got != "GET, POST, OPTIONS" {
					t.Errorf("Allow-Methods = %q", got)
				}
				if got := h.Get("Access-Control-Allow-Headers"); got != "Authorization, Content-Type" {
					t.Errorf("Allow-Headers = %q", got)
				}
				if got := h.Get("Access-Control-Max-Age"); got != "600" {
					t.Errorf("Max-Age = %q", got)
				}
			} else if h.Get("Access-Control-Allow-Methods") != "" {
				t.Errorf("Allow-Methods set on non-preflight response")
			}
		})
	}
}

func TestOriginFromURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{"default dev url", "http://localhost:5173", "http://localhost:5173", false},
		{"trailing slash", "http://localhost:5173/", "http://localhost:5173", false},
		{"path dropped", "https://app.example.com/some/path?x=1", "https://app.example.com", false},
		{"no port", "https://app.example.com", "https://app.example.com", false},
		{"bad scheme", "ftp://app.example.com", "", true},
		{"no host", "http://", "", true},
		{"not a url", "://", "", true},
		{"empty", "", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := cors.OriginFromURL(tc.raw)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("origin = %q, want %q", got, tc.want)
			}
		})
	}
}
