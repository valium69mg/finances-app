package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/valium69mg/finances-app/backend/internal/auth/domain"
)

const (
	loginEmailLimit  = 5
	loginIPLimit     = 20
	loginWindow      = 15 * time.Minute
	mailSendTimeout  = 15 * time.Second
	verifyLinkParam  = "token"
	verifyLinkSuffix = "/verify"
)

// Session is the token pair returned after a successful login or refresh.
type Session struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    time.Duration
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
	// sending in the background keeps login timing independent of the account state.
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

// Login authenticates a verified user. For an unknown or unverified email it
// returns (nil, nil) and, for an unverified one, sends a verification email in
// the background: callers must answer both cases identically.
func (s *Service) Login(ctx context.Context, rawEmail, password, ip string) (*Session, error) {
	email, err := domain.NormalizeEmail(rawEmail)
	if err != nil {
		return nil, err
	}
	if !s.Limiter.Allow("login:email:"+email, loginEmailLimit, loginWindow) ||
		!s.Limiter.Allow("login:ip:"+ip, loginIPLimit, loginWindow) {
		return nil, domain.ErrRateLimited
	}

	user, err := s.Users.FindByEmail(ctx, email)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find user: %w", err)
	}

	if !user.Verified {
		bg := context.WithoutCancel(ctx)
		s.Spawn(func() {
			bg, cancel := context.WithTimeout(bg, mailSendTimeout)
			defer cancel()
			if err := s.RequestVerification(bg, user); err != nil {
				s.Logger.Error("send verification email", "error", err)
			}
		})
		return nil, nil
	}

	if !domain.CheckPassword(user.PasswordHash, password) {
		return nil, domain.ErrInvalidCredentials
	}
	return s.issueSession(ctx, user.ID)
}

// RequestVerification creates a verification token for user and emails the link.
func (s *Service) RequestVerification(ctx context.Context, user domain.User) error {
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
	if err := s.Mailer.SendVerification(ctx, user.Email, link); err != nil {
		return fmt.Errorf("send email: %w", err)
	}
	return nil
}

// CompleteVerification consumes a verification token, sets the new password and
// marks the user verified. Existing refresh tokens are revoked.
func (s *Service) CompleteVerification(ctx context.Context, rawToken, newPassword string) error {
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
func (s *Service) Refresh(ctx context.Context, rawToken string) (*Session, error) {
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
	if !user.Verified {
		return nil, domain.ErrInvalidToken
	}
	return s.issueSession(ctx, user.ID)
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

// Authenticate validates an access token and returns the user id.
func (s *Service) Authenticate(accessToken string) (string, error) {
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

func (s *Service) issueSession(ctx context.Context, userID string) (*Session, error) {
	now := s.Clock.Now()
	access, err := domain.IssueAccessToken(s.JWTSecret, userID, now)
	if err != nil {
		return nil, fmt.Errorf("issue access token: %w", err)
	}
	raw, hash, err := domain.GenerateToken()
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}
	err = s.RefreshTokens.Create(ctx, domain.RefreshToken{
		Hash:      hash,
		UserID:    userID,
		ExpiresAt: now.Add(domain.RefreshTokenTTL),
	})
	if err != nil {
		return nil, fmt.Errorf("store refresh token: %w", err)
	}
	return &Session{AccessToken: access, RefreshToken: raw, ExpiresIn: domain.AccessTokenTTL}, nil
}
