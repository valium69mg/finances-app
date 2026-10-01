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

	authpg "github.com/valium69mg/finances-app/backend/internal/auth/adapters/postgres"
	"github.com/valium69mg/finances-app/backend/internal/auth/adapters/ratelimit"
	authapp "github.com/valium69mg/finances-app/backend/internal/auth/app"
	authdomain "github.com/valium69mg/finances-app/backend/internal/auth/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/session"
	"github.com/valium69mg/finances-app/backend/internal/users/adapters/postgres"
	"github.com/valium69mg/finances-app/backend/internal/users/app"
	"github.com/valium69mg/finances-app/backend/internal/users/domain"
)

var ctx = context.Background()

// newPool returns a pool on a throwaway schema holding the users, refresh and
// verification token tables and the roles migration, so the tests never touch
// real data. It skips the test when TEST_DATABASE_URL is not set.
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
	schema := "users_test_" + hex.EncodeToString(suffix)
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
		"000006_movements.up.sql", "000017_users_roles.up.sql",
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

func account(email string) domain.NewAccount {
	return domain.NewAccount{Email: email, Role: session.RoleHousehold, PasswordHash: "hash"}
}

func TestCreateListAndGet(t *testing.T) {
	repo := postgres.NewRepo(newPool(t))

	u, err := repo.Create(ctx, account("her@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	if u.ID == "" || u.Email != "her@example.com" || u.Role != session.RoleHousehold || !u.Active || u.Verified || u.CreatedAt.IsZero() {
		t.Fatalf("created = %+v; want an unverified, active household account", u)
	}

	got, err := repo.Get(ctx, u.ID)
	if err != nil || got.ID != u.ID {
		t.Fatalf("Get = %+v, %v", got, err)
	}
	if _, err := repo.Get(ctx, "not-a-uuid"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("malformed id: %v, want ErrNotFound", err)
	}
	if _, err := repo.Get(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown id: %v, want ErrNotFound", err)
	}

	if _, err := repo.Create(ctx, account("second@example.com")); err != nil {
		t.Fatal(err)
	}
	list, err := repo.List(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("List = %d, %v", len(list), err)
	}
}

func TestCreateRejectsADuplicateEmail(t *testing.T) {
	repo := postgres.NewRepo(newPool(t))
	if _, err := repo.Create(ctx, account("her@example.com")); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, account("her@example.com")); !errors.Is(err, domain.ErrEmailTaken) {
		t.Fatalf("err = %v, want ErrEmailTaken", err)
	}
}

func TestSetActive(t *testing.T) {
	repo := postgres.NewRepo(newPool(t))
	u, _ := repo.Create(ctx, account("her@example.com"))

	if err := repo.SetActive(ctx, u.ID, false); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.Get(ctx, u.ID); got.Active {
		t.Error("the user must be inactive")
	}
	if err := repo.SetActive(ctx, u.ID, true); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.Get(ctx, u.ID); !got.Active {
		t.Error("the user must be active again")
	}
	for _, id := range []string{"not-a-uuid", "00000000-0000-0000-0000-000000000000"} {
		if err := repo.SetActive(ctx, id, false); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("SetActive(%q) = %v, want ErrNotFound", id, err)
		}
	}
}

