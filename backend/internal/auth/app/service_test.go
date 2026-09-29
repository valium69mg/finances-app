package app_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/valium69mg/finances-app/backend/internal/auth/app"
	"github.com/valium69mg/finances-app/backend/internal/auth/domain"
)

var t0 = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

type fakeUsers struct{ byID map[string]domain.User }

func (f *fakeUsers) FindByEmail(_ context.Context, email string) (domain.User, error) {
	for _, u := range f.byID {
		if u.Email == email {
			return u, nil
		}
	}
	return domain.User{}, domain.ErrNotFound
}

func (f *fakeUsers) FindByID(_ context.Context, id string) (domain.User, error) {
	u, ok := f.byID[id]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	return u, nil
}

func (f *fakeUsers) SetPasswordAndVerify(_ context.Context, id, hash string) error {
	u := f.byID[id]
	u.PasswordHash, u.Verified = hash, true
	f.byID[id] = u
	return nil
}

type fakeRefresh struct {
	tokens map[string]*domain.RefreshToken
}

func (f *fakeRefresh) Create(_ context.Context, t domain.RefreshToken) error {
	f.tokens[t.Hash] = &t
	return nil
}

func (f *fakeRefresh) Find(_ context.Context, hash string) (domain.RefreshToken, error) {
	t, ok := f.tokens[hash]
	if !ok {
		return domain.RefreshToken{}, domain.ErrNotFound
	}
	return *t, nil
}

func (f *fakeRefresh) MarkUsed(_ context.Context, hash string, now time.Time) (bool, error) {
	t, ok := f.tokens[hash]
	if !ok || t.UsedAt != nil || t.RevokedAt != nil || t.Expired(now) {
		return false, nil
	}
	t.UsedAt = &now
	return true, nil
}

func (f *fakeRefresh) Revoke(_ context.Context, hash string, now time.Time) error {
	if t, ok := f.tokens[hash]; ok && t.RevokedAt == nil {
		t.RevokedAt = &now
	}
	return nil
}

func (f *fakeRefresh) RevokeAllForUser(_ context.Context, userID string, now time.Time) error {
	for _, t := range f.tokens {
		if t.UserID == userID && t.RevokedAt == nil {
			t.RevokedAt = &now
		}
	}
	return nil
}

func (f *fakeRefresh) liveCount(userID string) int {
	n := 0
	for _, t := range f.tokens {
		if t.UserID == userID && t.RevokedAt == nil {
			n++
		}
	}
	return n
}

type fakeVerification struct {
	tokens map[string]*domain.VerificationToken
}

func (f *fakeVerification) Create(_ context.Context, t domain.VerificationToken) error {
	f.tokens[t.Hash] = &t
	return nil
}

func (f *fakeVerification) Consume(_ context.Context, hash string, now time.Time) (string, error) {
	t, ok := f.tokens[hash]
	if !ok || t.UsedAt != nil || !now.Before(t.ExpiresAt) {
		return "", domain.ErrNotFound
	}
	t.UsedAt = &now
	return t.UserID, nil
}

type sentMail struct{ to, link string }

type fakeMailer struct {
	sent []sentMail
	err  error
}

func (f *fakeMailer) SendVerification(_ context.Context, to, link string) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, sentMail{to, link})
	return nil
}

type fakeLimiter struct{ deny bool }

func (f *fakeLimiter) Allow(string, int, time.Duration) bool { return !f.deny }

type env struct {
	svc     *app.Service
	clock   *fakeClock
	users   *fakeUsers
	refresh *fakeRefresh
	verif   *fakeVerification
	mailer  *fakeMailer
	limiter *fakeLimiter
}

const (
	verifiedEmail   = "ok@example.com"
	unverifiedEmail = "new@example.com"
	password        = "correct horse battery"
)

