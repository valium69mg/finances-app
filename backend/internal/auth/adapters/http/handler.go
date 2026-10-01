// Package authhttp exposes the authentication use cases over HTTP.
package authhttp

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/valium69mg/finances-app/backend/internal/auth/app"
	"github.com/valium69mg/finances-app/backend/internal/auth/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/clientip"
	"github.com/valium69mg/finances-app/backend/internal/platform/httpmw"
	"github.com/valium69mg/finances-app/backend/internal/platform/session"
)

const maxBodyBytes = 1 << 20

// Service is the set of use cases the handlers need.
type Service interface {
	Identify(ctx context.Context, email, ip string) (app.IdentifyStatus, error)
	Login(ctx context.Context, email, password, ip string) (*app.Session, error)
	Refresh(ctx context.Context, refreshToken, ip string) (*app.Session, error)
	Logout(ctx context.Context, refreshToken string) error
	CompleteVerification(ctx context.Context, token, newPassword, ip string) error
	Authenticate(accessToken string) (session.Identity, error)
	Me(ctx context.Context, userID string) (domain.User, error)
}

// Handler serves the /auth routes.
type Handler struct {
	svc    Service
	logger *slog.Logger
}

// New builds a Handler. A nil logger selects slog.Default().
func New(svc Service, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{svc: svc, logger: logger}
}

// Register mounts the auth routes on mux.
func (h *Handler) Register(mux httpmw.Router) {
	mux.HandleFunc("POST /auth/identify", h.identify)
	mux.HandleFunc("POST /auth/login", h.login)
	mux.HandleFunc("POST /auth/refresh", h.refresh)
	mux.HandleFunc("POST /auth/logout", h.logout)
	mux.HandleFunc("POST /auth/verify", h.verify)
	mux.Handle("GET /auth/me", h.RequireAuth(http.HandlerFunc(h.me)))
}

// householdAllowlist is the complete set of authenticated routes the household
// role may call, keyed by the ServeMux pattern. It is DEFAULT DENY: a route that
// is not listed here answers 403 forbidden to a household session, so a route
// added later (settings, system, a new module) is owner-only until it is added
// on purpose. The session-keeping routes (/auth/refresh, /auth/logout) are
// public and need no entry; /auth/me is how the client reads its own account.
var householdAllowlist = map[string]bool{
	"GET /auth/me":   true,
	"GET /dashboard": true, // answered with the reduced, budget-only payload

	// Expense requests (phase 9, step 3). The service scopes the household
	// listing to her own requests and only the requester may cancel.
	"POST /expense-requests":             true,
	"GET /expense-requests":              true,
	"GET /expense-requests/categories":   true, // Gasto category names only, no budgets
	"POST /expense-requests/{id}/cancel": true,
	// Not listed on purpose (owner-only): GET /expense-requests/{id}/budget-check,
	// POST /expense-requests/{id}/approve, POST /expense-requests/{id}/reject and
	// POST /expense-requests/{id}/revert.
}

// HouseholdAllowed reports whether the household role may call the route
// registered under pattern.
func HouseholdAllowed(pattern string) bool { return householdAllowlist[pattern] }

// HouseholdAllowlist returns a copy of the allowlist patterns, for tests.
func HouseholdAllowlist() []string {
	out := make([]string, 0, len(householdAllowlist))
	for p := range householdAllowlist {
		out = append(out, p)
	}
	return out
}

// UserIDFromContext returns the authenticated user id set by RequireAuth.
func UserIDFromContext(ctx context.Context) (string, bool) {
	id, ok := session.From(ctx)
	return id.UserID, ok
}

// RequireAuth rejects requests without a valid Bearer access token, stores the
// identity (user id and role) in the request context for next and enforces the
// role: the owner passes everywhere, the household role only on the allowlist.
// The matched pattern comes from the router (Request.Pattern), so a handler
// reached without the router has no pattern and is denied to a household
// session. Every module mounts its authenticated routes through this wrapper,
// which makes the denial the default for routes added in the future.
func (h *Handler) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || token == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		id, err := h.svc.Authenticate(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if id.Role != session.RoleOwner && !(id.Role == session.RoleHousehold && HouseholdAllowed(r.Pattern)) {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		next.ServeHTTP(w, r.WithContext(session.With(r.Context(), id)))
	})
}

