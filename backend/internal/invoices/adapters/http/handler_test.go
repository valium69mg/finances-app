package invoiceshttp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	invoiceshttp "github.com/valium69mg/finances-app/backend/internal/invoices/adapters/http"
	"github.com/valium69mg/finances-app/backend/internal/invoices/app"
	invoices "github.com/valium69mg/finances-app/backend/internal/invoices/domain"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

const uuidA = "6F1C2B3A-4D5E-4F60-8A7B-9C0D1E2F3A4B"

type fakeService struct {
	detail   app.Detail
	warnings []invoices.Warning
	list     []invoices.Invoice
	docRes   app.DocumentResult
	doc      invoices.Document
	body     string
	err      error

	gotPrepare     app.PrepareInput
	gotIssue       app.IssueInput
	gotID          int
	gotKind        invoices.DocumentKind
	gotUpload      app.Upload
	gotPeriod      string
	gotStatus      invoices.Status
	gotPeriodicity string
	gotDocID       int
}

func (f *fakeService) Prepare(_ context.Context, in app.PrepareInput) (app.Result, error) {
	f.gotPrepare = in
	return app.Result{Detail: f.detail, Warnings: f.warnings}, f.err
}
func (f *fakeService) List(_ context.Context, period string, status invoices.Status) ([]invoices.Invoice, error) {
	f.gotPeriod, f.gotStatus = period, status
	return f.list, f.err
}
func (f *fakeService) Get(_ context.Context, id int, periodicity string) (app.Detail, error) {
	f.gotID, f.gotPeriodicity = id, periodicity
	return f.detail, f.err
}
func (f *fakeService) Issue(_ context.Context, id int, in app.IssueInput) (app.Result, error) {
	f.gotID, f.gotIssue = id, in
	return app.Result{Detail: f.detail, Warnings: f.warnings}, f.err
}
func (f *fakeService) AttachDocument(_ context.Context, id int, kind invoices.DocumentKind, up app.Upload) (app.DocumentResult, error) {
	f.gotID, f.gotKind, f.gotUpload = id, kind, up
	return f.docRes, f.err
}
func (f *fakeService) Download(_ context.Context, invoiceID, docID int) (invoices.Document, io.ReadCloser, error) {
	f.gotID, f.gotDocID = invoiceID, docID
	if f.err != nil {
		return invoices.Document{}, nil, f.err
	}
	return f.doc, io.NopCloser(strings.NewReader(f.body)), nil
}
func (f *fakeService) Cancel(_ context.Context, id int) (invoices.Invoice, error) {
	f.gotID = id
	return f.detail.Invoice, f.err
}

func requireGoodToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func newServer(svc *fakeService) http.Handler {
	mux := http.NewServeMux()
	invoiceshttp.New(svc, slog.New(slog.NewTextHandler(io.Discard, nil))).Register(mux, requireGoodToken)
	return mux
}

func do(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer good")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

type part struct {
	field, filename, contentType, content string
}

func multipartBody(t *testing.T, fields map[string]string, files ...part) (string, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range files {
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, f.field, f.filename))
		h.Set("Content-Type", f.contentType)
		pw, err := mw.CreatePart(h)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(pw, f.content)
	}
	_ = mw.Close()
	return buf.String(), mw.FormDataContentType()
}

func doMultipart(h http.Handler, path, body, contentType string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer good")
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func sampleDetail() app.Detail {
	rate := d("17.74")
	return app.Detail{
		Invoice: invoices.Invoice{
			ID: 7, ClientID: "usa", CollectionDate: "2026-10-15", Period: "2026-10", Currency: "USD", ExchangeRate: &rate,
			Status: invoices.StatusIssued, UUID: uuidA, CreatedAt: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC),
			Amounts: invoices.Amounts{
				Subtotal: d("3383.33"), SubtotalMXN: d("60020.27"), Total: d("3383.33"), ExpectedDepositMXN: d("60020.27"),
			},
		},
		Documents: []invoices.Document{{
			ID: 3, InvoiceID: 7, Kind: invoices.DocumentXML, Key: "invoices/7/secret-key.xml", Name: "cfdi.xml",
			ContentType: "application/xml", Size: 12, SHA256: strings.Repeat("a", 64), UploadedAt: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC),
		}},
		Checklist: invoices.Checklist{
			Period: "2026-10", DueDate: "2026-11-17",
			Voucher: invoices.Voucher{Type: "I", Currency: "USD", ExchangeRate: &rate, PaymentForm: "03", PaymentMethod: "PUE", Export: true},
			Concept: invoices.Concept{Quantity: 1, UnitValue: d("3383.33")},
		},
	}
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("body %q: %v", rec.Body, err)
	}
	return out
}

