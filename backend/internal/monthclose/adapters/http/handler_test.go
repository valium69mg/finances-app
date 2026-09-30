package monthclosehttp_test

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

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	monthclosehttp "github.com/valium69mg/finances-app/backend/internal/monthclose/adapters/http"
	"github.com/valium69mg/finances-app/backend/internal/monthclose/app"
	monthclose "github.com/valium69mg/finances-app/backend/internal/monthclose/domain"
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
	preview app.Preview
	closeOf monthclose.Close
	list    []monthclose.Close
	err     error

	gotPreview, gotCreate, gotGet, gotDelete string
}

func (f *fakeService) Preview(_ context.Context, period string) (app.Preview, error) {
	f.gotPreview = period
	return f.preview, f.err
}
func (f *fakeService) Create(_ context.Context, period string) (monthclose.Close, error) {
	f.gotCreate = period
	return f.closeOf, f.err
}
func (f *fakeService) List(context.Context) ([]monthclose.Close, error) { return f.list, f.err }
func (f *fakeService) Get(_ context.Context, period string) (monthclose.Close, error) {
	f.gotGet = period
	return f.closeOf, f.err
}
func (f *fakeService) Delete(_ context.Context, period string) error {
	f.gotDelete = period
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

func do(svc *fakeService, method, path, body string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	monthclosehttp.New(svc, slog.New(slog.NewTextHandler(io.Discard, nil))).Register(mux, requireGoodToken)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer good")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func sample() monthclose.Close {
	return monthclose.Close{
		Period:   "2026-09",
		ClosedAt: time.Date(2026, 10, 2, 15, 4, 5, 0, time.UTC),
		Categories: []monthclose.Category{
			{Name: "Mandado", Budget: p("1000"), Spent: d("1500"), Remaining: p("-500"), OverBudget: true},
			{Name: "Ocio", Spent: d("200")},
		},
		Income: d("50000"), Expenses: d("1700"), Savings: d("5000"), Available: d("43300"),
		Emergency:    savings.EmergencyStatus{Accumulated: d("10000"), Goal: d("120000")},
		Suggestion:   &monthclose.Suggestion{ToEmergencyFund: d("43300"), ToInvestments: d("0"), ToFutureExpenses: d("0")},
		Adjustments:  []monthclose.Adjustment{{Name: "Mandado", Kind: ledger.KindExpense, Budget: d("1000"), Real: d("1500"), DeviationPct: d("50")}},
		FilingStatus: taxfiling.PaymentNone,
	}
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	return out
}

func TestRequiresAuth(t *testing.T) {
	mux := http.NewServeMux()
	monthclosehttp.New(&fakeService{}, nil).Register(mux, requireGoodToken)
	for _, tc := range []struct{ method, path string }{
		{"GET", "/month-close/preview"}, {"POST", "/month-close"}, {"GET", "/month-close"},
		{"GET", "/month-close/2026-09"}, {"DELETE", "/month-close/2026-09"},
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, want 401", tc.method, tc.path, rec.Code)
		}
	}
}

func TestPreviewShape(t *testing.T) {
	c := sample()
	c.ClosedAt = time.Time{}
	svc := &fakeService{preview: app.Preview{Close: c}}
	rec := do(svc, "GET", "/month-close/preview?period=2026-09", "")
	if rec.Code != http.StatusOK || svc.gotPreview != "2026-09" {
		t.Fatalf("status %d period %q body %s", rec.Code, svc.gotPreview, rec.Body)
	}
	body := decode(t, rec)
	if body["existing"] != nil {
		t.Errorf("existing = %v, want null", body["existing"])
	}
	pv := body["preview"].(map[string]any)
	if pv["period"] != "2026-09" || pv["closed_at"] != nil || pv["income"] != "50000" || pv["available"] != "43300" ||
		pv["tax_filing_status"] != "ninguna" {
		t.Errorf("preview = %v", pv)
	}
	cats := pv["categories"].([]any)
	first, second := cats[0].(map[string]any), cats[1].(map[string]any)
	if first["category"] != "Mandado" || first["budget"] != "1000" || first["remaining"] != "-500" || first["over_budget"] != true {
		t.Errorf("first category = %v", first)
	}
	if second["budget"] != nil || second["remaining"] != nil || second["over_budget"] != false {
		t.Errorf("category without budget = %v", second)
	}
	if em := pv["emergency"].(map[string]any); em["accumulated"] != "10000" || em["goal"] != "120000" {
		t.Errorf("emergency = %v", em)
	}
	if s := pv["suggestion"].(map[string]any); s["to_emergency_fund"] != "43300" || s["investments_paused"] != false || s["to_future_expenses"] != "0" {
		t.Errorf("suggestion = %v", s)
	}
	adj := pv["adjustments"].([]any)[0].(map[string]any)
	if adj["category"] != "Mandado" || adj["kind"] != "Gasto" || adj["deviation_pct"] != "50" {
		t.Errorf("adjustment = %v", adj)
	}
}

