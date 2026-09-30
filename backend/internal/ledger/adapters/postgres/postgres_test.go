package postgres_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/ledger/adapters/postgres"
	"github.com/valium69mg/finances-app/backend/internal/ledger/domain"
)

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// newRepo returns a Repo on a throwaway schema holding a fresh copy of the
// movements migration, so the tests never touch real data. It skips the test
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
	schema := "movements_test_" + hex.EncodeToString(suffix)
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

	sql, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "migrations", "000006_movements.up.sql"))
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
	return postgres.NewRepo(pool), pool
}

func expense(date, category, amount string) domain.Movement {
	a := dec(amount)
	return domain.Movement{
		Date: date, Description: "d", Category: category, Kind: domain.KindExpense,
		PaymentMethod: "Débito", Currency: "MXN", Amount: a, AmountMXN: a,
	}
}

func TestCreateGetRoundTrip(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()

	rate := dec("17.7450")
	usd := domain.Movement{
		Date: "2026-10-15", Description: "Claude", Category: "Suscripciones", Kind: domain.KindExpense,
		PaymentMethod: "Crédito", Currency: "USD", Amount: dec("3.33"), ExchangeRate: &rate, AmountMXN: dec("59.09"),
	}
	created, err := repo.Create(ctx, usd)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("expected a generated ID")
	}
	got, err := repo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Date != "2026-10-15" || got.Description != "Claude" || got.Category != "Suscripciones" ||
		got.Kind != domain.KindExpense || got.PaymentMethod != "Crédito" || got.Currency != "USD" ||
		got.Instrument != "" {
		t.Errorf("round trip mismatch: %+v", got)
	}
	if !got.Amount.Equal(dec("3.33")) || !got.AmountMXN.Equal(dec("59.09")) || got.ExchangeRate == nil || !got.ExchangeRate.Equal(rate) {
		t.Errorf("money mismatch: %+v", got)
	}

	mxn, err := repo.Create(ctx, expense("2026-10-16", "Mandado", "120.5"))
	if err != nil {
		t.Fatal(err)
	}
	got, _ = repo.GetByID(ctx, mxn.ID)
	if got.ExchangeRate != nil || !got.Amount.Equal(dec("120.5")) {
		t.Errorf("MXN movement: %+v", got)
	}
}

func TestInstrumentStoredAsNull(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	m, err := repo.Create(ctx, expense("2026-10-01", "Mandado", "1"))
	if err != nil {
		t.Fatal(err)
	}
	var isNull bool
	if err := pool.QueryRow(ctx, `SELECT instrument IS NULL FROM movements WHERE id = $1`, int64(m.ID)).Scan(&isNull); err != nil || !isNull {
		t.Errorf("instrument IS NULL = %v, %v", isNull, err)
	}
	saving := domain.Movement{
		Date: "2026-10-02", Category: "Inversiones", Instrument: "voo", Kind: domain.KindSavings,
		PaymentMethod: "Transferencia", Currency: "MXN", Amount: dec("-500"), AmountMXN: dec("-500"),
	}
	created, err := repo.Create(ctx, saving)
	if err != nil {
		t.Fatalf("savings withdrawal must be allowed: %v", err)
	}
	got, _ := repo.GetByID(ctx, created.ID)
	if got.Instrument != "voo" || !got.Amount.Equal(dec("-500")) {
		t.Errorf("savings: %+v", got)
	}
}

func TestUpdateAndDelete(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	m, _ := repo.Create(ctx, expense("2026-10-01", "Mandado", "100"))

	rate := dec("20")
	m.Category, m.Date, m.Currency, m.Amount, m.ExchangeRate, m.AmountMXN = "Ocio", "2026-09-30", "USD", dec("5"), &rate, dec("100")
	if err := repo.Update(ctx, m); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := repo.GetByID(ctx, m.ID)
	if got.Category != "Ocio" || got.Date != "2026-09-30" || got.Currency != "USD" || got.ExchangeRate == nil || !got.AmountMXN.Equal(dec("100")) {
		t.Errorf("after update: %+v", got)
	}

	missing := m
	missing.ID = 9999
	if err := repo.Update(ctx, missing); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Update missing: %v", err)
	}
	if err := repo.Delete(ctx, m.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.GetByID(ctx, m.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("GetByID after delete: %v", err)
	}
	if err := repo.Delete(ctx, m.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Delete missing: %v", err)
	}
}

func TestListByMonth(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	ids := map[string]int{}
	for _, m := range []domain.Movement{
		expense("2026-09-30", "Mandado", "1"), // previous month
		expense("2026-10-01", "Mandado", "2"), // first day
		expense("2026-10-31", "Mandado", "3"), // last day
		expense("2026-10-31", "Ocio", "4"),    // same date, later id
		expense("2026-11-01", "Mandado", "5"), // next month
		{Date: "2026-10-10", Category: "Sueldo", Kind: domain.KindIncome, PaymentMethod: "Transferencia", Currency: "MXN", Amount: dec("9"), AmountMXN: dec("9")},
	} {
		c, err := repo.Create(ctx, m)
		if err != nil {
			t.Fatal(err)
		}
		ids[m.Amount.String()] = c.ID
	}

	got, err := repo.ListByMonth(ctx, "2026-10", domain.KindExpense, 0)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, m := range got {
		order = append(order, m.Amount.String())
	}
	if want := []string{"4", "3", "2"}; len(order) != 3 || order[0] != want[0] || order[1] != want[1] || order[2] != want[2] {
		t.Errorf("expense order = %v, want %v (date desc, id desc)", order, want)
	}

	got, _ = repo.ListByMonth(ctx, "2026-10", domain.KindExpense, 2)
	if len(got) != 2 || got[0].ID != ids["4"] {
		t.Errorf("limit 2: %+v", got)
	}
	got, _ = repo.ListByMonth(ctx, "2026-10", "", 0)
	if len(got) != 4 {
		t.Errorf("all kinds: got %d, want 4", len(got))
	}
	got, err = repo.ListByMonth(ctx, "2027-01", domain.KindExpense, 0)
	if err != nil || got == nil || len(got) != 0 {
		t.Errorf("empty month: %v, %v (want empty non-nil slice)", got, err)
	}
}

func TestConstraints(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	rate := dec("17")

	zeroExpense := expense("2026-10-01", "Mandado", "0")
	negExpense := expense("2026-10-01", "Mandado", "-1")
	usdNoRate := expense("2026-10-01", "Mandado", "1")
	usdNoRate.Currency = "USD"
	mxnWithRate := expense("2026-10-01", "Mandado", "1")
	mxnWithRate.ExchangeRate = &rate
	badKind := expense("2026-10-01", "Mandado", "1")
	badKind.Kind = "Otro"
	badCurrency := expense("2026-10-01", "Mandado", "1")
	badCurrency.Currency = "EUR"

	for name, m := range map[string]domain.Movement{
		"zero expense": zeroExpense, "negative expense": negExpense, "USD without rate": usdNoRate,
		"MXN with rate": mxnWithRate, "bad kind": badKind, "bad currency": badCurrency,
	} {
		if _, err := repo.Create(ctx, m); err == nil {
			t.Errorf("%s: expected a constraint violation", name)
		}
	}
}
