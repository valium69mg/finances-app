package app_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/invoices/app"
	invoices "github.com/valium69mg/finances-app/backend/internal/invoices/domain"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain/settingstest"
)

var d = settingstest.D

func ptr(s string) *decimal.Decimal { v := d(s); return &v }

const uuidA = "6F1C2B3A-4D5E-4F60-8A7B-9C0D1E2F3A4B"
const uuidB = "11111111-2222-3333-4444-555555555555"

// --- fakes ---------------------------------------------------------------

type fakeStore struct {
	objects map[string][]byte
	putErr  error
	putN    int
	failAt  int // fail the Nth Put (1-based) when > 0
	delErr  error
}

func newStore() *fakeStore { return &fakeStore{objects: map[string][]byte{}} }

func (f *fakeStore) Put(_ context.Context, key string, r io.Reader, size int64, _ string) error {
	f.putN++
	if f.putErr != nil || (f.failAt > 0 && f.putN == f.failAt) {
		return errors.New("store down")
	}
	b, _ := io.ReadAll(r)
	if int64(len(b)) != size {
		return fmt.Errorf("size %d != %d", len(b), size)
	}
	f.objects[key] = b
	return nil
}

func (f *fakeStore) Get(_ context.Context, key string) (io.ReadCloser, error) {
	b, ok := f.objects[key]
	if !ok {
		return nil, app.ErrObjectNotFound
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func (f *fakeStore) Delete(_ context.Context, key string) error {
	if f.delErr != nil {
		return f.delErr
	}
	delete(f.objects, key)
	return nil
}

type fakeRepo struct {
	invoices map[int]invoices.Invoice
	docs     map[int][]invoices.Document // by invoice
	nextID   int
	nextDoc  int
	issueErr error
	// cancelCalls counts the Cancel calls that reached the write.
	cancelCalls int
	// staleGet makes Get hide declaration_period, as a read taken just before
	// a filing is registered would.
	staleGet bool
}

func newRepo() *fakeRepo {
	return &fakeRepo{invoices: map[int]invoices.Invoice{}, docs: map[int][]invoices.Document{}, nextID: 1, nextDoc: 1}
}

func (f *fakeRepo) Create(_ context.Context, inv invoices.Invoice) (invoices.Invoice, error) {
	inv.ID = f.nextID
	f.nextID++
	inv.CreatedAt = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	f.invoices[inv.ID] = inv
	return inv, nil
}

func (f *fakeRepo) Get(_ context.Context, id int) (invoices.Invoice, error) {
	inv, ok := f.invoices[id]
	if !ok {
		return invoices.Invoice{}, invoices.ErrNotFound
	}
	if f.staleGet {
		inv.DeclarationPeriod = ""
	}
	return inv, nil
}

func (f *fakeRepo) List(_ context.Context, fl app.ListFilter) ([]invoices.Invoice, error) {
	var out []invoices.Invoice
	for _, inv := range f.invoices {
		if (fl.Period == "" || inv.Period == fl.Period) && (fl.Status == "" || inv.Status == fl.Status) {
			out = append(out, inv)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}

func (f *fakeRepo) FindByUUID(_ context.Context, uuid string) (invoices.Invoice, bool, error) {
	for _, inv := range f.invoices {
		if inv.UUID == uuid {
			return inv, true, nil
		}
	}
	return invoices.Invoice{}, false, nil
}

func (f *fakeRepo) put(doc invoices.Document) (invoices.Document, string) {
	var replaced string
	list := f.docs[doc.InvoiceID]
	for i, e := range list {
		if e.Kind == doc.Kind {
			replaced = e.Key
			doc.ID = e.ID
			list[i] = doc
			f.docs[doc.InvoiceID] = list
			return doc, replaced
		}
	}
	doc.ID = f.nextDoc
	f.nextDoc++
	f.docs[doc.InvoiceID] = append(list, doc)
	return doc, ""
}

func (f *fakeRepo) Issue(_ context.Context, id int, uuid string, docs []invoices.Document) ([]string, error) {
	if f.issueErr != nil {
		return nil, f.issueErr
	}
	inv := f.invoices[id]
	if inv.Status != invoices.StatusPrepared {
		return nil, invoices.ErrStateChanged
	}
	inv.Status, inv.UUID = invoices.StatusIssued, uuid
	f.invoices[id] = inv
	var replaced []string
	for _, doc := range docs {
		_, r := f.put(doc)
		if r != "" {
			replaced = append(replaced, r)
		}
	}
	return replaced, nil
}

func (f *fakeRepo) Cancel(_ context.Context, id int) error {
	inv := f.invoices[id]
	if inv.Status == invoices.StatusCancelled {
		return invoices.ErrStateChanged
	}
	f.cancelCalls++
	if inv.DeclarationPeriod != "" {
		return invoices.ErrDeclared
	}
	inv.Status = invoices.StatusCancelled
	f.invoices[id] = inv
	return nil
}

func (f *fakeRepo) ListDocuments(_ context.Context, id int) ([]invoices.Document, error) {
	return f.docs[id], nil
}

func (f *fakeRepo) GetDocument(_ context.Context, invoiceID, docID int) (invoices.Document, error) {
	for _, doc := range f.docs[invoiceID] {
		if doc.ID == docID {
			return doc, nil
		}
	}
	return invoices.Document{}, invoices.ErrDocumentMissing
}

func (f *fakeRepo) PutDocument(_ context.Context, doc invoices.Document) (invoices.Document, string, error) {
	if f.invoices[doc.InvoiceID].Status != invoices.StatusIssued {
		return invoices.Document{}, "", invoices.ErrStateChanged
	}
	saved, replaced := f.put(doc)
	return saved, replaced, nil
}

type fakeMovements map[int]ledger.Movement

func (f fakeMovements) GetByID(_ context.Context, id int) (ledger.Movement, error) {
	m, ok := f[id]
	if !ok {
		return ledger.Movement{}, ledger.ErrNotFound
	}
	return m, nil
}

type fakeSettings struct{ cfg settings.Config }

func (f fakeSettings) Get(context.Context) (settings.Config, error) { return f.cfg, nil }

type env struct {
	svc   *app.Service
	repo  *fakeRepo
	store *fakeStore
}

func newEnv() env {
	repo, store := newRepo(), newStore()
	cfg := settingstest.RealConfig()
	cfg.Issuer = settings.Issuer{RFC: "AAA010101AAA", Name: "Juan", PostalCode: "64000"}
	now := func() time.Time { return time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC) }
	movs := fakeMovements{
		5: {ID: 5, Kind: ledger.KindIncome},
		6: {ID: 6, Kind: ledger.KindExpense},
	}
	svc := app.NewService(repo, store, movs, fakeSettings{cfg}, now, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return env{svc: svc, repo: repo, store: store}
}

func cfdiXML(uuid, total, subtotal, currency string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<cfdi:Comprobante xmlns:cfdi="http://www.sat.gob.mx/cfd/4" Total="` + total + `" SubTotal="` + subtotal + `" Moneda="` + currency + `">
<cfdi:Complemento><tfd:TimbreFiscalDigital xmlns:tfd="http://www.sat.gob.mx/TimbreFiscalDigital" UUID="` + uuid + `"/></cfdi:Complemento>
</cfdi:Comprobante>`)
}

func xmlUpload(uuid, total, subtotal, currency string) *app.Upload {
	return &app.Upload{Name: "cfdi.xml", ContentType: "text/xml", Data: cfdiXML(uuid, total, subtotal, currency)}
}

func pdfUpload() *app.Upload {
	return &app.Upload{Name: "factura.pdf", ContentType: "application/pdf", Data: []byte("%PDF-1.7 body")}
}

func (e env) prepareUSA(t *testing.T) int {
	t.Helper()
	res, err := e.svc.Prepare(context.Background(), app.PrepareInput{ClientID: "usa", Date: "2026-10-15"})
	if err != nil {
		t.Fatal(err)
	}
	return res.Invoice.ID
}

// --- tests ---------------------------------------------------------------

func TestPrepareRoundsAndReturnsChecklist(t *testing.T) {
	e := newEnv()
	res, err := e.svc.Prepare(context.Background(), app.PrepareInput{
		ClientID: "usa", Date: "2026-10-15", Subtotal: ptr("3383.33"), ExchangeRate: ptr("17.74"),
	})
	if err != nil {
		t.Fatal(err)
	}
	inv := res.Invoice
	if inv.ID != 1 || inv.Status != invoices.StatusPrepared || !inv.SubtotalMXN.Equal(d("60020.27")) ||
		!inv.ExpectedDepositMXN.Equal(d("60020.27")) {
		t.Errorf("invoice = %+v", inv)
	}
	if res.Checklist.DueDate != "2026-11-17" || !res.Checklist.Voucher.Export || len(res.Warnings) != 0 {
		t.Errorf("checklist = %+v warnings = %+v", res.Checklist, res.Warnings)
	}

	b, err := e.svc.Prepare(context.Background(), app.PrepareInput{ClientID: "b", Date: "2026-10-31", Amount: ptr("35000"), Periodicity: "quincenal"})
	if err != nil {
		t.Fatal(err)
	}
	if !b.Invoice.Subtotal.Equal(d("30172.41")) || !b.Invoice.IVA.Equal(d("4827.59")) || b.Checklist.Voucher.Global.Code != "03" {
		t.Errorf("client B = %+v", b)
	}
}

func TestPrepareDefaultsDateToToday(t *testing.T) {
	e := newEnv()
	res, err := e.svc.Prepare(context.Background(), app.PrepareInput{ClientID: "usa"})
	if err != nil || res.Invoice.CollectionDate != "2026-10-15" {
		t.Errorf("got %+v, %v", res.Invoice, err)
	}
}

func TestPrepareDuplicateWarning(t *testing.T) {
	e := newEnv()
	first := e.prepareUSA(t)
	res, err := e.svc.Prepare(context.Background(), app.PrepareInput{ClientID: "usa", Date: "2026-10-15"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 1 || res.Warnings[0].Code != invoices.WarningPossibleDuplicate || res.Warnings[0].InvoiceIDs[0] != first {
		t.Errorf("warnings = %+v", res.Warnings)
	}
	// A cancelled invoice is not a duplicate.
	if _, err := e.svc.Cancel(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Cancel(context.Background(), res.Invoice.ID); err != nil {
		t.Fatal(err)
	}
	res, _ = e.svc.Prepare(context.Background(), app.PrepareInput{ClientID: "usa", Date: "2026-10-15"})
	if len(res.Warnings) != 0 {
		t.Errorf("cancelled invoices must not warn: %+v", res.Warnings)
	}
}

func TestPrepareValidation(t *testing.T) {
	e := newEnv()
	ctx := context.Background()
	if _, err := e.svc.Prepare(ctx, app.PrepareInput{ClientID: "zzz"}); !errors.Is(err, invoices.ErrUnknownClient) {
		t.Errorf("unknown client: %v", err)
	}
	if _, err := e.svc.Prepare(ctx, app.PrepareInput{ClientID: "usa", Subtotal: ptr("0.001"), ExchangeRate: ptr("1")}); !errors.Is(err, invoices.ErrInvalidInput) {
		t.Errorf("amount rounding to zero: %v", err)
	}
	mov := 6
	if _, err := e.svc.Prepare(ctx, app.PrepareInput{ClientID: "usa", MovementID: &mov}); !errors.Is(err, invoices.ErrInvalidInput) {
		t.Errorf("expense movement link: %v", err)
	}
	missing := 99
	if _, err := e.svc.Prepare(ctx, app.PrepareInput{ClientID: "usa", MovementID: &missing}); !errors.Is(err, invoices.ErrInvalidInput) {
		t.Errorf("missing movement link: %v", err)
	}
	ok := 5
	res, err := e.svc.Prepare(ctx, app.PrepareInput{ClientID: "usa", MovementID: &ok})
	if err != nil || res.Invoice.MovementID == nil || *res.Invoice.MovementID != 5 {
		t.Errorf("income movement link: %+v, %v", res.Invoice, err)
	}
	if len(e.repo.invoices) != 1 {
		t.Errorf("failed prepares must not store anything, have %d", len(e.repo.invoices))
	}
}

func TestIssueWithXMLAndPDF(t *testing.T) {
	e := newEnv()
	id := e.prepareUSA(t)
	res, err := e.svc.Issue(context.Background(), id, app.IssueInput{
		XML: xmlUpload(strings.ToLower(uuidA), "3500.00", "3500.00", "USD"), PDF: pdfUpload(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Invoice.Status != invoices.StatusIssued || res.Invoice.UUID != uuidA {
		t.Errorf("invoice = %+v", res.Invoice)
	}
	if len(res.Warnings) != 0 || len(res.Documents) != 2 || len(e.store.objects) != 2 {
		t.Errorf("warnings = %+v docs = %d objects = %d", res.Warnings, len(res.Documents), len(e.store.objects))
	}
	for _, doc := range res.Documents {
		if _, ok := e.store.objects[doc.Key]; !ok || doc.SHA256 == "" || doc.InvoiceID != id {
			t.Errorf("document %+v not stored", doc)
		}
		if !strings.HasPrefix(doc.Key, fmt.Sprintf("invoices/%d/", id)) {
			t.Errorf("key = %q", doc.Key)
		}
	}
}

func TestIssueTotalMismatchIsAWarning(t *testing.T) {
	e := newEnv()
	id := e.prepareUSA(t)
	res, err := e.svc.Issue(context.Background(), id, app.IssueInput{XML: xmlUpload(uuidA, "3400", "3500", "USD")})
	if err != nil {
		t.Fatalf("a mismatch must not fail the issue: %v", err)
	}
	if res.Invoice.Status != invoices.StatusIssued || len(res.Warnings) != 1 || res.Warnings[0].Code != invoices.WarningTotalMismatch ||
		res.Warnings[0].Expected != "3500.00" || res.Warnings[0].Actual != "3400.00" {
		t.Errorf("warnings = %+v", res.Warnings)
	}
}

func TestIssueWithManualUUID(t *testing.T) {
	e := newEnv()
	id := e.prepareUSA(t)
	res, err := e.svc.Issue(context.Background(), id, app.IssueInput{UUID: " " + strings.ToLower(uuidA) + " "})
	if err != nil {
		t.Fatal(err)
	}
	if res.Invoice.UUID != uuidA || len(res.Documents) != 0 || len(e.store.objects) != 0 {
		t.Errorf("result = %+v", res)
	}
}

func TestIssueRejections(t *testing.T) {
	ctx := context.Background()
	e := newEnv()
	id := e.prepareUSA(t)
	other := e.prepareUSA(t)
	if _, err := e.svc.Issue(ctx, other, app.IssueInput{UUID: uuidB}); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		in   app.IssueInput
		want error
	}{
		{"nothing", app.IssueInput{}, invoices.ErrIssueInput},
		{"pdf only", app.IssueInput{PDF: pdfUpload()}, invoices.ErrIssueInput},
		{"bad manual uuid", app.IssueInput{UUID: "nope"}, invoices.ErrInvalidUUID},
		{"uuid used by another invoice", app.IssueInput{UUID: uuidB}, invoices.ErrDuplicateUUID},
		{"xml uuid used by another invoice", app.IssueInput{XML: xmlUpload(uuidB, "3500", "3500", "USD")}, invoices.ErrDuplicateUUID},
		{"manual uuid differs from xml", app.IssueInput{UUID: uuidA, XML: xmlUpload(uuidB, "3500", "3500", "USD")}, invoices.ErrUUIDMismatch},
		{"unstamped xml", app.IssueInput{XML: &app.Upload{Name: "a.xml", Data: []byte(`<cfdi:Comprobante xmlns:cfdi="http://www.sat.gob.mx/cfd/4" Total="1" SubTotal="1" Moneda="MXN"/>`)}}, invoices.ErrNotStamped},
		{"garbage xml", app.IssueInput{XML: &app.Upload{Name: "a.xml", Data: []byte("nope")}}, invoices.ErrInvalidCFDI},
		{"wrong extension", app.IssueInput{XML: &app.Upload{Name: "a.txt", Data: cfdiXML(uuidA, "1", "1", "MXN")}}, invoices.ErrInvalidDocument},
		{"bad pdf", app.IssueInput{UUID: uuidA, PDF: &app.Upload{Name: "a.pdf", Data: []byte("html")}}, invoices.ErrInvalidDocument},
	}
	for _, tc := range cases {
		if _, err := e.svc.Issue(ctx, id, tc.in); !errors.Is(err, tc.want) {
			t.Errorf("%s: error = %v, want %v", tc.name, err, tc.want)
		}
	}
	if got := e.repo.invoices[id]; got.Status != invoices.StatusPrepared || len(e.store.objects) != 0 {
		t.Errorf("rejected issues must change nothing: %+v, %d objects", got, len(e.store.objects))
	}
}

func TestIssueStateGuards(t *testing.T) {
	ctx := context.Background()
	e := newEnv()
	id := e.prepareUSA(t)
	if _, err := e.svc.Issue(ctx, id, app.IssueInput{UUID: uuidA}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Issue(ctx, id, app.IssueInput{UUID: uuidB}); !errors.Is(err, invoices.ErrAlreadyIssued) {
		t.Errorf("re-issue: %v", err)
	}
	if _, err := e.svc.Cancel(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Issue(ctx, id, app.IssueInput{UUID: uuidB}); !errors.Is(err, invoices.ErrCancelled) {
		t.Errorf("issue cancelled: %v", err)
	}
	if _, err := e.svc.Issue(ctx, 99, app.IssueInput{UUID: uuidB}); !errors.Is(err, invoices.ErrNotFound) {
		t.Errorf("issue missing: %v", err)
	}
}

func TestIssueCleansUpObjectsOnFailure(t *testing.T) {
	ctx := context.Background()

	t.Run("second upload fails", func(t *testing.T) {
		e := newEnv()
		id := e.prepareUSA(t)
		e.store.failAt = 2
		_, err := e.svc.Issue(ctx, id, app.IssueInput{XML: xmlUpload(uuidA, "3500", "3500", "USD"), PDF: pdfUpload()})
		if !errors.Is(err, app.ErrStorage) {
			t.Fatalf("error = %v, want ErrStorage", err)
		}
		if len(e.store.objects) != 0 || e.repo.invoices[id].Status != invoices.StatusPrepared {
			t.Errorf("objects = %d status = %s", len(e.store.objects), e.repo.invoices[id].Status)
		}
	})

	t.Run("database fails after the upload", func(t *testing.T) {
		e := newEnv()
		id := e.prepareUSA(t)
		e.repo.issueErr = invoices.ErrDuplicateUUID
		_, err := e.svc.Issue(ctx, id, app.IssueInput{XML: xmlUpload(uuidA, "3500", "3500", "USD")})
		if !errors.Is(err, invoices.ErrDuplicateUUID) {
			t.Fatalf("error = %v", err)
		}
		if len(e.store.objects) != 0 {
			t.Errorf("orphan objects left: %d", len(e.store.objects))
		}
	})
}

func TestAttachDocument(t *testing.T) {
	ctx := context.Background()
	e := newEnv()
	id := e.prepareUSA(t)

	if _, err := e.svc.AttachDocument(ctx, id, invoices.DocumentPDF, *pdfUpload()); !errors.Is(err, invoices.ErrNotIssued) {
		t.Errorf("attach to a prepared invoice: %v", err)
	}
	if _, err := e.svc.Issue(ctx, id, app.IssueInput{UUID: uuidA}); err != nil {
		t.Fatal(err)
	}

	first, err := e.svc.AttachDocument(ctx, id, invoices.DocumentPDF, *pdfUpload())
	if err != nil || first.Document.ID == 0 {
		t.Fatalf("attach pdf: %+v, %v", first, err)
	}
	oldKey := first.Document.Key

	// Replacing removes the previous object.
	second, err := e.svc.AttachDocument(ctx, id, invoices.DocumentPDF, app.Upload{Name: "v2.pdf", Data: []byte("%PDF-2")})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := e.store.objects[oldKey]; ok || second.Document.Key == oldKey || len(e.store.objects) != 1 {
		t.Errorf("old object should be replaced: %v", e.store.objects)
	}

	// An XML must carry the invoice UUID.
	if _, err := e.svc.AttachDocument(ctx, id, invoices.DocumentXML, *xmlUpload(uuidB, "3500", "3500", "USD")); !errors.Is(err, invoices.ErrUUIDMismatch) {
		t.Errorf("xml with another uuid: %v", err)
	}
	xmlRes, err := e.svc.AttachDocument(ctx, id, invoices.DocumentXML, *xmlUpload(uuidA, "3400", "3500", "USD"))
	if err != nil || len(xmlRes.Warnings) != 1 {
		t.Errorf("xml attach: %+v, %v", xmlRes, err)
	}

	if _, err := e.svc.AttachDocument(ctx, id, "exe", *pdfUpload()); !errors.Is(err, invoices.ErrInvalidDocument) {
		t.Errorf("unknown kind: %v", err)
	}

	if _, err := e.svc.Cancel(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.AttachDocument(ctx, id, invoices.DocumentPDF, *pdfUpload()); !errors.Is(err, invoices.ErrCancelled) {
		t.Errorf("attach to a cancelled invoice: %v", err)
	}
}

func TestDownload(t *testing.T) {
	ctx := context.Background()
	e := newEnv()
	id := e.prepareUSA(t)
	res, err := e.svc.Issue(ctx, id, app.IssueInput{UUID: uuidA, PDF: pdfUpload()})
	if err != nil {
		t.Fatal(err)
	}
	doc := res.Documents[0]

	got, body, err := e.svc.Download(ctx, id, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(body)
	_ = body.Close()
	if got.Name != "factura.pdf" || string(data) != "%PDF-1.7 body" {
		t.Errorf("doc = %+v data = %q", got, data)
	}

	if _, _, err := e.svc.Download(ctx, id, 999); !errors.Is(err, invoices.ErrDocumentMissing) {
		t.Errorf("unknown document: %v", err)
	}
	if _, _, err := e.svc.Download(ctx, id+1, doc.ID); !errors.Is(err, invoices.ErrDocumentMissing) {
		t.Errorf("document of another invoice: %v", err)
	}
	delete(e.store.objects, doc.Key)
	if _, _, err := e.svc.Download(ctx, id, doc.ID); !errors.Is(err, invoices.ErrDocumentMissing) {
		t.Errorf("object missing from the store: %v", err)
	}
}

func TestCancel(t *testing.T) {
	ctx := context.Background()
	e := newEnv()
	id := e.prepareUSA(t)
	if _, err := e.svc.Issue(ctx, id, app.IssueInput{UUID: uuidA, PDF: pdfUpload()}); err != nil {
		t.Fatal(err)
	}
	got, err := e.svc.Cancel(ctx, id)
	if err != nil || got.Status != invoices.StatusCancelled || got.UUID != uuidA {
		t.Fatalf("cancel = %+v, %v", got, err)
	}
	if len(e.store.objects) != 1 {
		t.Error("cancelling keeps the stored documents")
	}
	if _, err := e.svc.Cancel(ctx, id); !errors.Is(err, invoices.ErrCancelled) {
		t.Errorf("cancel twice: %v", err)
	}
	if _, err := e.svc.Cancel(ctx, 42); !errors.Is(err, invoices.ErrNotFound) {
		t.Errorf("cancel missing: %v", err)
	}
}

// Cancelling an invoice that a saved tax filing includes would silently desync
// that filing: it is refused, and the invoice stays issued.
func TestCancelRefusesADeclaredInvoice(t *testing.T) {
	ctx := context.Background()
	e := newEnv()
	id := e.prepareUSA(t)
	if _, err := e.svc.Issue(ctx, id, app.IssueInput{UUID: uuidA}); err != nil {
		t.Fatal(err)
	}
	inv := e.repo.invoices[id]
	inv.DeclarationPeriod = "2026-10"
	e.repo.invoices[id] = inv

	_, err := e.svc.Cancel(ctx, id)
	if !errors.Is(err, invoices.ErrDeclared) || !strings.Contains(err.Error(), "2026-10") {
		t.Fatalf("cancel declared: %v, want ErrDeclared naming the period", err)
	}
	if got := e.repo.invoices[id]; got.Status != invoices.StatusIssued {
		t.Errorf("status = %s, want it untouched", got.Status)
	}
	if e.repo.cancelCalls != 0 {
		t.Error("the service must refuse before writing")
	}

	// The repository re-checks atomically: a filing registered after the
	// service read the invoice is still refused (the fake plays the UPDATE guard).
	e.repo.staleGet = true
	if _, err := e.svc.Cancel(ctx, id); !errors.Is(err, invoices.ErrDeclared) || e.repo.cancelCalls != 1 {
		t.Errorf("race: %v (write attempts %d), want the repository's ErrDeclared", err, e.repo.cancelCalls)
	}
	e.repo.staleGet = false
	if got := e.repo.invoices[id]; got.Status != invoices.StatusIssued {
		t.Errorf("status = %s, want it untouched", got.Status)
	}
}

type filingsFake struct {
	filed map[string]bool
	err   error
}

func (f filingsFake) IsFiled(_ context.Context, period string) (bool, error) {
	return f.filed[period], f.err
}

func TestIssueWarnsWhenThePeriodIsAlreadyFiled(t *testing.T) {
	ctx := context.Background()
	warnings := func(res app.Result) []string {
		var codes []string
		for _, w := range res.Warnings {
			codes = append(codes, w.Code)
		}
		return codes
	}

	t.Run("filed period", func(t *testing.T) {
		e := newEnv()
		e.svc.WithFilings(filingsFake{filed: map[string]bool{"2026-10": true}})
		id := e.prepareUSA(t)
		res, err := e.svc.Issue(ctx, id, app.IssueInput{UUID: uuidA})
		if err != nil {
			t.Fatal(err)
		}
		if got := warnings(res); len(got) != 1 || got[0] != invoices.WarningPeriodAlreadyFiled {
			t.Errorf("warnings = %v, want period_already_filed", got)
		}
		if res.Invoice.Status != invoices.StatusIssued || res.Invoice.DeclarationPeriod != "" {
			t.Errorf("invoice = %+v; it is issued and stays unlinked", res.Invoice)
		}
	})
	t.Run("period not filed", func(t *testing.T) {
		e := newEnv()
		e.svc.WithFilings(filingsFake{filed: map[string]bool{"2026-09": true}})
		res, err := e.svc.Issue(ctx, e.prepareUSA(t), app.IssueInput{UUID: uuidA})
		if err != nil || len(res.Warnings) != 0 {
			t.Errorf("res warnings %v, err %v", warnings(res), err)
		}
	})
	t.Run("the warning combines with the XML mismatch warnings", func(t *testing.T) {
		e := newEnv()
		e.svc.WithFilings(filingsFake{filed: map[string]bool{"2026-10": true}})
		res, err := e.svc.Issue(ctx, e.prepareUSA(t), app.IssueInput{XML: xmlUpload(uuidA, "1", "1", "USD")})
		if err != nil || len(res.Warnings) < 2 || res.Warnings[len(res.Warnings)-1].Code != invoices.WarningPeriodAlreadyFiled {
			t.Errorf("warnings = %v, err %v", warnings(res), err)
		}
	})
	t.Run("a failing lookup never fails an issued invoice", func(t *testing.T) {
		e := newEnv()
		e.svc.WithFilings(filingsFake{err: errors.New("db down")})
		id := e.prepareUSA(t)
		res, err := e.svc.Issue(ctx, id, app.IssueInput{UUID: uuidA})
		if err != nil || len(res.Warnings) != 0 || e.repo.invoices[id].Status != invoices.StatusIssued {
			t.Errorf("warnings %v, err %v", warnings(res), err)
		}
	})
	t.Run("no lookup configured", func(t *testing.T) {
		e := newEnv()
		res, err := e.svc.Issue(ctx, e.prepareUSA(t), app.IssueInput{UUID: uuidA})
		if err != nil || len(res.Warnings) != 0 {
			t.Errorf("warnings %v, err %v", warnings(res), err)
		}
	})
}

func TestListAndGet(t *testing.T) {
	ctx := context.Background()
	e := newEnv()
	a := e.prepareUSA(t)
	if _, err := e.svc.Prepare(ctx, app.PrepareInput{ClientID: "b", Date: "2026-11-30", Amount: ptr("100")}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Issue(ctx, a, app.IssueInput{UUID: uuidA}); err != nil {
		t.Fatal(err)
	}

	all, err := e.svc.List(ctx, "", "")
	if err != nil || len(all) != 2 || all[0].ID != 2 {
		t.Errorf("list = %+v, %v", all, err)
	}
	oct, _ := e.svc.List(ctx, "2026-10", "")
	issued, _ := e.svc.List(ctx, "", invoices.StatusIssued)
	if len(oct) != 1 || len(issued) != 1 || issued[0].ID != a {
		t.Errorf("filtered = %+v / %+v", oct, issued)
	}
	if _, err := e.svc.List(ctx, "2026-13", ""); !errors.Is(err, invoices.ErrInvalidInput) {
		t.Errorf("bad period: %v", err)
	}
	if _, err := e.svc.List(ctx, "", "zzz"); !errors.Is(err, invoices.ErrInvalidInput) {
		t.Errorf("bad state: %v", err)
	}

	det, err := e.svc.Get(ctx, a, "")
	if err != nil || det.Checklist.Period != "2026-10" || det.Invoice.UUID != uuidA {
		t.Errorf("detail = %+v, %v", det, err)
	}
	if _, err := e.svc.Get(ctx, 77, ""); !errors.Is(err, invoices.ErrNotFound) {
		t.Errorf("get missing: %v", err)
	}
}
