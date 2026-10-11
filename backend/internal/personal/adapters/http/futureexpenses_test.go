package personalhttp_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/valium69mg/finances-app/backend/internal/personal/app"
	"github.com/valium69mg/finances-app/backend/internal/personal/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/session"
)

func (f *fakeService) CreateFutureExpense(_ context.Context, a session.Identity, in domain.FutureInput) (domain.FuturePlanned, error) {
	f.gotActor, f.gotFuture = a, in
	return f.planned, f.err
}
func (f *fakeService) FutureExpense(_ context.Context, a session.Identity, userID string, id int) (domain.FuturePlanned, error) {
	f.gotActor, f.gotID, f.gotUserID = a, id, userID
	return f.planned, f.err
}
func (f *fakeService) UpdateFutureExpense(_ context.Context, a session.Identity, id int, in domain.FutureInput) (domain.FuturePlanned, error) {
	f.gotActor, f.gotID, f.gotFuture = a, id, in
	return f.planned, f.err
}
func (f *fakeService) DeleteFutureExpense(_ context.Context, a session.Identity, id int) error {
	f.gotActor, f.deletedItem = a, id
	return f.err
}
func (f *fakeService) FutureExpenses(_ context.Context, a session.Identity, userID string) (app.FutureListing, error) {
	f.gotActor, f.gotUserID = a, userID
	return f.listing, f.err
}
func (f *fakeService) ContributeFuture(_ context.Context, a session.Identity, id int, in app.FutureSavingInput) (domain.FuturePlanned, domain.Movement, error) {
	f.gotActor, f.gotID, f.gotSaving = a, id, in
	return f.planned, f.movement, f.err
}
func (f *fakeService) AssignFuture(_ context.Context, a session.Identity, id int, in app.FutureAssignInput) (domain.FuturePlanned, error) {
	f.gotActor, f.gotID, f.gotAssign = a, id, in
	return f.planned, f.err
}
func (f *fakeService) PayFuture(_ context.Context, a session.Identity, id int, in app.FuturePayInput) (domain.FuturePlanned, domain.Movement, error) {
	f.gotActor, f.gotID, f.gotPay = a, id, in
	return f.planned, f.movement, f.err
}

func activeItem() domain.FuturePlanned {
	return domain.FuturePlanned{
		FutureExpense: domain.FutureExpense{
			ID: 3, Name: "Viaje", Target: d("1000"), DueDate: "2026-12-20", Status: domain.FutureActive, Saved: d("250"),
			CreatedAt: created, UpdatedAt: created,
		},
		Remaining: d("750"), Suggested: d("375"), CyclesLeft: 2,
	}
}

const activeJSON = `{"id":3,"name":"Viaje","target_amount":"1000","due_date":"2026-12-20","status":"active","saved":"250","remaining":"750","suggested_monthly":"375","cycles_left":2,"paid_at":null,"amount_paid":null,"expense_movement_id":null,"created_at":"2026-10-01T12:00:00Z","updated_at":"2026-10-01T12:00:00Z"}`

func TestCreateAndUpdateFutureExpense(t *testing.T) {
	svc := &fakeService{planned: activeItem()}
	h := server(svc)
	rec := do(h, "POST", "/personal/future-expenses", `{"name":"Viaje","target_amount":"1000","due_date":"2026-12-20"}`)
	if rec.Code != 201 || strings.TrimSpace(rec.Body.String()) != activeJSON {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	if svc.gotFuture.Name != "Viaje" || !svc.gotFuture.Target.Equal(d("1000")) || svc.gotFuture.DueDate != "2026-12-20" || svc.gotActor != her {
		t.Errorf("service got %+v as %+v", svc.gotFuture, svc.gotActor)
	}
	rec = do(h, "PUT", "/personal/future-expenses/3", `{"name":"Viaje","target_amount":"1200","due_date":"2026-12-20"}`)
	if rec.Code != 200 || svc.gotID != 3 || !svc.gotFuture.Target.Equal(d("1200")) {
		t.Errorf("update: %d %s (id %d)", rec.Code, rec.Body, svc.gotID)
	}
	if rec := do(h, "GET", "/personal/future-expenses/3", ""); rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != activeJSON {
		t.Errorf("get: %d %s", rec.Code, rec.Body)
	}
	if rec := do(h, "POST", "/personal/future-expenses", `{"name":"x","target_amount":"1","due_date":"2026-12-20","status":"paid"}`); rec.Code != 400 {
		t.Errorf("unknown field: status %d, want 400", rec.Code)
	}
	if rec := do(h, "GET", "/personal/future-expenses/abc", ""); rec.Code != 404 {
		t.Errorf("malformed id: status %d, want 404", rec.Code)
	}
}

func TestListFutureExpenses(t *testing.T) {
	paid := activeItem()
	paid.ID, paid.Status, paid.PaidAt = 4, domain.FuturePaid, "2026-10-05"
	amount := d("300")
	paid.AmountPaid = &amount
	exp := 77
	paid.ExpenseMovementID = &exp
	paid.Remaining, paid.Suggested, paid.CyclesLeft = d("0"), d("0"), 0
	paid.Saved = d("0")
	svc := &fakeService{listing: app.FutureListing{
		Plan: domain.FuturePlan{Items: []domain.FuturePlanned{activeItem()}, Target: d("1000"), Saved: d("250"), Remaining: d("750"), Suggested: d("375"), FreeBalance: d("80")},
		Paid: []domain.FuturePlanned{paid},
	}}
	rec := do(server(svc), "GET", "/personal/future-expenses", "")
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`"active":[` + activeJSON + `]`,
		`"totals":{"target":"1000","saved":"250","remaining":"750","suggested_monthly":"375"}`,
		`"free_balance":"80"`,
		`"paid_at":"2026-10-05","amount_paid":"300","expense_movement_id":77`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %s:\n%s", want, body)
		}
	}
	empty := strings.TrimSpace(do(server(&fakeService{}), "GET", "/personal/future-expenses", "").Body.String())
	if !strings.Contains(empty, `"active":[]`) || !strings.Contains(empty, `"paid":[]`) {
		t.Errorf("empty lists must be arrays: %s", empty)
	}
}

