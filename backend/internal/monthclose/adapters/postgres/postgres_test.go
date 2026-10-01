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
	"github.com/shopspring/decimal"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	"github.com/valium69mg/finances-app/backend/internal/monthclose/adapters/postgres"
	monthclose "github.com/valium69mg/finances-app/backend/internal/monthclose/domain"
	savings "github.com/valium69mg/finances-app/backend/internal/savings/domain"
	taxfiling "github.com/valium69mg/finances-app/backend/internal/taxfiling/domain"
)

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }
func ptr(s string) *decimal.Decimal {
	v := dec(s)
	return &v
}

// newRepo returns a Repo on a throwaway schema holding a fresh copy of the
// movements and month closes migrations, so the tests never touch real data. It
// skips the test when TEST_DATABASE_URL is not set.
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
	schema := "monthclose_test_" + hex.EncodeToString(suffix)
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

	for _, name := range []string{"000002_users.up.sql", "000006_movements.up.sql", "000013_month_closes.up.sql", "000017_users_roles.up.sql"} {
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

func sampleClose(period string) monthclose.Close {
	return monthclose.Close{
		Period:   period,
		ClosedAt: time.Date(2026, 11, 2, 15, 4, 5, 0, time.UTC),
		Categories: []monthclose.Category{
			{Name: "Vivienda", Budget: ptr("3600"), Spent: dec("3700.50"), Remaining: ptr("-100.50"), OverBudget: true},
			{Name: "Ocio", Spent: dec("200")},
		},
		Income:       dec("50000.25"),
		Expenses:     dec("3900.5"),
		Savings:      dec("-300"),
		Available:    dec("46399.75"),
		Emergency:    savings.EmergencyStatus{Accumulated: dec("10000"), Goal: dec("120000.125")},
		Suggestion:   &monthclose.Suggestion{ToEmergencyFund: dec("40000"), ToInvestments: dec("0"), ToFutureExpenses: dec("6399.75"), InvestmentsPaused: true},
		Adjustments:  []monthclose.Adjustment{{Name: "Vivienda", Kind: ledger.KindExpense, Budget: dec("3600"), Real: dec("4500"), DeviationPct: dec("25")}},
		FilingStatus: taxfiling.PaymentPending,
	}
}

func TestCreateAndGetRoundTrip(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	want := sampleClose("2026-10")

	saved, err := repo.Create(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx, "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []monthclose.Close{saved, got} {
		if c.Period != "2026-10" || !c.ClosedAt.Equal(want.ClosedAt) || c.FilingStatus != taxfiling.PaymentPending {
			t.Errorf("header = %+v", c)
		}
		for name, pair := range map[string][2]decimal.Decimal{
			"income": {c.Income, want.Income}, "expenses": {c.Expenses, want.Expenses}, "savings": {c.Savings, want.Savings},
			"available": {c.Available, want.Available}, "accumulated": {c.Emergency.Accumulated, want.Emergency.Accumulated},
			"goal": {c.Emergency.Goal, want.Emergency.Goal},
		} {
			if !pair[0].Equal(pair[1]) {
				t.Errorf("%s = %s, want %s", name, pair[0], pair[1])
			}
		}
		if len(c.Categories) != 2 || c.Categories[0].Name != "Vivienda" || !c.Categories[0].OverBudget ||
			c.Categories[0].Budget == nil || !c.Categories[0].Budget.Equal(dec("3600")) ||
			c.Categories[0].Remaining == nil || !c.Categories[0].Remaining.Equal(dec("-100.5")) ||
			!c.Categories[0].Spent.Equal(dec("3700.5")) {
			t.Errorf("categories = %+v", c.Categories)
		}
		if c.Categories[1].Budget != nil || c.Categories[1].Remaining != nil || c.Categories[1].OverBudget {
			t.Errorf("category without budget = %+v", c.Categories[1])
		}
		s := c.Suggestion
		if s == nil || !s.ToEmergencyFund.Equal(dec("40000")) || !s.ToFutureExpenses.Equal(dec("6399.75")) || !s.InvestmentsPaused {
			t.Errorf("suggestion = %+v", s)
		}
		if len(c.Adjustments) != 1 || c.Adjustments[0].Kind != ledger.KindExpense || !c.Adjustments[0].DeviationPct.Equal(dec("25")) {
			t.Errorf("adjustments = %+v", c.Adjustments)
		}
	}
}

func TestCreateWithoutSuggestionOrFilingStatus(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	c := sampleClose("2026-09")
	c.Suggestion, c.FilingStatus, c.Adjustments = nil, "", nil

	if _, err := repo.Create(ctx, c); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx, "2026-09")
	if err != nil {
		t.Fatal(err)
	}
	if got.Suggestion != nil || got.FilingStatus != "" || got.Adjustments == nil || len(got.Adjustments) != 0 {
		t.Errorf("got = %+v", got)
	}
}

func TestCreateDefaultsClosedAtToNow(t *testing.T) {
	repo, _ := newRepo(t)
	c := sampleClose("2026-08")
	c.ClosedAt = time.Time{}
	got, err := repo.Create(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(got.ClosedAt) > time.Minute || got.ClosedAt.After(time.Now().Add(time.Minute)) {
		t.Errorf("closed_at = %v", got.ClosedAt)
	}
}

func TestCreateDuplicatePeriod(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	if _, err := repo.Create(ctx, sampleClose("2026-10")); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, sampleClose("2026-10")); !errors.Is(err, monthclose.ErrAlreadyClosed) {
		t.Errorf("err = %v, want ErrAlreadyClosed", err)
	}
}

func TestPeriodConstraint(t *testing.T) {
	repo, _ := newRepo(t)
	if _, err := repo.Create(context.Background(), sampleClose("2026-13")); err == nil {
		t.Error("an invalid period should be rejected by the CHECK")
	}
}

func TestGetAndDeleteNotFound(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	if _, err := repo.Get(ctx, "2026-10"); !errors.Is(err, monthclose.ErrNotFound) {
		t.Errorf("Get err = %v, want ErrNotFound", err)
	}
	if err := repo.Delete(ctx, "2026-10"); !errors.Is(err, monthclose.ErrNotFound) {
		t.Errorf("Delete err = %v, want ErrNotFound", err)
	}
}

func TestListNewestFirstAndDelete(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	if list, err := repo.List(ctx); err != nil || list == nil || len(list) != 0 {
		t.Fatalf("empty list = %v, %v", list, err)
	}
	for _, p := range []string{"2026-08", "2026-10", "2026-09"} {
		if _, err := repo.Create(ctx, sampleClose(p)); err != nil {
			t.Fatal(err)
		}
	}
	list, err := repo.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 || list[0].Period != "2026-10" || list[1].Period != "2026-09" || list[2].Period != "2026-08" {
		t.Errorf("list = %+v", list)
	}

	// Deleting a close never touches the movements.
	if _, err := pool.Exec(ctx, `INSERT INTO movements (date, kind, category, description, payment_method, currency, amount, amount_mxn)
		VALUES ('2026-10-02', 'Gasto', 'Vivienda', 'x', 'Débito', 'MXN', 10, 10)`); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(ctx, "2026-10"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Get(ctx, "2026-10"); !errors.Is(err, monthclose.ErrNotFound) {
		t.Errorf("after delete err = %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM movements`).Scan(&n); err != nil || n != 1 {
		t.Errorf("movements = %d, %v; want 1", n, err)
	}
	// The period can be closed again after a delete.
	if _, err := repo.Create(ctx, sampleClose("2026-10")); err != nil {
		t.Errorf("recreate: %v", err)
	}
}
