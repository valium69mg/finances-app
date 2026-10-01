package authhttp_test

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

	authhttp "github.com/valium69mg/finances-app/backend/internal/auth/adapters/http"
	"github.com/valium69mg/finances-app/backend/internal/auth/app"
	"github.com/valium69mg/finances-app/backend/internal/auth/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/clientip"
	"github.com/valium69mg/finances-app/backend/internal/platform/session"
)

type fakeService struct {
	identifyStatus app.IdentifyStatus
	identifyErr    error
	loginSession   *app.Session
	loginErr       error
	refreshSess    *app.Session
	refreshErr     error
	logoutErr      error
	verifyErr      error
	me             domain.User
	meErr          error

	identifyCalls int

	gotIP, gotEmail, gotRefresh string
	gotVerifyToken, gotPassword string
}

func (f *fakeService) Identify(_ context.Context, email, ip string) (app.IdentifyStatus, error) {
	f.gotEmail, f.gotIP = email, ip
	f.identifyCalls++
	return f.identifyStatus, f.identifyErr
}

func (f *fakeService) Login(_ context.Context, email, _ string, ip string) (*app.Session, error) {
	f.gotEmail, f.gotIP = email, ip
	return f.loginSession, f.loginErr
}

func (f *fakeService) Refresh(_ context.Context, tok, ip string) (*app.Session, error) {
	f.gotRefresh, f.gotIP = tok, ip
	return f.refreshSess, f.refreshErr
}

func (f *fakeService) Logout(_ context.Context, tok string) error {
	f.gotRefresh = tok
	return f.logoutErr
}

func (f *fakeService) CompleteVerification(_ context.Context, tok, pw, ip string) error {
	f.gotVerifyToken, f.gotPassword, f.gotIP = tok, pw, ip
	return f.verifyErr
}

// Authenticate knows three tokens: "good" and "owner" are the owner, "household"
// is a household account, anything else is invalid.
func (f *fakeService) Authenticate(token string) (session.Identity, error) {
	switch token {
	case "good", "owner":
		return session.Identity{UserID: "u1", Role: session.RoleOwner}, nil
	case "household":
		return session.Identity{UserID: "u2", Role: session.RoleHousehold}, nil
	case "norole":
		return session.Identity{UserID: "u3"}, nil
	}
	return session.Identity{}, domain.ErrInvalidToken
}

func (f *fakeService) Me(context.Context, string) (domain.User, error) { return f.me, f.meErr }

func newServer(svc *fakeService) http.Handler {
	mux := http.NewServeMux()
	authhttp.New(svc, slog.New(slog.NewTextHandler(io.Discard, nil))).Register(mux)
	return mux
}

func do(t *testing.T, h http.Handler, method, path, body, auth string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.RemoteAddr = "203.0.113.9:5555"
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeMap(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body %q is not JSON: %v", rec.Body.String(), err)
	}
	return m
}

var goodSession = &app.Session{AccessToken: "acc", RefreshToken: "ref", ExpiresIn: 15 * time.Minute}

func TestLogin(t *testing.T) {
	tests := []struct {
		name       string
		svc        fakeService
		body       string
		wantStatus int
		wantKey    string
		wantValue  any
	}{
		{"verified user gets tokens", fakeService{loginSession: goodSession}, `{"email":"a@b.co","password":"pw"}`,
			200, "access_token", "acc"},
		{"nil session without error is an internal error", fakeService{}, `{"email":"a@b.co","password":"pw"}`,
			500, "error", "internal_error"},
		{"wrong password", fakeService{loginErr: domain.ErrInvalidCredentials}, `{"email":"a@b.co","password":"x"}`,
			401, "error", "invalid_credentials"},
		{"rate limited", fakeService{loginErr: domain.ErrRateLimited}, `{"email":"a@b.co","password":"x"}`,
			429, "error", "rate_limited"},
		{"invalid email", fakeService{loginErr: domain.ErrInvalidEmail}, `{"email":"nope","password":"x"}`,
			400, "error", "invalid_email"},
		{"malformed json", fakeService{}, `{`, 400, "error", "invalid_request"},
		{"unknown field", fakeService{}, `{"email":"a@b.co","extra":1}`, 400, "error", "invalid_request"},
		{"internal error hides details", fakeService{loginErr: errors.New("db password=hunter2")}, `{"email":"a@b.co"}`,
			500, "error", "internal_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := tt.svc
			rec := do(t, newServer(&svc), "POST", "/auth/login", tt.body, "")
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if got := decodeMap(t, rec)[tt.wantKey]; got != tt.wantValue {
				t.Fatalf("%s = %v, want %v", tt.wantKey, got, tt.wantValue)
			}
			if strings.Contains(rec.Body.String(), "hunter2") {
				t.Fatal("internal error details leaked")
			}
			if rec.Header().Get("Cache-Control") != "no-store" {
				t.Error("responses must not be cacheable")
			}
		})
	}
}

