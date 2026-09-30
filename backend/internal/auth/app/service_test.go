package app_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/valium69mg/finances-app/backend/internal/auth/adapters/ratelimit"
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

type env struct {
	svc     *app.Service
	clock   *fakeClock
	users   *fakeUsers
	refresh *fakeRefresh
	verif   *fakeVerification
	mailer  *fakeMailer
}

const (
	verifiedEmail        = "ok@example.com"
	unverifiedEmail      = "new@example.com"
	otherUnverifiedEmail = "other@example.com"
	password             = "correct horse battery"
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
			"u3": {ID: "u3", Email: otherUnverifiedEmail, PasswordHash: hash, Verified: false},
		}},
		refresh: &fakeRefresh{tokens: map[string]*domain.RefreshToken{}},
		verif:   &fakeVerification{tokens: map[string]*domain.VerificationToken{}},
		mailer:  &fakeMailer{},
	}
	e.svc = app.NewService(app.Deps{
		Users:              e.users,
		RefreshTokens:      e.refresh,
		VerificationTokens: e.verif,
		Mailer:             e.mailer,
		Clock:              e.clock,
		Limiter:            ratelimit.New(e.clock.Now),
		JWTSecret:          []byte(strings.Repeat("k", 32)),
		AppBaseURL:         "https://app.example.com",
		Spawn:              func(fn func()) { fn() },
		Logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	return e
}

const wrongPassword = "nope nope nope"

var ctx = context.Background()

func ipN(i int) string { return fmt.Sprintf("198.51.100.%d", i) }

// testIP is the client of the refresh and verify tests that are not about rate limits.
const testIP = "203.0.113.50"

func TestIdentify(t *testing.T) {
	tests := []struct {
		name       string
		email      string
		wantStatus app.IdentifyStatus
		wantErr    error
		wantMailTo string
	}{
		{"verified account needs a password", verifiedEmail, app.IdentifyPasswordRequired, nil, ""},
		{"verified email is normalized", "  OK@Example.com ", app.IdentifyPasswordRequired, nil, ""},
		{"unverified account gets a verification email", unverifiedEmail, app.IdentifyVerificationSent, nil, unverifiedEmail},
		{"unverified email is normalized before sending", "\tNEW@EXAMPLE.COM\n", app.IdentifyVerificationSent, nil, unverifiedEmail},
		{"unknown email sends nothing", "ghost@example.com", app.IdentifyVerificationSent, nil, ""},
		{"empty", "", "", domain.ErrInvalidEmail, ""},
		{"only whitespace", "   ", "", domain.ErrInvalidEmail, ""},
		{"no at sign", "not-an-email", "", domain.ErrInvalidEmail, ""},
		{"two at signs", "a@b@c.com", "", domain.ErrInvalidEmail, ""},
		{"display name form", "Ana <ana@example.com>", "", domain.ErrInvalidEmail, ""},
		{"too long", strings.Repeat("a", 250) + "@example.com", "", domain.ErrInvalidEmail, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			status, err := e.svc.Identify(ctx, tt.email, "1.2.3.4")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if status != tt.wantStatus {
				t.Fatalf("status = %q, want %q", status, tt.wantStatus)
			}
			wantMails := 0
			if tt.wantMailTo != "" {
				wantMails = 1
			}
			if len(e.mailer.sent) != wantMails {
				t.Fatalf("mails = %d, want %d", len(e.mailer.sent), wantMails)
			}
			if wantMails == 1 && e.mailer.sent[0].to != tt.wantMailTo {
				t.Errorf("mail to %q, want %q", e.mailer.sent[0].to, tt.wantMailTo)
			}
		})
	}
}

func TestIdentifyUnknownAndUnverifiedAreIndistinguishable(t *testing.T) {
	e := newEnv(t)
	unknown, errUnknown := e.svc.Identify(ctx, "ghost@example.com", "ip-a")
	unverified, errUnverified := e.svc.Identify(ctx, unverifiedEmail, "ip-b")
	if unknown != unverified || errUnknown != nil || errUnverified != nil {
		t.Fatalf("unknown=(%q,%v) unverified=(%q,%v): want identical", unknown, errUnknown, unverified, errUnverified)
	}
}

