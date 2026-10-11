package postgres_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/valium69mg/finances-app/backend/internal/auth/adapters/postgres"
	"github.com/valium69mg/finances-app/backend/internal/auth/adapters/ratelimit"
	"github.com/valium69mg/finances-app/backend/internal/auth/app"
	"github.com/valium69mg/finances-app/backend/internal/auth/domain"
)

var ctx = context.Background()

// newPool returns a pool on a throwaway schema holding the auth tables, so the
// tests never touch real data. It skips the test when TEST_DATABASE_URL is not set.
func newPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database integration test")
	}
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(admin.Close)

	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	schema := "auth_test_" + hex.EncodeToString(suffix)
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`) })

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect to schema: %v", err)
	}
	t.Cleanup(pool.Close)

	// 000006 is needed because 000017 also adds movements.created_by.
	for _, name := range []string{
		"000002_users.up.sql", "000003_refresh_tokens.up.sql", "000004_verification_tokens.up.sql",
		"000006_movements.up.sql", "000017_users_roles.up.sql", "000025_verification_token_purpose.up.sql",
	} {
		sql, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "migrations", name))
		if err != nil {
			t.Fatalf("read migration: %v", err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	return pool
}

func newUser(t *testing.T, pool *pgxpool.Pool, email string) domain.User {
	t.Helper()
	repo := postgres.NewUserRepo(pool)
	hash, err := domain.HashPassword("an old long password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateIfMissing(ctx, email, hash); err != nil {
		t.Fatal(err)
	}
	u, err := repo.FindByEmail(ctx, email)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SetPasswordAndVerify(ctx, u.ID, hash); err != nil {
		t.Fatal(err)
	}
	return u
}

func token(t *testing.T, repo *postgres.VerificationTokenRepo, user domain.User, purpose domain.TokenPurpose, ttl time.Duration) string {
	t.Helper()
	raw, hash, err := domain.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	err = repo.Create(ctx, domain.VerificationToken{Hash: hash, UserID: user.ID, Purpose: purpose, ExpiresAt: time.Now().Add(ttl)})
	if err != nil {
		t.Fatal(err)
	}
	return domain.HashToken(raw)
}

func TestTokenStoredHashedWithPurpose(t *testing.T) {
	pool := newPool(t)
	repo := postgres.NewVerificationTokenRepo(pool)
	u := newUser(t, pool, "a@example.com")
	hash := token(t, repo, u, domain.PurposeReset, time.Hour)

	var purpose string
	var ttl float64
	err := pool.QueryRow(ctx,
		`SELECT purpose, extract(epoch FROM expires_at - now()) FROM verification_tokens WHERE token_hash = $1`, hash).
		Scan(&purpose, &ttl)
	if err != nil {
		t.Fatal(err)
	}
	if purpose != "reset" || ttl < 3500 || ttl > 3601 {
		t.Errorf("purpose=%q ttl=%.0fs, want reset and about 1 hour", purpose, ttl)
	}
	if len(hash) != 64 {
		t.Errorf("stored hash %q is not a SHA-256 hex digest", hash)
	}
}

func TestEmptyPurposeDefaultsToVerification(t *testing.T) {
	pool := newPool(t)
	repo := postgres.NewVerificationTokenRepo(pool)
	u := newUser(t, pool, "a@example.com")
	hash := token(t, repo, u, "", time.Hour)
	if _, err := repo.Consume(ctx, hash, domain.PurposeReset, time.Now()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("verification token consumed as reset: %v", err)
	}
	if id, err := repo.Consume(ctx, hash, domain.PurposeVerification, time.Now()); err != nil || id != u.ID {
		t.Fatalf("consume = %q, %v", id, err)
	}
}

func TestConsumeSingleUseExpiryAndPurpose(t *testing.T) {
	pool := newPool(t)
	repo := postgres.NewVerificationTokenRepo(pool)
	u := newUser(t, pool, "a@example.com")

	hash := token(t, repo, u, domain.PurposeReset, time.Hour)
	if _, err := repo.Consume(ctx, hash, domain.PurposeVerification, time.Now()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("wrong purpose: %v, want ErrNotFound", err)
	}
	if id, err := repo.Consume(ctx, hash, domain.PurposeReset, time.Now()); err != nil || id != u.ID {
		t.Fatalf("first use = %q, %v", id, err)
	}
	if _, err := repo.Consume(ctx, hash, domain.PurposeReset, time.Now()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("second use: %v, want ErrNotFound", err)
	}

	expired := token(t, repo, u, domain.PurposeReset, -time.Minute)
	if _, err := repo.Consume(ctx, expired, domain.PurposeReset, time.Now()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expired: %v, want ErrNotFound", err)
	}
	if _, err := repo.Consume(ctx, "unknown", domain.PurposeReset, time.Now()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown: %v, want ErrNotFound", err)
	}
}

func TestNewerResetInvalidatesOlderUnusedResets(t *testing.T) {
	pool := newPool(t)
	repo := postgres.NewVerificationTokenRepo(pool)
	a := newUser(t, pool, "a@example.com")
	b := newUser(t, pool, "b@example.com")

	verification := token(t, repo, a, domain.PurposeVerification, time.Hour)
	older := token(t, repo, a, domain.PurposeReset, time.Hour)
	otherUser := token(t, repo, b, domain.PurposeReset, time.Hour)
	newer := token(t, repo, a, domain.PurposeReset, time.Hour)

	if _, err := repo.Consume(ctx, older, domain.PurposeReset, time.Now()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("older reset after a newer request: %v, want ErrNotFound", err)
	}
	for name, h := range map[string]struct {
		hash    string
		purpose domain.TokenPurpose
	}{
		"the newest reset":            {newer, domain.PurposeReset},
		"another user's reset":        {otherUser, domain.PurposeReset},
		"a verification token (kept)": {verification, domain.PurposeVerification},
	} {
		if _, err := repo.Consume(ctx, h.hash, h.purpose, time.Now()); err != nil {
			t.Errorf("%s: %v, want it still valid", name, err)
		}
	}
}

func TestVerificationRequestDoesNotInvalidateReset(t *testing.T) {
	pool := newPool(t)
	repo := postgres.NewVerificationTokenRepo(pool)
	u := newUser(t, pool, "a@example.com")
	reset := token(t, repo, u, domain.PurposeReset, time.Hour)
	token(t, repo, u, domain.PurposeVerification, time.Hour)
	if _, err := repo.Consume(ctx, reset, domain.PurposeReset, time.Now()); err != nil {
		t.Fatalf("reset after a verification request: %v", err)
	}
}

// TestPasswordResetEndToEnd runs the real service over the real repositories: the
// reset sets the new password, keeps the account verified and revokes every
// refresh token, so the existing session cannot refresh any more.
func TestPasswordResetEndToEnd(t *testing.T) {
	pool := newPool(t)
	users := postgres.NewUserRepo(pool)
	clock := func() time.Time { return time.Now() }
	svc := app.NewService(app.Deps{
		Users: users, RefreshTokens: postgres.NewRefreshTokenRepo(pool),
		VerificationTokens: postgres.NewVerificationTokenRepo(pool),
		Mailer:             &capturingMailer{},
		Limiter:            ratelimit.New(clock), JWTSecret: []byte("0123456789abcdef0123456789abcdef"),
		AppBaseURL: "https://app.example.com",
		Spawn:      func(fn func()) { fn() },
	})
	u := newUser(t, pool, "a@example.com")

	sess, err := svc.Login(ctx, "a@example.com", "an old long password", "198.51.100.1")
	if err != nil {
		t.Fatal(err)
	}
	mailer := svc.Mailer.(*capturingMailer)
	if err := svc.RequestPasswordReset(ctx, "a@example.com", "198.51.100.1"); err != nil {
		t.Fatal(err)
	}
	if len(mailer.resetLinks) != 1 {
		t.Fatalf("reset links = %d, want 1", len(mailer.resetLinks))
	}
	raw := mailer.resetLinks[0][len("https://app.example.com/restablecer?token="):]

	if err := svc.CompleteReset(ctx, raw, "short", "198.51.100.1"); !errors.Is(err, domain.ErrWeakPassword) {
		t.Fatalf("weak: %v", err)
	}
	if err := svc.CompleteReset(ctx, raw, "a brand new password", "198.51.100.1"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if err := svc.CompleteReset(ctx, raw, "yet another password", "198.51.100.1"); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("second use: %v", err)
	}

	if _, err := svc.Refresh(ctx, sess.RefreshToken, "198.51.100.1"); !errors.Is(err, domain.ErrInvalidToken) {
		t.Errorf("refresh with the pre-reset session: %v, want ErrInvalidToken", err)
	}
	var live int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM refresh_tokens WHERE user_id = $1::uuid AND revoked_at IS NULL`, u.ID).Scan(&live); err != nil {
		t.Fatal(err)
	}
	if live != 0 {
		t.Errorf("%d live refresh tokens after the reset, want 0", live)
	}
	after, err := users.FindByID(ctx, u.ID)
	if err != nil || !after.Verified {
		t.Fatalf("user after reset = %+v, %v; want verified", after, err)
	}
	if _, err := svc.Login(ctx, "a@example.com", "an old long password", "198.51.100.2"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("old password: %v", err)
	}
	if _, err := svc.Login(ctx, "a@example.com", "a brand new password", "198.51.100.2"); err != nil {
		t.Errorf("new password: %v", err)
	}
}

type capturingMailer struct{ resetLinks []string }

func (m *capturingMailer) SendVerification(context.Context, string, string) error { return nil }
func (m *capturingMailer) SendInvitation(context.Context, string, string) error   { return nil }
func (m *capturingMailer) SendPasswordReset(_ context.Context, _, link string) error {
	m.resetLinks = append(m.resetLinks, link)
	return nil
}
