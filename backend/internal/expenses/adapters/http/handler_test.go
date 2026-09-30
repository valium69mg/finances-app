package expenseshttp_test

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

	"github.com/shopspring/decimal"

	expenseshttp "github.com/valium69mg/finances-app/backend/internal/expenses/adapters/http"
	"github.com/valium69mg/finances-app/backend/internal/expenses/app"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

type fakeService struct {
	result   app.Result
	list     []ledger.Movement
	inferred string
	inferOK  bool
	err      error

	gotInput  app.Input
	gotID     int
	gotMonth  string
	gotLimit  int
	gotDesc   string
	deleteID  int
	deleteRan bool
}

func (f *fakeService) Create(_ context.Context, in app.Input) (app.Result, error) {
	f.gotInput = in
	return f.result, f.err
}
func (f *fakeService) Update(_ context.Context, id int, in app.Input) (app.Result, error) {
	f.gotID, f.gotInput = id, in
	return f.result, f.err
}
func (f *fakeService) Delete(_ context.Context, id int) error {
	f.deleteID, f.deleteRan = id, true
	return f.err
}
func (f *fakeService) List(_ context.Context, month string, limit int) ([]ledger.Movement, error) {
	f.gotMonth, f.gotLimit = month, limit
	return f.list, f.err
}
func (f *fakeService) InferCategory(_ context.Context, desc string) (string, bool, error) {
	f.gotDesc = desc
	return f.inferred, f.inferOK, f.err
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
	expenseshttp.New(svc, slog.New(slog.NewTextHandler(io.Discard, nil))).Register(mux, requireGoodToken)
	return mux
}

func do(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer good")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func sampleResult() app.Result {
	rate := d("17.74")
	budget, remaining := d("1000"), d("-100")
	return app.Result{
		Movement: ledger.Movement{
			ID: 7, Date: "2026-10-15", Description: "Uber", Category: "Transporte", Kind: ledger.KindExpense,
			PaymentMethod: "Débito", Currency: "USD", Amount: d("62.03"), ExchangeRate: &rate, AmountMXN: d("1100.4122"),
		},
		Feedback: &app.BudgetFeedback{
			Month: "2026-10", Category: "Transporte", Budget: &budget, Spent: d("1100"), Remaining: &remaining, OverBudget: true,
		},
	}
}

