package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/valium69mg/finances-app/backend/internal/auth/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/session"
)

// Rate limits. All windows are fixed windows measured by the limiter's clock.
const (
	// IdentifyIPLimit is the number of identify calls one client IP may make per
	// IdentifyIPWindow. There is deliberately no per-email identify limit: it
	// would let anyone lock the account owner out of the first step.
	IdentifyIPLimit  = 10
	IdentifyIPWindow = 15 * time.Minute

	// VerifyMailCooldown is the minimum gap between two verification emails to
	// the same address; VerifyMailHourlyLimit caps them per VerifyMailHourWindow.
	// When exceeded the email is silently not sent.
	VerifyMailCooldown    = 60 * time.Second
	VerifyMailHourlyLimit = 5
	VerifyMailHourWindow  = time.Hour

	// LoginEmailFailLimit and LoginIPFailLimit bound FAILED password attempts per
	// LoginFailWindow, per email and per client IP. Successful logins and
	// identify calls never consume this budget.
	LoginEmailFailLimit = 5
	LoginIPFailLimit    = 20
	LoginFailWindow     = 15 * time.Minute

	// RefreshIPLimit is the number of refresh calls (successful or not) one client
	// IP may make per RefreshIPWindow. A session refreshes about every 15 minutes
	// per tab, so this is far above normal use (many tabs, reloads, a shared NAT)
	// and only stops floods against the token table.
	RefreshIPLimit  = 60
	RefreshIPWindow = 15 * time.Minute

	// VerifyIPLimit is the number of verification completions one client IP may
	// attempt per VerifyIPWindow. Each attempt costs a bcrypt hash, and a legitimate
	// user needs one or two (a weak password is rejected without burning the token).
	VerifyIPLimit  = 10
	VerifyIPWindow = 15 * time.Minute
)

const (
	keyIdentifyIP     = "identify:ip:"
	keyVerifyCooldown = "verify:cooldown:"
	keyVerifyHourly   = "verify:hourly:"
	keyLoginFailEmail = "login:fail:email:"
	keyLoginFailIP    = "login:fail:ip:"
	keyRefreshIP      = "refresh:ip:"
	keyVerifyIP       = "verify:ip:"

	mailSendTimeout  = 15 * time.Second
	verifyLinkParam  = "token"
	verifyLinkSuffix = "/verify"
)

// IdentifyStatus is the outcome of the email-first identification step.
type IdentifyStatus string

const (
	// IdentifyPasswordRequired means the account exists and is verified.
	IdentifyPasswordRequired IdentifyStatus = "password_required"
	// IdentifyVerificationSent is returned for unverified AND unknown emails so
	// the two cases look identical; only "verified account" is disclosed.
	IdentifyVerificationSent IdentifyStatus = "verification_sent"
)

// Session is the token pair returned after a successful login or refresh.
type Session struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    time.Duration
	// Role is the role read from the database when the session was issued; the
	// client uses it to shape the navigation (the backend enforces it anyway).
	Role session.Role
}

// Deps are the collaborators of Service.
type Deps struct {
	Users              UserRepo
	RefreshTokens      RefreshTokenRepo
	VerificationTokens VerificationTokenRepo
	Mailer             Mailer
	Clock              Clock
	Limiter            RateLimiter
	JWTSecret          []byte
	// AppBaseURL is the frontend origin; the verification link is <AppBaseURL>/verify?token=...
	AppBaseURL string
	// Spawn runs background work (the verification email). Defaults to a goroutine;
	// sending in the background keeps identify timing independent of the account state.
	Spawn  func(func())
	Logger *slog.Logger
}

// Service implements the authentication use cases.
type Service struct {
	Deps
}

// NewService builds a Service, filling optional dependencies with defaults.
func NewService(d Deps) *Service {
	if d.Clock == nil {
		d.Clock = SystemClock{}
	}
	if d.Spawn == nil {
		d.Spawn = func(fn func()) { go fn() }
	}
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	return &Service{Deps: d}
}

// Identify is step one of the email-first login. It reports whether the email
// belongs to a verified account (password step next). For an unverified account
// it triggers the verification email in the background; for an unknown email (or a
// deactivated account) it does nothing but answers exactly like the unverified case. Enumeration of
// verified accounts is an accepted trade-off (see PLAN.md §5).
func (s *Service) Identify(ctx context.Context, rawEmail, ip string) (IdentifyStatus, error) {
	if !s.Limiter.Allow(keyIdentifyIP+ip, IdentifyIPLimit, IdentifyIPWindow) {
		return "", domain.ErrRateLimited
	}
	email, err := domain.NormalizeEmail(rawEmail)
	if err != nil {
		return "", err
	}

	user, err := s.Users.FindByEmail(ctx, email)
	if errors.Is(err, domain.ErrNotFound) {
		return IdentifyVerificationSent, nil
	}
	if err != nil {
		return "", fmt.Errorf("find user: %w", err)
	}
	if !user.Active {
		return IdentifyVerificationSent, nil
	}
	if user.Verified {
		return IdentifyPasswordRequired, nil
	}

	s.sendVerificationInBackground(ctx, user)
	return IdentifyVerificationSent, nil
}