func TestIdentifyMailFailureStaysGeneric(t *testing.T) {
	e := newEnv(t)
	e.mailer.err = errors.New("resend down")
	status, err := e.svc.Identify(ctx, unverifiedEmail, "ip")
	if status != app.IdentifyVerificationSent || err != nil {
		t.Fatalf("got %q, %v; want verification_sent, nil", status, err)
	}
}

func TestIdentifyVerificationEmailContent(t *testing.T) {
	e := newEnv(t)
	if _, err := e.svc.Identify(ctx, unverifiedEmail, "ip"); err != nil {
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

func TestIdentifyNeverCreatesTokensForVerifiedOrUnknown(t *testing.T) {
	e := newEnv(t)
	for _, email := range []string{verifiedEmail, "ghost@example.com"} {
		if _, err := e.svc.Identify(ctx, email, "ip"); err != nil {
			t.Fatal(err)
		}
	}
	if len(e.verif.tokens) != 0 || len(e.mailer.sent) != 0 {
		t.Fatalf("tokens=%d mails=%d, want none", len(e.verif.tokens), len(e.mailer.sent))
	}
}

func TestIdentifyIPLimit(t *testing.T) {
	e := newEnv(t)
	for i := 1; i <= app.IdentifyIPLimit; i++ {
		if _, err := e.svc.Identify(ctx, verifiedEmail, "1.1.1.1"); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	status, err := e.svc.Identify(ctx, verifiedEmail, "1.1.1.1")
	if !errors.Is(err, domain.ErrRateLimited) || status != "" {
		t.Fatalf("call %d: got %q, %v; want ErrRateLimited", app.IdentifyIPLimit+1, status, err)
	}
	if _, err := e.svc.Identify(ctx, verifiedEmail, "2.2.2.2"); err != nil {
		t.Fatalf("another IP must be unaffected: %v", err)
	}

	e.clock.now = t0.Add(app.IdentifyIPWindow - time.Second)
	if _, err := e.svc.Identify(ctx, verifiedEmail, "1.1.1.1"); !errors.Is(err, domain.ErrRateLimited) {
		t.Fatalf("still inside the window: %v", err)
	}
	e.clock.now = t0.Add(app.IdentifyIPWindow)
	if _, err := e.svc.Identify(ctx, verifiedEmail, "1.1.1.1"); err != nil {
		t.Fatalf("window must reset: %v", err)
	}
}

func TestIdentifyIPLimitCountsEveryKindOfCall(t *testing.T) {
	e := newEnv(t)
	emails := []string{verifiedEmail, unverifiedEmail, "ghost@example.com", "bad", verifiedEmail}
	for i := 0; i < app.IdentifyIPLimit; i++ {
		_, _ = e.svc.Identify(ctx, emails[i%len(emails)], "1.1.1.1")
	}
	if _, err := e.svc.Identify(ctx, verifiedEmail, "1.1.1.1"); !errors.Is(err, domain.ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
	if _, err := e.svc.Identify(ctx, "bad", "1.1.1.1"); !errors.Is(err, domain.ErrRateLimited) {
		t.Fatalf("limit must win over validation: %v", err)
	}
}

func TestIdentifyRateLimitedSendsNoEmail(t *testing.T) {
	e := newEnv(t)
	for range app.IdentifyIPLimit {
		_, _ = e.svc.Identify(ctx, verifiedEmail, "1.1.1.1")
	}
	if _, err := e.svc.Identify(ctx, unverifiedEmail, "1.1.1.1"); !errors.Is(err, domain.ErrRateLimited) {
		t.Fatal(err)
	}
	if len(e.mailer.sent) != 0 {
		t.Fatal("no email may be sent when rate limited")
	}
}

func TestIdentifyHasNoPerEmailLimit(t *testing.T) {
	e := newEnv(t)
	// Many different clients asking about the same account never get blocked:
	// a per-email limit would let an attacker lock the owner out of step one.
	for i := range 200 {
		status, err := e.svc.Identify(ctx, verifiedEmail, ipN(i))
		if err != nil || status != app.IdentifyPasswordRequired {
			t.Fatalf("call %d: %q, %v", i, status, err)
		}
	}
}

func TestIdentifyDoesNotConsumeLoginBudget(t *testing.T) {
	e := newEnv(t)
	for range app.IdentifyIPLimit {
		if _, err := e.svc.Identify(ctx, verifiedEmail, "1.1.1.1"); err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i <= app.LoginEmailFailLimit; i++ {
		if _, err := e.svc.Login(ctx, verifiedEmail, wrongPassword, "1.1.1.1"); !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("failure %d: err = %v, want ErrInvalidCredentials", i, err)
		}
	}
}

func TestVerificationEmailCooldown(t *testing.T) {
	steps := []struct {
		name     string
		at       time.Duration
		wantMail bool
	}{
		{"first request sends", 0, true},
		{"immediately again is suppressed", 0, false},
		{"just before the cooldown ends", VerifyCooldownMinusOne, false},
		{"exactly at the cooldown", app.VerifyMailCooldown, true},
		{"right after a send is suppressed again", app.VerifyMailCooldown + time.Second, false},
	}
	e := newEnv(t)
	for i, s := range steps {
		e.clock.now = t0.Add(s.at)
		before := len(e.mailer.sent)
		status, err := e.svc.Identify(ctx, unverifiedEmail, ipN(i))
		if err != nil || status != app.IdentifyVerificationSent {
			t.Fatalf("%s: %q, %v; response must always be verification_sent", s.name, status, err)
		}
		if sent := len(e.mailer.sent) > before; sent != s.wantMail {
			t.Fatalf("%s: sent = %v, want %v", s.name, sent, s.wantMail)
		}
	}
}

// VerifyCooldownMinusOne is one nanosecond short of the cooldown.
const VerifyCooldownMinusOne = app.VerifyMailCooldown - time.Nanosecond

func TestVerificationEmailHourlyCap(t *testing.T) {
	e := newEnv(t)
	call := func(i int, at time.Duration) bool {
		e.clock.now = t0.Add(at)
		before := len(e.mailer.sent)
		if _, err := e.svc.Identify(ctx, unverifiedEmail, ipN(i)); err != nil {
			t.Fatal(err)
		}
		return len(e.mailer.sent) > before
	}

	n := 0
	for k := range app.VerifyMailHourlyLimit {
		n++
		if !call(n, time.Duration(k)*app.VerifyMailCooldown) {
			t.Fatalf("email %d within the cap must be sent", k+1)
		}
	}
	over := time.Duration(app.VerifyMailHourlyLimit) * app.VerifyMailCooldown
	n++
	if call(n, over) {
		t.Fatal("email over the hourly cap must be suppressed")
	}
	n++
	if call(n, app.VerifyMailHourWindow-time.Second) {
		t.Fatal("still suppressed just before the hourly window ends")
	}
	n++
	if !call(n, app.VerifyMailHourWindow) {
		t.Fatal("hourly window must reset")
	}
	if got := len(e.mailer.sent); got != app.VerifyMailHourlyLimit+1 {
		t.Fatalf("total mails = %d, want %d", got, app.VerifyMailHourlyLimit+1)
	}
}

func TestSuppressedRequestsDoNotBurnTheHourlyCap(t *testing.T) {
	e := newEnv(t)
	if _, err := e.svc.Identify(ctx, unverifiedEmail, ipN(0)); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 30; i++ { // all inside the cooldown
		_, _ = e.svc.Identify(ctx, unverifiedEmail, ipN(i))
	}
	if len(e.mailer.sent) != 1 {
		t.Fatalf("mails = %d, want 1", len(e.mailer.sent))
	}
	for k := 1; k < app.VerifyMailHourlyLimit; k++ {
		e.clock.now = t0.Add(time.Duration(k) * app.VerifyMailCooldown)
		before := len(e.mailer.sent)
		_, _ = e.svc.Identify(ctx, unverifiedEmail, ipN(100+k))
		if len(e.mailer.sent) != before+1 {
			t.Fatalf("send %d must not be blocked by earlier suppressed requests", k+1)
		}
	}
}

func TestVerificationEmailLimitsArePerEmail(t *testing.T) {
	e := newEnv(t)
	for i, email := range []string{unverifiedEmail, otherUnverifiedEmail, unverifiedEmail, otherUnverifiedEmail} {
		if _, err := e.svc.Identify(ctx, email, ipN(i)); err != nil {
			t.Fatal(err)
		}
	}
	if len(e.mailer.sent) != 2 {
		t.Fatalf("mails = %d, want 2 (one per address)", len(e.mailer.sent))
	}
	if e.mailer.sent[0].to == e.mailer.sent[1].to {
		t.Fatal("each address must get its own email")
	}
}

func TestVerificationEmailLimitsDoNotConsumeLoginOrIdentifyBudgetsElsewhere(t *testing.T) {
	e := newEnv(t)
	for i := range 3 {
		if _, err := e.svc.Identify(ctx, unverifiedEmail, ipN(i)); err != nil {
			t.Fatal(err)
		}
	}
	// A different IP still has its full budget, and login failures are untouched.
	for i := 1; i <= app.LoginEmailFailLimit; i++ {
		if _, err := e.svc.Login(ctx, unverifiedEmail, wrongPassword, ipN(50)); !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("failure %d: %v", i, err)
		}
	}
}

func TestLogin(t *testing.T) {
	tests := []struct {
		name        string
		email       string
		password    string
		wantSession bool
		wantErr     error
	}{
		{"verified user with right password", verifiedEmail, password, true, nil},
		{"email is normalized", "  OK@Example.com ", password, true, nil},
		{"verified user with wrong password", verifiedEmail, wrongPassword, false, domain.ErrInvalidCredentials},
		{"empty password", verifiedEmail, "", false, domain.ErrInvalidCredentials},
		{"unknown email", "ghost@example.com", password, false, domain.ErrInvalidCredentials},
		{"unverified account even with its stored password", unverifiedEmail, password, false, domain.ErrInvalidCredentials},
		{"unverified account with a wrong password", unverifiedEmail, wrongPassword, false, domain.ErrInvalidCredentials},
		{"invalid email", "not-an-email", password, false, domain.ErrInvalidEmail},
		{"empty email", "", password, false, domain.ErrInvalidEmail},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			sess, err := e.svc.Login(ctx, tt.email, tt.password, "1.2.3.4")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if (sess != nil) != tt.wantSession {
				t.Fatalf("session = %v, want present=%v", sess, tt.wantSession)
			}
			if len(e.mailer.sent) != 0 || len(e.verif.tokens) != 0 {
				t.Fatalf("login must never send email: mails=%d tokens=%d", len(e.mailer.sent), len(e.verif.tokens))
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

func TestLoginUnknownUnverifiedAndWrongPasswordAreIdentical(t *testing.T) {
	e := newEnv(t)
	cases := []struct{ email, password string }{
		{verifiedEmail, wrongPassword},
		{"ghost@example.com", password},
		{unverifiedEmail, password},
	}
	for i, c := range cases {
		sess, err := e.svc.Login(ctx, c.email, c.password, ipN(i))
		if sess != nil || err != domain.ErrInvalidCredentials {
			t.Fatalf("case %d: got (%v, %v), want (nil, ErrInvalidCredentials)", i, sess, err)
		}
	}
}

func TestLoginOnlyFailuresConsumeTheBudget(t *testing.T) {
	e := newEnv(t)
	// Far more successes than either limit allows must never lock anything.
	for i := range app.LoginIPFailLimit + app.LoginEmailFailLimit + 5 {
		if _, err := e.svc.Login(ctx, verifiedEmail, password, "1.1.1.1"); err != nil {
			t.Fatalf("success %d: %v", i+1, err)
		}
	}
	// The full failure budget is still intact afterwards.
	for i := 1; i <= app.LoginEmailFailLimit; i++ {
		if _, err := e.svc.Login(ctx, verifiedEmail, wrongPassword, "1.1.1.1"); !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("failure %d: %v", i, err)
		}
	}
	if _, err := e.svc.Login(ctx, verifiedEmail, password, "1.1.1.1"); !errors.Is(err, domain.ErrRateLimited) {
		t.Fatalf("after %d failures the email is locked: %v", app.LoginEmailFailLimit, err)
	}
}

func TestLoginSuccessDoesNotResetFailures(t *testing.T) {
	e := newEnv(t)
	for range app.LoginEmailFailLimit - 1 {
		_, _ = e.svc.Login(ctx, verifiedEmail, wrongPassword, "1.1.1.1")
	}
	if _, err := e.svc.Login(ctx, verifiedEmail, password, "1.1.1.1"); err != nil {
		t.Fatal(err)
	}
	_, _ = e.svc.Login(ctx, verifiedEmail, wrongPassword, "1.1.1.1") // 5th failure
	if _, err := e.svc.Login(ctx, verifiedEmail, password, "1.1.1.1"); !errors.Is(err, domain.ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
}

func TestLoginLockoutPerEmail(t *testing.T) {
	tests := []struct {
		name  string
		email string
	}{
		{"verified account", verifiedEmail},
		{"unknown account", "ghost@example.com"},
		{"unverified account", unverifiedEmail},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			for i := 1; i <= app.LoginEmailFailLimit; i++ {
				if _, err := e.svc.Login(ctx, tt.email, wrongPassword, ipN(i)); !errors.Is(err, domain.ErrInvalidCredentials) {
					t.Fatalf("failure %d: %v", i, err)
				}
			}
			// The budget is checked before the password is verified, so even the
			// right password is refused, from any IP.
			sess, err := e.svc.Login(ctx, tt.email, password, "9.9.9.9")
			if !errors.Is(err, domain.ErrRateLimited) || sess != nil {
				t.Fatalf("got (%v, %v), want ErrRateLimited", sess, err)
			}
			if len(e.mailer.sent) != 0 {
				t.Fatal("no email may be sent")
			}
		})
	}
}

func TestLoginLockoutIsIsolatedPerEmail(t *testing.T) {
	e := newEnv(t)
	for range app.LoginEmailFailLimit {
		_, _ = e.svc.Login(ctx, "ghost@example.com", wrongPassword, "1.1.1.1")
	}
	if _, err := e.svc.Login(ctx, verifiedEmail, password, "1.1.1.1"); err != nil {
		t.Fatalf("another email must be unaffected: %v", err)
	}
}

func TestLoginIPLimitAcrossEmails(t *testing.T) {
	e := newEnv(t)
	for i := 1; i <= app.LoginIPFailLimit; i++ {
		email := fmt.Sprintf("ghost%d@example.com", i) // each email stays under its own limit
		if _, err := e.svc.Login(ctx, email, wrongPassword, "6.6.6.6"); !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("failure %d: %v", i, err)
		}
	}
	if _, err := e.svc.Login(ctx, verifiedEmail, password, "6.6.6.6"); !errors.Is(err, domain.ErrRateLimited) {
		t.Fatalf("IP must be locked out, got %v", err)
	}
	if _, err := e.svc.Login(ctx, verifiedEmail, password, "7.7.7.7"); err != nil {
		t.Fatalf("another IP must be unaffected: %v", err)
	}
}

func TestLoginBudgetResetsAfterWindow(t *testing.T) {
	e := newEnv(t)
	for range app.LoginEmailFailLimit {
		_, _ = e.svc.Login(ctx, verifiedEmail, wrongPassword, "1.1.1.1")
	}

	e.clock.now = t0.Add(app.LoginFailWindow / 2)
	if _, err := e.svc.Login(ctx, verifiedEmail, password, "1.1.1.1"); !errors.Is(err, domain.ErrRateLimited) {
		t.Fatalf("mid-window: %v", err)
	}
	e.clock.now = t0.Add(app.LoginFailWindow - time.Nanosecond)
	if _, err := e.svc.Login(ctx, verifiedEmail, password, "1.1.1.1"); !errors.Is(err, domain.ErrRateLimited) {
		t.Fatalf("just before the end: %v (blocked attempts must not extend the window)", err)
	}
	e.clock.now = t0.Add(app.LoginFailWindow)
	if _, err := e.svc.Login(ctx, verifiedEmail, password, "1.1.1.1"); err != nil {
		t.Fatalf("window over: %v", err)
	}
}

func TestLoginIPBudgetResetsAfterWindow(t *testing.T) {
	e := newEnv(t)
	for i := 1; i <= app.LoginIPFailLimit; i++ {
		_, _ = e.svc.Login(ctx, fmt.Sprintf("ghost%d@example.com", i), wrongPassword, "6.6.6.6")
	}
	e.clock.now = t0.Add(app.LoginFailWindow)
	if _, err := e.svc.Login(ctx, verifiedEmail, password, "6.6.6.6"); err != nil {
		t.Fatalf("IP budget must reset: %v", err)
	}
}

func TestLoginLockoutDoesNotBlockIdentify(t *testing.T) {
	e := newEnv(t)
	for range app.LoginEmailFailLimit {
		_, _ = e.svc.Login(ctx, verifiedEmail, wrongPassword, "1.1.1.1")
	}
	if _, err := e.svc.Login(ctx, verifiedEmail, password, "1.1.1.1"); !errors.Is(err, domain.ErrRateLimited) {
		t.Fatal("precondition: locked")
	}
	status, err := e.svc.Identify(ctx, verifiedEmail, "1.1.1.1")
	if err != nil || status != app.IdentifyPasswordRequired {
		t.Fatalf("identify while locked: %q, %v", status, err)
	}
}

func TestLoginInvalidEmailDoesNotConsumeBudget(t *testing.T) {
	e := newEnv(t)
	for range 50 {
		if _, err := e.svc.Login(ctx, "nope", password, "1.1.1.1"); !errors.Is(err, domain.ErrInvalidEmail) {
			t.Fatal(err)
		}
	}
	if _, err := e.svc.Login(ctx, verifiedEmail, password, "1.1.1.1"); err != nil {
		t.Fatal(err)
	}
}

func TestLoginNeverSendsEmail(t *testing.T) {
	e := newEnv(t)
	for i := range 30 {
		_, _ = e.svc.Login(ctx, unverifiedEmail, password, ipN(i))
	}
	if len(e.mailer.sent) != 0 || len(e.verif.tokens) != 0 {
		t.Fatalf("mails=%d tokens=%d, want none", len(e.mailer.sent), len(e.verif.tokens))
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

	second, err := e.svc.Refresh(context.Background(), first.RefreshToken, testIP)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if second.RefreshToken == first.RefreshToken || second.AccessToken == "" {
		t.Fatal("refresh must issue a new pair")
	}
	if e.refresh.tokens[domain.HashToken(first.RefreshToken)].UsedAt == nil {
		t.Fatal("old token must be marked used")
	}
	if _, err := e.svc.Refresh(context.Background(), second.RefreshToken, testIP); err != nil {
		t.Fatalf("rotated token must work once: %v", err)
	}
}

func TestRefreshReuseRevokesAll(t *testing.T) {
	e := newEnv(t)
	first := loginSession(t, e)
	second, err := e.svc.Refresh(context.Background(), first.RefreshToken, testIP)
	if err != nil {
		t.Fatal(err)
	}
	other := loginSession(t, e) // another device

	_, err = e.svc.Refresh(context.Background(), first.RefreshToken, testIP) // replay
	if !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("reuse err = %v, want ErrInvalidToken", err)
	}
	if n := e.refresh.liveCount("u1"); n != 0 {
		t.Fatalf("live tokens after reuse = %d, want 0", n)
	}
	for name, tok := range map[string]string{"rotated": second.RefreshToken, "other device": other.RefreshToken} {
		if _, err := e.svc.Refresh(context.Background(), tok, testIP); !errors.Is(err, domain.ErrInvalidToken) {
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
			sess, err := e.svc.Refresh(context.Background(), tok, testIP)
			if sess != nil || !errors.Is(err, domain.ErrInvalidToken) {
				t.Fatalf("got %v, %v; want ErrInvalidToken", sess, err)
			}
		})
	}
}

func TestRefreshRateLimitPerIP(t *testing.T) {
	e := newEnv(t)
	// Unknown tokens count too: the budget is per IP, whatever the outcome.
	for i := range app.RefreshIPLimit {
		if _, err := e.svc.Refresh(ctx, "unknown", testIP); !errors.Is(err, domain.ErrInvalidToken) {
			t.Fatalf("call %d: err = %v, want ErrInvalidToken", i, err)
		}
	}
	if _, err := e.svc.Refresh(ctx, "unknown", testIP); !errors.Is(err, domain.ErrRateLimited) {
		t.Fatalf("over the limit: err = %v, want ErrRateLimited", err)
	}
	// A valid token from the limited IP is rejected as well, without being consumed.
	sess := loginSession(t, e)
	if _, err := e.svc.Refresh(ctx, sess.RefreshToken, testIP); !errors.Is(err, domain.ErrRateLimited) {
		t.Fatalf("limited IP: err = %v, want ErrRateLimited", err)
	}
	if e.refresh.tokens[domain.HashToken(sess.RefreshToken)].UsedAt != nil {
		t.Fatal("a rate limited refresh must not consume the token")
	}
	// Another IP is unaffected and the window reopens.
	if _, err := e.svc.Refresh(ctx, sess.RefreshToken, ipN(1)); err != nil {
		t.Fatalf("other IP: %v", err)
	}
	e.clock.now = e.clock.now.Add(app.RefreshIPWindow)
	if _, err := e.svc.Refresh(ctx, "unknown", testIP); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("after the window: err = %v, want ErrInvalidToken", err)
	}
}

func TestRefreshNormalUseIsNeverLimited(t *testing.T) {
	e := newEnv(t)
	sess := loginSession(t, e)
	// A busy day from one IP: a rotation every 15 minutes for 8 hours, times 3 tabs.
	for i := range 8 * 4 * 3 {
		e.clock.now = t0.Add(time.Duration(i) * 5 * time.Minute)
		next, err := e.svc.Refresh(ctx, sess.RefreshToken, testIP)
		if err != nil {
			t.Fatalf("rotation %d: %v", i, err)
		}
		sess = next
	}
}

func TestVerifyRateLimitPerIP(t *testing.T) {
	e := newEnv(t)
	for i := range app.VerifyIPLimit {
		if err := e.svc.CompleteVerification(ctx, "bogus", "a valid passphrase", testIP); !errors.Is(err, domain.ErrInvalidToken) {
			t.Fatalf("call %d: err = %v, want ErrInvalidToken", i, err)
		}
	}
	raw := requestToken(t, e)
	if err := e.svc.CompleteVerification(ctx, raw, "a valid passphrase", testIP); !errors.Is(err, domain.ErrRateLimited) {
		t.Fatalf("over the limit: err = %v, want ErrRateLimited", err)
	}
	if e.users.byID["u2"].Verified {
		t.Fatal("a rate limited verification must not verify the user")
	}
	if err := e.svc.CompleteVerification(ctx, raw, "a valid passphrase", ipN(1)); err != nil {
		t.Fatalf("other IP: %v", err)
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
	if _, err := e.svc.Refresh(context.Background(), a.RefreshToken, testIP); !errors.Is(err, domain.ErrInvalidToken) {
		t.Errorf("logged-out token usable: %v", err)
	}
	if _, err := e.svc.Refresh(context.Background(), b.RefreshToken, testIP); err != nil {
		t.Errorf("other token must survive: %v", err)
	}
}

func requestToken(t *testing.T, e *env) string {
	t.Helper()
	if _, err := e.svc.Identify(context.Background(), unverifiedEmail, "ip"); err != nil {
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

			err := e.svc.CompleteVerification(context.Background(), tt.token(raw), tt.password, testIP)
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
	if err := e.svc.CompleteVerification(context.Background(), raw, "brand new passphrase", testIP); err != nil {
		t.Fatal(err)
	}
	err := e.svc.CompleteVerification(context.Background(), raw, "another passphrase!", testIP)
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
	if err := e.svc.CompleteVerification(context.Background(), raw, "short", testIP); !errors.Is(err, domain.ErrWeakPassword) {
		t.Fatal(err)
	}
	if err := e.svc.CompleteVerification(context.Background(), raw, "a valid passphrase", testIP); err != nil {
		t.Fatalf("token must still be usable: %v", err)
	}
}

func TestCompleteVerificationRevokesRefreshTokens(t *testing.T) {
	e := newEnv(t)
	e.users.byID["u2"] = domain.User{ID: "u2", Email: unverifiedEmail, PasswordHash: "x"}
	e.refresh.tokens["h"] = &domain.RefreshToken{Hash: "h", UserID: "u2", ExpiresAt: t0.Add(time.Hour)}
	raw := requestToken(t, e)
	if err := e.svc.CompleteVerification(context.Background(), raw, "brand new passphrase", testIP); err != nil {
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
