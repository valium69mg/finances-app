// Package app holds the users use cases (owner-only account administration)
// and the ports they depend on.
package app

import (
	"context"
	"time"

	"github.com/valium69mg/finances-app/backend/internal/platform/session"
	"github.com/valium69mg/finances-app/backend/internal/users/domain"
)

// Repo persists user accounts.
type Repo interface {
	// List returns every account, oldest first.
	List(ctx context.Context) ([]domain.User, error)
	// Get returns domain.ErrNotFound for an unknown (or malformed) id.
	Get(ctx context.Context, id string) (domain.User, error)
	// Create stores an unverified, active account and returns it. It returns
	// domain.ErrEmailTaken when the email is already used.
	Create(ctx context.Context, acc domain.NewAccount) (domain.User, error)
	// SetActive flips the active flag; domain.ErrNotFound for an unknown id.
	SetActive(ctx context.Context, id string, active bool) error
}

// Sessions revokes the refresh tokens of a user (implemented by the auth module).
type Sessions interface {
	RevokeSessions(ctx context.Context, userID string) error
}

// Inviter emails the invitation link to a user (implemented by the auth
// module, which owns the verification tokens).
type Inviter interface {
	SendInvitation(ctx context.Context, userID string) error
}

// RateLimiter counts events per key in fixed windows.
type RateLimiter interface {
	Allow(key string, limit int, window time.Duration) bool
	Peek(key string, limit int, window time.Duration) bool
	Record(key string, window time.Duration)
}

// Identity is the caller, taken from the request context by the adapters.
type Identity = session.Identity
