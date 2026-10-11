package users_test

import (
	"context"
	"errors"
	"testing"

	"github.com/valium69mg/finances-app/backend/internal/personal/adapters/users"
	"github.com/valium69mg/finances-app/backend/internal/personal/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/session"
	usersdomain "github.com/valium69mg/finances-app/backend/internal/users/domain"
)

type fakeGetter struct {
	user usersdomain.User
	err  error
}

func (f fakeGetter) Get(context.Context, string) (usersdomain.User, error) { return f.user, f.err }

func TestRole(t *testing.T) {
	ctx := context.Background()
	got, err := users.New(fakeGetter{user: usersdomain.User{Role: session.RoleHousehold}}).Role(ctx, "x")
	if err != nil || got != session.RoleHousehold {
		t.Fatalf("Role = %q, %v", got, err)
	}
	if _, err := users.New(fakeGetter{err: usersdomain.ErrNotFound}).Role(ctx, "x"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown user: %v, want ErrNotFound", err)
	}
	boom := errors.New("boom")
	if _, err := users.New(fakeGetter{err: boom}).Role(ctx, "x"); !errors.Is(err, boom) {
		t.Fatalf("other error: %v", err)
	}
}
