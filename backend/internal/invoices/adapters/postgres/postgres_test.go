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

	"github.com/valium69mg/finances-app/backend/internal/invoices/adapters/postgres"
	"github.com/valium69mg/finances-app/backend/internal/invoices/app"
	invoices "github.com/valium69mg/finances-app/backend/internal/invoices/domain"
)

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

const uuidA = "6F1C2B3A-4D5E-4F60-8A7B-9C0D1E2F3A4B"
const uuidB = "11111111-2222-3333-4444-555555555555"

// newRepo returns a Repo on a throwaway schema holding a fresh copy of the
// movements and invoices migrations, so the tests never touch real data. It
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
	schema := "invoices_test_" + hex.EncodeToString(suffix)
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

	for _, name := range []string{"000006_movements.up.sql", "000010_invoices.up.sql"} {
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

func usaInvoice(date string) invoices.Invoice {
	rate := dec("17.74")
	a := invoices.ComputeUSAInvoice(dec("3383.33"), rate).Rounded()
	return invoices.Invoice{
		ClientID: "usa", CollectionDate: date, Period: date[:7], Currency: "USD", ExchangeRate: &rate,
		Status: invoices.StatusPrepared, Amounts: a,
	}
}

func clientBInvoice(date string) invoices.Invoice {
	return invoices.Invoice{
		ClientID: "b", CollectionDate: date, Period: date[:7], Currency: "MXN", Status: invoices.StatusPrepared,
		Amounts: invoices.ComputeClientBInvoice(dec("35000"), dec("0.16")).Rounded(),
	}
}

func doc(invoiceID int, kind invoices.DocumentKind, key string) invoices.Document {
	ct := "application/pdf"
	if kind == invoices.DocumentXML {
		ct = "application/xml"
	}
	return invoices.Document{
		InvoiceID: invoiceID, Kind: kind, Key: key, Name: "f." + string(kind), ContentType: ct, Size: 10,
		SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}
}

func TestCreateGetRoundTrip(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()

	var movementID int
	if err := pool.QueryRow(ctx, `
		INSERT INTO movements (date, category, kind, payment_method, currency, amount, amount_mxn)
		VALUES ('2026-10-15', 'Sueldo', 'Ingreso', 'Transferencia', 'MXN', 10, 10) RETURNING id`).Scan(&movementID); err != nil {
		t.Fatal(err)
	}

	inv := usaInvoice("2026-10-15")
	inv.MovementID = &movementID
	saved, err := repo.Create(ctx, inv)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID == 0 || saved.CreatedAt.IsZero() || saved.Status != invoices.StatusPrepared {
		t.Fatalf("saved = %+v", saved)
	}

	got, err := repo.Get(ctx, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ClientID != "usa" || got.CollectionDate != "2026-10-15" || got.Period != "2026-10" || got.Currency != "USD" ||
		got.ExchangeRate == nil || !got.ExchangeRate.Equal(dec("17.74")) ||
		!got.Subtotal.Equal(dec("3383.33")) || !got.SubtotalMXN.Equal(dec("60020.27")) ||
		!got.Total.Equal(dec("3383.33")) || !got.ExpectedDepositMXN.Equal(dec("60020.27")) || !got.IVA.IsZero() ||
		got.UUID != "" || got.DeclarationPeriod != "" || got.MovementID == nil || *got.MovementID != movementID {
		t.Errorf("got = %+v", got)
	}

	b, err := repo.Create(ctx, clientBInvoice("2026-10-31"))
	if err != nil {
		t.Fatal(err)
	}
	if b.ExchangeRate != nil || !b.IVA.Equal(dec("4827.59")) || !b.Subtotal.Equal(dec("30172.41")) || b.MovementID != nil {
		t.Errorf("client B = %+v", b)
	}

	if _, err := repo.Get(ctx, 9999); !errors.Is(err, invoices.ErrNotFound) {
		t.Errorf("missing invoice error = %v", err)
	}
}

func TestCreateRejectsUnknownMovement(t *testing.T) {
	repo, _ := newRepo(t)
	inv := usaInvoice("2026-10-15")
	missing := 12345
	inv.MovementID = &missing
	if _, err := repo.Create(context.Background(), inv); !errors.Is(err, invoices.ErrInvalidInput) {
		t.Errorf("error = %v, want ErrInvalidInput", err)
	}
}

func TestListFiltersAndOrder(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()

	got, err := repo.List(ctx, app.ListFilter{})
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty list = %v, %v (want empty non-nil)", got, err)
	}
	oct, _ := repo.Create(ctx, usaInvoice("2026-10-15"))
	nov, _ := repo.Create(ctx, clientBInvoice("2026-11-30"))
	if _, err := repo.Issue(ctx, nov.ID, uuidA, nil); err != nil {
		t.Fatal(err)
	}

	all, _ := repo.List(ctx, app.ListFilter{})
	if len(all) != 2 || all[0].ID != nov.ID || all[1].ID != oct.ID {
		t.Errorf("all = %+v (want newest first)", all)
	}
	period, _ := repo.List(ctx, app.ListFilter{Period: "2026-10"})
	status, _ := repo.List(ctx, app.ListFilter{Status: invoices.StatusIssued})
	both, _ := repo.List(ctx, app.ListFilter{Period: "2026-10", Status: invoices.StatusIssued})
	if len(period) != 1 || period[0].ID != oct.ID || len(status) != 1 || status[0].ID != nov.ID || len(both) != 0 {
		t.Errorf("period=%d status=%d both=%d", len(period), len(status), len(both))
	}
}

func TestIssueStoresDocumentsAtomically(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	inv, _ := repo.Create(ctx, usaInvoice("2026-10-15"))

	replaced, err := repo.Issue(ctx, inv.ID, uuidA, []invoices.Document{
		doc(inv.ID, invoices.DocumentPDF, "invoices/1/pdf-a.pdf"), doc(inv.ID, invoices.DocumentXML, "invoices/1/xml-a.xml"),
	})
	if err != nil || len(replaced) != 0 {
		t.Fatalf("issue = %v, %v", replaced, err)
	}
	got, _ := repo.Get(ctx, inv.ID)
	if got.Status != invoices.StatusIssued || got.UUID != uuidA {
		t.Errorf("issued invoice = %+v", got)
	}
	docs, err := repo.ListDocuments(ctx, inv.ID)
	if err != nil || len(docs) != 2 || docs[0].Kind != invoices.DocumentXML || docs[1].Kind != invoices.DocumentPDF {
		t.Fatalf("docs = %+v, %v (want xml first)", docs, err)
	}
	if docs[0].ID == 0 || docs[0].UploadedAt.IsZero() || docs[0].Size != 10 || docs[0].InvoiceID != inv.ID {
		t.Errorf("doc = %+v", docs[0])
	}

	// A second issue is a state conflict and leaves everything untouched.
	if _, err := repo.Issue(ctx, inv.ID, uuidB, []invoices.Document{doc(inv.ID, invoices.DocumentPDF, "invoices/1/pdf-b.pdf")}); !errors.Is(err, invoices.ErrStateChanged) {
		t.Errorf("second issue error = %v", err)
	}
	after, _ := repo.ListDocuments(ctx, inv.ID)
	if len(after) != 2 || after[1].Key != "invoices/1/pdf-a.pdf" {
		t.Errorf("documents changed by a failed issue: %+v", after)
	}

	// A duplicate UUID rolls the whole transaction back, documents included.
	other, _ := repo.Create(ctx, usaInvoice("2026-10-20"))
	_, err = repo.Issue(ctx, other.ID, uuidA, []invoices.Document{doc(other.ID, invoices.DocumentPDF, "invoices/2/pdf-c.pdf")})
	if !errors.Is(err, invoices.ErrDuplicateUUID) {
		t.Fatalf("duplicate uuid error = %v", err)
	}
	var n int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM invoice_documents WHERE invoice_id = $1`, other.ID).Scan(&n)
	if o, _ := repo.Get(ctx, other.ID); o.Status != invoices.StatusPrepared || n != 0 {
		t.Errorf("rolled back invoice = %+v with %d documents", o, n)
	}
}

func TestFindByUUID(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	inv, _ := repo.Create(ctx, usaInvoice("2026-10-15"))
	if _, found, err := repo.FindByUUID(ctx, uuidA); err != nil || found {
		t.Errorf("before issue: %v, %v", found, err)
	}
	if _, err := repo.Issue(ctx, inv.ID, uuidA, nil); err != nil {
		t.Fatal(err)
	}
	got, found, err := repo.FindByUUID(ctx, uuidA)
	if err != nil || !found || got.ID != inv.ID {
		t.Errorf("after issue: %+v, %v, %v", got, found, err)
	}
}

func TestPutDocumentReplacesAndReturnsOldKey(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	inv, _ := repo.Create(ctx, usaInvoice("2026-10-15"))

	if _, _, err := repo.PutDocument(ctx, doc(inv.ID, invoices.DocumentPDF, "k1")); !errors.Is(err, invoices.ErrStateChanged) {
		t.Errorf("put on a prepared invoice error = %v", err)
	}
	if _, _, err := repo.PutDocument(ctx, doc(9999, invoices.DocumentPDF, "k0")); !errors.Is(err, invoices.ErrNotFound) {
		t.Errorf("put on a missing invoice error = %v", err)
	}
	if _, err := repo.Issue(ctx, inv.ID, uuidA, nil); err != nil {
		t.Fatal(err)
	}

	first, old, err := repo.PutDocument(ctx, doc(inv.ID, invoices.DocumentPDF, "k1"))
	if err != nil || old != "" || first.ID == 0 {
		t.Fatalf("first put = %+v, %q, %v", first, old, err)
	}
	second, old, err := repo.PutDocument(ctx, doc(inv.ID, invoices.DocumentPDF, "k2"))
	if err != nil || old != "k1" || second.ID != first.ID || second.Key != "k2" {
		t.Fatalf("replacing put = %+v, %q, %v", second, old, err)
	}
	docs, _ := repo.ListDocuments(ctx, inv.ID)
	if len(docs) != 1 || docs[0].Key != "k2" {
		t.Errorf("docs = %+v", docs)
	}

	got, err := repo.GetDocument(ctx, inv.ID, first.ID)
	if err != nil || got.Key != "k2" {
		t.Errorf("GetDocument = %+v, %v", got, err)
	}
	if _, err := repo.GetDocument(ctx, inv.ID+1, first.ID); !errors.Is(err, invoices.ErrDocumentMissing) {
		t.Errorf("document of another invoice error = %v", err)
	}

	if err := repo.Cancel(ctx, inv.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.PutDocument(ctx, doc(inv.ID, invoices.DocumentXML, "k3")); !errors.Is(err, invoices.ErrStateChanged) {
		t.Errorf("put on a cancelled invoice error = %v", err)
	}
}

func TestCancel(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	inv, _ := repo.Create(ctx, usaInvoice("2026-10-15"))

	if err := repo.Cancel(ctx, inv.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.Get(ctx, inv.ID); got.Status != invoices.StatusCancelled {
		t.Errorf("status = %s", got.Status)
	}
	if err := repo.Cancel(ctx, inv.ID); !errors.Is(err, invoices.ErrStateChanged) {
		t.Errorf("second cancel error = %v", err)
	}
	if _, err := repo.Issue(ctx, inv.ID, uuidA, nil); !errors.Is(err, invoices.ErrStateChanged) {
		t.Errorf("issue after cancel error = %v", err)
	}
}

// An invoice a tax filing includes cannot be cancelled: the UPDATE itself
// refuses it, so a filing registered between a read and the write cannot slip by.
func TestCancelRefusesADeclaredInvoice(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	inv, _ := repo.Create(ctx, usaInvoice("2026-10-15"))
	if _, err := repo.Issue(ctx, inv.ID, uuidA, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE invoices SET declaration_period = '2026-10' WHERE id = $1`, int64(inv.ID)); err != nil {
		t.Fatal(err)
	}

	err := repo.Cancel(ctx, inv.ID)
	if !errors.Is(err, invoices.ErrDeclared) {
		t.Fatalf("cancel declared error = %v, want ErrDeclared", err)
	}
	if got, _ := repo.Get(ctx, inv.ID); got.Status != invoices.StatusIssued || got.DeclarationPeriod != "2026-10" {
		t.Errorf("invoice = %+v, want it untouched", got)
	}

	// Once the filing is deleted (the link cleared) the invoice can be cancelled.
	if _, err := pool.Exec(ctx, `UPDATE invoices SET declaration_period = NULL WHERE id = $1`, int64(inv.ID)); err != nil {
		t.Fatal(err)
	}
	if err := repo.Cancel(ctx, inv.ID); err != nil {
		t.Errorf("cancel after the filing was deleted: %v", err)
	}
	if err := repo.Cancel(ctx, 999999); !errors.Is(err, invoices.ErrNotFound) {
		t.Errorf("cancel unknown invoice error = %v, want ErrNotFound", err)
	}
}

