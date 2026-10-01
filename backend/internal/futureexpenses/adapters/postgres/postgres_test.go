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

	"github.com/valium69mg/finances-app/backend/internal/futureexpenses/adapters/postgres"
	"github.com/valium69mg/finances-app/backend/internal/futureexpenses/app"
	domain "github.com/valium69mg/finances-app/backend/internal/futureexpenses/domain"
	ledgerpg "github.com/valium69mg/finances-app/backend/internal/ledger/adapters/postgres"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// newRepos returns the repository and a ledger repository on a throwaway schema
// holding a fresh copy of the movements and future expenses migrations, so the
// tests never touch real data. It skips the test when TEST_DATABASE_URL is not set.
func newRepos(t *testing.T) (*postgres.Repo, *ledgerpg.Repo, *pgxpool.Pool) {
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
	schema := "future_test_" + hex.EncodeToString(suffix)
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

	for _, name := range []string{"000002_users.up.sql", "000006_movements.up.sql", "000009_movements_transfer_id.up.sql", "000016_future_expenses.up.sql", "000017_users_roles.up.sql"} {
		sql, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "migrations", name))
		if err != nil {
			t.Fatalf("read migration: %v", err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	return postgres.NewRepo(pool), ledgerpg.NewRepo(pool), pool
}

func validated(name, due, target string) domain.Validated {
	return domain.Validated{Name: name, Target: d(target), DueDate: due}
}

func saving(category, amount string, itemID int) ledger.Movement {
	a := d(amount)
	return ledger.Movement{
		Date: "2026-10-01", Description: "s", Category: category, Kind: ledger.KindSavings, Instrument: "cetes-28",
		PaymentMethod: "Transferencia", Currency: "MXN", Amount: a, AmountMXN: a, FutureExpenseID: itemID,
	}
}

func expense(amount string) ledger.Movement {
	a := d(amount)
	return ledger.Movement{
		Date: "2026-10-05", Description: "Laptop", Category: "Otros", Kind: ledger.KindExpense,
		PaymentMethod: "Débito", Currency: "MXN", Amount: a, AmountMXN: a,
	}
}

func mustCreate(t *testing.T, r *postgres.Repo, name, due, target string) domain.FutureExpense {
	t.Helper()
	it, err := r.Create(context.Background(), validated(name, due, target))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return it
}

func countMovements(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM movements`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCreateGetUpdateListDelete(t *testing.T) {
	repo, _, _ := newRepos(t)
	ctx := context.Background()

	it := mustCreate(t, repo, "Laptop", "2027-01-20", "8000.50")
	if it.ID == 0 || it.Name != "Laptop" || !it.Target.Equal(d("8000.50")) || it.DueDate != "2027-01-20" || it.Status != domain.StatusActive ||
		!it.Saved.IsZero() || it.PaidAt != "" || it.AmountPaid != nil || it.ExpenseMovementID != nil || it.CreatedAt.IsZero() {
		t.Fatalf("created = %+v", it)
	}
	later := mustCreate(t, repo, "Viaje", "2026-12-15", "24000")

	got, err := repo.Update(ctx, it.ID, validated("Laptop nueva", "2027-02-01", "9000"))
	if err != nil || got.Name != "Laptop nueva" || !got.Target.Equal(d("9000")) || got.DueDate != "2027-02-01" || !got.UpdatedAt.After(it.CreatedAt.Add(-1)) {
		t.Errorf("Update = %+v, %v", got, err)
	}
	if _, err := repo.Update(ctx, 9999, validated("x", "2027-01-01", "1")); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Update unknown: err = %v", err)
	}

	list, err := repo.List(ctx)
	if err != nil || len(list) != 2 || list[0].ID != later.ID || list[1].ID != it.ID {
		t.Errorf("List = %+v, %v, want ordered by due date", list, err)
	}
	if _, err := repo.Get(ctx, 9999); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Get unknown: err = %v", err)
	}
	if err := repo.Delete(ctx, later.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(ctx, later.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("second Delete: err = %v", err)
	}
}

func TestTableConstraints(t *testing.T) {
	_, _, pool := newRepos(t)
	ctx := context.Background()
	bad := []string{
		`INSERT INTO future_expenses (name, target_amount, due_date) VALUES ('', 1, '2027-01-01')`,
		`INSERT INTO future_expenses (name, target_amount, due_date) VALUES ('a', 0, '2027-01-01')`,
		`INSERT INTO future_expenses (name, target_amount, due_date, status) VALUES ('a', 1, '2027-01-01', 'other')`,
		`INSERT INTO future_expenses (name, target_amount, due_date, status) VALUES ('a', 1, '2027-01-01', 'paid')`,
		`INSERT INTO future_expenses (name, target_amount, due_date, paid_at) VALUES ('a', 1, '2027-01-01', '2027-01-02')`,
	}
	for _, q := range bad {
		if _, err := pool.Exec(ctx, q); err == nil {
			t.Errorf("accepted: %s", q)
		}
	}
}

func TestLinkColumnRoundTripAndSaved(t *testing.T) {
	repo, movements, pool := newRepos(t)
	ctx := context.Background()
	a := mustCreate(t, repo, "Laptop", "2027-01-20", "8000")
	b := mustCreate(t, repo, "Viaje", "2026-12-15", "24000")

	linked, err := movements.Create(ctx, saving("Gastos futuros", "300.25", a.ID))
	if err != nil {
		t.Fatalf("Create linked: %v", err)
	}
	got, err := movements.GetByID(ctx, linked.ID)
	if err != nil || got.FutureExpenseID != a.ID {
		t.Fatalf("round trip = %+v, %v, want the link", got, err)
	}
	plain, _ := movements.Create(ctx, saving("Gastos futuros", "50", 0))
	if g, _ := movements.GetByID(ctx, plain.ID); g.FutureExpenseID != 0 {
		t.Errorf("unlinked saving got link %d", g.FutureExpenseID)
	}
	if _, err := movements.CreateMany(ctx, []ledger.Movement{saving("Gastos futuros", "-100", a.ID), saving("Gastos futuros", "40", b.ID)}); err != nil {
		t.Fatal(err)
	}
	// Updating through the ledger repository keeps the link.
	linked.Description = "edited"
	if err := movements.Update(ctx, linked); err != nil {
		t.Fatal(err)
	}
	if g, _ := movements.GetByID(ctx, linked.ID); g.FutureExpenseID != a.ID {
		t.Errorf("the link was lost on update: %d", g.FutureExpenseID)
	}

	gotA, _ := repo.Get(ctx, a.ID)
	gotB, _ := repo.Get(ctx, b.ID)
	if !gotA.Saved.Equal(d("200.25")) || !gotB.Saved.Equal(d("40")) {
		t.Errorf("saved a %s b %s, want 200.25 and 40: the net of each item's own rows", gotA.Saved, gotB.Saved)
	}
	list, _ := repo.List(ctx)
	if len(list) != 2 || !list[0].Saved.Equal(d("40")) || !list[1].Saved.Equal(d("200.25")) {
		t.Errorf("List saved = %+v", list)
	}

	// Only savings can be linked, and only to an existing item.
	if _, err := pool.Exec(ctx, `INSERT INTO movements (date, category, kind, payment_method, currency, amount, amount_mxn, future_expense_id)
		VALUES ('2026-10-01', 'Otros', 'Gasto', 'Débito', 'MXN', 1, 1, $1)`, int64(a.ID)); err == nil {
		t.Error("an expense was linked to a future expense")
	}
	if _, err := movements.Create(ctx, saving("Gastos futuros", "1", 99999)); err == nil {
		t.Error("a saving was linked to an item that does not exist")
	}
}

func TestFreeBalance(t *testing.T) {
	repo, movements, _ := newRepos(t)
	ctx := context.Background()
	item := mustCreate(t, repo, "Laptop", "2027-01-20", "8000")
	if free, err := repo.FreeBalance(ctx); err != nil || !free.IsZero() {
		t.Fatalf("empty free balance = %s, %v", free, err)
	}
	for _, m := range []ledger.Movement{
		saving("Gastos futuros", "1000", 0),
		saving("Gastos futuros", "-150.5", 0),
		saving("Gastos futuros", "400", item.ID), // linked: not free
		saving("Inversiones", "9999", 0),         // another category
	} {
		if _, err := movements.Create(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	income := expense("77")
	income.Category = "Gastos futuros" // not a saving: never counts
	if _, err := movements.Create(ctx, income); err != nil {
		t.Fatal(err)
	}
	free, err := repo.FreeBalance(ctx)
	if err != nil || !free.Equal(d("849.5")) {
		t.Errorf("free = %s, %v, want 849.5", free, err)
	}
}

func TestDeleteUnlinksSavings(t *testing.T) {
	repo, movements, pool := newRepos(t)
	ctx := context.Background()
	item := mustCreate(t, repo, "Laptop", "2027-01-20", "8000")
	m, _ := movements.Create(ctx, saving("Gastos futuros", "250", item.ID))
	if err := repo.Delete(ctx, item.ID); err != nil {
		t.Fatal(err)
	}
	got, err := movements.GetByID(ctx, m.ID)
	if err != nil || got.FutureExpenseID != 0 || !got.AmountMXN.Equal(d("250")) {
		t.Errorf("saving after delete = %+v, %v, want it kept and unlinked", got, err)
	}
	if free, _ := repo.FreeBalance(ctx); !free.Equal(d("250")) {
		t.Errorf("free = %s, want 250: the money returns to the free balance", free)
	}
	if n := countMovements(t, pool); n != 1 {
		t.Errorf("%d movements, want 1", n)
	}
}

// settle builds the settlement of the tests: the expense plus the release rows
// domain.Release computes for the saved amount it is given.
func settle(itemID int, paid string) func(saved decimal.Decimal) (app.Settlement, error) {
	return func(saved decimal.Decimal) (app.Settlement, error) {
		st := app.Settlement{Expense: expense(paid)}
		linked, remainder := domain.Release(saved, d(paid))
		if linked.IsPositive() {
			st.Savings = append(st.Savings, saving("Gastos futuros", linked.Neg().String(), itemID))
		}
		if remainder.IsPositive() {
			st.Savings = append(st.Savings, saving("Gastos futuros", remainder.String(), 0))
		}
		return st, nil
	}
}

func TestMarkPaidIsAtomicAndReleasesWhatWasSaved(t *testing.T) {
	repo, movements, pool := newRepos(t)
	ctx := context.Background()
	item := mustCreate(t, repo, "Laptop", "2027-01-20", "8000")
	for _, amount := range []string{"500", "100"} {
		if _, err := movements.Create(ctx, saving("Gastos futuros", amount, item.ID)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := movements.Create(ctx, saving("Gastos futuros", "70", 0)); err != nil { // pre-existing free money
		t.Fatal(err)
	}

	paid, err := repo.MarkPaid(ctx, item.ID, settle(item.ID, "450"))
	if err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if paid.Status != domain.StatusPaid || paid.PaidAt != "2026-10-05" || paid.AmountPaid == nil || !paid.AmountPaid.Equal(d("450")) ||
		paid.ExpenseMovementID == nil || !paid.Saved.IsZero() {
		t.Fatalf("paid item = %+v", paid)
	}
	exp, err := movements.GetByID(ctx, *paid.ExpenseMovementID)
	if err != nil || exp.Kind != ledger.KindExpense || !exp.AmountMXN.Equal(d("450")) || exp.FutureExpenseID != 0 {
		t.Errorf("expense = %+v, %v", exp, err)
	}
	// 600 saved, 450 paid: 150 goes back to the free balance (70 + 150).
	if free, _ := repo.FreeBalance(ctx); !free.Equal(d("220")) {
		t.Errorf("free = %s, want 220", free)
	}
	// The savings portfolio lost exactly what was spent: 670 - 450.
	all, _ := movements.ListAllByKind(ctx, ledger.KindSavings)
	if total := ledger.SumBy(all, ledger.Filter{Kind: ledger.KindSavings}); !total.Equal(d("220")) {
		t.Errorf("net savings = %s, want 220", total)
	}
	if n := countMovements(t, pool); n != 3+1+2 {
		t.Errorf("%d movements, want 3 savings + 1 expense + 2 release rows", n)
	}

	if _, err := repo.MarkPaid(ctx, item.ID, settle(item.ID, "450")); !errors.Is(err, domain.ErrAlreadyPaid) {
		t.Errorf("second MarkPaid: err = %v, want ErrAlreadyPaid", err)
	}
	if _, err := repo.Update(ctx, item.ID, validated("x", "2027-01-01", "1")); !errors.Is(err, domain.ErrAlreadyPaid) {
		t.Errorf("Update paid: err = %v, want ErrAlreadyPaid", err)
	}
	if n := countMovements(t, pool); n != 6 {
		t.Errorf("a rejected payment wrote rows: %d movements", n)
	}
	if _, err := repo.MarkPaid(ctx, 99999, settle(1, "1")); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown item: err = %v", err)
	}
}

func TestMarkPaidWithLessSavedThanPaid(t *testing.T) {
	repo, movements, _ := newRepos(t)
	ctx := context.Background()
	item := mustCreate(t, repo, "Laptop", "2027-01-20", "8000")
	if _, err := movements.Create(ctx, saving("Gastos futuros", "300", item.ID)); err != nil {
		t.Fatal(err)
	}
	paid, err := repo.MarkPaid(ctx, item.ID, settle(item.ID, "400"))
	if err != nil {
		t.Fatal(err)
	}
	all, _ := movements.ListAllByKind(ctx, ledger.KindSavings)
	if total := ledger.SumBy(all, ledger.Filter{Kind: ledger.KindSavings}); !total.IsZero() || !paid.Saved.IsZero() {
		t.Errorf("net savings %s saved %s, want both 0: only what existed was released", total, paid.Saved)
	}
	if free, _ := repo.FreeBalance(ctx); !free.IsZero() {
		t.Errorf("free = %s, want 0", free)
	}
}

func TestMarkPaidRollsBackEverythingOnFailure(t *testing.T) {
	repo, movements, pool := newRepos(t)
	ctx := context.Background()
	item := mustCreate(t, repo, "Laptop", "2027-01-20", "8000")
	if _, err := movements.Create(ctx, saving("Gastos futuros", "300", item.ID)); err != nil {
		t.Fatal(err)
	}
	before := countMovements(t, pool)

	assertUntouched := func(label string) {
		t.Helper()
		got, err := repo.Get(ctx, item.ID)
		if err != nil || got.Status != domain.StatusActive || got.PaidAt != "" || got.AmountPaid != nil || got.ExpenseMovementID != nil || !got.Saved.Equal(d("300")) {
			t.Errorf("%s: item = %+v, %v, want it untouched", label, got, err)
		}
		if n := countMovements(t, pool); n != before {
			t.Errorf("%s: %d movements, want %d: the expense must not survive", label, n, before)
		}
	}

	boom := errors.New("boom")
	if _, err := repo.MarkPaid(ctx, item.ID, func(decimal.Decimal) (app.Settlement, error) { return app.Settlement{}, boom }); !errors.Is(err, boom) {
		t.Errorf("err = %v, want the planner error", err)
	}
	assertUntouched("planner error")

	// The expense inserts fine, then the release row is rejected by the database
	// (unknown currency): the expense must roll back with it.
	_, err := repo.MarkPaid(ctx, item.ID, func(decimal.Decimal) (app.Settlement, error) {
		release := saving("Gastos futuros", "-300", item.ID)
		release.Currency = "XXX"
		return app.Settlement{Expense: expense("400"), Savings: []ledger.Movement{release}}, nil
	})
	if err == nil {
		t.Fatal("want the database error")
	}
	assertUntouched("release row rejected")

	// A settlement row that is not a saving is refused before it is written.
	_, err = repo.MarkPaid(ctx, item.ID, func(decimal.Decimal) (app.Settlement, error) {
		return app.Settlement{Expense: expense("400"), Savings: []ledger.Movement{expense("1")}}, nil
	})
	if err == nil {
		t.Fatal("want an error for a release row that is not a saving")
	}
	assertUntouched("release row is not a saving")
}

func TestMarkPaidSeesSavingsCommittedBeforeTheLock(t *testing.T) {
	repo, movements, _ := newRepos(t)
	ctx := context.Background()
	item := mustCreate(t, repo, "Laptop", "2027-01-20", "8000")
	var seen decimal.Decimal
	_, err := repo.MarkPaid(ctx, item.ID, func(saved decimal.Decimal) (app.Settlement, error) {
		seen = saved
		return settle(item.ID, "10")(saved)
	})
	if err != nil || !seen.IsZero() {
		t.Fatalf("err = %v, saved seen = %s", err, seen)
	}
	other := mustCreate(t, repo, "Viaje", "2026-12-15", "100")
	_, _ = movements.Create(ctx, saving("Gastos futuros", "60", other.ID))
	_, err = repo.MarkPaid(ctx, other.ID, func(saved decimal.Decimal) (app.Settlement, error) {
		seen = saved
		return settle(other.ID, "10")(saved)
	})
	if err != nil || !seen.Equal(d("60")) {
		t.Errorf("err = %v, saved seen = %s, want the amount read inside the transaction (60)", err, seen)
	}
}