func newEnv(t *testing.T) *env {
	t.Helper()
	hash, err := domain.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	e := &env{
		clock: &fakeClock{now: t0},
		users: &fakeUsers{byID: map[string]domain.User{
			"u1": {ID: "u1", Email: verifiedEmail, PasswordHash: hash, Verified: true},
			"u2": {ID: "u2", Email: unverifiedEmail, PasswordHash: hash, Verified: false},
		}},
		refresh: &fakeRefresh{tokens: map[string]*domain.RefreshToken{}},
		verif:   &fakeVerification{tokens: map[string]*domain.VerificationToken{}},
		mailer:  &fakeMailer{},
		limiter: &fakeLimiter{},
	}
	e.svc = app.NewService(app.Deps{
		Users:              e.users,
		RefreshTokens:      e.refresh,
		VerificationTokens: e.verif,
		Mailer:             e.mailer,
		Clock:              e.clock,
		Limiter:            e.limiter,
		JWTSecret:          []byte(strings.Repeat("k", 32)),
		AppBaseURL:         "https://app.example.com",
		Spawn:              func(fn func()) { fn() },
		Logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	return e
}

func TestLogin(t *testing.T) {
	tests := []struct {
		name        string
		email       string
		password    string
		wantSession bool
		wantErr     error
		wantMails   int
	}{
		{"verified user with right password", verifiedEmail, password, true, nil, 0},
		{"email is normalized", "  OK@Example.com ", password, true, nil, 0},
		{"verified user with wrong password", verifiedEmail, "nope nope nope", false, domain.ErrInvalidCredentials, 0},
		{"unknown email is generic", "ghost@example.com", password, false, nil, 0},
		{"unverified is generic and sends mail", unverifiedEmail, "whatever", false, nil, 1},
		{"invalid email", "not-an-email", password, false, domain.ErrInvalidEmail, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			sess, err := e.svc.Login(context.Background(), tt.email, tt.password, "1.2.3.4")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if (sess != nil) != tt.wantSession {
				t.Fatalf("session = %v, want present=%v", sess, tt.wantSession)
			}
			if len(e.mailer.sent) != tt.wantMails {
				t.Fatalf("mails = %d, want %d", len(e.mailer.sent), tt.wantMails)
			}
			if sess != nil {
				uid, err := e.svc.Authenticate(sess.AccessToken)
				if err != nil || uid != "u1" {
					t.Errorf("access token invalid: %q, %v", uid, err)
				}
				if sess.RefreshToken == "" || sess.ExpiresIn != domain.AccessTokenTTL {
					t.Errorf("bad session: %+v", sess)
				}
			}
		})
	}
}

func TestLoginGenericResponseIsIndistinguishable(t *testing.T) {
	e := newEnv(t)
	unknown, errUnknown := e.svc.Login(context.Background(), "ghost@example.com", "x", "ip")
	unverified, errUnverified := e.svc.Login(context.Background(), unverifiedEmail, "x", "ip")
	if unknown != nil || unverified != nil || errUnknown != nil || errUnverified != nil {
		t.Fatalf("unknown=(%v,%v) unverified=(%v,%v): want identical (nil,nil)", unknown, errUnknown, unverified, errUnverified)
	}
}

func TestLoginRateLimited(t *testing.T) {
	e := newEnv(t)
	e.limiter.deny = true
	sess, err := e.svc.Login(context.Background(), unverifiedEmail, password, "ip")
	if !errors.Is(err, domain.ErrRateLimited) || sess != nil {
		t.Fatalf("got %v, %v; want ErrRateLimited", sess, err)
	}
	if len(e.mailer.sent) != 0 {
		t.Fatal("no email may be sent when rate limited")
	}
}

func TestLoginVerificationEmailContent(t *testing.T) {
	e := newEnv(t)
	if _, err := e.svc.Login(context.Background(), unverifiedEmail, "x", "ip"); err != nil {
		t.Fatal(err)
	}
	if len(e.mailer.sent) != 1 {
		t.Fatalf("mails = %d", len(e.mailer.sent))
	}
	m := e.mailer.sent[0]
	if m.to != unverifiedEmail {
		t.Errorf("to = %q", m.to)
	}
	u, err := url.Parse(m.link)
	if err != nil || u.Host != "app.example.com" || u.Path != "/verify" {
		t.Fatalf("link = %q (%v)", m.link, err)
	}
	raw := u.Query().Get("token")
	stored, ok := e.verif.tokens[domain.HashToken(raw)]
	if !ok {
		t.Fatal("token hash not stored")
	}
	if _, plain := e.verif.tokens[raw]; plain {
		t.Error("raw token must not be stored")
	}
	if want := t0.Add(time.Hour); !stored.ExpiresAt.Equal(want) {
		t.Errorf("expires %v, want %v", stored.ExpiresAt, want)
	}
}

func TestLoginMailFailureStaysGeneric(t *testing.T) {
	e := newEnv(t)
	e.mailer.err = errors.New("resend down")
	sess, err := e.svc.Login(context.Background(), unverifiedEmail, "x", "ip")
	if sess != nil || err != nil {
		t.Fatalf("got %v, %v; want (nil, nil)", sess, err)
	}
}

func loginSession(t *testing.T, e *env) *app.Session {
	t.Helper()
	sess, err := e.svc.Login(context.Background(), verifiedEmail, password, "ip")
	if err != nil || sess == nil {
		t.Fatalf("login: %v, %v", sess, err)
	}
	return sess
}

func TestRefreshRotation(t *testing.T) {
	e := newEnv(t)
	first := loginSession(t, e)

	second, err := e.svc.Refresh(context.Background(), first.RefreshToken)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if second.RefreshToken == first.RefreshToken || second.AccessToken == "" {
		t.Fatal("refresh must issue a new pair")
	}
	if e.refresh.tokens[domain.HashToken(first.RefreshToken)].UsedAt == nil {
		t.Fatal("old token must be marked used")
	}
	if _, err := e.svc.Refresh(context.Background(), second.RefreshToken); err != nil {
		t.Fatalf("rotated token must work once: %v", err)
	}
}

func TestRefreshReuseRevokesAll(t *testing.T) {
	e := newEnv(t)
	first := loginSession(t, e)
	second, err := e.svc.Refresh(context.Background(), first.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	other := loginSession(t, e) // another device

	_, err = e.svc.Refresh(context.Background(), first.RefreshToken) // replay
	if !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("reuse err = %v, want ErrInvalidToken", err)
	}
	if n := e.refresh.liveCount("u1"); n != 0 {
		t.Fatalf("live tokens after reuse = %d, want 0", n)
	}
	for name, tok := range map[string]string{"rotated": second.RefreshToken, "other device": other.RefreshToken} {
		if _, err := e.svc.Refresh(context.Background(), tok); !errors.Is(err, domain.ErrInvalidToken) {
			t.Errorf("%s token still usable: %v", name, err)
		}
	}
}

func TestRefreshInvalid(t *testing.T) {
	tests := []struct {
		name  string
		setup func(e *env, s *app.Session) string
	}{
		{"unknown token", func(*env, *app.Session) string { return "unknown" }},
		{"expired", func(e *env, s *app.Session) string {
			e.clock.now = t0.Add(domain.RefreshTokenTTL)
			return s.RefreshToken
		}},
		{"revoked", func(e *env, s *app.Session) string {
			_ = e.svc.Logout(context.Background(), s.RefreshToken)
			return s.RefreshToken
		}},
		{"user no longer verified", func(e *env, s *app.Session) string {
			u := e.users.byID["u1"]
			u.Verified = false
			e.users.byID["u1"] = u
			return s.RefreshToken
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			tok := tt.setup(e, loginSession(t, e))
			sess, err := e.svc.Refresh(context.Background(), tok)
			if sess != nil || !errors.Is(err, domain.ErrInvalidToken) {
				t.Fatalf("got %v, %v; want ErrInvalidToken", sess, err)
			}
		})
	}
}

func TestLogoutRevokesOnlyPresentedToken(t *testing.T) {
	e := newEnv(t)
	a, b := loginSession(t, e), loginSession(t, e)

	if err := e.svc.Logout(context.Background(), a.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Logout(context.Background(), "unknown"); err != nil {
		t.Fatalf("unknown token must be ignored: %v", err)
	}
	if _, err := e.svc.Refresh(context.Background(), a.RefreshToken); !errors.Is(err, domain.ErrInvalidToken) {
		t.Errorf("logged-out token usable: %v", err)
	}
	if _, err := e.svc.Refresh(context.Background(), b.RefreshToken); err != nil {
		t.Errorf("other token must survive: %v", err)
	}
}

func requestToken(t *testing.T, e *env) string {
	t.Helper()
	if _, err := e.svc.Login(context.Background(), unverifiedEmail, "x", "ip"); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(e.mailer.sent[len(e.mailer.sent)-1].link)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query().Get("token")
}

func TestCompleteVerification(t *testing.T) {
	const newPassword = "brand new passphrase"

	tests := []struct {
		name     string
		password string
		advance  time.Duration
		token    func(real string) string
		wantErr  error
	}{
		{"valid", newPassword, 0, func(r string) string { return r }, nil},
		{"weak password", "short", 0, func(r string) string { return r }, domain.ErrWeakPassword},
		{"unknown token", newPassword, 0, func(string) string { return "bogus" }, domain.ErrInvalidToken},
		{"expired", newPassword, domain.VerificationTokenTTL, func(r string) string { return r }, domain.ErrInvalidToken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			raw := requestToken(t, e)
			e.clock.now = t0.Add(tt.advance)

			err := e.svc.CompleteVerification(context.Background(), tt.token(raw), tt.password)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			u := e.users.byID["u2"]
			if verified := tt.wantErr == nil; u.Verified != verified {
				t.Fatalf("verified = %v, want %v", u.Verified, verified)
			}
			if tt.wantErr == nil && !domain.CheckPassword(u.PasswordHash, tt.password) {
				t.Error("new password not stored")
			}
		})
	}
}

func TestCompleteVerificationSingleUse(t *testing.T) {
	e := newEnv(t)
	raw := requestToken(t, e)
	if err := e.svc.CompleteVerification(context.Background(), raw, "brand new passphrase"); err != nil {
		t.Fatal(err)
	}
	err := e.svc.CompleteVerification(context.Background(), raw, "another passphrase!")
	if !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("second use err = %v, want ErrInvalidToken", err)
	}
	if !domain.CheckPassword(e.users.byID["u2"].PasswordHash, "brand new passphrase") {
		t.Error("second use must not change the password")
	}
}