func TestContributeAssignAndPay(t *testing.T) {
	fid := 3
	svc := &fakeService{planned: activeItem(), movement: domain.Movement{
		ID: 9, Kind: domain.KindSavings, CategoryID: 2, CategoryName: "Gastos futuros", Amount: d("100"), Description: "Ahorro para Viaje",
		Date: "2026-10-10", FutureExpenseID: &fid, CreatedAt: created, UpdatedAt: created,
	}}
	h := server(svc)

	rec := do(h, "POST", "/personal/future-expenses/3/savings", `{"amount":"100","date":"2026-10-10"}`)
	if rec.Code != 201 || svc.gotID != 3 || !svc.gotSaving.Amount.Equal(d("100")) || svc.gotSaving.Date != "2026-10-10" {
		t.Fatalf("contribute: %d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"saving":{"id":9`) || !strings.Contains(rec.Body.String(), `"future_expense_id":3`) {
		t.Errorf("contribute body = %s", rec.Body)
	}

	rec = do(h, "POST", "/personal/future-expenses/3/assign", `{"amount":"50"}`)
	if rec.Code != 200 || !svc.gotAssign.Amount.Equal(d("50")) || strings.TrimSpace(rec.Body.String()) != activeJSON {
		t.Errorf("assign: %d %s", rec.Code, rec.Body)
	}

	svc.movement.Kind, svc.movement.Description, svc.movement.FutureExpenseID = domain.KindExpense, "Viaje", nil
	rec = do(h, "POST", "/personal/future-expenses/3/pay", `{"amount":"300","date":"2026-10-05","category_id":5}`)
	if rec.Code != 200 || svc.gotPay.CategoryID != 5 || svc.gotPay.Amount == nil || !svc.gotPay.Amount.Equal(d("300")) || svc.gotPay.Date != "2026-10-05" {
		t.Fatalf("pay: %d %s (got %+v)", rec.Code, rec.Body, svc.gotPay)
	}
	if !strings.Contains(rec.Body.String(), `"expense":{"id":9,"kind":"Gasto"`) {
		t.Errorf("pay body = %s", rec.Body)
	}
	do(h, "POST", "/personal/future-expenses/3/pay", `{"category_id":5}`)
	if svc.gotPay.Amount != nil {
		t.Errorf("an omitted amount must stay nil (default to the target), got %v", svc.gotPay.Amount)
	}
}

func TestDeleteFutureExpense(t *testing.T) {
	svc := &fakeService{}
	rec := do(server(svc), "DELETE", "/personal/future-expenses/3", "")
	if rec.Code != 204 || svc.deletedItem != 3 || rec.Body.Len() != 0 || rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("delete: %d %q (item %d)", rec.Code, rec.Body, svc.deletedItem)
	}
}

func TestFutureExpenseErrors(t *testing.T) {
	for _, c := range []struct {
		err    error
		status int
		code   string
	}{
		{domain.ErrFutureInvalid, http.StatusBadRequest, "invalid_future_expense"},
		{domain.ErrInvalidInput, http.StatusBadRequest, "invalid_personal"},
		{domain.ErrNotFound, http.StatusNotFound, "not_found"},
		{domain.ErrAlreadyPaid, http.StatusConflict, "already_paid"},
		{domain.ErrInsufficientFreeBalance, http.StatusConflict, "insufficient_free_balance"},
		{domain.ErrForbidden, http.StatusForbidden, "forbidden"},
		{errors.New("boom"), http.StatusInternalServerError, "internal_error"},
	} {
		h := server(&fakeService{err: c.err})
		for _, r := range []struct{ method, path, body string }{
			{"POST", "/personal/future-expenses/3/pay", `{"category_id":1}`},
			{"POST", "/personal/future-expenses/3/assign", `{"amount":"1"}`},
			{"DELETE", "/personal/future-expenses/3", ""},
		} {
			rec := do(h, r.method, r.path, r.body)
			if rec.Code != c.status || !strings.Contains(rec.Body.String(), c.code) {
				t.Errorf("%v on %s: %d %s; want %d %s", c.err, r.path, rec.Code, rec.Body, c.status, c.code)
			}
		}
	}
}

func TestMovementExposesFutureExpenseID(t *testing.T) {
	m := ropaMovement()
	linked := 3
	m.Kind, m.FutureExpenseID = domain.KindSavings, &linked
	rec := do(server(&fakeService{movements: []domain.Movement{m, ropaMovement()}}), "GET", "/personal/movements", "")
	body := rec.Body.String()
	if !strings.Contains(body, `"future_expense_id":3`) || !strings.Contains(body, `"future_expense_id":null`) {
		t.Errorf("body = %s", body)
	}
}