func TestPreviewDefaultsPeriodAndCarriesTheExistingClose(t *testing.T) {
	stored := sample()
	svc := &fakeService{preview: app.Preview{Close: sample(), Existing: &stored}}
	rec := do(svc, "GET", "/month-close/preview", "")
	if rec.Code != http.StatusOK || svc.gotPreview != "" {
		t.Fatalf("status %d period %q", rec.Code, svc.gotPreview)
	}
	ex := decode(t, rec)["existing"].(map[string]any)
	if ex["closed_at"] != "2026-10-02T15:04:05Z" {
		t.Errorf("existing = %v", ex)
	}
}

func TestPreviewWithoutSuggestionOrStatus(t *testing.T) {
	c := sample()
	c.Suggestion, c.FilingStatus, c.Adjustments = nil, "", nil
	rec := do(&fakeService{preview: app.Preview{Close: c}}, "GET", "/month-close/preview", "")
	pv := decode(t, rec)["preview"].(map[string]any)
	if pv["suggestion"] != nil || pv["tax_filing_status"] != nil {
		t.Errorf("preview = %v", pv)
	}
	if adj, ok := pv["adjustments"].([]any); !ok || len(adj) != 0 {
		t.Errorf("adjustments = %v, want []", pv["adjustments"])
	}
}

func TestCreate(t *testing.T) {
	svc := &fakeService{closeOf: sample()}
	rec := do(svc, "POST", "/month-close", `{"period":"2026-09"}`)
	if rec.Code != http.StatusCreated || svc.gotCreate != "2026-09" {
		t.Fatalf("status %d period %q body %s", rec.Code, svc.gotCreate, rec.Body)
	}
	if body := decode(t, rec); body["closed_at"] != "2026-10-02T15:04:05Z" || body["period"] != "2026-09" {
		t.Errorf("body = %v", body)
	}
}

func TestCreateRejectsABadBody(t *testing.T) {
	for _, body := range []string{``, `{`, `{"period":"2026-09","x":1}`} {
		if rec := do(&fakeService{}, "POST", "/month-close", body); rec.Code != http.StatusBadRequest {
			t.Errorf("body %q = %d, want 400", body, rec.Code)
		}
	}
}

func TestListGetDelete(t *testing.T) {
	svc := &fakeService{list: []monthclose.Close{sample()}, closeOf: sample()}

	rec := do(svc, "GET", "/month-close", "")
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || rec.Code != 200 || len(list) != 1 || list[0]["period"] != "2026-09" {
		t.Errorf("list %d %s", rec.Code, rec.Body)
	}
	rec = do(&fakeService{}, "GET", "/month-close", "")
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("empty list = %q, want []", rec.Body.String())
	}

	if rec = do(svc, "GET", "/month-close/2026-09", ""); rec.Code != 200 || svc.gotGet != "2026-09" {
		t.Errorf("get %d %q", rec.Code, svc.gotGet)
	}
	if rec = do(svc, "DELETE", "/month-close/2026-09", ""); rec.Code != http.StatusNoContent || svc.gotDelete != "2026-09" || rec.Body.Len() != 0 {
		t.Errorf("delete %d %q", rec.Code, svc.gotDelete)
	}
}

func TestErrorMapping(t *testing.T) {
	tests := []struct {
		err    error
		status int
		code   string
	}{
		{monthclose.ErrInvalidInput, 400, "invalid_close"},
		{ledger.ErrInvalid, 400, "invalid_close"},
		{monthclose.ErrAlreadyClosed, 409, "already_closed"},
		{monthclose.ErrNotFound, 404, "not_found"},
		{settings.ErrMissingConfig, 422, "settings_incomplete"},
		{errors.New("boom"), 500, "internal_error"},
	}
	for _, tc := range tests {
		svc := &fakeService{err: tc.err}
		for _, req := range [][3]string{
			{"GET", "/month-close/preview", ""}, {"POST", "/month-close", `{"period":"2026-09"}`},
			{"GET", "/month-close", ""}, {"GET", "/month-close/2026-09", ""}, {"DELETE", "/month-close/2026-09", ""},
		} {
			rec := do(svc, req[0], req[1], req[2])
			if rec.Code != tc.status || decode(t, rec)["error"] != tc.code {
				t.Errorf("%v on %s %s = %d %s, want %d %s", tc.err, req[0], req[1], rec.Code, rec.Body, tc.status, tc.code)
			}
		}
	}
}

func TestInternalErrorDoesNotLeak(t *testing.T) {
	rec := do(&fakeService{err: errors.New("secret detail")}, "GET", "/month-close", "")
	if strings.Contains(rec.Body.String(), "secret") {
		t.Errorf("body leaks the internal error: %s", rec.Body)
	}
}
