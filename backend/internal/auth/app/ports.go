// Package app holds the authentication use cases and the ports they depend on.
package app

import (
	"context"
	"time"

	"github.com/valium69mg/finances-app/backend/internal/auth/domain"
)

// UserRepo persists users.
type UserRepo interface {
	// FindByEmail returns domain.ErrNotFound when the email is unknown.
	FindByEmail(ctx context.Context, email string) (domain.User, error)
	// FindByID returns domain.ErrNotFound when the id is unknown.
	FindByID(ctx context.Context, id string) (domain.User, error)
	// SetPasswordAndVerify stores a new password hash and marks the user verified.
	SetPasswordAndVerify(ctx context.Context, userID, passwordHash string) error
}

// RefreshTokenRepo persists hashed refresh tokens.
type RefreshTokenRepo interface {
	Create(ctx context.Context, token domain.RefreshToken) error
	// Find returns domain.ErrNotFound when the hash is unknown.
	Find(ctx context.Context, hash string) (domain.RefreshToken, error)
	// MarkUsed atomically marks an unused, unrevoked, unexpired token as used.
	// It reports false when the token was not in that state (e.g. lost a race).
	MarkUsed(ctx context.Context, hash string, now time.Time) (bool, error)
	// Revoke revokes one token; unknown or already revoked tokens are not an error.
	Revoke(ctx context.Context, hash string, now time.Time) error
	// RevokeAllForUser revokes every live refresh token of the user.
	RevokeAllForUser(ctx context.Context, userID string, now time.Time) error
}

// VerificationTokenRepo persists hashed verification tokens.
type VerificationTokenRepo interface {
	Create(ctx context.Context, token domain.VerificationToken) error
	// Consume atomically marks an unused, unexpired token as used and returns its
	// user id. It returns domain.ErrNotFound when the token is unknown, expired or used.
	Consume(ctx context.Context, hash string, now time.Time) (userID string, err error)
}

// Mailer sends the verification email.
type Mailer interface {
	SendVerification(ctx context.Context, to, link string) error
}

// Clock provides the current time.
type Clock interface {
	Now() time.Time
}

// RateLimiter reports whether one more event under key fits in the window.
type RateLimiter interface {
	Allow(key string, limit int, window time.Duration) bool
}

// SystemClock is the wall clock.
type SystemClock struct{}

// Now returns the current time.
func (SystemClock) Now() time.Time { return time.Now() }
