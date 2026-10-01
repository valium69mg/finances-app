package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/valium69mg/finances-app/backend/internal/auth/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/session"
)

func TestNormalizeEmail(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"lowercases and trims", "  Foo@Example.COM ", "foo@example.com", false},
		{"empty", "", "", true},
		{"no at sign", "foo", "", true},
		{"display name", "Foo <foo@example.com>", "", true},
		{"too long", strings.Repeat("a", 250) + "@x.io", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.NormalizeEmail(tt.in)
			if tt.wantErr {
				if !errors.Is(err, domain.ErrInvalidEmail) {
					t.Fatalf("err = %v, want ErrInvalidEmail", err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("got %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name string
		in   string
		ok   bool
	}{
		{"too short", "short", false},
		{"exactly minimum", "123456789012", true},
		{"long enough", "a much longer passphrase", true},
		{"over bcrypt limit", strings.Repeat("a", 73), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := domain.ValidatePassword(tt.in)
			if tt.ok != (err == nil) {
				t.Fatalf("err = %v, ok want %v", err, tt.ok)
			}
			if !tt.ok && !errors.Is(err, domain.ErrWeakPassword) {
				t.Fatalf("err = %v, want ErrWeakPassword", err)
			}
		})
	}
}

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := domain.HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "correct horse battery" {
		t.Fatal("hash must not equal the password")
	}
	if !domain.CheckPassword(hash, "correct horse battery") {
		t.Error("correct password rejected")
	}
	if domain.CheckPassword(hash, "wrong password!!") {
		t.Error("wrong password accepted")
	}
}

func TestGenerateToken(t *testing.T) {
	raw, hash, err := domain.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 43 { // 32 bytes, unpadded base64url
		t.Errorf("raw length = %d, want 43", len(raw))
	}
	if hash != domain.HashToken(raw) || hash == raw {
		t.Error("hash must be HashToken(raw) and differ from raw")
	}
	raw2, _, _ := domain.GenerateToken()
	if raw == raw2 {
		t.Error("tokens must be unique")
	}
}

func TestRefreshTokenExpired(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tok := domain.RefreshToken{ExpiresAt: now}
	tests := []struct {
		name string
		at   time.Time
		want bool
	}{
		{"before", now.Add(-time.Second), false},
		{"at expiry", now, true},
		{"after", now.Add(time.Second), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tok.Expired(tt.at); got != tt.want {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}

func TestAccessToken(t *testing.T) {
	secret := []byte(strings.Repeat("s", 32))
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	tok, err := domain.IssueAccessToken(secret, "user-1", session.RoleHousehold, now)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		secret []byte
		token  string
		at     time.Time
		want   session.Identity
		ok     bool
	}{
		{"valid", secret, tok, now.Add(time.Minute), session.Identity{UserID: "user-1", Role: session.RoleHousehold}, true},
		{"expired", secret, tok, now.Add(domain.AccessTokenTTL), session.Identity{}, false},
		{"wrong secret", []byte(strings.Repeat("x", 32)), tok, now, session.Identity{}, false},
		{"garbage", secret, "not.a.jwt", now, session.Identity{}, false},
		{"empty", secret, "", now, session.Identity{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.ParseAccessToken(tt.secret, tt.token, tt.at)
			if tt.ok != (err == nil) || got != tt.want {
				t.Fatalf("got %+v, %v", got, err)
			}
			if !tt.ok && !errors.Is(err, domain.ErrInvalidToken) {
				t.Fatalf("err = %v, want ErrInvalidToken", err)
			}
		})
	}
}

func TestAccessTokenRole(t *testing.T) {
	secret := []byte(strings.Repeat("s", 32))
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	if _, err := domain.IssueAccessToken(secret, "user-1", "admin", now); err == nil {
		t.Fatal("an unknown role must not be issued")
	}

	// A token signed with a valid secret but without a role claim (issued before
	// roles existed) or with an unknown one is not accepted.
	for name, claims := range map[string]jwt.MapClaims{
		"no role":      {"sub": "user-1", "exp": now.Add(time.Hour).Unix()},
		"unknown role": {"sub": "user-1", "role": "admin", "exp": now.Add(time.Hour).Unix()},
	} {
		t.Run(name, func(t *testing.T) {
			tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := domain.ParseAccessToken(secret, tok, now); !errors.Is(err, domain.ErrInvalidToken) {
				t.Fatalf("err = %v, want ErrInvalidToken", err)
			}
		})
	}
}
