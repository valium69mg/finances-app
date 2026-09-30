package incomehttp_test

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

	incomehttp "github.com/valium69mg/finances-app/backend/internal/income/adapters/http"
	"github.com/valium69mg/finances-app/backend/internal/income/app"
	income "github.com/valium69mg/finances-app/backend/internal/income/domain"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	savings "github.com/valium69mg/finances-app/backend/internal/savings/domain"
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
	incomehttp.New(svc, slog.New(slog.NewTextHandler(io.Discard, nil))).Register(mux, requireGoodToken)
	return mux
}

func do(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer good")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func sueldoResult() app.Result {
	rate, prev := d("17.74"), d("0.01")
	return app.Result{
		Movement: ledger.Movement{
			ID: 7, Date: "2026-10-01", Description: "Sueldo", Category: "Sueldo", Kind: ledger.KindIncome,
			PaymentMethod: "Transferencia", Currency: "USD", Amount: d("3383.33"), ExchangeRate: &rate, AmountMXN: d("60020.27"),
		},
		Summary: app.Summary{
			Month: "2026-10", MonthTotalMXN: d("60020.27"),
			Resico: &app.ResicoEstimate{Rate: d("0.015"), EstimatedISR: d("900.30405"), RateIncreased: true, PreviousRate: &prev},
		},
	}
}

func TestRoutesRequireAuth(t *testing.T) {
	h := newServer(&fakeService{})
	for _, tc := range []struct{ method, path string }{
		{"POST", "/income"}, {"GET", "/income"}, {"GET", "/income/infer-category"},
		{"PUT", "/income/1"}, {"DELETE", "/income/1"},
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
	svc := &fakeService{result: sueldoResult()}
	rec := do(newServer(svc), "POST", "/income",
		`{"date":"2026-10-01","description":"Sueldo","category":"Sueldo","currency":"USD","amount":"3383.33","exchange_rate":"17.74"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("missing Cache-Control: no-store")
	}
	in := svc.gotInput
	if in.Date != "2026-10-01" || in.Category != "Sueldo" || in.Currency != "USD" || !in.Amount.Equal(d("3383.33")) ||
		in.ExchangeRate == nil || !in.ExchangeRate.Equal(d("17.74")) {
		t.Errorf("input %+v", in)
	}
	want := `{"income":{"id":7,"date":"2026-10-01","description":"Sueldo","category":"Sueldo","payment_method":"Transferencia","currency":"USD","amount":"3383.33","exchange_rate":"17.74","amount_mxn":"60020.27"},` +
		`"summary":{"month":"2026-10","month_total_mxn":"60020.27","resico":{"rate":"0.015","estimated_isr":"900.30405","rate_increased":true,"previous_rate":"0.01"}},` +
		`"split":null}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Errorf("body\n got %s\nwant %s", got, want)
	}
}

func TestCreateExtraContractWithSplit(t *testing.T) {
	res := sueldoResult()
	res.Movement.Category, res.Movement.Currency, res.Movement.ExchangeRate = "Contrato extra", "MXN", nil
	res.Movement.Amount, res.Movement.AmountMXN = d("1000"), d("1000")
	res.Summary.Resico = &app.ResicoEstimate{Rate: d("0.01"), EstimatedISR: d("10")}
	res.Split = &app.Split{
		ExtraSplit: income.ExtraSplit{
			Amount: d("1000"), SATReserve: d("165"), EmergencyFund: d("418"), Investments: d("292"),
			AguinaldoVacation: d("125"), GoalReached: false,
		},
		Breakdown: []savings.Share{{Key: "voo", Amount: d("292")}},
	}
	rec := do(newServer(&fakeService{result: res}), "POST", "/income", `{"amount":"1000","category":"Contrato extra"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var body struct {
		Summary struct {
			Resico map[string]any `json:"resico"`
		} `json:"summary"`
		Split json.RawMessage `json:"split"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Summary.Resico["previous_rate"] != nil {
		t.Errorf("previous_rate = %v, want null", body.Summary.Resico["previous_rate"])
	}
	wantSplit := `{"sat_reserve":"165","emergency_fund":"418","investments":"292","aguinaldo_vacation":"125","goal_reached":false,` +
		`"investment_breakdown":[{"instrument":"voo","amount":"292"}]}`
	if string(body.Split) != wantSplit {
		t.Errorf("split\n got %s\nwant %s", body.Split, wantSplit)
	}
}

func TestNullResicoAndEmptyBreakdown(t *testing.T) {
	res := sueldoResult()
	res.Summary.Resico = nil
	res.Split = &app.Split{}
	rec := do(newServer(&fakeService{result: res}), "POST", "/income", `{"amount":"1"}`)
	var body map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body["summary"]), `"resico":null`) {
		t.Errorf("summary = %s, want resico null", body["summary"])
	}
	if !strings.Contains(string(body["split"]), `"investment_breakdown":[]`) {
		t.Errorf("split = %s, want an empty breakdown array", body["split"])
	}
}

