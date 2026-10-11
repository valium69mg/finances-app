// Package users adapts the users repository to the directory the owner's
// read-only view uses to validate its target user.
package users

import (
	"context"
	"errors"

	"github.com/valium69mg/finances-app/backend/internal/personal/app"
	"github.com/valium69mg/finances-app/backend/internal/personal/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/session"
	usersdomain "github.com/valium69mg/finances-app/backend/internal/users/domain"
)

// Getter is the part of the users repository used here.
type Getter interface {
	Get(ctx context.Context, id string) (usersdomain.User, error)
}

// Adapter looks users up by id.
type Adapter struct{ users Getter }

// New builds an Adapter.
func New(g Getter) *Adapter { return &Adapter{users: g} }

// Role returns the role of the user; an unknown id is domain.ErrNotFound.
func (a *Adapter) Role(ctx context.Context, userID string) (session.Role, error) {
	u, err := a.users.Get(ctx, userID)
	if errors.Is(err, usersdomain.ErrNotFound) {
		return "", domain.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return u.Role, nil
}

var _ app.Directory = (*Adapter)(nil)