// TestDeactivationEndToEnd runs the real services over the real repositories:
// a verified household user logs in, the owner deactivates her, and she can
// neither refresh nor log in; activating her again lets her log in once more.
func TestDeactivationEndToEnd(t *testing.T) {
	pool := newPool(t)
	usersRepo := postgres.NewRepo(pool)
	authUsers := authpg.NewUserRepo(pool)

	clock := func() time.Time { return time.Now() }
	authSvc := authapp.NewService(authapp.Deps{
		Users: authUsers, RefreshTokens: authpg.NewRefreshTokenRepo(pool),
		VerificationTokens: authpg.NewVerificationTokenRepo(pool),
		Limiter:            ratelimit.New(clock), JWTSecret: []byte("0123456789abcdef0123456789abcdef"),
		AppBaseURL: "https://app.example.com",
	})
	svc := app.NewService(app.Deps{
		Repo: usersRepo, Sessions: authSvc, Inviter: authSvc, Limiter: ratelimit.New(clock),
		NewPasswordHash: func() (string, error) { return "hash", nil },
	})

	const password = "a long enough password"
	owner := app.Identity{UserID: "00000000-0000-0000-0000-0000000000aa", Role: session.RoleOwner}
	her, err := svc.Create(ctx, owner, "Her@Example.com", session.RoleHousehold)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := authdomain.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if err := authUsers.SetPasswordAndVerify(ctx, her.ID, hash); err != nil {
		t.Fatal(err)
	}

	sess, err := authSvc.Login(ctx, "her@example.com", password, "198.51.100.1")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if sess.Role != session.RoleHousehold {
		t.Fatalf("role = %q, want household read from the database", sess.Role)
	}
	rotated, err := authSvc.Refresh(ctx, sess.RefreshToken, "198.51.100.1")
	if err != nil {
		t.Fatalf("refresh before deactivation: %v", err)
	}

	if err := svc.Deactivate(ctx, owner, her.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := authSvc.Refresh(ctx, rotated.RefreshToken, "198.51.100.1"); !errors.Is(err, authdomain.ErrInvalidToken) {
		t.Errorf("refresh after deactivation: %v, want ErrInvalidToken", err)
	}
	var live int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM refresh_tokens WHERE user_id = $1::uuid AND revoked_at IS NULL AND used_at IS NULL`, her.ID).Scan(&live); err != nil {
		t.Fatal(err)
	}
	if live != 0 {
		t.Errorf("%d live refresh tokens after deactivation, want 0", live)
	}
	if _, err := authSvc.Login(ctx, "her@example.com", password, "198.51.100.2"); !errors.Is(err, authdomain.ErrInvalidCredentials) {
		t.Errorf("login after deactivation: %v, want ErrInvalidCredentials", err)
	}

	if err := svc.Activate(ctx, owner, her.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := authSvc.Login(ctx, "her@example.com", password, "198.51.100.3"); err != nil {
		t.Errorf("login after reactivation: %v", err)
	}
}

func TestRoleIsReReadOnRefresh(t *testing.T) {
	pool := newPool(t)
	authUsers := authpg.NewUserRepo(pool)
	authSvc := authapp.NewService(authapp.Deps{
		Users: authUsers, RefreshTokens: authpg.NewRefreshTokenRepo(pool),
		VerificationTokens: authpg.NewVerificationTokenRepo(pool),
		Limiter:            ratelimit.New(nil), JWTSecret: []byte("0123456789abcdef0123456789abcdef"),
	})
	hash, _ := authdomain.HashPassword("a long enough password")
	if _, err := authUsers.CreateIfMissing(ctx, "boss@example.com", hash); err != nil {
		t.Fatal(err)
	}
	u, err := authUsers.FindByEmail(ctx, "boss@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if u.Role != session.RoleOwner || !u.Active {
		t.Fatalf("a user created like the admin seed = %q active=%v; want owner, active", u.Role, u.Active)
	}
	if err := authUsers.SetPasswordAndVerify(ctx, u.ID, hash); err != nil {
		t.Fatal(err)
	}

	sess, err := authSvc.Login(ctx, "boss@example.com", "a long enough password", "198.51.100.1")
	if err != nil || sess.Role != session.RoleOwner {
		t.Fatalf("login = %+v, %v", sess, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET role = 'household' WHERE id = $1::uuid`, u.ID); err != nil {
		t.Fatal(err)
	}
	next, err := authSvc.Refresh(ctx, sess.RefreshToken, "198.51.100.1")
	if err != nil {
		t.Fatal(err)
	}
	if next.Role != session.RoleHousehold {
		t.Errorf("role after refresh = %q, want household (re-read from the database)", next.Role)
	}
	if id, err := authSvc.Authenticate(next.AccessToken); err != nil || id.Role != session.RoleHousehold {
		t.Errorf("new access token = %+v, %v", id, err)
	}
}
