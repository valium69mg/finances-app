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
)

const maxBodyBytes = 1 << 20

// Service is the set of use cases the handlers need.
type Service interface {
	Identify(ctx context.Context, email, ip string) (app.IdentifyStatus, error)
	Login(ctx context.Context, email, password, ip string) (*app.Session, error)
	Refresh(ctx context.Context, refreshToken, ip string) (*app.Session, error)
	Logout(ctx context.Context, refreshToken string) error
	CompleteVerification(ctx context.Context, token, newPassword, ip string) error
	Authenticate(accessToken string) (userID string, err error)
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
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /auth/identify", h.identify)
	mux.HandleFunc("POST /auth/login", h.login)
	mux.HandleFunc("POST /auth/refresh", h.refresh)
	mux.HandleFunc("POST /auth/logout", h.logout)
	mux.HandleFunc("POST /auth/verify", h.verify)
	mux.Handle("GET /auth/me", h.RequireAuth(http.HandlerFunc(h.me)))
}

type ctxKey struct{}

// UserIDFromContext returns the authenticated user id set by RequireAuth.
func UserIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(ctxKey{}).(string)
	return id, ok
}

// RequireAuth rejects requests without a valid Bearer access token and stores
// the user id in the request context for next.
func (h *Handler) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || token == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		userID, err := h.svc.Authenticate(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, userID)))
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
}

type meResponse struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Verified bool   `json:"verified"`
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
		writeJSON(w, http.StatusOK, meResponse{ID: user.ID, Email: user.Email, Verified: user.Verified})
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
