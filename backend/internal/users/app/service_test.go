package app_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/valium69mg/finances-app/backend/internal/auth/adapters/ratelimit"
	"github.com/valium69mg/finances-app/backend/internal/platform/session"
	"github.com/valium69mg/finances-app/backend/internal/users/app"
	"github.com/valium69mg/finances-app/backend/internal/users/domain"
)

var (
	ctx   = context.Background()
	t0    = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	owner = app.Identity{UserID: "owner-1", Role: session.RoleOwner}
	her   = app.Identity{UserID: "u-her", Role: session.RoleHousehold}
)

type fakeRepo struct {
	users []domain.User
	next  int
}

func (f *fakeRepo) List(context.Context) ([]domain.User, error) { return f.users, nil }

func (f *fakeRepo) Get(_ context.Context, id string) (domain.User, error) {
	for _, u := range f.users {
		if u.ID == id {
			return u, nil
		}
	}
	return domain.User{}, domain.ErrNotFound
}

func (f *fakeRepo) Create(_ context.Context, acc domain.NewAccount) (domain.User, error) {
	for _, u := range f.users {
		if u.Email == acc.Email {
			return domain.User{}, domain.ErrEmailTaken
		}
	}
	f.next++
	u := domain.User{ID: fmt.Sprintf("new-%d", f.next), Email: acc.Email, Role: acc.Role, Active: true}
	f.users = append(f.users, u)
	return u, nil
}

func (f *fakeRepo) SetActive(_ context.Context, id string, active bool) error {
	for i := range f.users {
		if f.users[i].ID == id {
			f.users[i].Active = active
			return nil
		}
	}
	return domain.ErrNotFound
}

type fakeSessions struct{ revoked []string }

func (f *fakeSessions) RevokeSessions(_ context.Context, id string) error {
	f.revoked = append(f.revoked, id)
	return nil
}

type fakeInviter struct {
	sent []string
	err  error
}

func (f *fakeInviter) SendInvitation(_ context.Context, id string) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, id)
	return nil
}

type env struct {
	svc      *app.Service
	repo     *fakeRepo
	sessions *fakeSessions
	inviter  *fakeInviter
	now      time.Time
}

func newEnv() *env {
	e := &env{
		repo: &fakeRepo{users: []domain.User{
			{ID: "owner-1", Email: "owner@example.com", Role: session.RoleOwner, Active: true, Verified: true},
			{ID: "u-her", Email: "her@example.com", Role: session.RoleHousehold, Active: true},
			{ID: "u-done", Email: "done@example.com", Role: session.RoleHousehold, Active: true, Verified: true},
			{ID: "u-off", Email: "off@example.com", Role: session.RoleHousehold, Active: false},
		}},
		sessions: &fakeSessions{},
		inviter:  &fakeInviter{},
		now:      t0,
	}
	e.svc = app.NewService(app.Deps{
		Repo: e.repo, Sessions: e.sessions, Inviter: e.inviter,
		Limiter:         ratelimit.New(func() time.Time { return e.now }),
		NewPasswordHash: func() (string, error) { return "hash", nil },
	})
	return e
}

func TestCreate(t *testing.T) {
	tests := []struct {
		name    string
		actor   app.Identity
		email   string
		role    session.Role
		wantErr error
	}{
		{"owner creates a household account", owner, "New@Example.com", session.RoleHousehold, nil},
		{"invalid email", owner, "nope", session.RoleHousehold, domain.ErrInvalidEmail},
		{"v1 creates household only", owner, "x@example.com", session.RoleOwner, domain.ErrInvalidRole},
		{"duplicate email", owner, "HER@example.com", session.RoleHousehold, domain.ErrEmailTaken},
		{"household cannot create", her, "x@example.com", session.RoleHousehold, domain.ErrForbidden},
		{"no identity", app.Identity{}, "x@example.com", session.RoleHousehold, domain.ErrForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv()
			u, err := e.svc.Create(ctx, tt.actor, tt.email, tt.role)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err == nil && (u.Email != "new@example.com" || u.Verified || !u.Active || u.Role != session.RoleHousehold) {
				t.Errorf("created %+v: want a normalized, unverified, active household account", u)
			}
		})
	}
}

func TestInvite(t *testing.T) {
	tests := []struct {
		name     string
		actor    app.Identity
		id       string
		wantErr  error
		wantSent bool
	}{
		{"unverified user", owner, "u-her", nil, true},
		{"unknown user", owner, "ghost", domain.ErrNotFound, false},
		{"verified user has nothing to accept", owner, "u-done", domain.ErrAlreadyVerified, false},
		{"deactivated user", owner, "u-off", domain.ErrInactive, false},
		{"household is forbidden", her, "u-her", domain.ErrForbidden, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv()
			err := e.svc.Invite(ctx, tt.actor, tt.id)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if sent := len(e.inviter.sent) == 1; sent != tt.wantSent {
				t.Fatalf("sent = %v, want %v", sent, tt.wantSent)
			}
		})
	}
}

