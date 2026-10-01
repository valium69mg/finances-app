package session_test

import (
	"context"
	"testing"

	"github.com/valium69mg/finances-app/backend/internal/platform/session"
)

func TestIdentityRoundTrip(t *testing.T) {
	ctx := session.With(context.Background(), session.Identity{UserID: "u1", Role: session.RoleHousehold})
	id, ok := session.From(ctx)
	if !ok || id.UserID != "u1" || id.Role != session.RoleHousehold {
		t.Fatalf("From = %+v, %v", id, ok)
	}
	if session.UserID(ctx) != "u1" {
		t.Fatalf("UserID = %q", session.UserID(ctx))
	}
}

func TestEmptyContextHasNoIdentity(t *testing.T) {
	if _, ok := session.From(context.Background()); ok {
		t.Fatal("empty context must carry no identity")
	}
	if got := session.UserID(context.Background()); got != "" {
		t.Fatalf("UserID = %q, want empty", got)
	}
}

func TestRoleValid(t *testing.T) {
	for r, want := range map[session.Role]bool{session.RoleOwner: true, session.RoleHousehold: true, "": false, "admin": false} {
		if r.Valid() != want {
			t.Errorf("Role(%q).Valid() = %v, want %v", r, !want, want)
		}
	}
}