// sendVerificationInBackground emails a verification link unless the address
// hit its cooldown or hourly cap, in which case it silently does nothing.
// Sending in the background keeps response timing independent of the account state.
func (s *Service) sendVerificationInBackground(ctx context.Context, user domain.User) {
	// A request suppressed by one limit must not consume the other: the hourly
	// budget is only peeked first and recorded once the cooldown lets it through.
	if !s.Limiter.Peek(keyVerifyHourly+user.Email, VerifyMailHourlyLimit, VerifyMailHourWindow) ||
		!s.Limiter.Allow(keyVerifyCooldown+user.Email, 1, VerifyMailCooldown) {
		s.Logger.Info("verification email suppressed by rate limit", "user_id", user.ID)
		return
	}
	s.Limiter.Record(keyVerifyHourly+user.Email, VerifyMailHourWindow)
	bg := context.WithoutCancel(ctx)
	s.Spawn(func() {
		bg, cancel := context.WithTimeout(bg, mailSendTimeout)
		defer cancel()
		if err := s.RequestVerification(bg, user); err != nil {
			s.Logger.Error("send verification email", "error", err)
		}
	})
}

// Login is step two: it only authenticates. Unknown, unverified, deactivated and
// wrong-password cases all return domain.ErrInvalidCredentials, and no email is ever sent.
// Only failed attempts consume the per-email and per-IP budgets. The budget is
// reserved up front with Allow, so parallel guesses cannot all pass a check
// before the first one fails, and released again unless the attempt failed.
func (s *Service) Login(ctx context.Context, rawEmail, password, ip string) (*Session, error) {
	email, err := domain.NormalizeEmail(rawEmail)
	if err != nil {
		return nil, err
	}
	emailKey, ipKey := keyLoginFailEmail+email, keyLoginFailIP+ip
	if !s.Limiter.Allow(emailKey, LoginEmailFailLimit, LoginFailWindow) {
		return nil, domain.ErrRateLimited
	}
	if !s.Limiter.Allow(ipKey, LoginIPFailLimit, LoginFailWindow) {
		s.Limiter.Release(emailKey) // the rejected attempt must not cost the email its budget
		return nil, domain.ErrRateLimited
	}
	release := func() {
		s.Limiter.Release(emailKey)
		s.Limiter.Release(ipKey)
	}

	user, err := s.Users.FindByEmail(ctx, email)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		release() // an infrastructure error is not a wrong password
		return nil, fmt.Errorf("find user: %w", err)
	}
	if err != nil || !user.Verified || !user.Active || !domain.CheckPassword(user.PasswordHash, password) {
		return nil, domain.ErrInvalidCredentials // the reservation stays: it is the recorded failure
	}
	release()
	return s.issueSession(ctx, user)
}

// RequestVerification creates a verification token for user and emails the link.
func (s *Service) RequestVerification(ctx context.Context, user domain.User) error {
	return s.sendLink(ctx, user, s.Mailer.SendVerification)
}

// SendInvitation emails the invitation to the user with the given id. It reuses
// the verification token flow (1 hour, single use, a new token per call) with
// the invitation text. The caller (the users module) owns authorization and the
// invitation rate limits.
func (s *Service) SendInvitation(ctx context.Context, userID string) error {
	user, err := s.Users.FindByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("find user: %w", err)
	}
	return s.sendLink(ctx, user, s.Mailer.SendInvitation)
}

// RevokeSessions revokes every live refresh token of the user.
func (s *Service) RevokeSessions(ctx context.Context, userID string) error {
	if err := s.RefreshTokens.RevokeAllForUser(ctx, userID, s.Clock.Now()); err != nil {
		return fmt.Errorf("revoke refresh tokens: %w", err)
	}
	return nil
}

func (s *Service) sendLink(ctx context.Context, user domain.User, send func(ctx context.Context, to, link string) error) error {
	raw, hash, err := domain.GenerateToken()
	if err != nil {
		return fmt.Errorf("generate token: %w", err)
	}
	err = s.VerificationTokens.Create(ctx, domain.VerificationToken{
		Hash:      hash,
		UserID:    user.ID,
		ExpiresAt: s.Clock.Now().Add(domain.VerificationTokenTTL),
	})
	if err != nil {
		return fmt.Errorf("store verification token: %w", err)
	}
	link := s.AppBaseURL + verifyLinkSuffix + "?" + verifyLinkParam + "=" + url.QueryEscape(raw)
	if err := send(ctx, user.Email, link); err != nil {
		return fmt.Errorf("send email: %w", err)
	}
	return nil
}

