package postgres_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	"github.com/valium69mg/finances-app/backend/internal/savings/adapters/postgres"
)

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// newRepo returns a Repo on a throwaway schema holding a fresh copy of the
// valuations migration, so the tests never touch real data. It skips the test
// when TEST_DATABASE_URL is not set.
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
	schema := "valuations_test_" + hex.EncodeToString(suffix)
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

	sql, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "migrations", "000008_valuations.up.sql"))
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
	return postgres.NewRepo(pool), pool
}

func val(date, instrument, value string) ledger.Valuation {
	return ledger.Valuation{Date: date, Instrument: instrument, ValueMXN: dec(value)}
}

func TestAddListRoundTrip(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()

	got, err := repo.List(ctx)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty list: %v, %v (want empty non-nil slice)", got, err)
	}

	if err := repo.Add(ctx, val("2026-10-15", "voo", "12345.67")); err != nil {
		t.Fatalf("Add: %v", err)
	}
	got, err = repo.List(ctx)
	if err != nil || len(got) != 1 {
		t.Fatalf("List: %v, %v", got, err)
	}
	if got[0].Date != "2026-10-15" || got[0].Instrument != "voo" || !got[0].ValueMXN.Equal(dec("12345.67")) {
		t.Errorf("round trip mismatch: %+v", got[0])
	}
}

func TestListOrder(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	for _, v := range []ledger.Valuation{
		val("2026-10-05", "voo", "300"),
		val("2025-01-01", "voo", "100"),
		val("2026-10-05", "voo", "350"), // same date, appended later
		val("2026-09-01", "cetes", "50"),
	} {
		if err := repo.Add(ctx, v); err != nil {
			t.Fatal(err)
		}
	}
	got, err := repo.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"100", "50", "300", "350"}
	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].ValueMXN.String() != w {
			t.Errorf("order[%d] = %s, want %s (date asc, id asc)", i, got[i].ValueMXN, w)
		}
	}
}

func TestConstraints(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	for name, v := range map[string]ledger.Valuation{
		"zero value":     val("2026-10-01", "voo", "0"),
		"negative value": val("2026-10-01", "voo", "-1"),
		"bad date":       val("not-a-date", "voo", "1"),
	} {
		if err := repo.Add(ctx, v); err == nil {
			t.Errorf("%s: expected a constraint violation", name)
		}
	}
}

func TestValuationsAreAppendOnly(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	if err := repo.Add(ctx, val("2026-10-01", "voo", "100")); err != nil {
		t.Fatal(err)
	}
	// The same instrument and date can be valued again; both rows are kept.
	if err := repo.Add(ctx, val("2026-10-01", "voo", "110")); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM valuations`).Scan(&count); err != nil || count != 2 {
		t.Errorf("rows = %d, %v, want 2", count, err)
	}
}
