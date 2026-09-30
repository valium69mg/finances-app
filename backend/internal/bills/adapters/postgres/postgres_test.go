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

	"github.com/valium69mg/finances-app/backend/internal/bills/adapters/postgres"
	"github.com/valium69mg/finances-app/backend/internal/bills/app"
	bills "github.com/valium69mg/finances-app/backend/internal/bills/domain"
)

func dec(s string) *decimal.Decimal {
	v := decimal.RequireFromString(s)
	return &v
}

// newRepo returns a Repo on a throwaway schema holding a fresh copy of the
// movements and bills migrations, so the tests never touch real data. It skips
// the test when TEST_DATABASE_URL is not set.
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
	schema := "bills_test_" + hex.EncodeToString(suffix)
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

	for _, name := range []string{"000006_movements.up.sql", "000012_bills.up.sql"} {
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

func input(name, due string, amount *decimal.Decimal) bills.Validated {
	day, _ := bills.AnchorDayOf(due)
	return bills.Validated{
		Name: name, Category: "Servicios", Amount: amount, Currency: "MXN", Recurrence: bills.Monthly,
		NextDueDate: due, AnchorDay: day, ReminderLeadDays: 3, Active: true, Notes: "n",
	}
}

func insertExpense(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var id int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO movements (date, description, category, kind, payment_method, currency, amount, amount_mxn)
		VALUES ('2026-10-05', 'Megacable', 'Servicios', 'Gasto', 'Débito', 'MXN', 550, 550) RETURNING id`).Scan(&id)
	if err != nil {
		t.Fatalf("insert expense: %v", err)
	}
	return id
}

func TestCreateGetRoundTrip(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	created, err := repo.Create(ctx, input("Megacable", "2026-11-01", dec("550")))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "Megacable" || got.Category != "Servicios" || got.Amount == nil || !got.Amount.Equal(*dec("550")) ||
		got.Currency != "MXN" || got.Recurrence != bills.Monthly || got.NextDueDate != "2026-11-01" ||
		got.AnchorDay != 1 || got.ReminderLeadDays != 3 || !got.Active || got.Notes != "n" || got.CreatedAt.IsZero() {
		t.Errorf("bill = %+v", got)
	}
	if got.Pending == nil || got.Pending.DueDate != "2026-11-01" || got.Pending.Status != bills.StatusPending {
		t.Errorf("pending = %+v", got.Pending)
	}
}

func TestVariableBillHasNullAmount(t *testing.T) {
	repo, _ := newRepo(t)
	b, err := repo.Create(context.Background(), input("Luz", "2026-11-05", nil))
	if err != nil {
		t.Fatal(err)
	}
	if b.Amount != nil {
		t.Errorf("amount = %v, want nil", b.Amount)
	}
}

func TestGetUnknown(t *testing.T) {
	repo, _ := newRepo(t)
	if _, err := repo.Get(context.Background(), 999); !errors.Is(err, bills.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestListOrderAndInactiveFilter(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	late, _ := repo.Create(ctx, input("Zeta", "2026-11-20", nil))
	early, _ := repo.Create(ctx, input("beta", "2026-11-02", dec("100")))
	tie, _ := repo.Create(ctx, input("Alfa", "2026-11-02", dec("100")))
	gone, _ := repo.Create(ctx, input("Vieja", "2026-11-01", dec("100")))
	if err := repo.Deactivate(ctx, gone.ID); err != nil {
		t.Fatal(err)
	}

	list, err := repo.List(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int
	for _, b := range list {
		ids = append(ids, b.ID)
		if b.Pending == nil {
			t.Errorf("bill %d has no pending occurrence", b.ID)
		}
	}
	want := []int{tie.ID, early.ID, late.ID}
	if len(ids) != 3 || ids[0] != want[0] || ids[1] != want[1] || ids[2] != want[2] {
		t.Errorf("ids = %v, want %v", ids, want)
	}
	all, _ := repo.List(ctx, true)
	if len(all) != 4 || all[0].ID != gone.ID {
		t.Errorf("with inactive: %d bills, first %d", len(all), all[0].ID)
	}
}

func TestListEmptyIsNotNil(t *testing.T) {
	repo, _ := newRepo(t)
	list, err := repo.List(context.Background(), false)
	if err != nil || list == nil || len(list) != 0 {
		t.Errorf("list = %v, %v", list, err)
	}
}

func TestResolvePaidGeneratesNextAndKeepsHistory(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	b, _ := repo.Create(ctx, input("Megacable", "2026-11-01", dec("550")))
	expense := insertExpense(t, pool)

	got, err := repo.Resolve(ctx, app.Resolution{
		BillID: b.ID, OccurrenceID: b.Pending.ID, Status: bills.StatusPaid,
		PaidOn: "2026-10-30", AmountPaid: *dec("499.50"), Currency: "MXN", ExpenseID: &expense, NextDueDate: "2026-12-01",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.NextDueDate != "2026-12-01" || got.Pending == nil || got.Pending.DueDate != "2026-12-01" || got.Pending.ID == b.Pending.ID {
		t.Errorf("after pay: %+v pending %+v", got, got.Pending)
	}
	history, err := repo.History(ctx, b.ID)
	if err != nil || len(history) != 1 {
		t.Fatalf("history = %v, %v", history, err)
	}
	h := history[0]
	if h.Status != bills.StatusPaid || h.DueDate != "2026-11-01" || h.PaidOn != "2026-10-30" || h.Currency != "MXN" ||
		h.AmountPaid == nil || !h.AmountPaid.Equal(*dec("499.50")) || h.ExpenseID == nil || *h.ExpenseID != expense || h.ResolvedAt == nil {
		t.Errorf("history[0] = %+v", h)
	}
}

func TestResolveSkippedRegistersNothing(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	b, _ := repo.Create(ctx, input("Megacable", "2026-11-01", dec("550")))
	got, err := repo.Resolve(ctx, app.Resolution{BillID: b.ID, OccurrenceID: b.Pending.ID, Status: bills.StatusSkipped, NextDueDate: "2026-12-01"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Pending == nil || got.Pending.DueDate != "2026-12-01" {
		t.Errorf("pending = %+v", got.Pending)
	}
	history, _ := repo.History(ctx, b.ID)
	if len(history) != 1 || history[0].Status != bills.StatusSkipped || history[0].AmountPaid != nil ||
		history[0].ExpenseID != nil || history[0].PaidOn != "" || history[0].ResolvedAt == nil {
		t.Errorf("history = %+v", history)
	}
}

func TestResolveTwiceIsRejectedAndChangesNothing(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	b, _ := repo.Create(ctx, input("Megacable", "2026-11-01", dec("550")))
	res := app.Resolution{BillID: b.ID, OccurrenceID: b.Pending.ID, Status: bills.StatusSkipped, NextDueDate: "2026-12-01"}
	if _, err := repo.Resolve(ctx, res); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Resolve(ctx, res); !errors.Is(err, bills.ErrNotPending) {
		t.Errorf("second resolve err = %v, want ErrNotPending", err)
	}
	var pending int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM bill_occurrences WHERE bill_id = $1 AND status = 'pending'`, b.ID).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 1 {
		t.Errorf("pending occurrences = %d, want 1", pending)
	}
}

