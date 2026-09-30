package futureexpenseshttp_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	futureexpenseshttp "github.com/valium69mg/finances-app/backend/internal/futureexpenses/adapters/http"
	"github.com/valium69mg/finances-app/backend/internal/futureexpenses/app"
	domain "github.com/valium69mg/finances-app/backend/internal/futureexpenses/domain"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

type fakeService struct {
	item     domain.Planned
	movement ledger.Movement
	listing  app.Listing
	err      error

	gotID     int
	gotInput  domain.Input
	gotSaving app.SavingInput
	gotAssign app.AssignInput
	gotPay    app.PayInput
	deleted   int
}

func (f *fakeService) Create(_ context.Context, in domain.Input) (domain.Planned, error) {
	f.gotInput = in
	return f.item, f.err
}
func (f *fakeService) Get(_ context.Context, id int) (domain.Planned, error) {
	f.gotID = id
	return f.item, f.err
}
func (f *fakeService) Update(_ context.Context, id int, in domain.Input) (domain.Planned, error) {
	f.gotID, f.gotInput = id, in
	return f.item, f.err
}
func (f *fakeService) Delete(_ context.Context, id int) error {
	f.deleted = id
	return f.err
}
func (f *fakeService) List(context.Context) (app.Listing, error) { return f.listing, f.err }
func (f *fakeService) Contribute(_ context.Context, id int, in app.SavingInput) (domain.Planned, ledger.Movement, error) {
	f.gotID, f.gotSaving = id, in
	return f.item, f.movement, f.err
}
func (f *fakeService) Assign(_ context.Context, id int, in app.AssignInput) (domain.Planned, error) {
	f.gotID, f.gotAssign = id, in
	return f.item, f.err
}
func (f *fakeService) Pay(_ context.Context, id int, in app.PayInput) (domain.Planned, ledger.Movement, error) {
	f.gotID, f.gotPay = id, in
	return f.item, f.movement, f.err
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

func server(svc *fakeService) http.Handler {
	mux := http.NewServeMux()
	futureexpenseshttp.New(svc, slog.New(slog.NewTextHandler(io.Discard, nil))).Register(mux, requireGoodToken)
	return mux
}

func do(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer good")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

var created = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func laptop() domain.Planned {
	return domain.Planned{
		FutureExpense: domain.FutureExpense{
			ID: 3, Name: "Laptop", Target: d("8000"), DueDate: "2027-01-20", Status: domain.StatusActive, Saved: d("2000"),
			CreatedAt: created, UpdatedAt: created,
		},
		Remaining: d("6000"), Suggested: d("1500"), CyclesLeft: 4,
	}
}

const laptopJSON = `{"id":3,"name":"Laptop","target_amount":"8000","due_date":"2027-01-20","status":"active","saved":"2000","remaining":"6000","suggested_monthly":"1500","cycles_left":4,"paid_at":null,"amount_paid":null,"expense_movement_id":null,"created_at":"2026-10-01T12:00:00Z","updated_at":"2026-10-01T12:00:00Z"}`

func TestRoutesRequireAuth(t *testing.T) {
	h := server(&fakeService{})
	for _, r := range []struct{ method, path string }{
		{"GET", "/future-expenses"}, {"POST", "/future-expenses"}, {"GET", "/future-expenses/1"}, {"PUT", "/future-expenses/1"},
		{"DELETE", "/future-expenses/1"}, {"POST", "/future-expenses/1/savings"}, {"POST", "/future-expenses/1/assign"},
		{"POST", "/future-expenses/1/pay"},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(r.method, r.path, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: status %d, want 401", r.method, r.path, rec.Code)
		}
	}
}

func TestCreate(t *testing.T) {
	svc := &fakeService{item: laptop()}
	rec := do(server(svc), "POST", "/future-expenses", `{"name":"Laptop","target_amount":"8000","due_date":"2027-01-20"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if svc.gotInput.Name != "Laptop" || !svc.gotInput.Target.Equal(d("8000")) || svc.gotInput.DueDate != "2027-01-20" {
		t.Errorf("input %+v", svc.gotInput)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("missing Cache-Control: no-store")
	}
	if got := strings.TrimSpace(rec.Body.String()); got != laptopJSON {
		t.Errorf("body\n got %s\nwant %s", got, laptopJSON)
	}
}

func TestBadBodies(t *testing.T) {
	h := server(&fakeService{})
	for _, body := range []string{``, `{`, `{"target_amount":"abc"}`, `{"name":"a","unknown":1}`} {
		if rec := do(h, "POST", "/future-expenses", body); rec.Code != http.StatusBadRequest {
			t.Errorf("body %q: status %d, want 400", body, rec.Code)
		}
	}
	if rec := do(h, "GET", "/future-expenses/abc", ""); rec.Code != http.StatusNotFound {
		t.Errorf("bad id: status %d, want 404", rec.Code)
	}
}

func TestErrorMapping(t *testing.T) {
	tests := []struct {
		err    error
		status int
		code   string
	}{
		{fmt.Errorf("%w: name is required", domain.ErrInvalidInput), 400, "invalid_future_expense"},
		{fmt.Errorf("%w: amount must not be zero", ledger.ErrInvalid), 400, "invalid_movement"},
		{domain.ErrNotFound, 404, "not_found"},
		{domain.ErrAlreadyPaid, 409, "already_paid"},
		{domain.ErrInsufficientFreeBalance, 409, "insufficient_free_balance"},
		{settings.ErrMissingConfig, 422, "settings_incomplete"},
		{errors.New("boom"), 500, "internal_error"},
	}
	for _, tt := range tests {
		rec := do(server(&fakeService{err: tt.err}), "POST", "/future-expenses/1/pay", `{}`)
		if rec.Code != tt.status || !strings.Contains(rec.Body.String(), `"error":"`+tt.code+`"`) {
			t.Errorf("%v: status %d body %s, want %d %s", tt.err, rec.Code, rec.Body, tt.status, tt.code)
		}
	}
	if rec := do(server(&fakeService{err: errors.New("secret detail")}), "GET", "/future-expenses/1", ""); strings.Contains(rec.Body.String(), "secret detail") {
		t.Error("an internal error leaked its message")
	}
}

func TestUpdateAndGet(t *testing.T) {
	svc := &fakeService{item: laptop()}
	h := server(svc)
	rec := do(h, "PUT", "/future-expenses/3", `{"name":"Laptop","target_amount":"8500.50","due_date":"2027-02-01"}`)
	if rec.Code != http.StatusOK || svc.gotID != 3 || !svc.gotInput.Target.Equal(d("8500.50")) {
		t.Errorf("PUT status %d id %d input %+v", rec.Code, svc.gotID, svc.gotInput)
	}
	rec = do(h, "GET", "/future-expenses/3", "")
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != laptopJSON {
		t.Errorf("GET status %d body %s", rec.Code, rec.Body)
	}
}

func TestDelete(t *testing.T) {
	svc := &fakeService{}
	rec := do(server(svc), "DELETE", "/future-expenses/3", "")
	if rec.Code != http.StatusNoContent || svc.deleted != 3 || rec.Body.Len() != 0 {
		t.Errorf("status %d deleted %d body %q", rec.Code, svc.deleted, rec.Body)
	}
}

func TestList(t *testing.T) {
	paid := domain.Planned{FutureExpense: domain.FutureExpense{
		ID: 5, Name: "Old", Target: d("100"), DueDate: "2026-09-01", Status: domain.StatusPaid, PaidAt: "2026-09-02",
		AmountPaid: func() *decimal.Decimal { v := d("90"); return &v }(), ExpenseMovementID: func() *int { v := 41; return &v }(),
		CreatedAt: created, UpdatedAt: created,
	}}
	svc := &fakeService{listing: app.Listing{
		Plan: domain.Plan{
			Items:  []domain.Planned{laptop()},
			Target: d("8000"), Saved: d("2000"), Remaining: d("6000"), Suggested: d("1500"), FreeBalance: d("40.25"),
		},
		Paid: []domain.Planned{paid},
	}}
	rec := do(server(svc), "GET", "/future-expenses", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	want := `{"active":[` + laptopJSON + `],` +
		`"paid":[{"id":5,"name":"Old","target_amount":"100","due_date":"2026-09-01","status":"paid","saved":"0","remaining":"0","suggested_monthly":"0","cycles_left":0,"paid_at":"2026-09-02","amount_paid":"90","expense_movement_id":41,"created_at":"2026-10-01T12:00:00Z","updated_at":"2026-10-01T12:00:00Z"}],` +
		`"totals":{"target":"8000","saved":"2000","remaining":"6000","suggested_monthly":"1500"},"free_balance":"40.25"}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Errorf("body\n got %s\nwant %s", got, want)
	}
}

func TestListEmptyIsArrays(t *testing.T) {
	rec := do(server(&fakeService{listing: app.Listing{Plan: domain.Plan{Items: []domain.Planned{}}, Paid: []domain.Planned{}}}), "GET", "/future-expenses", "")
	if !strings.Contains(rec.Body.String(), `"active":[]`) || !strings.Contains(rec.Body.String(), `"paid":[]`) {
		t.Errorf("body %s, want empty arrays, never null", rec.Body)
	}
}

func TestContribute(t *testing.T) {
	svc := &fakeService{
		item:     laptop(),
		movement: ledger.Movement{ID: 12, Date: "2026-10-02", Description: "Ahorro para Laptop", Category: "Gastos futuros", Amount: d("500"), AmountMXN: d("500")},
	}
	rec := do(server(svc), "POST", "/future-expenses/3/savings", `{"amount":"500","date":"2026-10-02"}`)
	if rec.Code != http.StatusCreated || svc.gotID != 3 || !svc.gotSaving.Amount.Equal(d("500")) || svc.gotSaving.Date != "2026-10-02" {
		t.Fatalf("status %d id %d input %+v: %s", rec.Code, svc.gotID, svc.gotSaving, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"saving":{"id":12,"date":"2026-10-02","description":"Ahorro para Laptop","category":"Gastos futuros","amount":"500","amount_mxn":"500"}`) {
		t.Errorf("body %s", rec.Body)
	}
}

func TestAssign(t *testing.T) {
	svc := &fakeService{item: laptop()}
	rec := do(server(svc), "POST", "/future-expenses/3/assign", `{"amount":"120.40"}`)
	if rec.Code != http.StatusOK || svc.gotID != 3 || !svc.gotAssign.Amount.Equal(d("120.40")) {
		t.Errorf("status %d id %d input %+v", rec.Code, svc.gotID, svc.gotAssign)
	}
}

func TestPay(t *testing.T) {
	svc := &fakeService{
		item:     laptop(),
		movement: ledger.Movement{ID: 44, Date: "2026-10-05", Description: "Laptop", Category: "Ocio", Amount: d("7900"), AmountMXN: d("7900")},
	}
	rec := do(server(svc), "POST", "/future-expenses/3/pay", `{"amount":"7900","date":"2026-10-05","category":"Ocio"}`)
	if rec.Code != http.StatusOK || svc.gotID != 3 || svc.gotPay.Amount == nil || !svc.gotPay.Amount.Equal(d("7900")) || svc.gotPay.Category != "Ocio" || svc.gotPay.Date != "2026-10-05" {
		t.Fatalf("status %d id %d input %+v: %s", rec.Code, svc.gotID, svc.gotPay, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"expense":{"id":44,"date":"2026-10-05","description":"Laptop","category":"Ocio","amount":"7900","amount_mxn":"7900"}`) {
		t.Errorf("body %s", rec.Body)
	}
	// An empty body is valid: everything defaults.
	svc2 := &fakeService{item: laptop()}
	if rec := do(server(svc2), "POST", "/future-expenses/3/pay", `{}`); rec.Code != http.StatusOK || svc2.gotPay.Amount != nil {
		t.Errorf("empty pay: status %d input %+v", rec.Code, svc2.gotPay)
	}
}