func TestLoginBodyAndClientIP(t *testing.T) {
	svc := &fakeService{loginSession: goodSession}
	rec := do(t, newServer(svc), "POST", "/auth/login", `{"email":"a@b.co","password":"pw"}`, "")
	body := decodeMap(t, rec)
	if body["refresh_token"] != "ref" || body["token_type"] != "Bearer" || body["expires_in"] != float64(900) {
		t.Fatalf("session body = %v", body)
	}
	if svc.gotIP != "203.0.113.9" || svc.gotEmail != "a@b.co" {
		t.Fatalf("ip=%q email=%q", svc.gotIP, svc.gotEmail)
	}
}

func TestLoginNeverAnswers202(t *testing.T) {
	// A nil session with no error must not surface as a "pending" response any more.
	// The service contract is (session, nil) or (nil, error); guard the JSON anyway.
	for _, err := range []error{domain.ErrInvalidCredentials, domain.ErrRateLimited, domain.ErrInvalidEmail} {
		rec := do(t, newServer(&fakeService{loginErr: err}), "POST", "/auth/login", `{"email":"a@b.co","password":"x"}`, "")
		if rec.Code == http.StatusAccepted {
			t.Fatalf("err %v produced 202", err)
		}
	}
}

func TestLoginRateLimitedSetsRetryAfter(t *testing.T) {
	rec := do(t, newServer(&fakeService{loginErr: domain.ErrRateLimited}), "POST", "/auth/login", `{"email":"a@b.co","password":"x"}`, "")
	if rec.Header().Get("Retry-After") != "900" {
		t.Fatalf("Retry-After = %q", rec.Header().Get("Retry-After"))
	}
}

func TestLoginAndUnknownUnverifiedAreIdentical(t *testing.T) {
	// Wrong password, unknown and unverified all reach the handler as ErrInvalidCredentials.
	a := do(t, newServer(&fakeService{loginErr: domain.ErrInvalidCredentials}), "POST", "/auth/login", `{"email":"ghost@b.co","password":"x"}`, "")
	b := do(t, newServer(&fakeService{loginErr: domain.ErrInvalidCredentials}), "POST", "/auth/login", `{"email":"new@b.co","password":"x"}`, "")
	if a.Code != 401 || a.Code != b.Code || a.Body.String() != b.Body.String() {
		t.Fatalf("responses differ: %d %q vs %d %q", a.Code, a.Body, b.Code, b.Body)
	}
}

func TestIdentify(t *testing.T) {
	tests := []struct {
		name       string
		svc        fakeService
		body       string
		wantStatus int
		wantKey    string
		wantValue  any
	}{
		{"verified account", fakeService{identifyStatus: app.IdentifyPasswordRequired}, `{"email":"a@b.co"}`,
			200, "status", "password_required"},
		{"unverified or unknown", fakeService{identifyStatus: app.IdentifyVerificationSent}, `{"email":"a@b.co"}`,
			200, "status", "verification_sent"},
		{"invalid email", fakeService{identifyErr: domain.ErrInvalidEmail}, `{"email":"nope"}`,
			400, "error", "invalid_email"},
		{"rate limited", fakeService{identifyErr: domain.ErrRateLimited}, `{"email":"a@b.co"}`,
			429, "error", "rate_limited"},
		{"internal error hides details", fakeService{identifyErr: errors.New("db password=hunter2")}, `{"email":"a@b.co"}`,
			500, "error", "internal_error"},
		{"malformed json", fakeService{}, `{`, 400, "error", "invalid_request"},
		{"empty body", fakeService{}, ``, 400, "error", "invalid_request"},
		{"not an object", fakeService{}, `["a@b.co"]`, 400, "error", "invalid_request"},
		{"wrong type", fakeService{}, `{"email":42}`, 400, "error", "invalid_request"},
		{"unknown field", fakeService{}, `{"email":"a@b.co","password":"x"}`, 400, "error", "invalid_request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := tt.svc
			rec := do(t, newServer(&svc), "POST", "/auth/identify", tt.body, "")
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tt.wantStatus, rec.Body)
			}
			body := decodeMap(t, rec)
			if body[tt.wantKey] != tt.wantValue {
				t.Fatalf("%s = %v, want %v", tt.wantKey, body[tt.wantKey], tt.wantValue)
			}
			if len(body) != 1 {
				t.Fatalf("body must contain only %q: %v", tt.wantKey, body)
			}
			if strings.Contains(rec.Body.String(), "hunter2") {
				t.Fatal("internal error details leaked")
			}
			if got := rec.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("content type = %q", got)
			}
			if rec.Header().Get("Cache-Control") != "no-store" {
				t.Error("responses must not be cacheable")
			}
			if (tt.wantStatus == 429) != (rec.Header().Get("Retry-After") != "") {
				t.Errorf("Retry-After = %q on status %d", rec.Header().Get("Retry-After"), tt.wantStatus)
			}
			if tt.wantStatus == 400 && tt.svc.identifyErr == nil && svc.identifyCalls != 0 {
				t.Error("service must not be called for an undecodable body")
			}
		})
	}
}