func TestRoutesRequireAuth(t *testing.T) {
	h := newServer(&fakeService{})
	for _, tc := range []struct{ method, path string }{
		{"POST", "/invoices"}, {"GET", "/invoices"}, {"GET", "/invoices/1"}, {"POST", "/invoices/1/issue"},
		{"POST", "/invoices/1/documents"}, {"GET", "/invoices/1/documents/2"}, {"POST", "/invoices/1/cancel"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, want 401", tc.method, tc.path, rec.Code)
		}
	}
}

func TestPrepare(t *testing.T) {
	svc := &fakeService{detail: sampleDetail(), warnings: invoices.DuplicateWarning([]int{3})}
	rec := do(newServer(svc), "POST", "/invoices",
		`{"client_id":"usa","date":"2026-10-15","subtotal":"3383.33","exchange_rate":"17.74","movement_id":5,"periodicity":"quincenal"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	in := svc.gotPrepare
	if in.ClientID != "usa" || in.Date != "2026-10-15" || in.Subtotal == nil || !in.Subtotal.Equal(d("3383.33")) ||
		in.ExchangeRate == nil || in.MovementID == nil || *in.MovementID != 5 || in.Periodicity != "quincenal" || in.Amount != nil {
		t.Errorf("input = %+v", in)
	}
	body := decode(t, rec)
	inv := body["invoice"].(map[string]any)
	if inv["state"] != "emitida" || inv["subtotal"] != "3383.33" || inv["exchange_rate"] != "17.74" || inv["uuid"] != uuidA || inv["id"] != float64(7) {
		t.Errorf("invoice = %v", inv)
	}
	ws := body["warnings"].([]any)
	if len(ws) != 1 || ws[0].(map[string]any)["code"] != "possible_duplicate" {
		t.Errorf("warnings = %v", ws)
	}
	if _, ok := body["checklist"].(map[string]any)["voucher"]; !ok {
		t.Errorf("checklist = %v", body["checklist"])
	}
	if strings.Contains(rec.Body.String(), "secret-key") {
		t.Error("the storage key must never be exposed")
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q", got)
	}
}

func TestPrepareRejectsBadJSON(t *testing.T) {
	for _, body := range []string{`not json`, `{"client_id":"usa","unknown":1}`, `{"subtotal":"abc"}`} {
		if rec := do(newServer(&fakeService{}), "POST", "/invoices", body); rec.Code != http.StatusBadRequest {
			t.Errorf("body %q: status %d", body, rec.Code)
		}
	}
}

func TestListPassesFiltersAndReturnsEmptyArray(t *testing.T) {
	svc := &fakeService{}
	rec := do(newServer(svc), "GET", "/invoices?period=2026-10&state=emitida", "")
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("status %d body %q", rec.Code, rec.Body)
	}
	if svc.gotPeriod != "2026-10" || svc.gotStatus != invoices.StatusIssued {
		t.Errorf("filters = %q %q", svc.gotPeriod, svc.gotStatus)
	}

	svc = &fakeService{list: []invoices.Invoice{sampleDetail().Invoice}}
	rec = do(newServer(svc), "GET", "/invoices", "")
	var out []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out) != 1 || out[0]["state"] != "emitida" || out[0]["declaration_period"] != nil {
		t.Errorf("list = %v", out)
	}
}

func TestGet(t *testing.T) {
	svc := &fakeService{detail: sampleDetail()}
	rec := do(newServer(svc), "GET", "/invoices/7?periodicity=quincenal", "")
	if rec.Code != http.StatusOK || svc.gotID != 7 || svc.gotPeriodicity != "quincenal" {
		t.Fatalf("status %d id %d periodicity %q", rec.Code, svc.gotID, svc.gotPeriodicity)
	}
	body := decode(t, rec)
	docs := body["documents"].([]any)
	if len(docs) != 1 || docs[0].(map[string]any)["name"] != "cfdi.xml" || docs[0].(map[string]any)["kind"] != "xml" {
		t.Errorf("documents = %v", docs)
	}
	if body["warnings"] == nil || len(body["warnings"].([]any)) != 0 {
		t.Errorf("warnings must be an empty array: %v", body["warnings"])
	}
	if strings.Contains(rec.Body.String(), "secret-key") {
		t.Error("the storage key must never be exposed")
	}
	if rec := do(newServer(svc), "GET", "/invoices/abc", ""); rec.Code != http.StatusNotFound {
		t.Errorf("bad id status = %d", rec.Code)
	}
}

func TestIssueMultipart(t *testing.T) {
	svc := &fakeService{detail: sampleDetail(), warnings: []invoices.Warning{{Code: invoices.WarningTotalMismatch, Expected: "3500.00", Actual: "3400.00"}}}
	body, ct := multipartBody(t, map[string]string{"uuid": uuidA},
		part{"xml", "cfdi.xml", "text/xml", "<a/>"}, part{"pdf", "factura.pdf", "application/pdf", "%PDF-1"})
	rec := doMultipart(newServer(svc), "/invoices/7/issue", body, ct)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	in := svc.gotIssue
	if svc.gotID != 7 || in.UUID != uuidA || in.XML == nil || in.XML.Name != "cfdi.xml" || in.XML.ContentType != "text/xml" ||
		string(in.XML.Data) != "<a/>" || in.PDF == nil || string(in.PDF.Data) != "%PDF-1" {
		t.Errorf("input = %+v / xml %+v / pdf %+v", in, in.XML, in.PDF)
	}
	ws := decode(t, rec)["warnings"].([]any)
	if len(ws) != 1 || ws[0].(map[string]any)["expected"] != "3500.00" || ws[0].(map[string]any)["actual"] != "3400.00" {
		t.Errorf("warnings = %v", ws)
	}
}

func TestIssueUUIDOnly(t *testing.T) {
	svc := &fakeService{detail: sampleDetail()}
	body, ct := multipartBody(t, map[string]string{"uuid": uuidA})
	if rec := doMultipart(newServer(svc), "/invoices/7/issue", body, ct); rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if svc.gotIssue.XML != nil || svc.gotIssue.PDF != nil || svc.gotIssue.UUID != uuidA {
		t.Errorf("input = %+v", svc.gotIssue)
	}
}

func TestIssueRejectsNonMultipart(t *testing.T) {
	rec := do(newServer(&fakeService{}), "POST", "/invoices/7/issue", `{"uuid":"x"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d", rec.Code)
	}
}

func TestIssueRejectsOversizedRequest(t *testing.T) {
	big := strings.Repeat("a", invoices.MaxXMLBytes+invoices.MaxPDFBytes+2<<20)
	body, ct := multipartBody(t, nil, part{"pdf", "a.pdf", "application/pdf", big})
	rec := doMultipart(newServer(&fakeService{}), "/invoices/7/issue", body, ct)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", rec.Code)
	}
}