func TestRoutesRequireAuth(t *testing.T) {
	h := newServer(&fakeService{})
	for _, tc := range []struct{ method, path string }{
		{"POST", "/expenses"}, {"GET", "/expenses"}, {"GET", "/expenses/infer-category"},
		{"PUT", "/expenses/1"}, {"DELETE", "/expenses/1"},
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
	svc := &fakeService{result: sampleResult()}
	rec := do(newServer(svc), "POST", "/expenses",
		`{"date":"2026-10-15","description":"Uber","category":"Transporte","payment_method":"Débito","currency":"USD","amount":"62.03","exchange_rate":"17.74"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("missing Cache-Control: no-store")
	}
	in := svc.gotInput
	if in.Date != "2026-10-15" || in.Category != "Transporte" || in.Currency != "USD" || !in.Amount.Equal(d("62.03")) ||
		in.ExchangeRate == nil || !in.ExchangeRate.Equal(d("17.74")) {
		t.Errorf("input %+v", in)
	}
	want := `{"expense":{"id":7,"date":"2026-10-15","description":"Uber","category":"Transporte","payment_method":"Débito","currency":"USD","amount":"62.03","exchange_rate":"17.74","amount_mxn":"1100.4122"},` +
		`"budget":{"month":"2026-10","category":"Transporte","budget":"1000","spent":"1100","remaining":"-100","over_budget":true}}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Errorf("body\n got %s\nwant %s", got, want)
	}
}

func TestCreateMinimalBodyAndNullBudget(t *testing.T) {
	res := app.Result{Movement: ledger.Movement{ID: 1, Date: "2026-10-15", Category: "Ocio", Kind: ledger.KindExpense,
		PaymentMethod: "Débito", Currency: "MXN", Amount: d("5"), AmountMXN: d("5")}}
	svc := &fakeService{result: res}
	rec := do(newServer(svc), "POST", "/expenses", `{"amount":"5","description":"cine"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if string(body["budget"]) != "null" {
		t.Errorf("budget = %s, want null", body["budget"])
	}
	if svc.gotInput.Category != "" || svc.gotInput.ExchangeRate != nil {
		t.Errorf("input %+v", svc.gotInput)
	}
}

func TestBadBodies(t *testing.T) {
	h := newServer(&fakeService{})
	for _, body := range []string{`{`, `{"amount":"1","unknown":1}`, `{"amount":"abc"}`, ``} {
		rec := do(h, "POST", "/expenses", body)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_request") {
			t.Errorf("body %q: %d %s", body, rec.Code, rec.Body)
		}
	}
}

func TestErrorMapping(t *testing.T) {
	invalid := `{"error":"invalid_expense","message":"invalid movement: amount must not be zero"}`
	tests := []struct {
		name   string
		err    error
		status int
		body   string
	}{
		{"unexpected", errors.New("x"), http.StatusInternalServerError, `{"error":"internal_error"}`},
		{"validation", ledgerInvalid(), http.StatusBadRequest, invalid},
		{"not found", ledger.ErrNotFound, http.StatusNotFound, `{"error":"not_found"}`},
	}
	for _, tc := range tests {
		rec := do(newServer(&fakeService{err: tc.err}), "POST", "/expenses", `{"amount":"1"}`)
		if rec.Code != tc.status || strings.TrimSpace(rec.Body.String()) != tc.body {
			t.Errorf("%s: %d %s", tc.name, rec.Code, rec.Body)
		}
	}
}

func ledgerInvalid() error {
	_, err := ledger.NewMovement(ledger.MovementInput{Kind: ledger.KindExpense, Category: "A"},
		ledger.Catalog{Categories: []ledger.CatalogCategory{{Name: "A", Kind: ledger.KindExpense}}, PaymentMethods: []string{"Débito"}}, "2026-10-01")
	return err
}

func TestUpdate(t *testing.T) {
	svc := &fakeService{result: sampleResult()}
	rec := do(newServer(svc), "PUT", "/expenses/7", `{"category":"Ocio","amount":"10"}`)
	if rec.Code != http.StatusOK || svc.gotID != 7 || svc.gotInput.Category != "Ocio" {
		t.Errorf("%d id=%d input=%+v", rec.Code, svc.gotID, svc.gotInput)
	}
	for _, path := range []string{"/expenses/abc", "/expenses/0", "/expenses/-3"} {
		if rec := do(newServer(svc), "PUT", path, `{"amount":"1"}`); rec.Code != http.StatusNotFound {
			t.Errorf("PUT %s = %d, want 404", path, rec.Code)
		}
	}
	missing := &fakeService{err: ledger.ErrNotFound}
	if rec := do(newServer(missing), "PUT", "/expenses/9", `{"amount":"1"}`); rec.Code != http.StatusNotFound {
		t.Errorf("missing = %d, want 404", rec.Code)
	}
}

func TestDelete(t *testing.T) {
	svc := &fakeService{}
	rec := do(newServer(svc), "DELETE", "/expenses/12", "")
	if rec.Code != http.StatusNoContent || svc.deleteID != 12 || rec.Body.Len() != 0 {
		t.Errorf("%d id=%d body=%q", rec.Code, svc.deleteID, rec.Body)
	}
	if rec := do(newServer(&fakeService{err: ledger.ErrNotFound}), "DELETE", "/expenses/12", ""); rec.Code != http.StatusNotFound {
		t.Errorf("missing = %d, want 404", rec.Code)
	}
	if rec := do(newServer(svc), "DELETE", "/expenses/x", ""); rec.Code != http.StatusNotFound {
		t.Errorf("bad id = %d, want 404", rec.Code)
	}
}

func TestList(t *testing.T) {
	svc := &fakeService{list: []ledger.Movement{sampleResult().Movement}}
	rec := do(newServer(svc), "GET", "/expenses?month=2026-10&limit=5", "")
	if rec.Code != http.StatusOK || svc.gotMonth != "2026-10" || svc.gotLimit != 5 {
		t.Fatalf("%d month=%q limit=%d", rec.Code, svc.gotMonth, svc.gotLimit)
	}
	var out []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || len(out) != 1 || out[0]["amount"] != "62.03" {
		t.Errorf("body %s (%v)", rec.Body, err)
	}

	svc = &fakeService{}
	rec = do(newServer(svc), "GET", "/expenses", "")
	if rec.Code != http.StatusOK || svc.gotMonth != "" || svc.gotLimit != 0 || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("defaults: %d month=%q limit=%d body=%s", rec.Code, svc.gotMonth, svc.gotLimit, rec.Body)
	}

	for _, q := range []string{"limit=0", "limit=-1", "limit=abc", "limit=1000"} {
		if rec := do(newServer(svc), "GET", "/expenses?"+q, ""); rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", q, rec.Code)
		}
	}
	if rec := do(newServer(&fakeService{err: ledgerInvalid()}), "GET", "/expenses?month=bad", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("bad month = %d, want 400", rec.Code)
	}
}

func TestInferCategory(t *testing.T) {
	svc := &fakeService{inferred: "Mandado", inferOK: true}
	rec := do(newServer(svc), "GET", "/expenses/infer-category?description=walmart", "")
	if rec.Code != http.StatusOK || svc.gotDesc != "walmart" || strings.TrimSpace(rec.Body.String()) != `{"category":"Mandado"}` {
		t.Errorf("%d desc=%q body=%s", rec.Code, svc.gotDesc, rec.Body)
	}
	rec = do(newServer(&fakeService{}), "GET", "/expenses/infer-category?description=zzz", "")
	if strings.TrimSpace(rec.Body.String()) != `{"category":null}` {
		t.Errorf("no match body %s", rec.Body)
	}
}