func TestInviteSendFailure(t *testing.T) {
	e := newEnv()
	e.inviter.err = errors.New("resend down")
	if err := e.svc.Invite(ctx, owner, "u-her"); !errors.Is(err, domain.ErrSendFailed) {
		t.Fatalf("err = %v, want ErrSendFailed", err)
	}
}

func TestInviteCooldownAndResend(t *testing.T) {
	e := newEnv()
	if err := e.svc.Invite(ctx, owner, "u-her"); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Invite(ctx, owner, "u-her"); !errors.Is(err, domain.ErrRateLimited) {
		t.Fatalf("immediate resend: %v, want ErrRateLimited", err)
	}
	e.now = t0.Add(app.InviteUserCooldown)
	if err := e.svc.Invite(ctx, owner, "u-her"); err != nil {
		t.Fatalf("resend after the cooldown: %v", err)
	}
	if len(e.inviter.sent) != 2 {
		t.Fatalf("sent = %d, want 2 (a resend issues a new token)", len(e.inviter.sent))
	}
}

func TestInviteHourlyLimitPerUser(t *testing.T) {
	e := newEnv()
	for i := range app.InviteUserHourlyLimit {
		e.now = t0.Add(time.Duration(i) * app.InviteUserCooldown)
		if err := e.svc.Invite(ctx, owner, "u-her"); err != nil {
			t.Fatalf("invite %d: %v", i+1, err)
		}
	}
	e.now = t0.Add(time.Duration(app.InviteUserHourlyLimit) * app.InviteUserCooldown)
	if err := e.svc.Invite(ctx, owner, "u-her"); !errors.Is(err, domain.ErrRateLimited) {
		t.Fatalf("over the hourly cap: %v, want ErrRateLimited", err)
	}
	e.now = t0.Add(app.InviteHourWindow)
	if err := e.svc.Invite(ctx, owner, "u-her"); err != nil {
		t.Fatalf("window must reset: %v", err)
	}
}

func TestInviteHourlyLimitPerOwner(t *testing.T) {
	e := newEnv()
	for i := range app.InviteOwnerHourlyLimit {
		id := fmt.Sprintf("bulk-%d", i)
		e.repo.users = append(e.repo.users, domain.User{ID: id, Email: id + "@example.com", Role: session.RoleHousehold, Active: true})
		if err := e.svc.Invite(ctx, owner, id); err != nil {
			t.Fatalf("invite %d: %v", i+1, err)
		}
	}
	if err := e.svc.Invite(ctx, owner, "u-her"); !errors.Is(err, domain.ErrRateLimited) {
		t.Fatalf("over the owner cap: %v, want ErrRateLimited", err)
	}
	if len(e.inviter.sent) != app.InviteOwnerHourlyLimit {
		t.Fatalf("sent = %d, want %d", len(e.inviter.sent), app.InviteOwnerHourlyLimit)
	}
}

func TestDeactivate(t *testing.T) {
	e := newEnv()
	if err := e.svc.Deactivate(ctx, owner, "u-her"); err != nil {
		t.Fatal(err)
	}
	u, _ := e.repo.Get(ctx, "u-her")
	if u.Active {
		t.Error("the user must be inactive")
	}
	if len(e.sessions.revoked) != 1 || e.sessions.revoked[0] != "u-her" {
		t.Errorf("revoked = %v, want the refresh tokens of u-her", e.sessions.revoked)
	}
}

func TestDeactivateGuards(t *testing.T) {
	tests := []struct {
		name    string
		actor   app.Identity
		id      string
		wantErr error
	}{
		{"owner cannot deactivate themselves", owner, "owner-1", domain.ErrSelf},
		{"unknown user", owner, "ghost", domain.ErrNotFound},
		{"household is forbidden", her, "u-done", domain.ErrForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv()
			if err := e.svc.Deactivate(ctx, tt.actor, tt.id); !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if len(e.sessions.revoked) != 0 {
				t.Error("nothing may be revoked when the call is refused")
			}
			if u, _ := e.repo.Get(ctx, tt.id); tt.id != "ghost" && !u.Active {
				t.Error("a refused call must not change the user")
			}
		})
	}
}

func TestActivate(t *testing.T) {
	e := newEnv()
	if err := e.svc.Activate(ctx, owner, "u-off"); err != nil {
		t.Fatal(err)
	}
	if u, _ := e.repo.Get(ctx, "u-off"); !u.Active {
		t.Error("the user must be active again")
	}
	if err := e.svc.Activate(ctx, owner, "ghost"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown: %v", err)
	}
	if err := e.svc.Activate(ctx, her, "u-off"); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("household: %v", err)
	}
}

func TestListIsOwnerOnly(t *testing.T) {
	e := newEnv()
	if us, err := e.svc.List(ctx, owner); err != nil || len(us) != 4 {
		t.Fatalf("owner: %d, %v", len(us), err)
	}
	if _, err := e.svc.List(ctx, her); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("household: %v, want ErrForbidden", err)
	}
}