func TestAttach(t *testing.T) {
	svc := &fakeService{docRes: app.DocumentResult{Document: sampleDetail().Documents[0]}}
	body, ct := multipartBody(t, map[string]string{"kind": "xml"}, part{"file", "cfdi.xml", "application/xml", "<a/>"})
	rec := doMultipart(newServer(svc), "/invoices/7/documents", body, ct)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if svc.gotID != 7 || svc.gotKind != invoices.DocumentXML || svc.gotUpload.Name != "cfdi.xml" || string(svc.gotUpload.Data) != "<a/>" {
		t.Errorf("got %d %s %+v", svc.gotID, svc.gotKind, svc.gotUpload)
	}
	if decode(t, rec)["document"].(map[string]any)["id"] != float64(3) {
		t.Errorf("body = %s", rec.Body)
	}

	for name, tc := range map[string]struct {
		fields map[string]string
		files  []part
	}{
		"missing kind": {nil, []part{{"file", "a.pdf", "application/pdf", "x"}}},
		"bad kind":     {map[string]string{"kind": "exe"}, []part{{"file", "a.exe", "x", "x"}}},
		"missing file": {map[string]string{"kind": "pdf"}, nil},
	} {
		body, ct := multipartBody(t, tc.fields, tc.files...)
		if rec := doMultipart(newServer(&fakeService{}), "/invoices/7/documents", body, ct); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d", name, rec.Code)
		}
	}
}

