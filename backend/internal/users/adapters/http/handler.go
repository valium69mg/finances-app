// Package usershttp exposes the owner-only user administration over HTTP. The
// routes sit behind the authentication middleware passed to Register, which
// also denies them to the household role (they are not on its allowlist).
package usershttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/valium69mg/finances-app/backend/internal/platform/httpjson"
	"github.com/valium69mg/finances-app/backend/internal/platform/httpmw"
	"github.com/valium69mg/finances-app/backend/internal/platform/session"
	"github.com/valium69mg/finances-app/backend/internal/users/domain"
)

// Service is the set of use cases the handlers need.
type Service interface {
	List(ctx context.Context, actor session.Identity) ([]domain.User, error)
	Create(ctx context.Context, actor session.Identity, email string, role session.Role) (domain.User, error)
	Invite(ctx context.Context, actor session.Identity, id string) error
	Deactivate(ctx context.Context, actor session.Identity, id string) error
	Activate(ctx context.Context, actor session.Identity, id string) error
}

// Handler serves the /users routes.
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

// Register mounts the routes on mux, each wrapped by requireAuth.
func (h *Handler) Register(mux httpmw.Router, requireAuth func(http.Handler) http.Handler) {
	route := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, requireAuth(fn))
	}
	route("GET /users", h.list)
	route("POST /users", h.create)
	route("POST /users/{id}/invite", h.invite)
	route("POST /users/{id}/deactivate", h.deactivate)
	route("POST /users/{id}/activate", h.activate)
}

type createRequest struct {
	Email string       `json:"email"`
	Role  session.Role `json:"role"`
}

type userDTO struct {
	ID        string       `json:"id"`
	Email     string       `json:"email"`
	Role      session.Role `json:"role"`
	Active    bool         `json:"active"`
	Verified  bool         `json:"verified"`
	CreatedAt time.Time    `json:"created_at"`
}

func toDTO(u domain.User) userDTO {
	return userDTO{ID: u.ID, Email: u.Email, Role: u.Role, Active: u.Active, Verified: u.Verified, CreatedAt: u.CreatedAt}
}

func actor(r *http.Request) session.Identity {
	id, _ := session.From(r.Context())
	return id
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	users, err := h.svc.List(r.Context(), actor(r))
	if err != nil {
		h.fail(w, "list", err)
		return
	}
	out := make([]userDTO, len(users))
	for i, u := range users {
		out[i] = toDTO(u)
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	u, err := h.svc.Create(r.Context(), actor(r), req.Email, req.Role)
	if err != nil {
		h.fail(w, "create", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, toDTO(u))
}

func (h *Handler) invite(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Invite(r.Context(), actor(r), r.PathValue("id")); err != nil {
		h.fail(w, "invite", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) deactivate(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Deactivate(r.Context(), actor(r), r.PathValue("id")); err != nil {
		h.fail(w, "deactivate", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) activate(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Activate(r.Context(), actor(r), r.PathValue("id")); err != nil {
		h.fail(w, "activate", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// fail maps the use case errors to the JSON envelope; every unmapped error is
// logged and answered with a generic internal_error.
func (h *Handler) fail(w http.ResponseWriter, op string, err error) {
	switch {
	case errors.Is(err, domain.ErrForbidden):
		httpjson.WriteError(w, http.StatusForbidden, "forbidden")
	case errors.Is(err, domain.ErrNotFound):
		httpjson.WriteError(w, http.StatusNotFound, "not_found")
	case errors.Is(err, domain.ErrInvalidEmail):
		httpjson.WriteError(w, http.StatusBadRequest, "invalid_email")
	case errors.Is(err, domain.ErrInvalidRole):
		httpjson.WriteError(w, http.StatusBadRequest, "invalid_role")
	case errors.Is(err, domain.ErrEmailTaken):
		httpjson.WriteError(w, http.StatusConflict, "email_taken")
	case errors.Is(err, domain.ErrSelf):
		httpjson.WriteError(w, http.StatusConflict, "cannot_change_self")
	case errors.Is(err, domain.ErrAlreadyVerified):
		httpjson.WriteError(w, http.StatusConflict, "already_verified")
	case errors.Is(err, domain.ErrInactive):
		httpjson.WriteError(w, http.StatusConflict, "user_inactive")
	case errors.Is(err, domain.ErrRateLimited):
		w.Header().Set("Retry-After", "60")
		httpjson.WriteError(w, http.StatusTooManyRequests, "rate_limited")
	case errors.Is(err, domain.ErrSendFailed):
		h.logger.Error("invitation email failed", "error", err)
		httpjson.WriteError(w, http.StatusBadGateway, "email_failed")
	default:
		h.logger.Error("users request failed", "op", op, "error", err)
		httpjson.WriteError(w, http.StatusInternalServerError, "internal_error")
	}
}
