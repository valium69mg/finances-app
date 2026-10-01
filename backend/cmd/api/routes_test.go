package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	authhttp "github.com/valium69mg/finances-app/backend/internal/auth/adapters/http"
	authapp "github.com/valium69mg/finances-app/backend/internal/auth/app"
	authdomain "github.com/valium69mg/finances-app/backend/internal/auth/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/session"
)

// publicRoutes is the complete list of routes that need no session. Anything
// else registered in registerRoutes must answer 401 without a token, so a route
// added without the authentication wrapper (and therefore without the role
// gate) fails this test until it is wrapped or listed here on purpose.
var publicRoutes = []string{
	"GET /healthz",
	"POST /auth/identify",
	"POST /auth/login",
	"POST /auth/refresh",
	"POST /auth/logout",
	"POST /auth/verify",
}

// fakeAuth is the auth use case behind the real auth handler: only
// Authenticate matters here, the tokens are the role names.
type fakeAuth struct{}

func (fakeAuth) Identify(context.Context, string, string) (authapp.IdentifyStatus, error) {
	return authapp.IdentifyPasswordRequired, nil
}
func (fakeAuth) Login(context.Context, string, string, string) (*authapp.Session, error) {
	return &authapp.Session{AccessToken: "a", RefreshToken: "r", Role: session.RoleOwner}, nil
}
func (fakeAuth) Refresh(context.Context, string, string) (*authapp.Session, error) {
	return &authapp.Session{AccessToken: "a", RefreshToken: "r", Role: session.RoleOwner}, nil
}
func (fakeAuth) Logout(context.Context, string) error                               { return nil }
func (fakeAuth) CompleteVerification(context.Context, string, string, string) error { return nil }
func (fakeAuth) Authenticate(token string) (session.Identity, error) {
	switch token {
	case "owner":
		return session.Identity{UserID: "u-owner", Role: session.RoleOwner}, nil
	case "household":
		return session.Identity{UserID: "u-household", Role: session.RoleHousehold}, nil
	}
	return session.Identity{}, authdomain.ErrInvalidToken
}
func (fakeAuth) Me(_ context.Context, id string) (authdomain.User, error) {
	return authdomain.User{ID: id, Email: "someone@example.com", Verified: true, Role: session.RoleOwner, Active: true}, nil
}

// recordingRouter mounts routes on a real ServeMux and remembers their patterns.
type recordingRouter struct {
	mux      *http.ServeMux
	patterns []string
}

func (r *recordingRouter) Handle(pattern string, h http.Handler) {
	r.patterns = append(r.patterns, pattern)
	r.mux.Handle(pattern, h)
}

func (r *recordingRouter) HandleFunc(pattern string, h func(http.ResponseWriter, *http.Request)) {
	r.Handle(pattern, http.HandlerFunc(h))
}

var wildcard = regexp.MustCompile(`\{[^}]*\}`)

// mountAllRoutes registers the real route table with the real authentication
// middleware, but with every module handler replaced by a stub that answers
// 204: the test is about who gets through, not about what the handlers do.
func mountAllRoutes(t *testing.T) *recordingRouter {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	auth := authhttp.New(fakeAuth{}, logger)
	stub := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })

	rec := &recordingRouter{mux: http.NewServeMux()}
	registerRoutes(rec, routeDeps{Health: stub, Auth: auth},
		func(http.Handler) http.Handler { return auth.RequireAuth(stub) }, logger)
	return rec
}

func call(h http.Handler, pattern, token string) *httptest.ResponseRecorder {
	method, path, _ := strings.Cut(pattern, " ")
	req := httptest.NewRequest(method, wildcard.ReplaceAllString(path, "1"), strings.NewReader("{}"))
	req.RemoteAddr = "203.0.113.9:5555"
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func isPublic(pattern string) bool { return slices.Contains(publicRoutes, pattern) }

func TestEveryRouteIsPublicOrAuthenticated(t *testing.T) {
	router := mountAllRoutes(t)
	if len(router.patterns) < 60 {
		t.Fatalf("only %d routes recorded; the route table is not being enumerated", len(router.patterns))
	}
	for _, pattern := range router.patterns {
		rec := call(router.mux, pattern, "")
		switch {
		case isPublic(pattern):
			if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
				t.Errorf("%s is public but answered %d without a token", pattern, rec.Code)
			}
		case rec.Code != http.StatusUnauthorized:
			t.Errorf("%s answered %d without a token: wrap it with requireAuth, or list it in publicRoutes on purpose", pattern, rec.Code)
		}
	}
	for _, p := range publicRoutes {
		if !slices.Contains(router.patterns, p) {
			t.Errorf("publicRoutes lists %q, which is not registered: update the list", p)
		}
	}
}

