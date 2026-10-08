package settingshttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	settingshttp "github.com/valium69mg/finances-app/backend/internal/settings/adapters/http"
	"github.com/valium69mg/finances-app/backend/internal/settings/app"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain/settingstest"
)

type fakeService struct {
	cfg     domain.Config
	err     error
	budgets []app.CategoryBudget

	gotGeneral    domain.General
	gotCategories []domain.Category
	gotPause      *domain.PausePlan
	pauseCalled   bool
	gotMonth      string
	calls         []string
}

func (f *fakeService) Get(context.Context) (domain.Config, error) { return f.cfg, f.err }
func (f *fakeService) UpdateGeneral(_ context.Context, g domain.General) error {
	f.calls = append(f.calls, "general")
	f.gotGeneral = g
	return f.err
}
func (f *fakeService) UpdateCategories(_ context.Context, c []domain.Category) error {
	f.calls = append(f.calls, "categories")
	f.gotCategories = c
	return f.err
}
func (f *fakeService) UpdateClients(context.Context, []domain.Client) error {
	f.calls = append(f.calls, "clients")
	return f.err
}
func (f *fakeService) UpdateInstruments(context.Context, []domain.Instrument, map[string]string) error {
	f.calls = append(f.calls, "instruments")
	return f.err
}
func (f *fakeService) UpdateBrackets(context.Context, []domain.Bracket) error {
	f.calls = append(f.calls, "brackets")
	return f.err
}
func (f *fakeService) UpdatePaymentMethods(context.Context, []string) error {
	f.calls = append(f.calls, "payment")
	return f.err
}
func (f *fakeService) UpdateIssuer(context.Context, domain.Issuer) error {
	f.calls = append(f.calls, "issuer")
	return f.err
}
func (f *fakeService) UpdatePause(_ context.Context, p *domain.PausePlan) error {
	f.calls = append(f.calls, "pause")
	f.gotPause, f.pauseCalled = p, true
	return f.err
}
func (f *fakeService) MonthBudgets(_ context.Context, month string) ([]app.CategoryBudget, error) {
	f.gotMonth = month
	return f.budgets, f.err
}

// requireGoodToken stands in for authhttp.RequireAuth.
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
	settingshttp.New(svc, slog.New(slog.NewTextHandler(io.Discard, nil))).Register(mux, requireGoodToken)
	return mux
}