func TestBadBodies(t *testing.T) {
	h := newServer(&fakeService{})
	for _, body := range []string{`{`, `{"amount":"1","unknown":1}`, `{"amount":"abc"}`, ``} {
		rec := do(h, "POST", "/income", body)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_request") {
			t.Errorf("body %q: %d %s", body, rec.Code, rec.Body)
		}
	}
}

func TestErrorMapping(t *testing.T) {
	invalid := `{"error":"invalid_income","message":"invalid movement: amount must not be zero"}`
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
		rec := do(newServer(&fakeService{err: tc.err}), "POST", "/income", `{"amount":"1"}`)
		if rec.Code != tc.status || strings.TrimSpace(rec.Body.String()) != tc.body {
			t.Errorf("%s: %d %s", tc.name, rec.Code, rec.Body)
		}
	}
}

func ledgerInvalid() error {
	_, err := ledger.NewMovement(ledger.MovementInput{Kind: ledger.KindIncome, Category: "A"},
		ledger.Catalog{Categories: []ledger.CatalogCategory{{Name: "A", Kind: ledger.KindIncome}}, PaymentMethods: []string{"Transferencia"}}, "2026-10-01")
	return err
}

func TestUpdate(t *testing.T) {
	svc := &fakeService{result: sueldoResult()}
	rec := do(newServer(svc), "PUT", "/income/7", `{"category":"Sueldo","amount":"10"}`)
	if rec.Code != http.StatusOK || svc.gotID != 7 || svc.gotInput.Category != "Sueldo" {
		t.Errorf("%d id=%d input=%+v", rec.Code, svc.gotID, svc.gotInput)
	}
	for _, path := range []string{"/income/abc", "/income/0", "/income/-3"} {
		if rec := do(newServer(svc), "PUT", path, `{"amount":"1"}`); rec.Code != http.StatusNotFound {
			t.Errorf("PUT %s = %d, want 404", path, rec.Code)
		}
	}
	missing := &fakeService{err: ledger.ErrNotFound}
	if rec := do(newServer(missing), "PUT", "/income/9", `{"amount":"1"}`); rec.Code != http.StatusNotFound {
		t.Errorf("missing = %d, want 404", rec.Code)
	}
}

func TestDelete(t *testing.T) {
	svc := &fakeService{}
	rec := do(newServer(svc), "DELETE", "/income/12", "")
	if rec.Code != http.StatusNoContent || svc.deleteID != 12 || rec.Body.Len() != 0 {
		t.Errorf("%d id=%d body=%q", rec.Code, svc.deleteID, rec.Body)
	}
	if rec := do(newServer(&fakeService{err: ledger.ErrNotFound}), "DELETE", "/income/12", ""); rec.Code != http.StatusNotFound {
		t.Errorf("missing = %d, want 404", rec.Code)
	}
	if rec := do(newServer(svc), "DELETE", "/income/x", ""); rec.Code != http.StatusNotFound {
		t.Errorf("bad id = %d, want 404", rec.Code)
	}
}

func TestList(t *testing.T) {
	svc := &fakeService{list: []ledger.Movement{sueldoResult().Movement}}
	rec := do(newServer(svc), "GET", "/income?month=2026-10&limit=5", "")
	if rec.Code != http.StatusOK || svc.gotMonth != "2026-10" || svc.gotLimit != 5 {
		t.Fatalf("%d month=%q limit=%d", rec.Code, svc.gotMonth, svc.gotLimit)
	}
	var out []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || len(out) != 1 || out[0]["amount"] != "3383.33" {
		t.Errorf("body %s (%v)", rec.Body, err)
	}

	svc = &fakeService{}
	rec = do(newServer(svc), "GET", "/income", "")
	if rec.Code != http.StatusOK || svc.gotMonth != "" || svc.gotLimit != 0 || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("defaults: %d month=%q limit=%d body=%s", rec.Code, svc.gotMonth, svc.gotLimit, rec.Body)
	}

	for _, q := range []string{"limit=0", "limit=-1", "limit=abc", "limit=201"} {
		if rec := do(newServer(svc), "GET", "/income?"+q, ""); rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", q, rec.Code)
		}
	}
	if rec := do(newServer(svc), "GET", "/income?limit=200", ""); rec.Code != http.StatusOK || svc.gotLimit != 200 {
		t.Errorf("limit=200: %d limit=%d", rec.Code, svc.gotLimit)
	}
	if rec := do(newServer(&fakeService{err: ledgerInvalid()}), "GET", "/income?month=bad", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("bad month = %d, want 400", rec.Code)
	}
}

func TestInferCategory(t *testing.T) {
	svc := &fakeService{inferred: "Sueldo", inferOK: true}
	rec := do(newServer(svc), "GET", "/income/infer-category?description=pago+usa", "")
	if rec.Code != http.StatusOK || svc.gotDesc != "pago usa" || strings.TrimSpace(rec.Body.String()) != `{"category":"Sueldo"}` {
		t.Errorf("%d desc=%q body=%s", rec.Code, svc.gotDesc, rec.Body)
	}
	rec = do(newServer(&fakeService{}), "GET", "/income/infer-category?description=zzz", "")
	if strings.TrimSpace(rec.Body.String()) != `{"category":null}` {
		t.Errorf("no match body %s", rec.Body)
	}
}
