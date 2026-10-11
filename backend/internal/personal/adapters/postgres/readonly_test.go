package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	personalusers "github.com/valium69mg/finances-app/backend/internal/personal/adapters/users"
	"github.com/valium69mg/finances-app/backend/internal/personal/app"
	"github.com/valium69mg/finances-app/backend/internal/personal/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/session"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
	userspg "github.com/valium69mg/finances-app/backend/internal/users/adapters/postgres"
)

type noSettings struct{}

func (noSettings) Get(context.Context) (settings.Config, error) { return settings.Config{}, nil }

// The owner's read-only view against the real tables: the target is validated
// through the users table and the read never seeds.
func TestOwnerReadOnlyViewOnTheRealTables(t *testing.T) {
	repo, pool := newRepo(t)
	her := addUser(t, pool, "her@example.com")
	var owner string
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, verified, role, active) VALUES ('owner@example.com', 'x', true, 'owner', true) RETURNING id::text`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	now := func() time.Time { return time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC) }
	svc := app.NewService(repo, noSettings{}, now).WithUsers(personalusers.New(userspg.NewRepo(pool)))
	asOwner := session.Identity{UserID: owner, Role: session.RoleOwner}
	asHer := session.Identity{UserID: her, Role: session.RoleHousehold}

	cats, err := svc.Categories(ctx, asOwner, her, "")
	if err != nil || len(cats) != 0 {
		t.Fatalf("owner read before any use = %v, %v; want no categories", cats, err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM personal_categories WHERE user_id = $1::uuid`, her).Scan(&n); err != nil || n != 0 {
		t.Fatalf("categories stored after an owner read = %d, %v; a read-only view never writes", n, err)
	}

	if got, err := svc.Categories(ctx, asHer, "", ""); err != nil || len(got) == 0 {
		t.Fatalf("her own read must seed: %v, %v", got, err)
	}
	if got, err := svc.Categories(ctx, asOwner, her, "Gasto"); err != nil || len(got) == 0 {
		t.Fatalf("owner read after her first use = %v, %v", got, err)
	}

	if _, err := svc.Wishlist(ctx, asOwner, owner); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("owner targeting an owner id: %v; want ErrNotFound", err)
	}
	if _, err := svc.Wishlist(ctx, asOwner, "99999999-9999-4999-8999-999999999999"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("owner targeting an unknown id: %v; want ErrNotFound", err)
	}

	// Inactive household users stay readable.
	if _, err := pool.Exec(ctx, `UPDATE users SET active = false WHERE id = $1::uuid`, her); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Wishlist(ctx, asOwner, her); err != nil {
		t.Errorf("owner reading an inactive household user: %v", err)
	}
}