func do(h http.Handler, method, path, body string, authed bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if authed {
		req.Header.Set("Authorization", "Bearer good")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestEveryRouteRequiresAuth(t *testing.T) {
	h := newServer(&fakeService{})
	routes := []string{
		"GET /settings", "GET /settings/general", "PUT /settings/general", "GET /settings/categories",
		"PUT /settings/categories", "GET /settings/clients", "PUT /settings/clients", "GET /settings/instruments",
		"PUT /settings/instruments", "GET /settings/brackets", "PUT /settings/brackets",
		"GET /settings/payment-methods", "PUT /settings/payment-methods", "GET /settings/issuer",
		"PUT /settings/issuer", "GET /settings/investment-pause", "PUT /settings/investment-pause",
		"DELETE /settings/investment-pause", "GET /settings/budgets?month=2026-10",
	}
	for _, route := range routes {
		method, path, _ := strings.Cut(route, " ")
		if rec := do(h, method, path, "{}", false); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without token = %d, want 401", route, rec.Code)
		}
	}
}

func TestGetAllUsesDecimalStrings(t *testing.T) {
	cfg := settingstest.RealConfig()
	pause := settingstest.RealPause()
	cfg.Pause = &pause
	rec := do(newServer(&fakeService{cfg: cfg}), http.MethodGet, "/settings", "", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var got struct {
		General struct {
			FX            string `json:"fx_rate_applied"`
			CycleStartDay *int   `json:"cycle_start_day"`
		} `json:"general"`
		Categories []struct {
			Name   string  `json:"name"`
			Budget *string `json:"budget"`
		} `json:"categories"`
		Pause *struct {
			Resume string            `json:"resume_month"`
			Plan   map[string]string `json:"future_expenses_plan"`
		} `json:"investment_pause"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body is not the expected shape (decimals must be strings): %v\n%s", err, rec.Body)
	}
	if got.General.FX != "17.74" {
		t.Errorf("fx_rate_applied = %q", got.General.FX)
	}
	if got.General.CycleStartDay == nil || *got.General.CycleStartDay != 0 {
		t.Errorf("cycle_start_day = %v, want the number 0", got.General.CycleStartDay)
	}
	if got.Categories[0].Budget != nil || got.Categories[3].Budget == nil || *got.Categories[3].Budget != "3600" {
		t.Errorf("category budgets wrong: %+v", got.Categories[:4])
	}
	if got.Pause == nil || got.Pause.Resume != "2027-02" || got.Pause.Plan["2027-01"] != "10000" {
		t.Errorf("pause = %+v", got.Pause)
	}
}

func TestGetEmptyConfigStillAnswers(t *testing.T) {
	rec := do(newServer(&fakeService{}), http.MethodGet, "/settings", "", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"issuer":null`) || !strings.Contains(rec.Body.String(), `"investment_pause":null`) {
		t.Fatalf("empty sections should be null: %s", rec.Body)
	}
}

func TestPutCategoriesParsesDecimals(t *testing.T) {
	svc := &fakeService{}
	body := `[{"name":"Vivienda","kind":"Gasto","budget":"3600.50","includes":"x","keywords":["renta"]},
	          {"name":"Sueldo","kind":"Ingreso","budget":null,"includes":"","keywords":[]}]`
	rec := do(newServer(svc), http.MethodPut, "/settings/categories", body, true)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if len(svc.gotCategories) != 2 || !svc.gotCategories[0].Budget.Equal(decimal.RequireFromString("3600.50")) ||
		svc.gotCategories[1].Budget != nil || string(svc.gotCategories[0].Kind) != "Gasto" {
		t.Fatalf("categories = %+v", svc.gotCategories)
	}
}

func TestPutGeneral(t *testing.T) {
	svc := &fakeService{}
	body := `{"salary_usd":"3500","fx_rate_applied":"17.74","morse_fee_rate":"0.001","emergency_months":"6",
	          "extra_income_estimate_mxn":"35000","budget_includes_extra_income":false,"cycle_start_day":31,
	          "extra_income_split":{"inversiones":"0.35"},"investment_allocation":[{"key":"voo","value":"1"}]}`
	if rec := do(newServer(svc), http.MethodPut, "/settings/general", body, true); rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if svc.gotGeneral.FXRateApplied.String() != "17.74" || len(svc.gotGeneral.InvestmentAllocation) != 1 || svc.gotGeneral.CycleStartDay != 31 {
		t.Fatalf("general = %+v", svc.gotGeneral)
	}
}

func TestPutPauseAndDelete(t *testing.T) {
	svc := &fakeService{}
	h := newServer(svc)
	body := `{"months":["2026-10"],"normal_budget":"5000","resume_month":"2026-11","future_expenses_plan":{"2026-10":"12000"},"note":""}`
	if rec := do(h, http.MethodPut, "/settings/investment-pause", body, true); rec.Code != http.StatusNoContent {
		t.Fatalf("put status = %d: %s", rec.Code, rec.Body)
	}
	if svc.gotPause == nil || svc.gotPause.ResumeMonth != "2026-11" || svc.gotPause.FutureExpensesPlan["2026-10"].String() != "12000" {
		t.Fatalf("pause = %+v", svc.gotPause)
	}
	if rec := do(h, http.MethodDelete, "/settings/investment-pause", "", true); rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", rec.Code)
	}
	if !svc.pauseCalled || svc.gotPause != nil {
		t.Fatalf("delete should call UpdatePause(nil), got %+v", svc.gotPause)
	}
}

func TestBadBodies(t *testing.T) {
	for name, body := range map[string]string{
		"not json":      `{`,
		"unknown field": `[{"name":"A","kind":"Gasto","nope":1}]`,
		"bad decimal":   `[{"name":"A","kind":"Gasto","budget":"abc"}]`,
	} {
		svc := &fakeService{}
		rec := do(newServer(svc), http.MethodPut, "/settings/categories", body, true)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_request") {
			t.Errorf("%s: status = %d body = %s", name, rec.Code, rec.Body)
		}
		if len(svc.calls) != 0 {
			t.Errorf("%s: service was called", name)
		}
	}
}

func TestErrorMapping(t *testing.T) {
	tests := []struct {
		err  error
		code int
		body string
	}{
		{fmt.Errorf("%w: category name is required", domain.ErrInvalid), http.StatusBadRequest, "invalid_settings"},
		{fmt.Errorf("%w: salary_usd", domain.ErrMissingConfig), http.StatusConflict, "settings_incomplete"},
		{errors.New("db down"), http.StatusInternalServerError, "internal_error"},
	}
	for _, tc := range tests {
		rec := do(newServer(&fakeService{err: tc.err}), http.MethodPut, "/settings/brackets", `[]`, true)
		if rec.Code != tc.code || !strings.Contains(rec.Body.String(), tc.body) {
			t.Errorf("%v: status = %d body = %s", tc.err, rec.Code, rec.Body)
		}
	}
	// Internal details must not leak.
	rec := do(newServer(&fakeService{err: errors.New("secret dsn")}), http.MethodGet, "/settings", "", true)
	if strings.Contains(rec.Body.String(), "secret dsn") {
		t.Fatal("internal error leaked to the client")
	}
}

func TestBudgets(t *testing.T) {
	zero := decimal.Zero
	svc := &fakeService{budgets: []app.CategoryBudget{
		{Name: "Inversiones", Kind: "Ahorro", Budget: &zero},
		{Name: "Ocio", Kind: "Gasto"},
	}}
	rec := do(newServer(svc), http.MethodGet, "/settings/budgets?month=2026-10", "", true)
	if rec.Code != http.StatusOK || svc.gotMonth != "2026-10" {
		t.Fatalf("status = %d month = %q body = %s", rec.Code, svc.gotMonth, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"budget":"0"`) || !strings.Contains(rec.Body.String(), `"budget":null`) {
		t.Fatalf("body = %s", rec.Body)
	}
}

func TestClientsCarryPostalCode(t *testing.T) {
	cfg := settingstest.RealConfig()
	cfg.Clients[0].PostalCode = "64000"
	rec := do(newServer(&fakeService{cfg: cfg}), http.MethodGet, "/settings/clients", "", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var got []struct {
		PostalCode string `json:"postal_code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v: %s", err, rec.Body)
	}
	if len(got) == 0 || got[0].PostalCode != "64000" {
		t.Fatalf("clients = %s", rec.Body)
	}
}