func TestWeakPasswordDoesNotBurnToken(t *testing.T) {
	e := newEnv(t)
	raw := requestToken(t, e)
	if err := e.svc.CompleteVerification(context.Background(), raw, "short"); !errors.Is(err, domain.ErrWeakPassword) {
		t.Fatal(err)
	}
	if err := e.svc.CompleteVerification(context.Background(), raw, "a valid passphrase"); err != nil {
		t.Fatalf("token must still be usable: %v", err)
	}
}

func TestCompleteVerificationRevokesRefreshTokens(t *testing.T) {
	e := newEnv(t)
	e.users.byID["u2"] = domain.User{ID: "u2", Email: unverifiedEmail, PasswordHash: "x"}
	e.refresh.tokens["h"] = &domain.RefreshToken{Hash: "h", UserID: "u2", ExpiresAt: t0.Add(time.Hour)}
	raw := requestToken(t, e)
	if err := e.svc.CompleteVerification(context.Background(), raw, "brand new passphrase"); err != nil {
		t.Fatal(err)
	}
	if n := e.refresh.liveCount("u2"); n != 0 {
		t.Fatalf("live refresh tokens = %d, want 0", n)
	}
}

func TestAuthenticateAndMe(t *testing.T) {
	e := newEnv(t)
	sess := loginSession(t, e)

	uid, err := e.svc.Authenticate(sess.AccessToken)
	if err != nil || uid != "u1" {
		t.Fatalf("authenticate: %q, %v", uid, err)
	}
	me, err := e.svc.Me(context.Background(), uid)
	if err != nil || me.Email != verifiedEmail {
		t.Fatalf("me: %+v, %v", me, err)
	}
	if _, err := e.svc.Me(context.Background(), "ghost"); !errors.Is(err, domain.ErrInvalidToken) {
		t.Errorf("unknown id err = %v", err)
	}

	e.clock.now = t0.Add(domain.AccessTokenTTL)
	if _, err := e.svc.Authenticate(sess.AccessToken); !errors.Is(err, domain.ErrInvalidToken) {
		t.Errorf("expired token err = %v", err)
	}
}