func TestResolveUnknownBill(t *testing.T) {
	repo, _ := newRepo(t)
	_, err := repo.Resolve(context.Background(), app.Resolution{BillID: 42, OccurrenceID: 1, Status: bills.StatusSkipped, NextDueDate: "2026-12-01"})
	if !errors.Is(err, bills.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestResolveRollsBackWhenTheNextOccurrenceFails(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	b, _ := repo.Create(ctx, input("Megacable", "2026-11-01", dec("550")))
	// An invalid next date makes the insert of the next occurrence fail after the
	// current one was already closed: the whole transaction must roll back.
	_, err := repo.Resolve(ctx, app.Resolution{BillID: b.ID, OccurrenceID: b.Pending.ID, Status: bills.StatusSkipped, NextDueDate: "not-a-date"})
	if err == nil {
		t.Fatal("want an error")
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM bill_occurrences WHERE id = $1`, b.Pending.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Errorf("status = %s, want pending (rolled back)", status)
	}
}

func TestOnlyOnePendingOccurrencePerBill(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	b, _ := repo.Create(ctx, input("Megacable", "2026-11-01", dec("550")))
	if _, err := pool.Exec(ctx, `INSERT INTO bill_occurrences (bill_id, due_date) VALUES ($1, '2026-12-01')`, b.ID); err == nil {
		t.Error("a second pending occurrence must violate the partial unique index")
	}
	// A resolved occurrence does not count: history can grow freely.
	if _, err := pool.Exec(ctx, `
		INSERT INTO bill_occurrences (bill_id, due_date, status, resolved_at) VALUES ($1, '2026-10-01', 'skipped', now()), ($1, '2026-09-01', 'skipped', now())`, b.ID); err != nil {
		t.Errorf("history inserts: %v", err)
	}
}

func TestConstraintsRejectInconsistentOccurrences(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	b, _ := repo.Create(ctx, input("Megacable", "2026-11-01", dec("550")))
	for name, q := range map[string]string{
		"paid without amount":      `INSERT INTO bill_occurrences (bill_id, due_date, status, paid_on, currency, resolved_at) VALUES ($1, '2026-09-01', 'paid', '2026-09-01', 'MXN', now())`,
		"skipped with an amount":   `INSERT INTO bill_occurrences (bill_id, due_date, status, amount_paid, currency, resolved_at) VALUES ($1, '2026-09-01', 'skipped', 10, 'MXN', now())`,
		"skipped without resolved": `INSERT INTO bill_occurrences (bill_id, due_date, status) VALUES ($1, '2026-09-01', 'skipped')`,
		"unknown status":           `INSERT INTO bill_occurrences (bill_id, due_date, status) VALUES ($1, '2026-09-01', 'late')`,
	} {
		if _, err := pool.Exec(ctx, q, b.ID); err == nil {
			t.Errorf("%s: want a constraint error", name)
		}
	}
	for name, q := range map[string]string{
		"zero amount":       `INSERT INTO bills (name, category, amount, recurrence, next_due_date, anchor_day) VALUES ('x', 'c', 0, 'monthly', '2026-11-01', 1)`,
		"bad recurrence":    `INSERT INTO bills (name, category, recurrence, next_due_date, anchor_day) VALUES ('x', 'c', 'daily', '2026-11-01', 1)`,
		"bad currency":      `INSERT INTO bills (name, category, currency, recurrence, next_due_date, anchor_day) VALUES ('x', 'c', 'EUR', 'monthly', '2026-11-01', 1)`,
		"bad anchor":        `INSERT INTO bills (name, category, recurrence, next_due_date, anchor_day) VALUES ('x', 'c', 'monthly', '2026-11-01', 32)`,
		"negative reminder": `INSERT INTO bills (name, category, recurrence, next_due_date, anchor_day, reminder_lead_days) VALUES ('x', 'c', 'monthly', '2026-11-01', 1, -1)`,
	} {
		if _, err := pool.Exec(ctx, q); err == nil {
			t.Errorf("%s: want a constraint error", name)
		}
	}
}

func TestDeletingTheExpenseKeepsTheHistory(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	b, _ := repo.Create(ctx, input("Megacable", "2026-11-01", dec("550")))
	expense := insertExpense(t, pool)
	if _, err := repo.Resolve(ctx, app.Resolution{
		BillID: b.ID, OccurrenceID: b.Pending.ID, Status: bills.StatusPaid, PaidOn: "2026-10-30",
		AmountPaid: *dec("550"), Currency: "MXN", ExpenseID: &expense, NextDueDate: "2026-12-01",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM movements WHERE id = $1`, expense); err != nil {
		t.Fatal(err)
	}
	history, _ := repo.History(ctx, b.ID)
	if len(history) != 1 || history[0].ExpenseID != nil || history[0].AmountPaid == nil {
		t.Errorf("history = %+v", history)
	}
}

func TestUpdateMovesThePendingOccurrence(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	b, _ := repo.Create(ctx, input("Megacable", "2026-11-01", dec("550")))
	v := input("Megacable HD", "2026-11-15", nil)
	v.Recurrence, v.ReminderLeadDays, v.Notes = bills.Bimonthly, 7, "plan nuevo"
	got, err := repo.Update(ctx, b.ID, v)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Name != "Megacable HD" || got.Amount != nil || got.Recurrence != bills.Bimonthly || got.ReminderLeadDays != 7 ||
		got.NextDueDate != "2026-11-15" || got.AnchorDay != 15 || got.Pending == nil || got.Pending.DueDate != "2026-11-15" || got.Pending.ID != b.Pending.ID {
		t.Errorf("bill = %+v pending %+v", got, got.Pending)
	}
	if _, err := repo.Update(ctx, 999, v); !errors.Is(err, bills.ErrNotFound) {
		t.Errorf("unknown err = %v, want ErrNotFound", err)
	}
}

func TestDeactivateAndReactivate(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	b, _ := repo.Create(ctx, input("Megacable", "2026-11-01", dec("550")))
	for i := 0; i < 2; i++ { // idempotent
		if err := repo.Deactivate(ctx, b.ID); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := repo.Get(ctx, b.ID)
	if got.Active || got.Pending == nil {
		t.Errorf("after deactivate: %+v", got)
	}
	v := input("Megacable", "2026-11-01", dec("550"))
	got, err := repo.Update(ctx, b.ID, v)
	if err != nil || !got.Active {
		t.Errorf("reactivate: %+v, %v", got, err)
	}
	if err := repo.Deactivate(ctx, 999); !errors.Is(err, bills.ErrNotFound) {
		t.Errorf("unknown err = %v, want ErrNotFound", err)
	}
}

func TestImportRefusesANonEmptyTableUnlessForced(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	list := []bills.Validated{input("Agua", "2026-11-01", dec("400")), input("Luz", "2026-11-01", dec("200"))}
	n, err := repo.Import(ctx, list, false)
	if err != nil || n != 2 {
		t.Fatalf("Import = %d, %v", n, err)
	}
	if _, err := repo.Import(ctx, list, false); !errors.Is(err, postgres.ErrNotEmpty) {
		t.Errorf("second import err = %v, want ErrNotEmpty", err)
	}
	if n, err := repo.Import(ctx, list[:1], true); err != nil || n != 1 {
		t.Errorf("forced import = %d, %v", n, err)
	}
	all, _ := repo.List(ctx, true)
	if len(all) != 3 {
		t.Errorf("bills = %d, want 3 (force appends, never deletes)", len(all))
	}
	for _, b := range all {
		if b.Pending == nil {
			t.Errorf("imported bill %d has no pending occurrence", b.ID)
		}
	}
}
