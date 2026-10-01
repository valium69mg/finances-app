package expenserequestshttp_test

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
	"time"

	"github.com/shopspring/decimal"

	expenserequestshttp "github.com/valium69mg/finances-app/backend/internal/expenserequests/adapters/http"
	"github.com/valium69mg/finances-app/backend/internal/expenserequests/app"
	"github.com/valium69mg/finances-app/backend/internal/expenserequests/domain"
	expensesapp "github.com/valium69mg/finances-app/backend/internal/expenses/app"
	futuredomain "github.com/valium69mg/finances-app/backend/internal/futureexpenses/domain"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/session"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

type fakeService struct {
	err     error
	actor   session.Identity
	call    string
	id      int
	input   domain.Input
	status  string
	approve app.ApproveInput
	comment string
	cat     string
	date    string
	check   domain.BudgetCheck
	result  app.ApproveResult
	list    []domain.Request
	names   []string
}

var created = time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)

func request() domain.Request {
	return domain.Request{ID: 7, RequesterEmail: "spouse@example.com", Amount: d("250.50"), Description: "Tacos",
		SuggestedCategory: "Comida fuera", ExpenseDate: "2026-10-03", Status: domain.StatusPending, CreatedAt: created}
}

func (f *fakeService) Create(_ context.Context, a session.Identity, in domain.Input) (domain.Request, error) {
	f.actor, f.call, f.input = a, "create", in
	return request(), f.err
}
func (f *fakeService) List(_ context.Context, a session.Identity, status string) ([]domain.Request, error) {
	f.actor, f.call, f.status = a, "list", status
	return f.list, f.err
}
func (f *fakeService) Categories(_ context.Context, a session.Identity) ([]string, error) {
	f.actor, f.call = a, "categories"
	return f.names, f.err
}
func (f *fakeService) Cancel(_ context.Context, a session.Identity, id int) (domain.Request, error) {
	f.actor, f.call, f.id = a, "cancel", id
	r := request()
	r.Status = domain.StatusCancelled
	return r, f.err
}
func (f *fakeService) BudgetCheck(_ context.Context, a session.Identity, id int, category, date string) (domain.BudgetCheck, error) {
	f.actor, f.call, f.id, f.cat, f.date = a, "budget-check", id, category, date
	return f.check, f.err
}
func (f *fakeService) Approve(_ context.Context, a session.Identity, id int, in app.ApproveInput) (app.ApproveResult, error) {
	f.actor, f.call, f.id, f.approve = a, "approve", id, in
	return f.result, f.err
}
func (f *fakeService) Reject(_ context.Context, a session.Identity, id int, comment string) (domain.Request, error) {
	f.actor, f.call, f.id, f.comment = a, "reject", id, comment
	r := request()
	r.Status, r.DecisionComment = domain.StatusRejected, comment
	return r, f.err
}

func (f *fakeService) Revert(_ context.Context, a session.Identity, id int) (domain.Request, error) {
	f.actor, f.call, f.id = a, "revert", id
	r := request()
	r.RevertCount = 1
	reverted := created.Add(time.Hour)
	r.RevertedAt = &reverted
	return r, f.err
}

func as(id session.Identity) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(session.With(r.Context(), id)))
		})
	}
}

var (
	owner     = session.Identity{UserID: "owner-1", Role: session.RoleOwner}
	household = session.Identity{UserID: "spouse-1", Role: session.RoleHousehold}
)

func serve(t *testing.T, svc *fakeService, who session.Identity, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	expenserequestshttp.New(svc, slog.New(slog.NewTextHandler(io.Discard, nil))).Register(mux, as(who))
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("body %q is not a JSON object: %v", rec.Body.String(), err)
	}
	return out
}

