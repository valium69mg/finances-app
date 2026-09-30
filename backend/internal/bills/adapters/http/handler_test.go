package billshttp_test

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

	billshttp "github.com/valium69mg/finances-app/backend/internal/bills/adapters/http"
	"github.com/valium69mg/finances-app/backend/internal/bills/app"
	bills "github.com/valium69mg/finances-app/backend/internal/bills/domain"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

func d(s string) *decimal.Decimal {
	v := decimal.RequireFromString(s)
	return &v
}

type fakeService struct {
	status  app.Status
	list    []app.Status
	detail  app.Detail
	pay     app.PayResult
	err     error
	gotID   int
	gotIn   bills.Input
	gotPay  bills.PaymentInput
	gotAll  bool
	deleted int
	skipped int
}

func (f *fakeService) Create(_ context.Context, in bills.Input) (app.Status, error) {
	f.gotIn = in
	return f.status, f.err
}
func (f *fakeService) List(_ context.Context, all bool) ([]app.Status, error) {
	f.gotAll = all
	return f.list, f.err
}
func (f *fakeService) Get(_ context.Context, id int) (app.Detail, error) {
	f.gotID = id
	return f.detail, f.err
}
func (f *fakeService) Update(_ context.Context, id int, in bills.Input) (app.Status, error) {
	f.gotID, f.gotIn = id, in
	return f.status, f.err
}
func (f *fakeService) Deactivate(_ context.Context, id int) error {
	f.gotID = id
	f.deleted++
	return f.err
}
func (f *fakeService) Pay(_ context.Context, id int, in bills.PaymentInput) (app.PayResult, error) {
	f.gotID, f.gotPay = id, in
	return f.pay, f.err
}
func (f *fakeService) Skip(_ context.Context, id int) (app.Status, error) {
	f.gotID = id
	f.skipped++
	return f.status, f.err
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
	billshttp.New(svc, slog.New(slog.NewTextHandler(io.Discard, nil))).Register(mux, requireGoodToken)
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

func sampleStatus() app.Status {
	return app.Status{
		Bill: bills.Bill{
			ID: 7, Name: "Megacable", Category: "Servicios", Amount: d("550"), Currency: "MXN", Recurrence: bills.Monthly,
			NextDueDate: "2026-10-01", AnchorDay: 1, ReminderLeadDays: 3, Active: true, Notes: "n",
			CreatedAt: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
			Pending:   &bills.Occurrence{ID: 70, BillID: 7, DueDate: "2026-10-01", Status: bills.StatusPending},
		},
		Overdue: true, DaysUntilDue: -9,
	}
}

func TestRequiresAuth(t *testing.T) {
	h := newServer(&fakeService{})
	for _, tc := range []struct{ method, path string }{
		{"POST", "/bills"}, {"GET", "/bills"}, {"GET", "/bills/1"}, {"PUT", "/bills/1"},
		{"DELETE", "/bills/1"}, {"POST", "/bills/1/pay"}, {"POST", "/bills/1/skip"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, want 401", tc.method, tc.path, rec.Code)
		}
	}
}

func TestCreate(t *testing.T) {
	svc := &fakeService{status: sampleStatus()}
	rec := do(newServer(svc), "POST", "/bills", `{"name":"Megacable","category":"Servicios","amount":"550","recurrence":"monthly","next_due_date":"2026-10-01","reminder_lead_days":5,"notes":"n"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	in := svc.gotIn
	if in.Name != "Megacable" || in.Recurrence != bills.Monthly || in.Amount == nil || !in.Amount.Equal(*d("550")) ||
		in.ReminderLeadDays == nil || *in.ReminderLeadDays != 5 || !in.Active || in.NextDueDate != "2026-10-01" {
		t.Errorf("input = %+v", in)
	}
	body := decode(t, rec)
	if body["id"] != float64(7) || body["amount"] != "550" || body["overdue"] != true || body["due_soon"] != false ||
		body["days_until_due"] != float64(-9) || body["created_at"] != "2026-09-01T12:00:00Z" || body["recurrence"] != "monthly" {
		t.Errorf("body = %v", body)
	}
	po, _ := body["pending_occurrence"].(map[string]any)
	if po["due_date"] != "2026-10-01" || po["status"] != "pending" || po["paid_on"] != nil || po["expense_movement_id"] != nil {
		t.Errorf("pending_occurrence = %v", po)
	}
}

func TestCreateVariableBillAndInactive(t *testing.T) {
	svc := &fakeService{status: sampleStatus()}
	rec := do(newServer(svc), "POST", "/bills", `{"name":"Luz","category":"Servicios","amount":null,"recurrence":"bimonthly","next_due_date":"2026-10-01","active":false}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if svc.gotIn.Amount != nil || svc.gotIn.Active || svc.gotIn.ReminderLeadDays != nil {
		t.Errorf("input = %+v", svc.gotIn)
	}
}

func TestVariableBillSerializesNullAmount(t *testing.T) {
	st := sampleStatus()
	st.Bill.Amount = nil
	rec := do(newServer(&fakeService{status: st}), "POST", "/bills", `{"name":"Luz"}`)
	body := decode(t, rec)
	if v, ok := body["amount"]; !ok || v != nil {
		t.Errorf("amount = %v (present %v), want null", v, ok)
	}
}

func TestBadBodiesAreRejected(t *testing.T) {
	h := newServer(&fakeService{})
	for _, body := range []string{``, `{`, `{"unknown":1}`, `{"name":"x","amount":"abc"}`, `[]`} {
		if rec := do(h, "POST", "/bills", body); rec.Code != http.StatusBadRequest {
			t.Errorf("body %q = %d, want 400", body, rec.Code)
		}
	}
}

func TestList(t *testing.T) {
	soon := sampleStatus()
	soon.Overdue, soon.DueSoon, soon.DaysUntilDue = false, true, 2
	svc := &fakeService{list: []app.Status{sampleStatus(), soon}}
	rec := do(newServer(svc), "GET", "/bills?include_inactive=true", "")
	if rec.Code != http.StatusOK || !svc.gotAll {
		t.Fatalf("status = %d, includeInactive = %v", rec.Code, svc.gotAll)
	}
	var out []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || len(out) != 2 {
		t.Fatalf("body = %s (%v)", rec.Body, err)
	}
	if out[1]["due_soon"] != true || out[1]["days_until_due"] != float64(2) {
		t.Errorf("second = %v", out[1])
	}
}

func TestListEmptyIsAnArray(t *testing.T) {
	rec := do(newServer(&fakeService{}), "GET", "/bills", "")
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("body = %q, want []", rec.Body)
	}
}

func TestListRejectsABadFlag(t *testing.T) {
	if rec := do(newServer(&fakeService{}), "GET", "/bills?include_inactive=maybe", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestGetWithHistory(t *testing.T) {
	resolved := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)
	expense := 42
	svc := &fakeService{detail: app.Detail{
		Status: sampleStatus(),
		History: []bills.Occurrence{
			{ID: 60, DueDate: "2026-09-01", Status: bills.StatusPaid, PaidOn: "2026-09-03", ExpenseID: &expense, AmountPaid: d("499.5"), Currency: "MXN", ResolvedAt: &resolved},
			{ID: 50, DueDate: "2026-08-01", Status: bills.StatusSkipped, ResolvedAt: &resolved},
		},
	}}
	rec := do(newServer(svc), "GET", "/bills/7", "")
	if rec.Code != http.StatusOK || svc.gotID != 7 {
		t.Fatalf("status = %d, id %d", rec.Code, svc.gotID)
	}
	body := decode(t, rec)
	history, _ := body["history"].([]any)
	if len(history) != 2 {
		t.Fatalf("history = %v", body["history"])
	}
	paid, skipped := history[0].(map[string]any), history[1].(map[string]any)
	if paid["status"] != "paid" || paid["paid_on"] != "2026-09-03" || paid["expense_movement_id"] != float64(42) ||
		paid["amount_paid"] != "499.5" || paid["currency"] != "MXN" || paid["resolved_at"] != "2026-09-03T10:00:00Z" {
		t.Errorf("paid = %v", paid)
	}
	if skipped["status"] != "skipped" || skipped["amount_paid"] != nil || skipped["paid_on"] != nil || skipped["expense_movement_id"] != nil {
		t.Errorf("skipped = %v", skipped)
	}
}

func TestGetEmptyHistoryIsAnArray(t *testing.T) {
	svc := &fakeService{detail: app.Detail{Status: sampleStatus()}}
	body := decode(t, do(newServer(svc), "GET", "/bills/7", ""))
	if h, ok := body["history"].([]any); !ok || len(h) != 0 {
		t.Errorf("history = %v, want []", body["history"])
	}
}

func TestBadIDIsNotFound(t *testing.T) {
	h := newServer(&fakeService{})
	for _, tc := range []struct{ method, path string }{
		{"GET", "/bills/abc"}, {"GET", "/bills/0"}, {"PUT", "/bills/-1"}, {"DELETE", "/bills/x"},
		{"POST", "/bills/x/pay"}, {"POST", "/bills/0/skip"},
	} {
		if rec := do(h, tc.method, tc.path, `{}`); rec.Code != http.StatusNotFound {
			t.Errorf("%s %s = %d, want 404", tc.method, tc.path, rec.Code)
		}
	}
}

func TestUpdate(t *testing.T) {
	svc := &fakeService{status: sampleStatus()}
	rec := do(newServer(svc), "PUT", "/bills/7", `{"name":"Megacable","category":"Servicios","recurrence":"yearly","next_due_date":"2027-01-31"}`)
	if rec.Code != http.StatusOK || svc.gotID != 7 || svc.gotIn.Recurrence != bills.Yearly || svc.gotIn.NextDueDate != "2027-01-31" || !svc.gotIn.Active {
		t.Errorf("status %d id %d input %+v", rec.Code, svc.gotID, svc.gotIn)
	}
}

func TestDeactivate(t *testing.T) {
	svc := &fakeService{}
	rec := do(newServer(svc), "DELETE", "/bills/7", "")
	if rec.Code != http.StatusNoContent || svc.deleted != 1 || svc.gotID != 7 || rec.Body.Len() != 0 {
		t.Errorf("status %d deleted %d id %d body %q", rec.Code, svc.deleted, svc.gotID, rec.Body)
	}
}

func TestPay(t *testing.T) {
	next := sampleStatus()
	next.Bill.Pending = &bills.Occurrence{ID: 71, DueDate: "2026-11-01", Status: bills.StatusPending}
	next.Bill.NextDueDate, next.Overdue, next.DaysUntilDue = "2026-11-01", false, 22
	svc := &fakeService{pay: app.PayResult{
		Status: next,
		Paid:   bills.Occurrence{ID: 70, DueDate: "2026-10-01", Status: bills.StatusPaid, PaidOn: "2026-10-10", AmountPaid: d("499.99"), Currency: "MXN"},
		Expense: ledger.Movement{ID: 101, Date: "2026-10-10", Description: "Megacable", Category: "Servicios", Currency: "MXN",
			Amount: *d("499.99"), AmountMXN: *d("499.99")},
	}}
	rec := do(newServer(svc), "POST", "/bills/7/pay", `{"date":"2026-10-10","amount":"499.99","category":"Hogar","description":"descuento"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	p := svc.gotPay
	if svc.gotID != 7 || p.Date != "2026-10-10" || p.Amount == nil || !p.Amount.Equal(*d("499.99")) || p.Category != "Hogar" || p.Description != "descuento" {
		t.Errorf("payment = %+v (id %d)", p, svc.gotID)
	}
	body := decode(t, rec)
	bill, _ := body["bill"].(map[string]any)
	paid, _ := body["paid_occurrence"].(map[string]any)
	expense, _ := body["expense"].(map[string]any)
	if bill["next_due_date"] != "2026-11-01" || bill["overdue"] != false {
		t.Errorf("bill = %v", bill)
	}
	if paid["status"] != "paid" || paid["amount_paid"] != "499.99" {
		t.Errorf("paid = %v", paid)
	}
	if expense["id"] != float64(101) || expense["amount_mxn"] != "499.99" || expense["category"] != "Servicios" {
		t.Errorf("expense = %v", expense)
	}
}

func TestPayAcceptsAnEmptyObjectAndNumericAmounts(t *testing.T) {
	svc := &fakeService{}
	if rec := do(newServer(svc), "POST", "/bills/7/pay", `{}`); rec.Code != http.StatusOK || svc.gotPay.Amount != nil {
		t.Errorf("empty: status %d payment %+v", rec.Code, svc.gotPay)
	}
	if rec := do(newServer(svc), "POST", "/bills/7/pay", `{"amount":312.4}`); rec.Code != http.StatusOK || svc.gotPay.Amount == nil || !svc.gotPay.Amount.Equal(*d("312.4")) {
		t.Errorf("numeric: status %d payment %+v", rec.Code, svc.gotPay)
	}
	if rec := do(newServer(svc), "POST", "/bills/7/pay", `{"nope":1}`); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown field: status %d", rec.Code)
	}
}

func TestSkip(t *testing.T) {
	svc := &fakeService{status: sampleStatus()}
	rec := do(newServer(svc), "POST", "/bills/7/skip", "")
	if rec.Code != http.StatusOK || svc.skipped != 1 || svc.gotID != 7 {
		t.Errorf("status %d skipped %d id %d", rec.Code, svc.skipped, svc.gotID)
	}
}

func TestErrorMapping(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{bills.ErrInvalidInput, 400, "invalid_bill"},
		{ledger.ErrInvalid, 400, "invalid_expense"},
		{bills.ErrNotFound, 404, "not_found"},
		{bills.ErrNotPending, 409, "occurrence_resolved"},
		{bills.ErrInactive, 409, "bill_inactive"},
		{settings.ErrMissingConfig, 422, "settings_incomplete"},
		{errors.New("db down"), 500, "internal_error"},
	} {
		h := newServer(&fakeService{err: tc.err})
		for _, req := range []struct{ method, path, body string }{
			{"POST", "/bills", `{}`}, {"GET", "/bills", ""}, {"GET", "/bills/1", ""}, {"PUT", "/bills/1", `{}`},
			{"DELETE", "/bills/1", ""}, {"POST", "/bills/1/pay", `{}`}, {"POST", "/bills/1/skip", ""},
		} {
			rec := do(h, req.method, req.path, req.body)
			if rec.Code != tc.status {
				t.Errorf("%v on %s %s = %d, want %d", tc.err, req.method, req.path, rec.Code, tc.status)
				continue
			}
			if got := decode(t, rec)["error"]; got != tc.code {
				t.Errorf("%v on %s %s code = %v, want %s", tc.err, req.method, req.path, got, tc.code)
			}
		}
	}
}

func TestInternalErrorsDoNotLeakDetails(t *testing.T) {
	rec := do(newServer(&fakeService{err: errors.New("password=secret")}), "GET", "/bills", "")
	if strings.Contains(rec.Body.String(), "secret") {
		t.Errorf("body leaks: %s", rec.Body)
	}
}
