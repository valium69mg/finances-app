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

	"github.com/valium69mg/finances-app/backend/internal/ledger/adapters/postgres"
	"github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/session"
)

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// newRepo returns a Repo on a throwaway schema holding a fresh copy of the
// movements migrations, so the tests never touch real data. It skips the test
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

	for _, name := range []string{"000002_users.up.sql", "000006_movements.up.sql", "000007_income_amount_positive.up.sql", "000009_movements_transfer_id.up.sql", "000016_future_expenses.up.sql", "000017_users_roles.up.sql"} {
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

func expense(date, category, amount string) domain.Movement {
	a := dec(amount)
	return domain.Movement{
		Date: date, Description: "d", Category: category, Kind: domain.KindExpense,
		PaymentMethod: "Débito", Currency: "MXN", Amount: a, AmountMXN: a,
	}
}

func income(date, amount string) domain.Movement {
	a := dec(amount)
	return domain.Movement{
		Date: date, Description: "d", Category: "Sueldo", Kind: domain.KindIncome,
		PaymentMethod: "Transferencia", Currency: "MXN", Amount: a, AmountMXN: a,
	}
}

func saving(date, category, amount string) domain.Movement {
	a := dec(amount)
	return domain.Movement{
		Date: date, Category: category, Kind: domain.KindSavings, PaymentMethod: "Transferencia",
		Currency: "MXN", Amount: a, AmountMXN: a,
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

func TestUpdateIsGuardedByKind(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	exp, _ := repo.Create(ctx, expense("2026-10-01", "Mandado", "100"))

	// A savings-shaped update aimed at an expense row must not touch it.
	hijack := saving("2026-10-02", "Inversiones", "5")
	hijack.ID = exp.ID
	if err := repo.Update(ctx, hijack); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Update with another kind: err = %v, want ErrNotFound", err)
	}
	got, _ := repo.GetByID(ctx, exp.ID)
	if got.Kind != domain.KindExpense || got.Category != "Mandado" || !got.Amount.Equal(dec("100")) {
		t.Errorf("expense was overwritten: %+v", got)
	}

	// The same kind still updates.
	sav, _ := repo.Create(ctx, saving("2026-10-01", "Inversiones", "10"))
	sav.Amount, sav.AmountMXN = dec("20"), dec("20")
	if err := repo.Update(ctx, sav); err != nil {
		t.Fatalf("Update same kind: %v", err)
	}
}

func TestListByRange(t *testing.T) {
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

	got, err := repo.ListByRange(ctx, "2026-10-01", "2026-10-31", domain.KindExpense, 0)
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

	got, _ = repo.ListByRange(ctx, "2026-10-01", "2026-10-31", domain.KindExpense, 2)
	if len(got) != 2 || got[0].ID != ids["4"] {
		t.Errorf("limit 2: %+v", got)
	}
	got, _ = repo.ListByRange(ctx, "2026-10-01", "2026-10-31", "", 0)
	if len(got) != 4 {
		t.Errorf("all kinds: got %d, want 4", len(got))
	}
	// A cycle range crossing the month boundary: both ends are inclusive.
	got, err = repo.ListByRange(ctx, "2026-09-30", "2026-10-30", domain.KindExpense, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != ids["2"] || got[1].ID != ids["1"] {
		t.Errorf("cycle range 2026-09-30..2026-10-30: got %+v, want ids %d, %d", got, ids["2"], ids["1"])
	}
	got, err = repo.ListByRange(ctx, "2027-01-01", "2027-01-31", domain.KindExpense, 0)
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
	zeroIncome := income("2026-10-01", "0")
	negIncome := income("2026-10-01", "-1")

	for name, m := range map[string]domain.Movement{
		"zero expense": zeroExpense, "negative expense": negExpense, "USD without rate": usdNoRate,
		"MXN with rate": mxnWithRate, "bad kind": badKind, "bad currency": badCurrency,
		"zero income": zeroIncome, "negative income": negIncome,
	} {
		if _, err := repo.Create(ctx, m); err == nil {
			t.Errorf("%s: expected a constraint violation", name)
		}
	}
}

func TestListAllByKind(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	for _, m := range []domain.Movement{
		saving("2026-10-05", "Fondo de emergencia", "300"),
		saving("2025-01-01", "Fondo de emergencia", "100"),
		saving("2026-10-05", "Fondo de emergencia", "-50"), // same date, later id
		expense("2026-10-01", "Mandado", "9"),
		income("2026-10-01", "9"),
	} {
		if _, err := repo.Create(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	got, err := repo.ListAllByKind(ctx, domain.KindSavings)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, m := range got {
		order = append(order, m.Amount.String())
	}
	if want := []string{"100", "300", "-50"}; len(order) != 3 || order[0] != want[0] || order[1] != want[1] || order[2] != want[2] {
		t.Errorf("savings order = %v, want %v (all months, date asc, id asc)", order, want)
	}
	got, err = repo.ListAllByKind(ctx, domain.KindIncome)
	if err != nil || len(got) != 1 {
		t.Errorf("income: %v, %v", got, err)
	}
	got, err = repo.ListAllByKind(ctx, domain.KindExpense)
	if err != nil || len(got) != 1 {
		t.Errorf("expense: %v, %v", got, err)
	}
}

func TestCreateMany(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()

	out, err := repo.CreateMany(ctx, []domain.Movement{saving("2026-10-01", "Inversiones", "-100"), saving("2026-10-01", "Inversiones", "100")})
	if err != nil || len(out) != 2 || out[0].ID == 0 || out[1].ID <= out[0].ID || !out[0].Amount.Equal(dec("-100")) {
		t.Fatalf("CreateMany: %+v, %v", out, err)
	}

	// A failing row rolls the whole batch back.
	bad := income("2026-10-02", "-5") // incomes must be positive (CHECK)
	if _, err := repo.CreateMany(ctx, []domain.Movement{saving("2026-10-02", "Inversiones", "50"), bad}); err == nil {
		t.Fatal("expected the batch to fail")
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM movements`).Scan(&count); err != nil || count != 2 {
		t.Errorf("rows = %d, %v, want 2 (failed batch must roll back)", count, err)
	}
}

const transferA = "6f1c1a0e-8f5e-4a55-9d0a-3c1f0b2a7e11"

func TestTransferIDRoundTrip(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()

	plain, err := repo.Create(ctx, saving("2026-10-01", "Inversiones", "5"))
	if err != nil || plain.TransferID != "" {
		t.Fatalf("plain movement: %+v, %v", plain, err)
	}
	var isNull bool
	if err := pool.QueryRow(ctx, `SELECT transfer_id IS NULL FROM movements WHERE id = $1`, int64(plain.ID)).Scan(&isNull); err != nil || !isNull {
		t.Errorf("transfer_id IS NULL = %v, %v", isNull, err)
	}

	out, in := saving("2026-10-02", "Inversiones", "-10"), saving("2026-10-02", "Inversiones", "10")
	out.TransferID, in.TransferID = transferA, transferA
	saved, err := repo.CreateMany(ctx, []domain.Movement{out, in})
	if err != nil || saved[0].TransferID != transferA || saved[1].TransferID != transferA {
		t.Fatalf("CreateMany: %+v, %v", saved, err)
	}
	got, _ := repo.GetByID(ctx, saved[1].ID)
	if got.TransferID != transferA {
		t.Errorf("GetByID transfer id = %q", got.TransferID)
	}

	// Updating a leg leaves its link alone.
	saved[0].Description = "edited"
	if err := repo.Update(ctx, saved[0]); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.GetByID(ctx, saved[0].ID); got.TransferID != transferA || got.Description != "edited" {
		t.Errorf("after update: %+v", got)
	}

	// A malformed id is rejected by the database, not stored.
	bad := saving("2026-10-03", "Inversiones", "1")
	bad.TransferID = "not-a-uuid"
	if _, err := repo.Create(ctx, bad); err == nil {
		t.Error("expected an invalid uuid error")
	}
}

func TestDeleteByTransfer(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	out, in := saving("2026-10-02", "Inversiones", "-10"), saving("2026-10-02", "Inversiones", "10")
	out.TransferID, in.TransferID = transferA, transferA
	saved, err := repo.CreateMany(ctx, []domain.Movement{out, in})
	if err != nil {
		t.Fatal(err)
	}
	other, _ := repo.Create(ctx, saving("2026-10-02", "Inversiones", "7"))

	if err := repo.DeleteByTransfer(ctx, transferA); err != nil {
		t.Fatalf("DeleteByTransfer: %v", err)
	}
	for _, m := range saved {
		if _, err := repo.GetByID(ctx, m.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("leg %d survived: %v", m.ID, err)
		}
	}
	if _, err := repo.GetByID(ctx, other.ID); err != nil {
		t.Errorf("unrelated movement removed: %v", err)
	}
	if err := repo.DeleteByTransfer(ctx, transferA); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("second delete: %v, want ErrNotFound", err)
	}
}

func TestTransferIDMigrationDown(t *testing.T) {
	_, pool := newRepo(t)
	ctx := context.Background()
	sql, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "migrations", "000009_movements_transfer_id.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("down: %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'movements' AND column_name = 'transfer_id'`).Scan(&n); err != nil || n != 0 {
		t.Errorf("transfer_id columns after down = %d, %v", n, err)
	}
}

func TestImportMovements(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	rate := dec("17.74")
	created := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
	row := postgres.ImportedMovement{
		Movement: domain.Movement{
			Date: "2026-10-01", Description: "Sueldo octubre", Category: "Sueldo", Kind: domain.KindIncome,
			PaymentMethod: "Transferencia", Currency: "USD", Amount: dec("3383.33"), ExchangeRate: &rate, AmountMXN: dec("60020.27"),
		},
		CreatedAt: created,
	}

	n, err := repo.ImportMovements(ctx, []postgres.ImportedMovement{row}, false)
	if err != nil || n != 1 {
		t.Fatalf("import: %d, %v", n, err)
	}
	var got time.Time
	var instrumentNull bool
	if err := pool.QueryRow(ctx, `SELECT created_at, instrument IS NULL FROM movements`).Scan(&got, &instrumentNull); err != nil {
		t.Fatal(err)
	}
	if !got.Equal(created) || !instrumentNull {
		t.Errorf("created_at = %s, instrument null = %v", got, instrumentNull)
	}

	// A second run refuses and changes nothing, unless forced (which appends).
	if _, err := repo.ImportMovements(ctx, []postgres.ImportedMovement{row}, false); !errors.Is(err, postgres.ErrNotEmpty) {
		t.Errorf("second import: err = %v, want ErrNotEmpty", err)
	}
	if n, err := repo.ImportMovements(ctx, []postgres.ImportedMovement{row}, true); err != nil || n != 1 {
		t.Errorf("forced import: %d, %v", n, err)
	}
	var count int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM movements`).Scan(&count)
	if count != 2 {
		t.Errorf("rows = %d, want 2", count)
	}

	// One bad row rolls the whole import back.
	bad := row
	bad.Movement.Amount = dec("-1")
	if _, err := repo.ImportMovements(ctx, []postgres.ImportedMovement{row, bad}, true); err == nil {
		t.Error("expected a constraint violation")
	}
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM movements`).Scan(&count)
	if count != 2 {
		t.Errorf("rows after failed import = %d, want 2 (rolled back)", count)
	}
}

func TestFutureExpenseLinkRoundTrip(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	var itemID int
	if err := pool.QueryRow(ctx, `INSERT INTO future_expenses (name, target_amount, due_date) VALUES ('Laptop', 100, '2027-01-01') RETURNING id`).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	linked := saving("2026-10-01", "Gastos futuros", "25")
	linked.FutureExpenseID = itemID
	created, err := repo.Create(ctx, linked)
	if err != nil || created.FutureExpenseID != itemID {
		t.Fatalf("Create = %+v, %v, want the link returned", created, err)
	}
	got, _ := repo.GetByID(ctx, created.ID)
	if got.FutureExpenseID != itemID {
		t.Errorf("GetByID link = %d, want %d", got.FutureExpenseID, itemID)
	}
	plain, _ := repo.Create(ctx, saving("2026-10-01", "Gastos futuros", "5"))
	if g, _ := repo.GetByID(ctx, plain.ID); g.FutureExpenseID != 0 {
		t.Errorf("unlinked saving has link %d", g.FutureExpenseID)
	}
	all, _ := repo.ListAllByKind(ctx, domain.KindSavings)
	if len(all) != 2 || all[0].FutureExpenseID != itemID || all[1].FutureExpenseID != 0 {
		t.Errorf("ListAllByKind links = %+v", all)
	}
	// Only savings carry a link.
	bad := expense("2026-10-01", "Mandado", "1")
	bad.FutureExpenseID = itemID
	if _, err := repo.Create(ctx, bad); err == nil {
		t.Error("an expense was linked to a future expense")
	}
	// An update never changes the link.
	created.Description = "edited"
	created.FutureExpenseID = 0
	if err := repo.Update(ctx, created); err != nil {
		t.Fatal(err)
	}
	if g, _ := repo.GetByID(ctx, created.ID); g.FutureExpenseID != itemID || g.Description != "edited" {
		t.Errorf("after update: %+v", g)
	}
}

func createdBy(t *testing.T, pool *pgxpool.Pool, id int) *string {
	t.Helper()
	var by *string
	if err := pool.QueryRow(context.Background(), `SELECT created_by::text FROM movements WHERE id = $1`, int64(id)).Scan(&by); err != nil {
		t.Fatalf("read created_by: %v", err)
	}
	return by
}

func TestCreateRecordsTheActingUser(t *testing.T) {
	repo, pool := newRepo(t)
	bg := context.Background()

	var userID string
	if err := pool.QueryRow(bg, `INSERT INTO users (email, password_hash, role) VALUES ('her@example.com', 'x', 'household') RETURNING id::text`).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	acting := session.With(bg, session.Identity{UserID: userID, Role: session.RoleOwner})
	one, err := repo.Create(acting, expense("2026-10-02", "Comida", "10.00"))
	if err != nil {
		t.Fatal(err)
	}
	if by := createdBy(t, pool, one.ID); by == nil || *by != userID {
		t.Errorf("created_by = %v, want %s", by, userID)
	}

	many, err := repo.CreateMany(acting, []domain.Movement{expense("2026-10-03", "Comida", "5.00"), income("2026-10-03", "20.00")})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range many {
		if by := createdBy(t, pool, m.ID); by == nil || *by != userID {
			t.Errorf("CreateMany created_by = %v, want %s", by, userID)
		}
	}

	// No identity (imports, background jobs) leaves the author empty.
	anon, err := repo.Create(bg, expense("2026-10-04", "Comida", "1.00"))
	if err != nil {
		t.Fatal(err)
	}
	if by := createdBy(t, pool, anon.ID); by != nil {
		t.Errorf("created_by = %v, want NULL without an acting user", *by)
	}

	// Deleting the user keeps the movement and clears the author.
	if _, err := pool.Exec(bg, `DELETE FROM users WHERE id = $1::uuid`, userID); err != nil {
		t.Fatal(err)
	}
	if by := createdBy(t, pool, one.ID); by != nil {
		t.Errorf("created_by = %v after the user was deleted, want NULL", *by)
	}
}

func TestUsersRoleMigrationDefaults(t *testing.T) {
	_, pool := newRepo(t)
	bg := context.Background()
	var role string
	var active bool
	if err := pool.QueryRow(bg, `INSERT INTO users (email, password_hash) VALUES ('owner@example.com', 'x') RETURNING role, active`).Scan(&role, &active); err != nil {
		t.Fatal(err)
	}
	if role != "owner" || !active {
		t.Errorf("defaults = %q, %v; want owner, true so the existing admin stays owner", role, active)
	}
	if _, err := pool.Exec(bg, `INSERT INTO users (email, password_hash, role) VALUES ('x@example.com', 'x', 'admin')`); err == nil {
		t.Error("an unknown role must violate the CHECK constraint")
	}
}