// CompleteVerification consumes a verification token, sets the new password and
// marks the user verified. Existing refresh tokens are revoked.
func (s *Service) CompleteVerification(ctx context.Context, rawToken, newPassword, ip string) error {
	if !s.Limiter.Allow(keyVerifyIP+ip, VerifyIPLimit, VerifyIPWindow) {
		return domain.ErrRateLimited
	}
	// Validate first so a weak password does not burn the single-use token.
	if err := domain.ValidatePassword(newPassword); err != nil {
		return err
	}
	hash, err := domain.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	now := s.Clock.Now()
	userID, err := s.VerificationTokens.Consume(ctx, domain.HashToken(rawToken), now)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.ErrInvalidToken
	}
	if err != nil {
		return fmt.Errorf("consume verification token: %w", err)
	}
	if err := s.Users.SetPasswordAndVerify(ctx, userID, hash); err != nil {
		return fmt.Errorf("set password: %w", err)
	}
	if err := s.RefreshTokens.RevokeAllForUser(ctx, userID, now); err != nil {
		return fmt.Errorf("revoke refresh tokens: %w", err)
	}
	return nil
}

// Refresh rotates a refresh token. Presenting an already-used token revokes all
// of the user's refresh tokens.
func (s *Service) Refresh(ctx context.Context, rawToken, ip string) (*Session, error) {
	if !s.Limiter.Allow(keyRefreshIP+ip, RefreshIPLimit, RefreshIPWindow) {
		return nil, domain.ErrRateLimited
	}
	now := s.Clock.Now()
	hash := domain.HashToken(rawToken)

	stored, err := s.RefreshTokens.Find(ctx, hash)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.ErrInvalidToken
	}
	if err != nil {
		return nil, fmt.Errorf("find refresh token: %w", err)
	}

	if stored.UsedAt != nil {
		return nil, s.handleReuse(ctx, stored.UserID, now)
	}
	if stored.RevokedAt != nil || stored.Expired(now) {
		return nil, domain.ErrInvalidToken
	}

	ok, err := s.RefreshTokens.MarkUsed(ctx, hash, now)
	if err != nil {
		return nil, fmt.Errorf("mark refresh token used: %w", err)
	}
	if !ok { // lost a race with a concurrent use of the same token
		return nil, s.handleReuse(ctx, stored.UserID, now)
	}

	user, err := s.Users.FindByID(ctx, stored.UserID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.ErrInvalidToken
	}
	if err != nil {
		return nil, fmt.Errorf("find user: %w", err)
	}
	// The role and the active flag are read from the database on every refresh,
	// so a deactivation or a role change takes effect within one refresh cycle.
	if !user.Verified || !user.Active {
		return nil, domain.ErrInvalidToken
	}
	return s.issueSession(ctx, user)
}

func (s *Service) handleReuse(ctx context.Context, userID string, now time.Time) error {
	if err := s.RefreshTokens.RevokeAllForUser(ctx, userID, now); err != nil {
		return fmt.Errorf("revoke refresh tokens after reuse: %w", err)
	}
	s.Logger.Warn("refresh token reuse detected; all refresh tokens revoked", "user_id", userID)
	return domain.ErrInvalidToken
}

// Logout revokes the presented refresh token. Unknown tokens are ignored.
func (s *Service) Logout(ctx context.Context, rawToken string) error {
	if err := s.RefreshTokens.Revoke(ctx, domain.HashToken(rawToken), s.Clock.Now()); err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	return nil
}

// Authenticate validates an access token and returns the identity it carries.
func (s *Service) Authenticate(accessToken string) (session.Identity, error) {
	return domain.ParseAccessToken(s.JWTSecret, accessToken, s.Clock.Now())
}

// Me returns the user identified by id.
func (s *Service) Me(ctx context.Context, id string) (domain.User, error) {
	user, err := s.Users.FindByID(ctx, id)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.User{}, domain.ErrInvalidToken
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("find user: %w", err)
	}
	return user, nil
}

func (s *Service) issueSession(ctx context.Context, user domain.User) (*Session, error) {
	now := s.Clock.Now()
	access, err := domain.IssueAccessToken(s.JWTSecret, user.ID, user.Role, now)
	if err != nil {
		return nil, fmt.Errorf("issue access token: %w", err)
	}
	raw, hash, err := domain.GenerateToken()
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}
	err = s.RefreshTokens.Create(ctx, domain.RefreshToken{
		Hash:      hash,
		UserID:    user.ID,
		ExpiresAt: now.Add(domain.RefreshTokenTTL),
	})
	if err != nil {
		return nil, fmt.Errorf("store refresh token: %w", err)
	}
	return &Session{AccessToken: access, RefreshToken: raw, ExpiresIn: domain.AccessTokenTTL, Role: user.Role}, nil
}
