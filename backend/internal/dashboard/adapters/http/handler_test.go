package dashboardhttp_test

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

	"github.com/shopspring/decimal"

	dashboardhttp "github.com/valium69mg/finances-app/backend/internal/dashboard/adapters/http"
	dashboard "github.com/valium69mg/finances-app/backend/internal/dashboard/domain"
	future "github.com/valium69mg/finances-app/backend/internal/futureexpenses/domain"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	savings "github.com/valium69mg/finances-app/backend/internal/savings/domain"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
	taxfiling "github.com/valium69mg/finances-app/backend/internal/taxfiling/domain"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }
func p(s string) *decimal.Decimal {
	v := d(s)
	return &v
}

type fakeService struct {
	out      dashboard.Overview
	err      error
	gotMonth string
}

func (f *fakeService) Month(_ context.Context, month string) (dashboard.Overview, error) {
	f.gotMonth = month
	return f.out, f.err
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

func get(svc *fakeService, path string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	dashboardhttp.New(svc, slog.New(slog.NewTextHandler(io.Discard, nil))).Register(mux, requireGoodToken)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer good")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func sample() dashboard.Overview {
	return dashboard.Overview{
		Month:       "2026-10",
		PeriodStart: "2026-09-30",
		PeriodEnd:   "2026-10-30",
		Rows: []dashboard.Row{
			{Name: "Mandado", Kind: ledger.KindExpense, Real: d("1500"), Budget: p("1000"), Diff: p("-500")},
			{Name: "Ocio", Kind: ledger.KindExpense, Real: d("200")},
		},
		Totals:    dashboard.Totals{Income: d("50000"), Expenses: d("1700"), Savings: d("5000")},
		Available: d("43300"),
		Emergency: savings.EmergencyStatus{Accumulated: d("10000"), Goal: d("120000")},
		Tax: &dashboard.TaxCard{
			Rate: d("0.011"), EstimatedISR: d("550"),
			Filing: taxfiling.MonthStatus{Payment: taxfiling.PaymentPending, PreviousPeriod: "2026-09", PreviousPending: true},
		},
		Cycle: dashboard.CycleProgress{Today: "2026-10-15", Day: 16, Days: 31},
		Future: future.Plan{
			Items: []future.Planned{{
				FutureExpense: future.FutureExpense{ID: 4, Name: "Laptop", DueDate: "2027-01-20", Target: d("10000"), Saved: d("2500")},
				Remaining:     d("7500"), Suggested: d("2500"), CyclesLeft: 3,
			}},
			Target: d("10000"), Saved: d("2500"), Remaining: d("7500"), Suggested: d("2500"), FreeBalance: d("300.50"),
		},
		Upcoming: []dashboard.UpcomingBill{
			{ID: 7, Name: "Luz", Category: "Servicios", Amount: p("200"), Currency: "MXN", DueDate: "2026-10-17", DaysUntilDue: 2},
			{ID: 8, Name: "Agua", Category: "Servicios", Currency: "MXN", DueDate: "2026-10-14", DaysUntilDue: -1, Overdue: true},
		},
		Recent: []ledger.Movement{
			{ID: 3, Date: "2026-10-14", Kind: ledger.KindSavings, Description: "Apartado", Category: "Gastos futuros", AmountMXN: d("2500")},
		},
	}
}

func TestRequiresAuth(t *testing.T) {
	mux := http.NewServeMux()
	dashboardhttp.New(&fakeService{}, nil).Register(mux, requireGoodToken)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/dashboard", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status %d, want 401", rec.Code)
	}
}

func TestGet(t *testing.T) {
	svc := &fakeService{out: sample()}
	rec := get(svc, "/dashboard?month=2026-10")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if svc.gotMonth != "2026-10" {
		t.Errorf("month = %q", svc.gotMonth)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("missing Cache-Control: no-store")
	}
	want := `{"month":"2026-10","period_start":"2026-09-30","period_end":"2026-10-30","categories":[` +
		`{"category":"Mandado","spent":"1500","budget":"1000","remaining":"-500","over_budget":true},` +
		`{"category":"Ocio","spent":"200","budget":null,"remaining":null,"over_budget":false}],` +
		`"income":"50000","expenses":"1700","savings":"5000","available":"43300",` +
		`"emergency":{"accumulated":"10000","goal":"120000"},` +
		`"tax":{"rate":"0.011","estimated_isr":"550","filing_status":"pendiente","previous_period":"2026-09","previous_period_pending":true},` +
		`"cycle":{"today":"2026-10-15","day":16,"days":31},` +
		`"future_expenses":{"items":[{"id":4,"name":"Laptop","due_date":"2027-01-20","target":"10000","saved":"2500","remaining":"7500","suggested_monthly":"2500","cycles_left":3}],` +
		`"target":"10000","saved":"2500","remaining":"7500","suggested_monthly":"2500","free_balance":"300.5"},` +
		`"upcoming_bills":[{"id":7,"name":"Luz","category":"Servicios","amount":"200","currency":"MXN","due_date":"2026-10-17","days_until_due":2,"overdue":false},` +
		`{"id":8,"name":"Agua","category":"Servicios","amount":null,"currency":"MXN","due_date":"2026-10-14","days_until_due":-1,"overdue":true}],` +
		`"recent_movements":[{"id":3,"date":"2026-10-14","kind":"Ahorro","description":"Apartado","category":"Gastos futuros","amount_mxn":"2500"}]}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Errorf("body\n got %s\nwant %s", got, want)
	}
}

func TestGetDefaultMonthIsLeftToTheService(t *testing.T) {
	svc := &fakeService{out: sample()}
	if rec := get(svc, "/dashboard"); rec.Code != http.StatusOK || svc.gotMonth != "" {
		t.Errorf("status %d, month %q", rec.Code, svc.gotMonth)
	}
}

func TestGetWithoutTaxAndCategories(t *testing.T) {
	out := sample()
	out.Tax, out.Rows = nil, nil
	rec := get(&fakeService{out: out}, "/dashboard")
	body := rec.Body.String()
	if !strings.Contains(body, `"tax":null`) || !strings.Contains(body, `"categories":[]`) {
		t.Errorf("body %s, want null tax and an empty (not null) categories list", body)
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
		{"bad month", fmt.Errorf("%w: month \"x\" must be YYYY-MM", ledger.ErrInvalid), http.StatusBadRequest,
			`{"error":"invalid_dashboard","message":"invalid movement: month \"x\" must be YYYY-MM"}`},
		{"settings incomplete", fmt.Errorf("%w: emergency_months", settings.ErrMissingConfig), http.StatusUnprocessableEntity,
			`{"error":"settings_incomplete","message":"missing required config: emergency_months"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := get(&fakeService{err: tt.err}, "/dashboard?month=x")
			if rec.Code != tt.status {
				t.Errorf("status %d, want %d", rec.Code, tt.status)
			}
			if got := strings.TrimSpace(rec.Body.String()); got != tt.body {
				t.Errorf("body\n got %s\nwant %s", got, tt.body)
			}
		})
	}
}
