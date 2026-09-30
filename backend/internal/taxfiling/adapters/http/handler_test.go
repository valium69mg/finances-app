package taxfilinghttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	invoices "github.com/valium69mg/finances-app/backend/internal/invoices/domain"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
	taxfilinghttp "github.com/valium69mg/finances-app/backend/internal/taxfiling/adapters/http"
	"github.com/valium69mg/finances-app/backend/internal/taxfiling/app"
	taxfiling "github.com/valium69mg/finances-app/backend/internal/taxfiling/domain"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

type fakeService struct {
	preview app.Preview
	result  app.Result
	detail  app.Detail
	list    []taxfiling.Filing
	pending []taxfiling.PendingPeriod
	err     error

	gotPeriod   string
	gotCredit   decimal.Decimal
	gotRegister app.RegisterInput
	gotPayment  app.PaymentInput
	gotYear     int
	gotStatus   taxfiling.PaymentStatus
	deleted     string
}

func (f *fakeService) Preview(_ context.Context, period string, credit decimal.Decimal) (app.Preview, error) {
	f.gotPeriod, f.gotCredit = period, credit
	return f.preview, f.err
}
func (f *fakeService) Register(_ context.Context, in app.RegisterInput) (app.Result, error) {
	f.gotRegister = in
	return f.result, f.err
}
func (f *fakeService) Pay(_ context.Context, period string, in app.PaymentInput) (app.Result, error) {
	f.gotPeriod, f.gotPayment = period, in
	return f.result, f.err
}
func (f *fakeService) Get(_ context.Context, period string) (app.Detail, error) {
	f.gotPeriod = period
	return f.detail, f.err
}
func (f *fakeService) List(_ context.Context, year int, status taxfiling.PaymentStatus) ([]taxfiling.Filing, error) {
	f.gotYear, f.gotStatus = year, status
	return f.list, f.err
}
func (f *fakeService) Pending(context.Context) ([]taxfiling.PendingPeriod, error) {
	return f.pending, f.err
}
func (f *fakeService) Delete(_ context.Context, period string) error {
	f.deleted = period
	return f.err
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
	taxfilinghttp.New(svc, slog.New(slog.NewTextHandler(io.Discard, nil))).Register(mux, requireGoodToken)
	return mux
}

func do(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer good")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return out
}

func sampleFiling() taxfiling.Filing {
	return taxfiling.Filing{
		Period: "2026-10", FilingDate: "2026-11-05", IncomeCollected: d("92262.41"), ISRRate: d("0.02"),
		ISRAccrued: d("1845.25"), ISRDue: d("1845.25"), IVATransferred: d("4827.59"), IVADue: d("4827.59"),
		Folio: "ACUSE-1", InvoiceIDs: []int{1, 2}, CreatedAt: time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC),
	}
}

func TestRequiresAuth(t *testing.T) {
	h := newServer(&fakeService{})
	for _, tc := range []struct{ method, path string }{
		{"GET", "/tax-filing/preview"}, {"POST", "/tax-filing"}, {"GET", "/tax-filing"}, {"GET", "/tax-filing/pending-periods"},
		{"GET", "/tax-filing/2026-10"}, {"POST", "/tax-filing/2026-10/payment"}, {"DELETE", "/tax-filing/2026-10"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, want 401", tc.method, tc.path, rec.Code)
		}
	}
}

