package savingshttp_test

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

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	savingshttp "github.com/valium69mg/finances-app/backend/internal/savings/adapters/http"
	"github.com/valium69mg/finances-app/backend/internal/savings/app"
	savings "github.com/valium69mg/finances-app/backend/internal/savings/domain"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

type fakeService struct {
	movement   ledger.Movement
	list       []ledger.Movement
	transfer   app.Transfer
	valuation  ledger.Valuation
	valuations []ledger.Valuation
	portfolio  savings.Portfolio
	err        error

	gotInput     app.Input
	gotTransfer  app.TransferInput
	gotValuation app.ValuationInput
	gotID        int
	gotMonth     string
	gotLimit     int
	deleteID     int
}

func (f *fakeService) CreateSaving(_ context.Context, in app.Input) (ledger.Movement, error) {
	f.gotInput = in
	return f.movement, f.err
}
func (f *fakeService) UpdateSaving(_ context.Context, id int, in app.Input) (ledger.Movement, error) {
	f.gotID, f.gotInput = id, in
	return f.movement, f.err
}
func (f *fakeService) DeleteSaving(_ context.Context, id int) error {
	f.deleteID = id
	return f.err
}
func (f *fakeService) ListSavings(_ context.Context, month string, limit int) ([]ledger.Movement, error) {
	f.gotMonth, f.gotLimit = month, limit
	return f.list, f.err
}
func (f *fakeService) Transfer(_ context.Context, in app.TransferInput) (app.Transfer, error) {
	f.gotTransfer = in
	return f.transfer, f.err
}
func (f *fakeService) AddValuation(_ context.Context, in app.ValuationInput) (ledger.Valuation, error) {
	f.gotValuation = in
	return f.valuation, f.err
}
func (f *fakeService) ListValuations(context.Context) ([]ledger.Valuation, error) {
	return f.valuations, f.err
}
func (f *fakeService) Portfolio(context.Context) (savings.Portfolio, error) {
	return f.portfolio, f.err
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
	savingshttp.New(svc, slog.New(slog.NewTextHandler(io.Discard, nil))).Register(mux, requireGoodToken)
	return mux
}

func do(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer good")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func vooSaving() ledger.Movement {
	return ledger.Movement{
		ID: 7, Date: "2026-10-01", Description: "Aporte", Category: "Inversiones", Instrument: "voo", Kind: ledger.KindSavings,
		PaymentMethod: "Transferencia", Currency: "MXN", Amount: d("1000"), AmountMXN: d("1000"),
	}
}

func ledgerInvalid() error {
	_, err := ledger.NewMovement(ledger.MovementInput{Kind: ledger.KindSavings, Category: "A"},
		ledger.Catalog{Categories: []ledger.CatalogCategory{{Name: "A", Kind: ledger.KindSavings}}, PaymentMethods: []string{"Transferencia"}}, "2026-10-01")
	return err
}

func TestRoutesRequireAuth(t *testing.T) {
	h := newServer(&fakeService{})
	for _, tc := range []struct{ method, path string }{
		{"POST", "/savings"}, {"GET", "/savings"}, {"PUT", "/savings/1"}, {"DELETE", "/savings/1"},
		{"POST", "/savings/transfers"}, {"POST", "/savings/valuations"}, {"GET", "/savings/valuations"},
		{"GET", "/savings/portfolio"},
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
	svc := &fakeService{movement: vooSaving()}
	rec := do(newServer(svc), "POST", "/savings",
		`{"date":"2026-10-01","description":"Aporte","category":"Inversiones","instrument":"voo","amount":"-1000.50"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("missing Cache-Control: no-store")
	}
	in := svc.gotInput
	if in.Date != "2026-10-01" || in.Category != "Inversiones" || in.Instrument != "voo" || !in.Amount.Equal(d("-1000.50")) {
		t.Errorf("input %+v", in)
	}
	want := `{"id":7,"date":"2026-10-01","description":"Aporte","category":"Inversiones","instrument":"voo","payment_method":"Transferencia","currency":"MXN","amount":"1000","exchange_rate":null,"amount_mxn":"1000","transfer_id":null}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Errorf("body\n got %s\nwant %s", got, want)
	}
}

func TestBadBodies(t *testing.T) {
	h := newServer(&fakeService{})
	for _, path := range []string{"/savings", "/savings/transfers", "/savings/valuations"} {
		for _, body := range []string{`{`, `{"amount":"1","unknown":1}`, `{"amount":"abc"}`, ``} {
			rec := do(h, "POST", path, body)
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_request") {
				t.Errorf("POST %s body %q: %d %s", path, body, rec.Code, rec.Body)
			}
		}
	}
}

