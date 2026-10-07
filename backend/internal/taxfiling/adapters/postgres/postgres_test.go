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

	"github.com/valium69mg/finances-app/backend/internal/taxfiling/adapters/postgres"
	taxfiling "github.com/valium69mg/finances-app/backend/internal/taxfiling/domain"
)

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// newRepo returns a Repo on a throwaway schema holding a fresh copy of the
// movements, invoices and tax filings migrations, so the tests never touch real
// data. It skips the test when TEST_DATABASE_URL is not set.
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
	schema := "taxfiling_test_" + hex.EncodeToString(suffix)
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

	for _, name := range []string{"000002_users.up.sql", "000006_movements.up.sql", "000010_invoices.up.sql", "000011_tax_filings.up.sql", "000017_users_roles.up.sql", "000020_tax_filing_documents.up.sql"} {
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

// insertInvoice inserts an invoice of the period in the given state and returns its ID.
func insertInvoice(t *testing.T, pool *pgxpool.Pool, period, status, uuid string) int {
	t.Helper()
	var id int
	var uuidArg any
	if uuid != "" {
		uuidArg = uuid
	}
	err := pool.QueryRow(context.Background(), `
		INSERT INTO invoices (client_id, collection_date, period, currency, subtotal, subtotal_mxn, total, expected_deposit_mxn, status, uuid)
		VALUES ('b', ($1 || '-15')::date, $1, 'MXN', 100, 100, 116, 116, $2, $3) RETURNING id`, period, status, uuidArg).Scan(&id)
	if err != nil {
		t.Fatalf("insert invoice: %v", err)
	}
	return id
}

const (
	uuidA = "6F1C2B3A-4D5E-4F60-8A7B-9C0D1E2F3A4B"
	uuidB = "11111111-2222-3333-4444-555555555555"
)

func filing(period string, ids ...int) taxfiling.Filing {
	return taxfiling.Filing{
		Period: period, FilingDate: "2026-11-05", IncomeCollected: dec("92262.41"), ISRRate: dec("0.02"),
		ISRAccrued: dec("1845.25"), ISRWithheld: dec("0"), ISRDue: dec("1845.25"),
		IVATransferred: dec("4827.59"), IVAWithheld: dec("0"), IVACreditable: dec("0"), IVADue: dec("4827.59"),
		Folio: "ACUSE-1", InvoiceIDs: ids,
	}
}

func TestCreateGetRoundTripAndInvoiceLink(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	issued1 := insertInvoice(t, pool, "2026-10", "emitida", uuidA)
	issued2 := insertInvoice(t, pool, "2026-10", "emitida", uuidB)
	prepared := insertInvoice(t, pool, "2026-10", "preparada", "")
	cancelled := insertInvoice(t, pool, "2026-10", "cancelada", "")
	otherPeriod := insertInvoice(t, pool, "2026-11", "emitida", "22222222-2222-3333-4444-555555555555")

	saved, err := repo.Create(ctx, filing("2026-10", issued1, issued2))
	if err != nil {
		t.Fatal(err)
	}
	if saved.CreatedAt.IsZero() || len(saved.InvoiceIDs) != 2 || saved.PaymentStatus() != taxfiling.PaymentPending {
		t.Fatalf("saved = %+v", saved)
	}

	got, err := repo.Get(ctx, "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if got.FilingDate != "2026-11-05" || !got.ISRRate.Equal(dec("0.02")) || !got.ISRAccrued.Equal(dec("1845.25")) ||
		!got.IVADue.Equal(dec("4827.59")) || got.Folio != "ACUSE-1" || got.Payment != nil || got.ExpenseMovementID != nil {
		t.Errorf("got = %+v", got)
	}
	if len(got.InvoiceIDs) != 2 || got.InvoiceIDs[0] != issued1 || got.InvoiceIDs[1] != issued2 {
		t.Errorf("InvoiceIDs = %v, want [%d %d]", got.InvoiceIDs, issued1, issued2)
	}

	// Only the issued invoices of the period carry the declaration period.
	linked := map[int]string{}
	rows, err := pool.Query(ctx, `SELECT id, COALESCE(declaration_period, '') FROM invoices`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var p string
		if err := rows.Scan(&id, &p); err != nil {
			t.Fatal(err)
		}
		linked[id] = p
	}
	if linked[issued1] != "2026-10" || linked[issued2] != "2026-10" || linked[prepared] != "" || linked[cancelled] != "" || linked[otherPeriod] != "" {
		t.Errorf("declaration_period links = %v", linked)
	}
}

func TestCreateDuplicatePeriod(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	if _, err := repo.Create(ctx, filing("2026-10")); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, filing("2026-10")); !errors.Is(err, taxfiling.ErrAlreadyFiled) {
		t.Errorf("err = %v, want ErrAlreadyFiled", err)
	}
}

func TestCreateRollsBackWhenTheInvoicesChanged(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	id := insertInvoice(t, pool, "2026-10", "emitida", uuidA)
	insertInvoice(t, pool, "2026-10", "emitida", uuidB) // issued after the declaration was computed

	if _, err := repo.Create(ctx, filing("2026-10", id)); !errors.Is(err, taxfiling.ErrInvoicesChanged) {
		t.Fatalf("err = %v, want ErrInvoicesChanged", err)
	}
	if _, err := repo.Get(ctx, "2026-10"); !errors.Is(err, taxfiling.ErrNotFound) {
		t.Errorf("the filing must not be stored: %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM invoices WHERE declaration_period IS NOT NULL`).Scan(&n); err != nil || n != 0 {
		t.Errorf("linked invoices = %d, %v; want the link rolled back", n, err)
	}
}

func TestCreatePaidWithExpense(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	var movementID int
	if err := pool.QueryRow(ctx, `
		INSERT INTO movements (date, category, kind, payment_method, currency, amount, amount_mxn)
		VALUES ('2026-11-07', 'Impuestos', 'Gasto', 'Transferencia', 'MXN', 6672.84, 6672.84) RETURNING id`).Scan(&movementID); err != nil {
		t.Fatal(err)
	}
	f := filing("2026-10")
	f.Payment = &taxfiling.Payment{Date: "2026-11-07", ISRPaid: dec("1845.25"), IVAPaid: dec("4827.59")}
	f.ExpenseMovementID = &movementID
	if _, err := repo.Create(ctx, f); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx, "2026-10")
	if err != nil || got.PaymentStatus() != taxfiling.PaymentPaid || got.Payment.Date != "2026-11-07" ||
		!got.Payment.Total().Equal(dec("6672.84")) || got.ExpenseMovementID == nil || *got.ExpenseMovementID != movementID {
		t.Errorf("got = %+v, %v", got, err)
	}

	// Deleting the expense keeps the filing and clears the link.
	if _, err := pool.Exec(ctx, `DELETE FROM movements WHERE id = $1`, movementID); err != nil {
		t.Fatal(err)
	}
	got, err = repo.Get(ctx, "2026-10")
	if err != nil || got.ExpenseMovementID != nil || got.PaymentStatus() != taxfiling.PaymentPaid {
		t.Errorf("after deleting the expense: %+v, %v", got, err)
	}
}

func TestIVAInFavorAndPrecision(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	f := filing("2026-10")
	f.IVADue = dec("-172.41")
	f.ISRRate = dec("0.011")
	if _, err := repo.Create(ctx, f); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx, "2026-10")
	if err != nil || !got.IVADue.Equal(dec("-172.41")) || !got.ISRRate.Equal(dec("0.011")) {
		t.Errorf("got = %+v, %v", got, err)
	}
}