func TestPreview(t *testing.T) {
	usd := d("17.74")
	svc := &fakeService{preview: app.Preview{
		Declaration: taxfiling.Declaration{
			Period: "2026-10", DueDate: "2026-11-17", IncomeCollected: d("92262.41"), ISRRate: d("0.02"), ISRAccrued: d("1845.25"),
			ISRToPay: d("1845.25"), IVATransferred: d("4827.59"), IVAPayable: d("4827.59"), ExportBase: d("62090"),
			Invoices: []invoices.Invoice{{
				ID: 1, ClientID: "usa", CollectionDate: "2026-10-15", Currency: "USD", ExchangeRate: &usd, Status: invoices.StatusIssued,
				UUID: "6F1C2B3A-4D5E-4F60-8A7B-9C0D1E2F3A4B", Amounts: invoices.Amounts{SubtotalMXN: d("62090")},
			}},
		},
		Warnings: []app.Warning{{Code: app.WarningPreparedInvoices, Message: "m", InvoiceIDs: []int{3}}},
	}}
	rec := do(newServer(svc), "GET", "/tax-filing/preview?period=2026-10&iva_acreditable=100.50", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if svc.gotPeriod != "2026-10" || !svc.gotCredit.Equal(d("100.5")) {
		t.Errorf("service got %q, %s", svc.gotPeriod, svc.gotCredit)
	}
	body := decode(t, rec)
	for k, want := range map[string]string{
		"period": "2026-10", "due_date": "2026-11-17", "income_collected": "92262.41", "isr_rate": "0.02", "isr_accrued": "1845.25",
		"isr_due": "1845.25", "iva_due": "4827.59", "total_to_pay": "6672.84", "export_base": "62090", "iva_acreditable": "0",
	} {
		if body[k] != want {
			t.Errorf("%s = %v, want %s", k, body[k], want)
		}
	}
	invs := body["invoices"].([]any)
	if len(invs) != 1 || invs[0].(map[string]any)["state"] != "emitida" || invs[0].(map[string]any)["subtotal_mxn"] != "62090" {
		t.Errorf("invoices = %v", invs)
	}
	ws := body["warnings"].([]any)
	if len(ws) != 1 || ws[0].(map[string]any)["code"] != "prepared_invoices" {
		t.Errorf("warnings = %v", ws)
	}
	if body["filing"] != nil {
		t.Errorf("filing = %v, want null", body["filing"])
	}
}

func TestPreviewEmptyListsAreArrays(t *testing.T) {
	rec := do(newServer(&fakeService{}), "GET", "/tax-filing/preview", "")
	body := decode(t, rec)
	if rec.Code != 200 || body["invoices"] == nil || body["warnings"] == nil {
		t.Errorf("status %d body %s; want [] for invoices and warnings", rec.Code, rec.Body)
	}
}

func TestPreviewBadCreditable(t *testing.T) {
	rec := do(newServer(&fakeService{}), "GET", "/tax-filing/preview?iva_acreditable=abc", "")
	if rec.Code != http.StatusBadRequest || decode(t, rec)["error"] != "invalid_request" {
		t.Errorf("status = %d, body %s", rec.Code, rec.Body)
	}
}

func TestRegister(t *testing.T) {
	svc := &fakeService{result: app.Result{Filing: sampleFiling(), Warnings: []app.Warning{{Code: app.WarningPreparedInvoices, InvoiceIDs: []int{3}}}}}
	body := `{"period":"2026-10","filing_date":"2026-11-05","folio":"ACUSE-1","iva_acreditable":"800.5",
		"payment":{"date":"2026-11-07","isr_paid":"1845.25","iva_paid":"4827.59","record_expense":true}}`
	rec := do(newServer(svc), "POST", "/tax-filing", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	in := svc.gotRegister
	if in.Period != "2026-10" || in.Date != "2026-11-05" || in.Folio != "ACUSE-1" || !in.IVACreditable.Equal(d("800.5")) {
		t.Errorf("register input = %+v", in)
	}
	if in.Payment == nil || in.Payment.Date != "2026-11-07" || !in.Payment.ISRPaid.Equal(d("1845.25")) || !in.Payment.RecordExpense {
		t.Errorf("payment input = %+v", in.Payment)
	}
	out := decode(t, rec)
	filing := out["filing"].(map[string]any)
	if filing["status"] != "pendiente" || filing["due_date"] != "2026-11-17" || filing["payment"] != nil || filing["total_to_pay"] != "6672.84" {
		t.Errorf("filing = %v", filing)
	}
	if ids := filing["invoice_ids"].([]any); len(ids) != 2 {
		t.Errorf("invoice_ids = %v", ids)
	}
	if len(out["warnings"].([]any)) != 1 {
		t.Errorf("warnings = %v", out["warnings"])
	}
}

func TestRegisterWithoutPaymentSendsNil(t *testing.T) {
	svc := &fakeService{result: app.Result{Filing: sampleFiling()}}
	rec := do(newServer(svc), "POST", "/tax-filing", `{"period":"2026-10"}`)
	if rec.Code != http.StatusCreated || svc.gotRegister.Payment != nil || svc.gotRegister.Date != "" {
		t.Errorf("status %d, input %+v", rec.Code, svc.gotRegister)
	}
}

func TestRegisterRejectsBadBodies(t *testing.T) {
	h := newServer(&fakeService{})
	for _, body := range []string{``, `not json`, `{"period":"2026-10","extra":1}`, `{"period":"2026-10","payment":{"isr_paid":"abc"}}`} {
		rec := do(h, "POST", "/tax-filing", body)
		if rec.Code != http.StatusBadRequest || decode(t, rec)["error"] != "invalid_request" {
			t.Errorf("body %q: status %d %s", body, rec.Code, rec.Body)
		}
	}
}

func TestPaidFilingDTO(t *testing.T) {
	expense := 44
	f := sampleFiling()
	f.Payment = &taxfiling.Payment{Date: "2026-11-07", ISRPaid: d("1845.25"), IVAPaid: d("4827.59")}
	f.ExpenseMovementID = &expense
	svc := &fakeService{result: app.Result{Filing: f}}
	rec := do(newServer(svc), "POST", "/tax-filing/2026-10/payment", `{"date":"2026-11-07","isr_paid":"1845.25","iva_paid":"4827.59","record_expense":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if svc.gotPeriod != "2026-10" || !svc.gotPayment.RecordExpense || svc.gotPayment.Date != "2026-11-07" || !svc.gotPayment.IVAPaid.Equal(d("4827.59")) {
		t.Errorf("service got %q %+v", svc.gotPeriod, svc.gotPayment)
	}
	filing := decode(t, rec)["filing"].(map[string]any)
	pay := filing["payment"].(map[string]any)
	if filing["status"] != "pagada" || pay["total_paid"] != "6672.84" || pay["date"] != "2026-11-07" || filing["expense_movement_id"] != float64(44) {
		t.Errorf("filing = %v", filing)
	}
}

func TestPayDoesNotRecordAnExpenseByDefault(t *testing.T) {
	svc := &fakeService{result: app.Result{Filing: sampleFiling()}}
	do(newServer(svc), "POST", "/tax-filing/2026-10/payment", `{"isr_paid":"1","iva_paid":"0"}`)
	if svc.gotPayment.RecordExpense {
		t.Error("record_expense must default to false")
	}
}

func TestListParsesFilters(t *testing.T) {
	svc := &fakeService{list: []taxfiling.Filing{sampleFiling()}}
	rec := do(newServer(svc), "GET", "/tax-filing?year=2026&status=pendiente", "")
	if rec.Code != 200 || svc.gotYear != 2026 || svc.gotStatus != taxfiling.PaymentPending {
		t.Errorf("status %d year %d status %q", rec.Code, svc.gotYear, svc.gotStatus)
	}
	var out []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || len(out) != 1 || out[0]["period"] != "2026-10" {
		t.Errorf("body = %s, %v", rec.Body, err)
	}

	empty := &fakeService{}
	rec = do(newServer(empty), "GET", "/tax-filing", "")
	if strings.TrimSpace(rec.Body.String()) != "[]" || empty.gotYear != 0 || empty.gotStatus != "" {
		t.Errorf("empty list body %q, year %d", rec.Body, empty.gotYear)
	}

	for _, q := range []string{"year=abc", "year=0", "year=10000"} {
		if rec := do(newServer(&fakeService{}), "GET", "/tax-filing?"+q, ""); rec.Code != http.StatusBadRequest {
			t.Errorf("%s status = %d, want 400", q, rec.Code)
		}
	}
}

func TestGetIncludesInvoices(t *testing.T) {
	svc := &fakeService{detail: app.Detail{
		Filing:   sampleFiling(),
		Invoices: []invoices.Invoice{{ID: 1, ClientID: "b", CollectionDate: "2026-10-03", Currency: "MXN", Status: invoices.StatusIssued, Amounts: invoices.Amounts{SubtotalMXN: d("100")}}},
	}}
	rec := do(newServer(svc), "GET", "/tax-filing/2026-10", "")
	if rec.Code != 200 || svc.gotPeriod != "2026-10" {
		t.Fatalf("status %d period %q", rec.Code, svc.gotPeriod)
	}
	body := decode(t, rec)
	if body["period"] != "2026-10" || body["folio"] != "ACUSE-1" || len(body["invoices"].([]any)) != 1 {
		t.Errorf("body = %v", body)
	}
}

func TestPending(t *testing.T) {
	svc := &fakeService{pending: []taxfiling.PendingPeriod{{Period: "2026-10", DueDate: "2026-11-17", Overdue: true}}}
	rec := do(newServer(svc), "GET", "/tax-filing/pending-periods", "")
	var out []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || len(out) != 1 || out[0]["overdue"] != true || out[0]["due_date"] != "2026-11-17" {
		t.Errorf("body = %s, %v", rec.Body, err)
	}
	// The literal route wins over the {period} wildcard.
	if svc.gotPeriod != "" {
		t.Errorf("pending-periods was routed to the period handler (%q)", svc.gotPeriod)
	}
	rec = do(newServer(&fakeService{}), "GET", "/tax-filing/pending-periods", "")
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("empty pending body = %q, want []", rec.Body)
	}
}

func TestDelete(t *testing.T) {
	svc := &fakeService{}
	rec := do(newServer(svc), "DELETE", "/tax-filing/2026-10", "")
	if rec.Code != http.StatusNoContent || svc.deleted != "2026-10" {
		t.Errorf("status %d deleted %q", rec.Code, svc.deleted)
	}
}

func TestErrorMapping(t *testing.T) {
	tests := []struct {
		err    error
		status int
		code   string
	}{
		{taxfiling.ErrInvalidInput, 400, "invalid_filing"},
		{ledger.ErrInvalid, 400, "invalid_expense"},
		{taxfiling.ErrAlreadyFiled, 409, "already_filed"},
		{taxfiling.ErrAlreadyPaid, 409, "already_paid"},
		{taxfiling.ErrFilingPaid, 409, "filing_paid"},
		{taxfiling.ErrInvoicesChanged, 409, "invoices_changed"},
		{taxfiling.ErrNotFound, 404, "not_found"},
		{settings.ErrMissingConfig, 422, "settings_incomplete"},
		{errors.New("boom"), 500, "internal_error"},
	}
	for _, tc := range tests {
		svc := &fakeService{err: tc.err}
		h := newServer(svc)
		for _, req := range []struct{ method, path, body string }{
			{"GET", "/tax-filing/preview", ""}, {"POST", "/tax-filing", `{"period":"2026-10"}`}, {"GET", "/tax-filing", ""},
			{"GET", "/tax-filing/2026-10", ""}, {"POST", "/tax-filing/2026-10/payment", `{}`}, {"GET", "/tax-filing/pending-periods", ""},
			{"DELETE", "/tax-filing/2026-10", ""},
		} {
			rec := do(h, req.method, req.path, req.body)
			if rec.Code != tc.status || decode(t, rec)["error"] != tc.code {
				t.Errorf("%v on %s %s: status %d body %s; want %d %s", tc.err, req.method, req.path, rec.Code, rec.Body, tc.status, tc.code)
			}
			if tc.status == 500 && strings.Contains(rec.Body.String(), "boom") {
				t.Errorf("internal errors must not leak their message: %s", rec.Body)
			}
		}
	}
}