func TestIdentifyPassesEmailAndPeerIP(t *testing.T) {
	svc := &fakeService{identifyStatus: app.IdentifyPasswordRequired}
	do(t, newServer(svc), "POST", "/auth/identify", `{"email":" A@B.co "}`, "")
	if svc.gotEmail != " A@B.co " || svc.gotIP != "203.0.113.9" {
		t.Fatalf("email=%q ip=%q", svc.gotEmail, svc.gotIP)
	}
}

func TestIdentifyDoesNotTrustForwardingHeaders(t *testing.T) {
	svc := &fakeService{identifyStatus: app.IdentifyPasswordRequired}
	req := httptest.NewRequest("POST", "/auth/identify", strings.NewReader(`{"email":"a@b.co"}`))
	req.RemoteAddr = "203.0.113.9:5555"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.Header.Set("X-Real-IP", "5.6.7.8")
	req.Header.Set("Forwarded", "for=9.9.9.9")
	newServer(svc).ServeHTTP(httptest.NewRecorder(), req)
	if svc.gotIP != "203.0.113.9" {
		t.Fatalf("ip = %q, want the peer address", svc.gotIP)
	}
}

func TestRefreshAndVerifyRateLimitedSetRetryAfter(t *testing.T) {
	for path, body := range map[string]string{
		"/auth/refresh": `{"refresh_token":"r1"}`,
		"/auth/verify":  `{"token":"t","password":"pw"}`,
	} {
		rec := do(t, newServer(&fakeService{refreshErr: domain.ErrRateLimited, verifyErr: domain.ErrRateLimited}), "POST", path, body, "")
		if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "900" || decodeMap(t, rec)["error"] != "rate_limited" {
			t.Errorf("%s: status=%d Retry-After=%q body=%s", path, rec.Code, rec.Header().Get("Retry-After"), rec.Body)
		}
	}
}

func TestClientIPBehindTrustedProxy(t *testing.T) {
	trusted, err := clientip.ParseTrusted("172.18.0.0/16")
	if err != nil {
		t.Fatal(err)
	}
	call := func(remote, realIP string) string {
		svc := &fakeService{identifyStatus: app.IdentifyPasswordRequired}
		h := clientip.NewResolver(trusted).Middleware(newServer(svc))
		req := httptest.NewRequest("POST", "/auth/identify", strings.NewReader(`{"email":"a@b.co"}`))
		req.RemoteAddr = remote
		if realIP != "" {
			req.Header.Set("X-Real-IP", realIP)
		}
		h.ServeHTTP(httptest.NewRecorder(), req)
		return svc.gotIP
	}
	if got := call("172.18.0.4:4000", "198.51.100.7"); got != "198.51.100.7" {
		t.Errorf("trusted proxy: ip = %q, want the forwarded client", got)
	}
	if got := call("203.0.113.9:5555", "198.51.100.7"); got != "203.0.113.9" {
		t.Errorf("untrusted peer spoofing X-Real-IP: ip = %q, want the peer", got)
	}
	if got := call("[fd00::4]:4000", "2001:db8::1"); got != "fd00::4" {
		t.Errorf("IPv6 peer outside the list: ip = %q", got)
	}
}

func TestIdentifyBodySizeLimit(t *testing.T) {
	svc := &fakeService{identifyStatus: app.IdentifyPasswordRequired}
	huge := `{"email":"` + strings.Repeat("a", 2<<20) + `@b.co"}`
	rec := do(t, newServer(svc), "POST", "/auth/identify", huge, "")
	if rec.Code != http.StatusBadRequest || decodeMap(t, rec)["error"] != "invalid_request" {
		t.Fatalf("status = %d body = %.80s", rec.Code, rec.Body)
	}
	if svc.identifyCalls != 0 {
		t.Fatal("oversized bodies must not reach the service")
	}
}

