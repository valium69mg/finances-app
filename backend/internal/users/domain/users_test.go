package domain_test

import (
	"errors"
	"testing"

	authdomain "github.com/valium69mg/finances-app/backend/internal/auth/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/session"
	"github.com/valium69mg/finances-app/backend/internal/users/domain"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		email   string
		role    session.Role
		want    string
		wantErr error
	}{
		{"household", "  Her@Example.COM ", session.RoleHousehold, "her@example.com", nil},
		{"bad email", "not-an-email", session.RoleHousehold, "", domain.ErrInvalidEmail},
		{"display name form", "Ana <ana@example.com>", session.RoleHousehold, "", domain.ErrInvalidEmail},
		{"empty email", "", session.RoleHousehold, "", domain.ErrInvalidEmail},
		{"owner cannot be created", "her@example.com", session.RoleOwner, "", domain.ErrInvalidRole},
		{"unknown role", "her@example.com", "admin", "", domain.ErrInvalidRole},
		{"empty role", "her@example.com", "", "", domain.ErrInvalidRole},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.Validate(tt.email, tt.role)
			if !errors.Is(err, tt.wantErr) || got != tt.want {
				t.Fatalf("got %q, %v; want %q, %v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestRandomPasswordHashIsUniqueAndNeverGuessable(t *testing.T) {
	a, err := domain.RandomPasswordHash()
	if err != nil {
		t.Fatal(err)
	}
	b, err := domain.RandomPasswordHash()
	if err != nil {
		t.Fatal(err)
	}
	if a == b || a == "" {
		t.Fatal("each account needs its own random hash")
	}
	for _, guess := range []string{"", "password", "correct horse battery"} {
		if authdomain.CheckPassword(a, guess) {
			t.Fatalf("the random password must not match %q", guess)
		}
	}
}