func TestDownloadStreamsWithSafeHeaders(t *testing.T) {
	svc := &fakeService{
		doc:  invoices.Document{ID: 3, Name: "factura ñ.pdf", ContentType: "application/pdf", Size: 13},
		body: "%PDF-1.7 body",
	}
	rec := do(newServer(svc), "GET", "/invoices/7/documents/3", "")
	if rec.Code != http.StatusOK || rec.Body.String() != "%PDF-1.7 body" || svc.gotID != 7 || svc.gotDocID != 3 {
		t.Fatalf("status %d body %q", rec.Code, rec.Body)
	}
	h := rec.Header()
	if h.Get("Content-Type") != "application/pdf" || !strings.HasPrefix(h.Get("Content-Disposition"), "attachment;") ||
		h.Get("Cache-Control") != "no-store" || h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Content-Length") != "13" ||
		!strings.Contains(h.Get("Content-Security-Policy"), "sandbox") {
		t.Errorf("headers = %v", h)
	}
	if !strings.Contains(h.Get("Content-Disposition"), "filename*=") {
		t.Errorf("non-ASCII file names must be RFC 2231 encoded: %q", h.Get("Content-Disposition"))
	}
	if rec := do(newServer(svc), "GET", "/invoices/7/documents/zero", ""); rec.Code != http.StatusNotFound {
		t.Errorf("bad doc id status = %d", rec.Code)
	}
}

func TestCancel(t *testing.T) {
	svc := &fakeService{detail: sampleDetail()}
	rec := do(newServer(svc), "POST", "/invoices/7/cancel", "")
	if rec.Code != http.StatusOK || svc.gotID != 7 || decode(t, rec)["id"] != float64(7) {
		t.Errorf("status %d id %d body %s", rec.Code, svc.gotID, rec.Body)
	}
}

func TestErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{invoices.ErrUnknownClient, 400, "unknown_client"},
		{fmt.Errorf("%w: x", invoices.ErrInvalidInput), 400, "invalid_invoice"},
		{invoices.ErrIssueInput, 400, "xml_or_uuid_required"},
		{invoices.ErrInvalidUUID, 400, "invalid_uuid"},
		{invoices.ErrInvalidDocument, 400, "invalid_document"},
		{invoices.ErrInvalidCFDI, 422, "invalid_cfdi"},
		{invoices.ErrNotStamped, 422, "cfdi_not_stamped"},
		{invoices.ErrUUIDMismatch, 422, "uuid_mismatch"},
		{invoices.ErrDuplicateUUID, 409, "duplicate_uuid"},
		{invoices.ErrCancelled, 409, "invoice_cancelled"},
		{invoices.ErrAlreadyIssued, 409, "invoice_already_issued"},
		{invoices.ErrNotIssued, 409, "invoice_not_issued"},
		{invoices.ErrStateChanged, 409, "invoice_state_changed"},
		{invoices.ErrNotFound, 404, "not_found"},
		{invoices.ErrDocumentMissing, 404, "not_found"},
		{settings.ErrMissingConfig, 422, "settings_incomplete"},
		{fmt.Errorf("%w: boom", app.ErrStorage), 503, "storage_unavailable"},
		{errors.New("db exploded: secret"), 500, "internal_error"},
	}
	for _, tc := range cases {
		rec := do(newServer(&fakeService{err: tc.err}), "POST", "/invoices/7/cancel", "")
		if rec.Code != tc.status || decode(t, rec)["error"] != tc.code {
			t.Errorf("%v: status %d body %s, want %d %s", tc.err, rec.Code, rec.Body, tc.status, tc.code)
		}
	}
	rec := do(newServer(&fakeService{err: errors.New("db exploded: secret")}), "POST", "/invoices/7/cancel", "")
	if strings.Contains(rec.Body.String(), "secret") {
		t.Error("internal errors must not leak details")
	}
	if rec := do(newServer(&fakeService{err: invoices.ErrNotFound}), "GET", "/invoices/7/documents/1", ""); rec.Code != http.StatusNotFound {
		t.Errorf("download of a missing document = %d", rec.Code)
	}
}