func TestHouseholdIsDeniedEverywhereOutsideTheAllowlist(t *testing.T) {
	router := mountAllRoutes(t)
	allowed := 0
	for _, pattern := range router.patterns {
		if isPublic(pattern) {
			continue
		}
		rec := call(router.mux, pattern, "household")
		if authhttp.HouseholdAllowed(pattern) {
			allowed++
			if rec.Code < 200 || rec.Code > 299 {
				t.Errorf("%s is on the household allowlist but answered %d", pattern, rec.Code)
			}
			continue
		}
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"forbidden"`) {
			t.Errorf("%s answered %d %s to household; want 403 forbidden (default deny)", pattern, rec.Code, strings.TrimSpace(rec.Body.String()))
		}
	}
	if allowed != len(authhttp.HouseholdAllowlist()) {
		t.Errorf("%d allowlisted routes reached, allowlist has %d: an entry does not match a registered route", allowed, len(authhttp.HouseholdAllowlist()))
	}
}

func TestOwnerIsUnaffected(t *testing.T) {
	router := mountAllRoutes(t)
	for _, pattern := range router.patterns {
		if isPublic(pattern) {
			continue
		}
		if rec := call(router.mux, pattern, "owner"); rec.Code < 200 || rec.Code > 299 {
			t.Errorf("%s answered %d to the owner; want 2xx", pattern, rec.Code)
		}
	}
}

// The sensitive areas are named explicitly so the table above cannot pass by
// accident if they were renamed or dropped from the router.
func TestOwnerOnlyAreasAreRegisteredAndDenied(t *testing.T) {
	router := mountAllRoutes(t)
	prefixes := []string{
		"/settings", "/system", "/income", "/expenses", "/savings", "/invoices",
		"/tax-filing", "/month-close", "/bills", "/future-expenses", "/users",
	}
	for _, prefix := range prefixes {
		found := false
		for _, pattern := range router.patterns {
			_, path, _ := strings.Cut(pattern, " ")
			if !strings.HasPrefix(path, prefix) {
				continue
			}
			found = true
			if rec := call(router.mux, pattern, "household"); rec.Code != http.StatusForbidden {
				t.Errorf("%s answered %d to household, want 403", pattern, rec.Code)
			}
		}
		if !found {
			t.Errorf("no registered route under %s: update the list if the module was renamed", prefix)
		}
	}
}

func TestHouseholdAllowlistIsExactlyTheDecidedOne(t *testing.T) {
	got := authhttp.HouseholdAllowlist()
	sort.Strings(got)
	want := []string{
		"GET /auth/me", "GET /dashboard",
		"GET /expense-requests", "GET /expense-requests/categories", "POST /expense-requests", "POST /expense-requests/{id}/cancel",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("household allowlist = %v, want %v; widening it is a decision (PLAN.md phase 9)", got, want)
	}
}

// The expense request routes are split by role: household may create, list,
// cancel and read the Gasto category names; the budget check, the approval and
// the rejection are owner-only (default denied, not on the allowlist).
func TestExpenseRequestRoutesSplitByRole(t *testing.T) {
	router := mountAllRoutes(t)
	householdRoutes := []string{
		"POST /expense-requests", "GET /expense-requests", "GET /expense-requests/categories", "POST /expense-requests/{id}/cancel",
	}
	ownerOnly := []string{
		"GET /expense-requests/{id}/budget-check", "POST /expense-requests/{id}/approve", "POST /expense-requests/{id}/reject",
	}
	for _, pattern := range append(slices.Clone(householdRoutes), ownerOnly...) {
		if !slices.Contains(router.patterns, pattern) {
			t.Fatalf("%s is not registered: update the lists if the route was renamed", pattern)
		}
	}
	for _, pattern := range householdRoutes {
		if rec := call(router.mux, pattern, "household"); rec.Code < 200 || rec.Code > 299 {
			t.Errorf("%s answered %d to household, want 2xx", pattern, rec.Code)
		}
	}
	for _, pattern := range ownerOnly {
		if rec := call(router.mux, pattern, "household"); rec.Code != http.StatusForbidden {
			t.Errorf("%s answered %d to household, want 403", pattern, rec.Code)
		}
		if rec := call(router.mux, pattern, "owner"); rec.Code < 200 || rec.Code > 299 {
			t.Errorf("%s answered %d to the owner, want 2xx", pattern, rec.Code)
		}
	}
}
