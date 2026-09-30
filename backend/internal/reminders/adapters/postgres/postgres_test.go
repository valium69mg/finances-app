package postgres_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/valium69mg/finances-app/backend/internal/reminders/adapters/postgres"
	"github.com/valium69mg/finances-app/backend/internal/reminders/domain"
)

// newRepo returns a Repo on a throwaway schema holding a fresh copy of the users
// and reminder log migrations, so the tests never touch real data. It skips the
// test when TEST_DATABASE_URL is not set.
func newRepo(t *testing.T) (*postgres.Repo, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database integration test")
	}
	ctx := context.Background()

	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(admin.Close)

	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	schema := "reminders_test_" + hex.EncodeToString(suffix)
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

	for _, name := range []string{"000002_users.up.sql", "000015_reminder_log.up.sql"} {
		sql, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "migrations", name))
		if err != nil {
			t.Fatalf("read migration: %v", err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	return postgres.NewRepo(pool), pool
}

func TestRecordAndHas(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()

	if ok, err := repo.Has(ctx, "bill_due:1"); err != nil || ok {
		t.Fatalf("Has before Record = %v, %v", ok, err)
	}
	if err := repo.Record(ctx, "bill_due:1"); err != nil {
		t.Fatal(err)
	}
	if ok, err := repo.Has(ctx, "bill_due:1"); err != nil || !ok {
		t.Fatalf("Has after Record = %v, %v", ok, err)
	}
	if ok, _ := repo.Has(ctx, "bill_due:2"); ok {
		t.Error("an unrelated key is reported as sent")
	}
}

func TestRecordIsIdempotentAndRaceSafe(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- repo.Record(ctx, "weekly:2026-10-05")
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Record failed: %v", err)
		}
	}

	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM reminder_log WHERE key = 'weekly:2026-10-05'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("rows = %d, %v; want exactly 1", n, err)
	}
}

func TestLastSent(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()

	if last, err := repo.LastSent(ctx, domain.DiskKeyPrefix); err != nil || last != nil {
		t.Fatalf("LastSent on an empty log = %v, %v", last, err)
	}

	older := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	for key, at := range map[string]time.Time{
		"disk:2026-09-01": older, "disk:2026-09-20": newer, "weekly:2026-09-28": time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC),
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO reminder_log (key, sent_at) VALUES ($1, $2)`, key, at); err != nil {
			t.Fatal(err)
		}
	}

	last, err := repo.LastSent(ctx, domain.DiskKeyPrefix)
	if err != nil || last == nil {
		t.Fatalf("LastSent = %v, %v", last, err)
	}
	if !last.Equal(newer) {
		t.Errorf("LastSent = %v, want the newest disk alert %v (other prefixes must not count)", last, newer)
	}
}

func TestOwnerEmail(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()

	if _, err := repo.OwnerEmail(ctx); !errors.Is(err, domain.ErrNoOwner) {
		t.Fatalf("no users: err = %v, want ErrNoOwner", err)
	}

	insert := func(email string, verified bool, created time.Time) {
		t.Helper()
		if _, err := pool.Exec(ctx,
			`INSERT INTO users (email, password_hash, verified, created_at) VALUES ($1, 'x', $2, $3)`, email, verified, created); err != nil {
			t.Fatal(err)
		}
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	insert("unverified@example.com", false, base)
	if _, err := repo.OwnerEmail(ctx); !errors.Is(err, domain.ErrNoOwner) {
		t.Fatalf("only an unverified user: err = %v, want ErrNoOwner", err)
	}

	insert("second@example.com", true, base.Add(48*time.Hour))
	insert("first@example.com", true, base.Add(24*time.Hour))
	got, err := repo.OwnerEmail(ctx)
	if err != nil || got != "first@example.com" {
		t.Fatalf("OwnerEmail = %q, %v; want the first verified user", got, err)
	}
}