func TestIdentifyWrongMethod(t *testing.T) {
	for _, method := range []string{"GET", "PUT", "DELETE", "PATCH"} {
		rec := do(t, newServer(&fakeService{}), method, "/auth/identify", `{"email":"a@b.co"}`, "")
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: status = %d, want 405", method, rec.Code)
		}
	}
}

func TestIdentifyResponsesForUnverifiedAndUnknownAreIdentical(t *testing.T) {
	// Both reach the handler as IdentifyVerificationSent.
	a := do(t, newServer(&fakeService{identifyStatus: app.IdentifyVerificationSent}), "POST", "/auth/identify", `{"email":"ghost@b.co"}`, "")
	b := do(t, newServer(&fakeService{identifyStatus: app.IdentifyVerificationSent}), "POST", "/auth/identify", `{"email":"new@b.co"}`, "")
	if a.Code != b.Code || a.Body.String() != b.Body.String() {
		t.Fatalf("responses differ: %d %q vs %d %q", a.Code, a.Body, b.Code, b.Body)
	}
}

func TestRefresh(t *testing.T) {
	tests := []struct {
		name       string
		svc        fakeService
		body       string
		wantStatus int
	}{
		{"ok", fakeService{refreshSess: goodSession}, `{"refresh_token":"r1"}`, 200},
		{"invalid or reused", fakeService{refreshErr: domain.ErrInvalidToken}, `{"refresh_token":"r1"}`, 401},
		{"rate limited", fakeService{refreshErr: domain.ErrRateLimited}, `{"refresh_token":"r1"}`, 429},
		{"internal", fakeService{refreshErr: errors.New("boom")}, `{"refresh_token":"r1"}`, 500},
		{"bad body", fakeService{}, `nope`, 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := tt.svc
			rec := do(t, newServer(&svc), "POST", "/auth/refresh", tt.body, "")
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantStatus == 200 && svc.gotRefresh != "r1" {
				t.Errorf("refresh token passed = %q", svc.gotRefresh)
			}
		})
	}
}

func TestLogout(t *testing.T) {
	svc := &fakeService{}
	rec := do(t, newServer(svc), "POST", "/auth/logout", `{"refresh_token":"r9"}`, "")
	if rec.Code != http.StatusNoContent || svc.gotRefresh != "r9" {
		t.Fatalf("status=%d token=%q", rec.Code, svc.gotRefresh)
	}
	rec = do(t, newServer(&fakeService{logoutErr: errors.New("db")}), "POST", "/auth/logout", `{"refresh_token":"r"}`, "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestVerify(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantError  string
	}{
		{"ok", nil, 204, ""},
		{"invalid token", domain.ErrInvalidToken, 400, "invalid_token"},
		{"weak password", domain.ErrWeakPassword, 400, "weak_password"},
		{"rate limited", domain.ErrRateLimited, 429, "rate_limited"},
		{"internal", errors.New("boom"), 500, "internal_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeService{verifyErr: tt.err}
			rec := do(t, newServer(svc), "POST", "/auth/verify", `{"token":"t","password":"pw"}`, "")
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantError != "" && decodeMap(t, rec)["error"] != tt.wantError {
				t.Fatalf("body = %s", rec.Body)
			}
			if svc.gotVerifyToken != "t" || svc.gotPassword != "pw" {
				t.Errorf("args = %q %q", svc.gotVerifyToken, svc.gotPassword)
			}
		})
	}
}

func TestMe(t *testing.T) {
	user := domain.User{ID: "u1", Email: "a@b.co", Verified: true, PasswordHash: "secret-hash", Role: session.RoleOwner, Active: true}
	tests := []struct {
		name       string
		svc        fakeService
		auth       string
		wantStatus int
	}{
		{"ok", fakeService{me: user}, "Bearer good", 200},
		{"no header", fakeService{me: user}, "", 401},
		{"wrong scheme", fakeService{me: user}, "Basic good", 401},
		{"bad token", fakeService{me: user}, "Bearer bad", 401},
		{"user gone", fakeService{meErr: domain.ErrInvalidToken}, "Bearer good", 401},
		{"internal", fakeService{meErr: errors.New("boom")}, "Bearer good", 500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := tt.svc
			rec := do(t, newServer(&svc), "GET", "/auth/me", "", tt.auth)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantStatus == 200 {
				m := decodeMap(t, rec)
				if m["email"] != "a@b.co" || m["verified"] != true || m["id"] != "u1" || m["role"] != "owner" {
					t.Fatalf("body = %v", m)
				}
				if strings.Contains(rec.Body.String(), "secret-hash") {
					t.Fatal("password hash leaked")
				}
			}
		})
	}
}