type identifyRequest struct {
	Email string `json:"email"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type verifyRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

type sessionResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	// Role lets the client shape its navigation; the backend enforces it anyway.
	Role session.Role `json:"role"`
}

type meResponse struct {
	ID       string       `json:"id"`
	Email    string       `json:"email"`
	Verified bool         `json:"verified"`
	Role     session.Role `json:"role"`
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !decode(w, r, &req) {
		return
	}
	sess, err := h.svc.Login(r.Context(), req.Email, req.Password, clientIP(r))
	switch {
	case errors.Is(err, domain.ErrInvalidEmail):
		writeError(w, http.StatusBadRequest, "invalid_email")
	case errors.Is(err, domain.ErrRateLimited):
		w.Header().Set("Retry-After", "900")
		writeError(w, http.StatusTooManyRequests, "rate_limited")
	case errors.Is(err, domain.ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, "invalid_credentials")
	case err != nil:
		h.internalError(w, "login", err)
	case sess == nil:
		// Contract violation: Login returns a session or an error, never neither.
		h.internalError(w, "login", errors.New("login returned no session and no error"))
	default:
		writeSession(w, sess)
	}
}

func (h *Handler) identify(w http.ResponseWriter, r *http.Request) {
	var req identifyRequest
	if !decode(w, r, &req) {
		return
	}
	status, err := h.svc.Identify(r.Context(), req.Email, clientIP(r))
	switch {
	case errors.Is(err, domain.ErrInvalidEmail):
		writeError(w, http.StatusBadRequest, "invalid_email")
	case errors.Is(err, domain.ErrRateLimited):
		w.Header().Set("Retry-After", "900")
		writeError(w, http.StatusTooManyRequests, "rate_limited")
	case err != nil:
		h.internalError(w, "identify", err)
	default:
		writeJSON(w, http.StatusOK, map[string]string{"status": string(status)})
	}
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if !decode(w, r, &req) {
		return
	}
	sess, err := h.svc.Refresh(r.Context(), req.RefreshToken, clientIP(r))
	switch {
	case errors.Is(err, domain.ErrRateLimited):
		w.Header().Set("Retry-After", "900")
		writeError(w, http.StatusTooManyRequests, "rate_limited")
	case errors.Is(err, domain.ErrInvalidToken):
		writeError(w, http.StatusUnauthorized, "invalid_token")
	case err != nil:
		h.internalError(w, "refresh", err)
	default:
		writeSession(w, sess)
	}
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if !decode(w, r, &req) {
		return
	}
	if err := h.svc.Logout(r.Context(), req.RefreshToken); err != nil {
		h.internalError(w, "logout", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) verify(w http.ResponseWriter, r *http.Request) {
	var req verifyRequest
	if !decode(w, r, &req) {
		return
	}
	err := h.svc.CompleteVerification(r.Context(), req.Token, req.Password, clientIP(r))
	switch {
	case errors.Is(err, domain.ErrRateLimited):
		w.Header().Set("Retry-After", "900")
		writeError(w, http.StatusTooManyRequests, "rate_limited")
	case errors.Is(err, domain.ErrWeakPassword):
		writeError(w, http.StatusBadRequest, "weak_password")
	case errors.Is(err, domain.ErrInvalidToken):
		writeError(w, http.StatusBadRequest, "invalid_token")
	case err != nil:
		h.internalError(w, "verify", err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	id, _ := UserIDFromContext(r.Context())
	user, err := h.svc.Me(r.Context(), id)
	switch {
	case errors.Is(err, domain.ErrInvalidToken):
		writeError(w, http.StatusUnauthorized, "unauthorized")
	case err != nil:
		h.internalError(w, "me", err)
	default:
		writeJSON(w, http.StatusOK, meResponse{ID: user.ID, Email: user.Email, Verified: user.Verified, Role: user.Role})
	}
}

func (h *Handler) internalError(w http.ResponseWriter, op string, err error) {
	h.logger.Error("auth request failed", "op", op, "error", err)
	writeError(w, http.StatusInternalServerError, "internal_error")
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return false
	}
	return true
}

func writeSession(w http.ResponseWriter, s *app.Session) {
	writeJSON(w, http.StatusOK, sessionResponse{
		AccessToken:  s.AccessToken,
		RefreshToken: s.RefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(s.ExpiresIn.Seconds()),
		Role:         s.Role,
	})
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// clientIP returns the client address resolved by the clientip middleware: the
// peer address, or the proxy-provided one only when the peer is a configured
// trusted proxy. Without the middleware it is the peer address.
func clientIP(r *http.Request) string { return clientip.FromRequest(r) }
