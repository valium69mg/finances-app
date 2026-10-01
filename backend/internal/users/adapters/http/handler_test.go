package usershttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/valium69mg/finances-app/backend/internal/platform/session"
	usershttp "github.com/valium69mg/finances-app/backend/internal/users/adapters/http"
	"github.com/valium69mg/finances-app/backend/internal/users/domain"
)

type fakeService struct {
	users   []domain.User
	err     error
	actor   session.Identity
	call    string
	gotID   string
	gotMail string
	gotRole session.Role
}

func (f *fakeService) List(_ context.Context, a session.Identity) ([]domain.User, error) {
	f.actor, f.call = a, "list"
	return f.users, f.err
}

func (f *fakeService) Create(_ context.Context, a session.Identity, email string, role session.Role) (domain.User, error) {
	f.actor, f.call, f.gotMail, f.gotRole = a, "create", email, role
	return domain.User{ID: "new-1", Email: email, Role: role, Active: true, CreatedAt: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)}, f.err
}

func (f *fakeService) Invite(_ context.Context, a session.Identity, id string) error {
	f.actor, f.call, f.gotID = a, "invite", id
	return f.err
}

func (f *fakeService) Deactivate(_ context.Context, a session.Identity, id string) error {
	f.actor, f.call, f.gotID = a, "deactivate", id
	return f.err
}

func (f *fakeService) Activate(_ context.Context, a session.Identity, id string) error {
	f.actor, f.call, f.gotID = a, "activate", id
	return f.err
}

// asOwner stands in for the auth middleware and puts the owner in the context.
func asOwner(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(session.With(r.Context(), session.Identity{UserID: "owner-1", Role: session.RoleOwner})))
	})
}

func do(svc *fakeService, method, path, body string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	usershttp.New(svc, slog.New(slog.NewTextHandler(io.Discard, nil))).Register(mux, asOwner)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

func TestListNeverExposesPasswordMaterial(t *testing.T) {
	svc := &fakeService{users: []domain.User{
		{ID: "u1", Email: "owner@example.com", Role: session.RoleOwner, Active: true, Verified: true},
		{ID: "u2", Email: "her@example.com", Role: session.RoleHousehold, Active: false},
	}}
	rec := do(svc, "GET", "/users", "")
	if rec.Code != 200 || svc.actor.UserID != "owner-1" {
		t.Fatalf("status %d actor %+v", rec.Code, svc.actor)
	}
	var got []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || len(got) != 2 {
		t.Fatalf("body %s: %v", rec.Body, err)
	}
	if got[1]["email"] != "her@example.com" || got[1]["role"] != "household" || got[1]["active"] != false || got[1]["verified"] != false {
		t.Errorf("row = %v", got[1])
	}
	if strings.Contains(strings.ToLower(rec.Body.String()), "password") || strings.Contains(rec.Body.String(), "hash") {
		t.Error("password material leaked")
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("missing Cache-Control: no-store")
	}
}

func TestCreate(t *testing.T) {
	svc := &fakeService{}
	rec := do(svc, "POST", "/users", `{"email":"her@example.com","role":"household"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if svc.gotMail != "her@example.com" || svc.gotRole != session.RoleHousehold {
		t.Errorf("service got %q %q", svc.gotMail, svc.gotRole)
	}
	var got map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got["id"] != "new-1" || got["active"] != true || got["verified"] != false {
		t.Errorf("body = %v", got)
	}
}

func TestActionsReachTheService(t *testing.T) {
	for _, call := range []string{"invite", "deactivate", "activate"} {
		svc := &fakeService{}
		rec := do(svc, "POST", "/users/abc-123/"+call, "")
		if rec.Code != http.StatusNoContent || svc.call != call || svc.gotID != "abc-123" || svc.actor.UserID != "owner-1" {
			t.Errorf("%s: status %d call %q id %q actor %+v", call, rec.Code, svc.call, svc.gotID, svc.actor)
		}
	}
}

func TestErrorMapping(t *testing.T) {
	tests := []struct {
		err    error
		status int
		code   string
	}{
		{domain.ErrForbidden, 403, "forbidden"},
		{domain.ErrNotFound, 404, "not_found"},
		{domain.ErrInvalidEmail, 400, "invalid_email"},
		{domain.ErrInvalidRole, 400, "invalid_role"},
		{domain.ErrEmailTaken, 409, "email_taken"},
		{domain.ErrSelf, 409, "cannot_change_self"},
		{domain.ErrAlreadyVerified, 409, "already_verified"},
		{domain.ErrInactive, 409, "user_inactive"},
		{domain.ErrRateLimited, 429, "rate_limited"},
		{domain.ErrSendFailed, 502, "email_failed"},
		{errors.New("db password=hunter2"), 500, "internal_error"},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			rec := do(&fakeService{err: tt.err}, "POST", "/users/u1/invite", "")
			if rec.Code != tt.status {
				t.Fatalf("status %d, want %d", rec.Code, tt.status)
			}
			var got map[string]string
			_ = json.Unmarshal(rec.Body.Bytes(), &got)
			if got["error"] != tt.code {
				t.Errorf("error = %q, want %q", got["error"], tt.code)
			}
			if strings.Contains(rec.Body.String(), "hunter2") {
				t.Error("internal error details leaked")
			}
			if tt.status == 429 && rec.Header().Get("Retry-After") == "" {
				t.Error("429 needs Retry-After")
			}
		})
	}
}

func TestCreateRejectsABadBody(t *testing.T) {
	for _, body := range []string{`{`, `{"email":"a@example.com","role":"household","extra":1}`} {
		if rec := do(&fakeService{}, "POST", "/users", body); rec.Code != 400 {
			t.Errorf("body %q: status %d, want 400", body, rec.Code)
		}
	}
}