func TestMethodNotAllowed(t *testing.T) {
	rec := do(t, newServer(&fakeService{}), "GET", "/auth/login", "", "")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestSessionResponsesCarryTheRole(t *testing.T) {
	hh := &app.Session{AccessToken: "acc", RefreshToken: "ref", ExpiresIn: 15 * time.Minute, Role: session.RoleHousehold}
	tests := []struct {
		path, body string
		svc        *fakeService
	}{
		{"/auth/login", `{"email":"a@b.co","password":"pw"}`, &fakeService{loginSession: hh}},
		{"/auth/refresh", `{"refresh_token":"r"}`, &fakeService{refreshSess: hh}},
	}
	for _, tt := range tests {
		rec := do(t, newServer(tt.svc), "POST", tt.path, tt.body, "")
		if rec.Code != 200 || decodeMap(t, rec)["role"] != "household" {
			t.Errorf("%s: status %d body %s; want role household", tt.path, rec.Code, rec.Body)
		}
	}
}

// echoRoute mounts an authenticated route under the given pattern that records
// whether it was reached and which identity it saw.
func echoRoute(pattern string, h *authhttp.Handler, reached *session.Identity) http.Handler {
	mux := http.NewServeMux()
	mux.Handle(pattern, h.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*reached, _ = session.From(r.Context())
		w.WriteHeader(http.StatusNoContent)
	})))
	return mux
}

func TestRequireAuthEnforcesTheRole(t *testing.T) {
	h := authhttp.New(&fakeService{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	tests := []struct {
		name    string
		pattern string
		path    string
		method  string
		token   string
		want    int
	}{
		{"owner on an allowlisted route", "GET /dashboard", "/dashboard", "GET", "owner", 204},
		{"owner on any other route", "GET /settings", "/settings", "GET", "owner", 204},
		{"household on the allowlisted dashboard", "GET /dashboard", "/dashboard", "GET", "household", 204},
		{"household on me", "GET /auth/me", "/auth/me", "GET", "household", 204},
		{"household on settings", "GET /settings", "/settings", "GET", "household", 403},
		{"household on system", "GET /system/status", "/system/status", "GET", "household", 403},
		{"household on a route added later", "GET /brand-new", "/brand-new", "GET", "household", 403},
		{"household on a write of an allowlisted path", "POST /dashboard", "/dashboard", "POST", "household", 403},
		{"household with a wildcard route", "POST /users/{id}/invite", "/users/abc/invite", "POST", "household", 403},
		{"token without a role", "GET /dashboard", "/dashboard", "GET", "norole", 403},
		{"invalid token", "GET /dashboard", "/dashboard", "GET", "bad", 401},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reached session.Identity
			rec := do(t, echoRoute(tt.pattern, h, &reached), tt.method, tt.path, "", "Bearer "+tt.token)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tt.want, rec.Body)
			}
			if tt.want == 403 {
				if decodeMap(t, rec)["error"] != "forbidden" {
					t.Errorf("body = %s, want the forbidden envelope", rec.Body)
				}
				if reached.UserID != "" {
					t.Error("a denied request must not reach the handler")
				}
			}
			if tt.want == 204 && reached.UserID == "" {
				t.Error("the handler must see the identity in the context")
			}
		})
	}
}

func TestRequireAuthDeniesHouseholdWithoutARouterPattern(t *testing.T) {
	h := authhttp.New(&fakeService{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	called := false
	wrapped := h.RequireAuth(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	req := httptest.NewRequest("GET", "/dashboard", nil) // not routed: Request.Pattern is empty
	req.Header.Set("Authorization", "Bearer household")
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	if rec.Code != 403 || called {
		t.Fatalf("status %d called=%v; an unrouted request must be denied to household", rec.Code, called)
	}
}

func TestHouseholdAllowlistIsExplicit(t *testing.T) {
	got := map[string]bool{}
	for _, p := range authhttp.HouseholdAllowlist() {
		got[p] = true
	}
	want := map[string]bool{
		"GET /auth/me": true, "GET /dashboard": true,
		"POST /expense-requests": true, "GET /expense-requests": true, "GET /expense-requests/categories": true,
		"POST /expense-requests/{id}/cancel": true,
	}
	if len(got) != len(want) {
		t.Fatalf("allowlist = %v, want exactly %v", got, want)
	}
	for p := range want {
		if !got[p] || !authhttp.HouseholdAllowed(p) {
			t.Errorf("%q must be allowed", p)
		}
	}
	if authhttp.HouseholdAllowed("GET /system/status") || authhttp.HouseholdAllowed("") {
		t.Error("system and the empty pattern must be denied")
	}
}