func TestGetUnknownPeriod(t *testing.T) {
	repo, _ := newRepo(t)
	if _, err := repo.Get(context.Background(), "2026-01"); !errors.Is(err, taxfiling.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestListNewestFirstWithInvoiceIDs(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	if got, err := repo.List(ctx); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty list = %#v, %v; want an empty non-nil slice", got, err)
	}
	a := insertInvoice(t, pool, "2026-09", "emitida", uuidA)
	b := insertInvoice(t, pool, "2026-10", "emitida", uuidB)
	for _, f := range []taxfiling.Filing{filing("2026-09", a), filing("2026-10", b), filing("2026-08")} {
		if _, err := repo.Create(ctx, f); err != nil {
			t.Fatal(err)
		}
	}
	got, err := repo.List(ctx)
	if err != nil || len(got) != 3 {
		t.Fatalf("list = %+v, %v", got, err)
	}
	if got[0].Period != "2026-10" || got[1].Period != "2026-09" || got[2].Period != "2026-08" {
		t.Errorf("order = %s, %s, %s", got[0].Period, got[1].Period, got[2].Period)
	}
	if len(got[0].InvoiceIDs) != 1 || got[0].InvoiceIDs[0] != b || len(got[1].InvoiceIDs) != 1 || got[2].InvoiceIDs == nil || len(got[2].InvoiceIDs) != 0 {
		t.Errorf("invoice ids = %v %v %v", got[0].InvoiceIDs, got[1].InvoiceIDs, got[2].InvoiceIDs)
	}
}

func TestMarkPaid(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	if _, err := repo.Create(ctx, filing("2026-10")); err != nil {
		t.Fatal(err)
	}
	var movementID int
	if err := pool.QueryRow(ctx, `
		INSERT INTO movements (date, category, kind, payment_method, currency, amount, amount_mxn)
		VALUES ('2026-11-12', 'Impuestos', 'Gasto', 'Transferencia', 'MXN', 10, 10) RETURNING id`).Scan(&movementID); err != nil {
		t.Fatal(err)
	}

	p := taxfiling.Payment{Date: "2026-11-12", ISRPaid: dec("1845.25"), IVAPaid: dec("4827.59")}
	got, err := repo.MarkPaid(ctx, "2026-10", p, &movementID)
	if err != nil {
		t.Fatal(err)
	}
	if got.PaymentStatus() != taxfiling.PaymentPaid || got.Payment.Date != "2026-11-12" || !got.Payment.ISRPaid.Equal(dec("1845.25")) ||
		got.ExpenseMovementID == nil || *got.ExpenseMovementID != movementID {
		t.Errorf("got = %+v", got)
	}

	if _, err := repo.MarkPaid(ctx, "2026-10", p, nil); !errors.Is(err, taxfiling.ErrAlreadyPaid) {
		t.Errorf("paying twice err = %v, want ErrAlreadyPaid", err)
	}
	if _, err := repo.MarkPaid(ctx, "2026-01", p, nil); !errors.Is(err, taxfiling.ErrNotFound) {
		t.Errorf("unknown period err = %v, want ErrNotFound", err)
	}
	// The failed second payment must not have changed anything.
	again, _ := repo.Get(ctx, "2026-10")
	if again.ExpenseMovementID == nil || *again.ExpenseMovementID != movementID {
		t.Errorf("the recorded payment was overwritten: %+v", again)
	}
}

func TestMarkPaidWithoutExpense(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	if _, err := repo.Create(ctx, filing("2026-10")); err != nil {
		t.Fatal(err)
	}
	got, err := repo.MarkPaid(ctx, "2026-10", taxfiling.Payment{Date: "2026-11-12"}, nil)
	if err != nil || got.PaymentStatus() != taxfiling.PaymentPaid || got.ExpenseMovementID != nil || !got.Payment.Total().IsZero() {
		t.Errorf("got = %+v, %v", got, err)
	}
}

func TestDeleteOnlyPending(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	id := insertInvoice(t, pool, "2026-09", "emitida", uuidA)
	paidInvoice := insertInvoice(t, pool, "2026-10", "emitida", uuidB)
	if _, err := repo.Create(ctx, filing("2026-09", id)); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, filing("2026-10", paidInvoice)); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MarkPaid(ctx, "2026-10", taxfiling.Payment{Date: "2026-11-12"}, nil); err != nil {
		t.Fatal(err)
	}

	if _, err := repo.Delete(ctx, "2026-10"); !errors.Is(err, taxfiling.ErrFilingPaid) {
		t.Errorf("deleting a paid filing err = %v, want ErrFilingPaid", err)
	}
	if _, err := repo.Delete(ctx, "2026-01"); !errors.Is(err, taxfiling.ErrNotFound) {
		t.Errorf("deleting an unknown filing err = %v, want ErrNotFound", err)
	}
	if _, err := repo.Delete(ctx, "2026-09"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Get(ctx, "2026-09"); !errors.Is(err, taxfiling.ErrNotFound) {
		t.Errorf("the filing must be gone: %v", err)
	}

	var p9, p10 *string
	if err := pool.QueryRow(ctx, `SELECT declaration_period FROM invoices WHERE id = $1`, id).Scan(&p9); err != nil || p9 != nil {
		t.Errorf("invoice of the deleted filing still linked: %v, %v", p9, err)
	}
	if err := pool.QueryRow(ctx, `SELECT declaration_period FROM invoices WHERE id = $1`, paidInvoice).Scan(&p10); err != nil || p10 == nil || *p10 != "2026-10" {
		t.Errorf("invoice of the paid filing lost its link: %v, %v", p10, err)
	}
	// The period can be filed again after the deletion.
	if _, err := repo.Create(ctx, filing("2026-09", id)); err != nil {
		t.Errorf("re-registering the period: %v", err)
	}
}

func TestConstraintsRejectInconsistentPayments(t *testing.T) {
	_, pool := newRepo(t)
	ctx := context.Background()
	base := `INSERT INTO tax_filings (period, filing_date, income_collected, isr_rate, isr_accrued, isr_withheld, isr_due,
	           iva_transferred, iva_withheld, iva_creditable, iva_due, payment_date, isr_paid, iva_paid)
	         VALUES ('2026-10', '2026-11-05', 0, 0.01, 0, 0, 0, 0, 0, 0, 0, `
	for _, tail := range []string{"'2026-11-06', NULL, NULL)", "NULL, 10, NULL)", "'2026-11-06', -1, 0)"} {
		if _, err := pool.Exec(ctx, base+tail); err == nil {
			t.Errorf("expected a constraint violation for payment values %s", tail)
		}
	}
}