func TestCreate(t *testing.T) {
	svc := &fakeService{}
	rec := serve(t, svc, household, "POST", "/expense-requests",
		`{"amount":"250.50","description":"Tacos","suggested_category":"Comida fuera","date":"2026-10-03"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
	if svc.actor != household || !svc.input.Amount.Equal(d("250.50")) || svc.input.Description != "Tacos" || svc.input.SuggestedCategory != "Comida fuera" || svc.input.Date != "2026-10-03" {
		t.Errorf("service got %+v as %+v", svc.input, svc.actor)
	}
	got := decode(t, rec)
	if got["amount"] != "250.5" || got["status"] != "solicitada" || got["id"] != float64(7) || got["decision_comment"] != nil || got["result_kind"] != nil {
		t.Errorf("body = %v", got)
	}
	if rec := serve(t, svc, household, "POST", "/expense-requests", `{"amount":`); rec.Code != http.StatusBadRequest {
		t.Errorf("malformed body = %d", rec.Code)
	}
}

func TestListAndCategories(t *testing.T) {
	r := request()
	svc := &fakeService{list: []domain.Request{r}, names: []string{"Mandado", "Ocio"}}
	rec := serve(t, svc, owner, "GET", "/expense-requests?status=solicitada", "")
	if rec.Code != http.StatusOK || svc.status != "solicitada" || svc.actor != owner {
		t.Fatalf("list = %d, status filter %q, actor %+v", rec.Code, svc.status, svc.actor)
	}
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) != 1 || list[0]["requester_email"] != "spouse@example.com" {
		t.Errorf("list body = %s", rec.Body)
	}
	svc.list = nil
	if rec := serve(t, svc, household, "GET", "/expense-requests", ""); strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("empty list = %q, want []", rec.Body)
	}

	rec = serve(t, svc, household, "GET", "/expense-requests/categories", "")
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `["Mandado","Ocio"]` || svc.call != "categories" {
		t.Errorf("categories = %d %s (call %s)", rec.Code, rec.Body, svc.call)
	}
}

func TestCancelAndReject(t *testing.T) {
	svc := &fakeService{}
	rec := serve(t, svc, household, "POST", "/expense-requests/7/cancel", "")
	if rec.Code != http.StatusOK || svc.id != 7 || decode(t, rec)["status"] != "cancelada" {
		t.Errorf("cancel = %d %s", rec.Code, rec.Body)
	}
	rec = serve(t, svc, owner, "POST", "/expense-requests/7/reject", `{"comment":"No este mes"}`)
	got := decode(t, rec)
	if rec.Code != http.StatusOK || svc.comment != "No este mes" || got["status"] != "rechazada" || got["decision_comment"] != "No este mes" {
		t.Errorf("reject = %d %s", rec.Code, rec.Body)
	}
	if rec := serve(t, svc, owner, "POST", "/expense-requests/abc/cancel", ""); rec.Code != http.StatusNotFound {
		t.Errorf("non numeric id = %d", rec.Code)
	}
}

func TestBudgetCheckBodyFormatsEveryFigure(t *testing.T) {
	budget, remaining, projected, fits := d("1000"), d("200"), d("-100"), false
	svc := &fakeService{check: domain.BudgetCheck{
		Category: "Transporte", Budget: &budget, Spent: d("800"), Remaining: &remaining, Amount: d("300"),
		ProjectedSpent: d("1100"), ProjectedRemaining: &projected, Fits: &fits, OverBy: d("100"),
	}}
	rec := serve(t, svc, owner, "GET", "/expense-requests/7/budget-check?category=Transporte&date=2026-10-20", "")
	if rec.Code != http.StatusOK || svc.id != 7 || svc.cat != "Transporte" || svc.date != "2026-10-20" {
		t.Fatalf("status %d, service got %d %q %q", rec.Code, svc.id, svc.cat, svc.date)
	}
	want := map[string]any{
		"category": "Transporte", "budget": "1000.00", "spent": "800.00", "remaining": "200.00", "amount": "300.00",
		"projected_spent": "1100.00", "projected_remaining": "-100.00", "fits": false, "over_by": "100.00",
	}
	got := decode(t, rec)
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}
}

func TestBudgetCheckWithoutBudgetIsNullNotZero(t *testing.T) {
	svc := &fakeService{check: domain.NewBudgetCheck("Ocio", nil, d("40"), d("10"))}
	got := decode(t, serve(t, svc, owner, "GET", "/expense-requests/7/budget-check?category=Ocio", ""))
	for _, k := range []string{"budget", "remaining", "projected_remaining", "fits"} {
		if v, ok := got[k]; !ok || v != nil {
			t.Errorf("%s = %v (present %v), want null", k, v, ok)
		}
	}
	if got["spent"] != "40.00" || got["projected_spent"] != "50.00" || got["over_by"] != "0.00" {
		t.Errorf("body = %v", got)
	}
}

func TestApproveBodyAndBudgetEcho(t *testing.T) {
	budget, rem := d("1000"), d("-100")
	approved := request()
	approved.Status, approved.ResultKind = domain.StatusApproved, domain.DestinationExpense
	movement := 42
	approved.ResultMovementID = &movement
	svc := &fakeService{result: app.ApproveResult{Request: approved, Feedback: &expensesapp.BudgetFeedback{
		Month: "2026-10", Category: "Transporte", Budget: &budget, Spent: d("1100"), Remaining: &rem, OverBudget: true,
	}}}
	rec := serve(t, svc, owner, "POST", "/expense-requests/7/approve",
		`{"destination":"gasto","category":"Transporte","date":"2026-10-20","payment_method":"Efectivo"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
	if svc.approve.Destination != domain.DestinationExpense || svc.approve.Category != "Transporte" || svc.approve.Date != "2026-10-20" || svc.approve.PaymentMethod != "Efectivo" {
		t.Errorf("service got %+v", svc.approve)
	}
	got := decode(t, rec)
	req, _ := got["request"].(map[string]any)
	fb, _ := got["budget"].(map[string]any)
	if req["status"] != "aprobada" || req["result_kind"] != "gasto" || req["result_movement_id"] != float64(42) || fb["over_budget"] != true || fb["spent"] != "1100" {
		t.Errorf("body = %v", got)
	}

	svc.result = app.ApproveResult{Request: approved}
	rec = serve(t, svc, owner, "POST", "/expense-requests/7/approve", `{"destination":"gasto_futuro","due_date":"2026-12-20"}`)
	if svc.approve.Destination != domain.DestinationFuture || svc.approve.DueDate != "2026-12-20" || decode(t, rec)["budget"] != nil {
		t.Errorf("future approval: service got %+v, body %s", svc.approve, rec.Body)
	}
}

func TestErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{fmt.Errorf("%w: amount must be greater than zero", domain.ErrInvalidInput), 400, "invalid_expense_request"},
		{fmt.Errorf("%w: due date", futuredomain.ErrInvalidInput), 400, "invalid_future_expense"},
		{fmt.Errorf("%w: unknown payment method", ledger.ErrInvalid), 400, "invalid_expense"},
		{domain.ErrForbidden, 403, "forbidden"},
		{domain.ErrNotFound, 404, "not_found"},
		{domain.ErrInvalidState, 409, "invalid_state"},
		{domain.ErrFutureExpensePaid, 409, "future_expense_paid"},
		{domain.ErrRateLimited, 429, "rate_limited"},
		{settings.ErrMissingConfig, 422, "settings_incomplete"},
		{errors.New("boom: secret detail"), 500, "internal_error"},
	}
	for _, c := range cases {
		rec := serve(t, &fakeService{err: c.err}, household, "POST", "/expense-requests/7/cancel", "")
		if rec.Code != c.status || decode(t, rec)["error"] != c.code {
			t.Errorf("%v -> %d %s, want %d %s", c.err, rec.Code, rec.Body, c.status, c.code)
		}
		if c.status == 500 && strings.Contains(rec.Body.String(), "secret detail") {
			t.Error("an internal error leaked its detail")
		}
	}
	rec := serve(t, &fakeService{err: domain.ErrRateLimited}, household, "POST", "/expense-requests", `{"amount":"1","description":"x"}`)
	if rec.Header().Get("Retry-After") == "" {
		t.Error("429 without Retry-After")
	}
}

// The household role never reaches the owner-only use cases with owner powers:
// the handler passes the real identity down and the service refuses.
func TestHouseholdIdentityIsPassedDownForTheOwnerOnlyRoutes(t *testing.T) {
	for _, r := range []struct{ method, path, body string }{
		{"GET", "/expense-requests/7/budget-check?category=Ocio", ""},
		{"POST", "/expense-requests/7/approve", `{"destination":"gasto","category":"Ocio"}`},
		{"POST", "/expense-requests/7/reject", `{"comment":"x"}`},
		{"POST", "/expense-requests/7/revert", ""},
	} {
		svc := &fakeService{err: domain.ErrForbidden}
		rec := serve(t, svc, household, r.method, r.path, r.body)
		if rec.Code != http.StatusForbidden || svc.actor != household {
			t.Errorf("%s %s = %d as %+v, want 403 with the household identity", r.method, r.path, rec.Code, svc.actor)
		}
	}
}

func TestRevertReturnsTheRefreshedRequestWithItsAuditTrail(t *testing.T) {
	svc := &fakeService{}
	rec := serve(t, svc, owner, "POST", "/expense-requests/7/revert", "")
	got := decode(t, rec)
	if rec.Code != http.StatusOK || svc.call != "revert" || svc.id != 7 || svc.actor != owner {
		t.Fatalf("revert = %d %s (call %s id %d as %+v)", rec.Code, rec.Body, svc.call, svc.id, svc.actor)
	}
	if got["status"] != "solicitada" || got["revert_count"] != float64(1) || got["reverted_at"] == nil || got["result_kind"] != nil {
		t.Errorf("body = %v", got)
	}
	if rec := serve(t, svc, owner, "POST", "/expense-requests/abc/revert", ""); rec.Code != http.StatusNotFound {
		t.Errorf("non numeric id = %d", rec.Code)
	}
	for _, c := range []struct {
		err    error
		status int
		code   string
	}{
		{domain.ErrInvalidState, 409, "invalid_state"},
		{domain.ErrFutureExpensePaid, 409, "future_expense_paid"},
		{domain.ErrNotFound, 404, "not_found"},
		{domain.ErrForbidden, 403, "forbidden"},
	} {
		rec := serve(t, &fakeService{err: c.err}, owner, "POST", "/expense-requests/7/revert", "")
		if rec.Code != c.status || decode(t, rec)["error"] != c.code {
			t.Errorf("%v -> %d %s", c.err, rec.Code, rec.Body)
		}
	}
}