func TestErrorMapping(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		body   string
	}{
		{"unexpected", errors.New("x"), http.StatusInternalServerError, `{"error":"internal_error"}`},
		{"validation", ledgerInvalid(), http.StatusBadRequest, `{"error":"invalid_saving","message":"invalid movement: amount must not be zero"}`},
		{"valuation", savings.ValidateValuation(ledger.Valuation{}), http.StatusBadRequest,
			`{"error":"invalid_valuation","message":"invalid valuation: invalid date \"\", use YYYY-MM-DD"}`},
		{"locked leg", savings.ErrTransferLegLocked, http.StatusConflict,
			`{"error":"transfer_leg_locked","message":"transfer legs cannot be edited: delete the transfer and record it again"}`},
		{"not found", ledger.ErrNotFound, http.StatusNotFound, `{"error":"not_found"}`},
		{"settings", settings.ErrMissingConfig, http.StatusUnprocessableEntity, `{"error":"settings_incomplete","message":"missing required config"}`},
	}
	for _, tc := range tests {
		rec := do(newServer(&fakeService{err: tc.err}), "POST", "/savings", `{"amount":"1"}`)
		if rec.Code != tc.status || strings.TrimSpace(rec.Body.String()) != tc.body {
			t.Errorf("%s: %d %s", tc.name, rec.Code, rec.Body)
		}
	}
}