func TestSchemaConstraints(t *testing.T) {
	_, pool := newRepo(t)
	ctx := context.Background()
	base := `INSERT INTO invoices (client_id, collection_date, period, currency, exchange_rate, subtotal, subtotal_mxn, total, expected_deposit_mxn, status, uuid) VALUES `
	for name, values := range map[string]string{
		"bad period":          `('usa', '2026-10-15', '2026-13', 'USD', 17, 1, 1, 1, 1, 'preparada', NULL)`,
		"USD without rate":    `('usa', '2026-10-15', '2026-10', 'USD', NULL, 1, 1, 1, 1, 'preparada', NULL)`,
		"MXN with rate":       `('b', '2026-10-15', '2026-10', 'MXN', 17, 1, 1, 1, 1, 'preparada', NULL)`,
		"zero total":          `('b', '2026-10-15', '2026-10', 'MXN', NULL, 1, 1, 0, 1, 'preparada', NULL)`,
		"unknown status":      `('b', '2026-10-15', '2026-10', 'MXN', NULL, 1, 1, 1, 1, 'pagada', NULL)`,
		"issued without uuid": `('b', '2026-10-15', '2026-10', 'MXN', NULL, 1, 1, 1, 1, 'emitida', NULL)`,
		"lowercase uuid":      `('b', '2026-10-15', '2026-10', 'MXN', NULL, 1, 1, 1, 1, 'emitida', '6f1c2b3a-4d5e-4f60-8a7b-9c0d1e2f3a4b')`,
	} {
		if _, err := pool.Exec(ctx, base+values); err == nil {
			t.Errorf("%s: insert should fail", name)
		}
	}
}
