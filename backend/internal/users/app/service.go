package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/valium69mg/finances-app/backend/internal/platform/session"
	"github.com/valium69mg/finances-app/backend/internal/users/domain"
)

// Invitation rate limits (fixed windows, per process). They mirror the
// verification email limits of the auth module and add a cap per owner.
const (
	// InviteUserCooldown is the minimum gap between two invitations to one user.
	InviteUserCooldown = 60 * time.Second
	// InviteUserHourlyLimit caps invitations per user per InviteHourWindow.
	InviteUserHourlyLimit = 5
	// InviteOwnerHourlyLimit caps invitations sent by one owner per InviteHourWindow.
	InviteOwnerHourlyLimit = 20
	InviteHourWindow       = time.Hour
)

const (
	keyInviteCooldown = "invite:cooldown:"
	keyInviteUser     = "invite:user:"
	keyInviteOwner    = "invite:owner:"
)

// Deps are the collaborators of Service.
type Deps struct {
	Repo     Repo
	Sessions Sessions
	Inviter  Inviter
	Limiter  RateLimiter
	// NewPasswordHash returns the hash of a random password nobody sees; tests
	// replace it to avoid the bcrypt cost.
	NewPasswordHash func() (string, error)
}

// Service implements the owner-only user administration.
type Service struct{ Deps }

// NewService builds a Service.
func NewService(d Deps) *Service {
	if d.NewPasswordHash == nil {
		d.NewPasswordHash = domain.RandomPasswordHash
	}
	return &Service{Deps: d}
}

// requireOwner is the defense in depth behind the HTTP default deny.
func requireOwner(actor Identity) error {
	if actor.UserID == "" || actor.Role != session.RoleOwner {
		return domain.ErrForbidden
	}
	return nil
}

// List returns every account.
func (s *Service) List(ctx context.Context, actor Identity) ([]domain.User, error) {
	if err := requireOwner(actor); err != nil {
		return nil, err
	}
	return s.Repo.List(ctx)
}

// Create stores a new unverified account with a random password hash.
func (s *Service) Create(ctx context.Context, actor Identity, rawEmail string, role session.Role) (domain.User, error) {
	if err := requireOwner(actor); err != nil {
		return domain.User{}, err
	}
	email, err := domain.Validate(rawEmail, role)
	if err != nil {
		return domain.User{}, err
	}
	hash, err := s.NewPasswordHash()
	if err != nil {
		return domain.User{}, err
	}
	return s.Repo.Create(ctx, domain.NewAccount{Email: email, Role: role, PasswordHash: hash})
}

// Invite emails the invitation (a fresh verification token each time, so
// resending is the same call). A verified or deactivated account is refused.
func (s *Service) Invite(ctx context.Context, actor Identity, id string) error {
	if err := requireOwner(actor); err != nil {
		return err
	}
	user, err := s.Repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if !user.Active {
		return domain.ErrInactive
	}
	if user.Verified {
		return domain.ErrAlreadyVerified
	}

	// A request suppressed by one limit must not consume the others: the hourly
	// budgets are peeked first and recorded once the cooldown lets it through.
	userKey, ownerKey := keyInviteUser+user.ID, keyInviteOwner+actor.UserID
	if !s.Limiter.Peek(userKey, InviteUserHourlyLimit, InviteHourWindow) ||
		!s.Limiter.Peek(ownerKey, InviteOwnerHourlyLimit, InviteHourWindow) ||
		!s.Limiter.Allow(keyInviteCooldown+user.ID, 1, InviteUserCooldown) {
		return domain.ErrRateLimited
	}
	s.Limiter.Record(userKey, InviteHourWindow)
	s.Limiter.Record(ownerKey, InviteHourWindow)

	if err := s.Inviter.SendInvitation(ctx, user.ID); err != nil {
		return fmt.Errorf("%w: %v", domain.ErrSendFailed, err)
	}
	return nil
}

// Deactivate blocks new sessions and revokes the refresh tokens of the user.
// The owner cannot deactivate themselves. It is idempotent.
func (s *Service) Deactivate(ctx context.Context, actor Identity, id string) error {
	if err := requireOwner(actor); err != nil {
		return err
	}
	if id == actor.UserID {
		return domain.ErrSelf
	}
	if _, err := s.Repo.Get(ctx, id); err != nil {
		return err
	}
	if err := s.Repo.SetActive(ctx, id, false); err != nil {
		return err
	}
	if err := s.Sessions.RevokeSessions(ctx, id); err != nil {
		return fmt.Errorf("revoke sessions: %w", err)
	}
	return nil
}

// Activate lets a deactivated account sign in again (it must authenticate anew:
// its refresh tokens stay revoked).
func (s *Service) Activate(ctx context.Context, actor Identity, id string) error {
	if err := requireOwner(actor); err != nil {
		return err
	}
	if err := s.Repo.SetActive(ctx, id, true); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return err
		}
		return fmt.Errorf("activate: %w", err)
	}
	return nil
}