func TestAddValuationAbsurdValueIs400(t *testing.T) {
	// The huge exponent must survive JSON decoding and come back as a 400,
	// never a 500 from the database.
	svc := &fakeService{}
	rec := do(newServer(svc), "POST", "/savings/valuations", `{"instrument":"voo","value_mxn":"1e999999999"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("decode: %d %s", rec.Code, rec.Body)
	}
	bad := savings.ValidateValuation(ledger.Valuation{Date: "2026-10-01", Instrument: "voo", ValueMXN: svc.gotValuation.ValueMXN})
	rec = do(newServer(&fakeService{err: bad}), "POST", "/savings/valuations", `{"instrument":"voo","value_mxn":"1e999999999"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_valuation") {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
}

func TestUpdate(t *testing.T) {
	svc := &fakeService{movement: vooSaving()}
	rec := do(newServer(svc), "PUT", "/savings/7", `{"category":"Inversiones","amount":"10"}`)
	if rec.Code != http.StatusOK || svc.gotID != 7 || svc.gotInput.Category != "Inversiones" {
		t.Errorf("%d id=%d input=%+v", rec.Code, svc.gotID, svc.gotInput)
	}
	for _, path := range []string{"/savings/abc", "/savings/0", "/savings/-3"} {
		if rec := do(newServer(svc), "PUT", path, `{"amount":"1"}`); rec.Code != http.StatusNotFound {
			t.Errorf("PUT %s = %d, want 404", path, rec.Code)
		}
	}
	missing := &fakeService{err: ledger.ErrNotFound}
	if rec := do(newServer(missing), "PUT", "/savings/9", `{"amount":"1"}`); rec.Code != http.StatusNotFound {
		t.Errorf("missing = %d, want 404", rec.Code)
	}
}

func TestUpdateLockedLegIs409(t *testing.T) {
	rec := do(newServer(&fakeService{err: savings.ErrTransferLegLocked}), "PUT", "/savings/7", `{"amount":"1"}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"transfer_leg_locked"`) {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
}

func TestDelete(t *testing.T) {
	svc := &fakeService{}
	rec := do(newServer(svc), "DELETE", "/savings/12", "")
	if rec.Code != http.StatusNoContent || svc.deleteID != 12 || rec.Body.Len() != 0 {
		t.Errorf("%d id=%d body=%q", rec.Code, svc.deleteID, rec.Body)
	}
	if rec := do(newServer(&fakeService{err: ledger.ErrNotFound}), "DELETE", "/savings/12", ""); rec.Code != http.StatusNotFound {
		t.Errorf("missing = %d, want 404", rec.Code)
	}
	if rec := do(newServer(svc), "DELETE", "/savings/x", ""); rec.Code != http.StatusNotFound {
		t.Errorf("bad id = %d, want 404", rec.Code)
	}
}

func TestList(t *testing.T) {
	svc := &fakeService{list: []ledger.Movement{vooSaving()}}
	rec := do(newServer(svc), "GET", "/savings?month=2026-10&limit=5", "")
	if rec.Code != http.StatusOK || svc.gotMonth != "2026-10" || svc.gotLimit != 5 {
		t.Fatalf("%d month=%q limit=%d", rec.Code, svc.gotMonth, svc.gotLimit)
	}
	var out []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || len(out) != 1 || out[0]["amount"] != "1000" || out[0]["instrument"] != "voo" {
		t.Errorf("body %s (%v)", rec.Body, err)
	}

	svc = &fakeService{}
	rec = do(newServer(svc), "GET", "/savings", "")
	if rec.Code != http.StatusOK || svc.gotMonth != "" || svc.gotLimit != 0 || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("defaults: %d month=%q limit=%d body=%s", rec.Code, svc.gotMonth, svc.gotLimit, rec.Body)
	}

	for _, q := range []string{"limit=0", "limit=-1", "limit=abc", "limit=201"} {
		if rec := do(newServer(svc), "GET", "/savings?"+q, ""); rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", q, rec.Code)
		}
	}
	if rec := do(newServer(&fakeService{err: ledgerInvalid()}), "GET", "/savings?month=bad", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("bad month = %d, want 400", rec.Code)
	}
}

func TestTransfer(t *testing.T) {
	out, in := vooSaving(), vooSaving()
	out.ID, out.Instrument, out.Amount, out.AmountMXN = 1, "cetes-28", d("-500"), d("-500")
	in.ID, in.Amount, in.AmountMXN = 2, d("500"), d("500")
	out.TransferID, in.TransferID = "6f1c1a0e-8f5e-4a55-9d0a-3c1f0b2a7e11", "6f1c1a0e-8f5e-4a55-9d0a-3c1f0b2a7e11"
	svc := &fakeService{transfer: app.Transfer{Out: out, In: in}}

	rec := do(newServer(svc), "POST", "/savings/transfers",
		`{"from":"cetes-28","to":"voo","amount":"500","date":"2026-10-03","description":"Rebalance","category":"Inversiones"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	got := svc.gotTransfer
	if got.From != "cetes-28" || got.To != "voo" || !got.Amount.Equal(d("500")) || got.Date != "2026-10-03" ||
		got.Description != "Rebalance" || got.Category != "Inversiones" {
		t.Errorf("input %+v", got)
	}
	var body struct {
		Out map[string]any `json:"out"`
		In  map[string]any `json:"in"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Out["id"] != float64(1) || body.Out["amount"] != "-500" || body.Out["instrument"] != "cetes-28" ||
		body.In["id"] != float64(2) || body.In["amount"] != "500" || body.In["instrument"] != "voo" ||
		body.Out["transfer_id"] != "6f1c1a0e-8f5e-4a55-9d0a-3c1f0b2a7e11" || body.In["transfer_id"] != body.Out["transfer_id"] {
		t.Errorf("body %s", rec.Body)
	}

	rec = do(newServer(&fakeService{err: ledgerInvalid()}), "POST", "/savings/transfers", `{"from":"a","to":"a","amount":"1"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_saving") {
		t.Errorf("invalid transfer: %d %s", rec.Code, rec.Body)
	}
}

func TestValuations(t *testing.T) {
	v := ledger.Valuation{Date: "2026-10-15", Instrument: "voo", ValueMXN: d("12345.67"), Note: "statement"}
	svc := &fakeService{valuation: v, valuations: []ledger.Valuation{v}}

	rec := do(newServer(svc), "POST", "/savings/valuations", `{"date":"2026-10-15","instrument":"voo","value_mxn":"12345.67","note":"statement"}`)
	want := `{"date":"2026-10-15","instrument":"voo","value_mxn":"12345.67","note":"statement"}`
	if rec.Code != http.StatusCreated || strings.TrimSpace(rec.Body.String()) != want {
		t.Errorf("create: %d %s", rec.Code, rec.Body)
	}
	if g := svc.gotValuation; g.Date != "2026-10-15" || g.Instrument != "voo" || !g.ValueMXN.Equal(d("12345.67")) || g.Note != "statement" {
		t.Errorf("input %+v", g)
	}

	rec = do(newServer(svc), "GET", "/savings/valuations", "")
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "["+want+"]" {
		t.Errorf("list: %d %s", rec.Code, rec.Body)
	}
	rec = do(newServer(&fakeService{}), "GET", "/savings/valuations", "")
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("empty list body %s, want []", rec.Body)
	}

	bad := &fakeService{err: savings.ValidateValuation(ledger.Valuation{})}
	rec = do(newServer(bad), "POST", "/savings/valuations", `{"value_mxn":"0"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_valuation") {
		t.Errorf("invalid valuation: %d %s", rec.Code, rec.Body)
	}
}

func TestPortfolio(t *testing.T) {
	svc := &fakeService{portfolio: savings.Portfolio{
		Rows: []savings.PortfolioRow{
			{ID: "voo", Name: "VOO", Type: "renta_variable", Platform: "GBM", Contributed: d("600"), Value: d("900"), ValueDate: "2026-10-15",
				Gain: d("300"), GainPct: d("50"), PctOfTotal: d("75")},
			{ID: "cetes-28", Name: "CETES", Type: "deuda", Platform: "GBM", Contributed: d("400"), Value: d("400"), Unvalued: true, PctOfTotal: d("25")},
		},
		TotalContributed: d("1000"), TotalValue: d("1300"),
		ByType:        []savings.TypeTotal{{Type: "renta_variable", Contributed: d("600"), Value: d("900")}},
		ByDestination: []savings.DestinationTotal{{Category: "Inversiones", Balance: d("1000")}},
		Emergency:     savings.EmergencyStatus{Accumulated: d("0"), Goal: d("166922.64")},
	}}
	rec := do(newServer(svc), "GET", "/savings/portfolio", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	want := `{"rows":[` +
		`{"id":"voo","name":"VOO","type":"renta_variable","platform":"GBM","contributed":"600","value":"900","value_date":"2026-10-15","unvalued":false,"gain":"300","gain_pct":"50","pct_of_total":"75"},` +
		`{"id":"cetes-28","name":"CETES","type":"deuda","platform":"GBM","contributed":"400","value":"400","value_date":null,"unvalued":true,"gain":"0","gain_pct":"0","pct_of_total":"25"}],` +
		`"total_contributed":"1000","total_value":"1300",` +
		`"by_type":[{"type":"renta_variable","contributed":"600","value":"900"}],` +
		`"by_destination":[{"category":"Inversiones","balance":"1000"}],` +
		`"emergency":{"accumulated":"0","goal":"166922.64"}}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Errorf("body\n got %s\nwant %s", got, want)
	}

	rec = do(newServer(&fakeService{err: settings.ErrMissingConfig}), "GET", "/savings/portfolio", "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("missing config = %d, want 422", rec.Code)
	}
	rec = do(newServer(&fakeService{}), "GET", "/savings/portfolio", "")
	if !strings.Contains(rec.Body.String(), `"rows":[]`) || !strings.Contains(rec.Body.String(), `"by_type":[]`) {
		t.Errorf("empty portfolio body %s, want empty arrays", rec.Body)
	}
}
