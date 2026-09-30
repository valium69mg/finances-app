package app_test

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/bills/app"
	bills "github.com/valium69mg/finances-app/backend/internal/bills/domain"
	expensesapp "github.com/valium69mg/finances-app/backend/internal/expenses/app"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain/settingstest"
)

func dec(s string) *decimal.Decimal {
	v := decimal.RequireFromString(s)
	return &v
}

type fakeSettings struct {
	cfg settings.Config
	err error
}

func (f *fakeSettings) Get(context.Context) (settings.Config, error) { return f.cfg, f.err }

type fakeExpenses struct {
	created   []expensesapp.Input
	deleted   []int
	createErr error
	nextID    int
}

func (f *fakeExpenses) Create(_ context.Context, in expensesapp.Input) (expensesapp.Result, error) {
	if f.createErr != nil {
		return expensesapp.Result{}, f.createErr
	}
	f.created = append(f.created, in)
	f.nextID++
	return expensesapp.Result{Movement: ledger.Movement{
		ID: 100 + f.nextID, Date: in.Date, Description: in.Description, Category: in.Category,
		Kind: ledger.KindExpense, Currency: in.Currency, Amount: in.Amount, AmountMXN: in.Amount,
	}}, nil
}

func (f *fakeExpenses) Delete(_ context.Context, id int) error {
	f.deleted = append(f.deleted, id)
	return nil
}

// fakeRepo mimics the Postgres repository: one pending occurrence per bill,
// history kept.
type fakeRepo struct {
	bills      map[int]bills.Bill
	history    map[int][]bills.Occurrence
	nextID     int
	resolveErr error
	resolved   []app.Resolution
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{bills: map[int]bills.Bill{}, history: map[int][]bills.Occurrence{}}
}

func (f *fakeRepo) Create(_ context.Context, v bills.Validated) (bills.Bill, error) {
	f.nextID++
	b := bills.Bill{
		ID: f.nextID, Name: v.Name, Category: v.Category, Amount: v.Amount, Currency: v.Currency, Recurrence: v.Recurrence,
		NextDueDate: v.NextDueDate, AnchorDay: v.AnchorDay, ReminderLeadDays: v.ReminderLeadDays, Active: v.Active, Notes: v.Notes,
		Pending: &bills.Occurrence{ID: f.nextID * 1000, BillID: f.nextID, DueDate: v.NextDueDate, Status: bills.StatusPending},
	}
	f.bills[b.ID] = b
	return b, nil
}

func (f *fakeRepo) Get(_ context.Context, id int) (bills.Bill, error) {
	b, ok := f.bills[id]
	if !ok {
		return bills.Bill{}, bills.ErrNotFound
	}
	return b, nil
}

func (f *fakeRepo) List(_ context.Context, includeInactive bool) ([]bills.Bill, error) {
	out := []bills.Bill{}
	for _, b := range f.bills {
		if b.Active || includeInactive {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID }) // unsorted by due date on purpose
	return out, nil
}

func (f *fakeRepo) History(_ context.Context, id int) ([]bills.Occurrence, error) {
	return f.history[id], nil
}

func (f *fakeRepo) Update(_ context.Context, id int, v bills.Validated) (bills.Bill, error) {
	b, ok := f.bills[id]
	if !ok {
		return bills.Bill{}, bills.ErrNotFound
	}
	b.Name, b.Category, b.Amount, b.Currency, b.Recurrence = v.Name, v.Category, v.Amount, v.Currency, v.Recurrence
	b.NextDueDate, b.AnchorDay, b.ReminderLeadDays, b.Active, b.Notes = v.NextDueDate, v.AnchorDay, v.ReminderLeadDays, v.Active, v.Notes
	p := *b.Pending
	p.DueDate = v.NextDueDate
	b.Pending = &p
	f.bills[id] = b
	return b, nil
}

func (f *fakeRepo) Deactivate(_ context.Context, id int) error {
	b, ok := f.bills[id]
	if !ok {
		return bills.ErrNotFound
	}
	b.Active = false
	f.bills[id] = b
	return nil
}

func (f *fakeRepo) Resolve(_ context.Context, r app.Resolution) (bills.Bill, error) {
	if f.resolveErr != nil {
		return bills.Bill{}, f.resolveErr
	}
	b, ok := f.bills[r.BillID]
	if !ok {
		return bills.Bill{}, bills.ErrNotFound
	}
	if b.Pending == nil || b.Pending.ID != r.OccurrenceID {
		return bills.Bill{}, bills.ErrNotPending
	}
	done := *b.Pending
	done.Status = r.Status
	if r.Status == bills.StatusPaid {
		amount := r.AmountPaid
		done.PaidOn, done.AmountPaid, done.Currency, done.ExpenseID = r.PaidOn, &amount, r.Currency, r.ExpenseID
	}
	f.history[b.ID] = append([]bills.Occurrence{done}, f.history[b.ID]...)
	f.nextID++
	b.Pending = &bills.Occurrence{ID: f.nextID * 1000, BillID: b.ID, DueDate: r.NextDueDate, Status: bills.StatusPending}
	b.NextDueDate = r.NextDueDate
	f.bills[b.ID] = b
	f.resolved = append(f.resolved, r)
	return b, nil
}

type fixture struct {
	svc      *app.Service
	repo     *fakeRepo
	expenses *fakeExpenses
	settings *fakeSettings
}

// today is 2026-10-10.
func newFixture() *fixture {
	fx := &fixture{repo: newFakeRepo(), expenses: &fakeExpenses{}, settings: &fakeSettings{cfg: settingstest.RealConfig()}}
	now := func() time.Time { return time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC) }
	fx.svc = app.NewService(fx.repo, fx.expenses, fx.settings, now, nil)
	return fx
}

func megacable() bills.Input {
	return bills.Input{
		Name: "Megacable", Category: "Servicios", Amount: dec("550"), Recurrence: bills.Monthly,
		NextDueDate: "2026-10-01", Active: true,
	}
}

func (fx *fixture) create(t *testing.T, in bills.Input) app.Status {
	t.Helper()
	st, err := fx.svc.Create(context.Background(), in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return st
}

func TestCreateValidatesAndCanonicalizesTheCategory(t *testing.T) {
	fx := newFixture()
	in := megacable()
	in.Category = " servicios "
	st := fx.create(t, in)
	if st.Bill.Category != "Servicios" || st.Bill.Currency != "MXN" || st.Bill.ReminderLeadDays != 3 || st.Bill.Pending == nil {
		t.Errorf("bill = %+v", st.Bill)
	}

	for name, mut := range map[string]func(*bills.Input){
		"unknown category":          func(i *bills.Input) { i.Category = "Nope" },
		"an income category":        func(i *bills.Input) { i.Category = "Otros ingresos" },
		"a domain validation error": func(i *bills.Input) { i.Name = "" },
	} {
		in := megacable()
		mut(&in)
		if _, err := fx.svc.Create(context.Background(), in); !errors.Is(err, bills.ErrInvalidInput) {
			t.Errorf("%s: err = %v, want ErrInvalidInput", name, err)
		}
	}
}

func TestCreatePropagatesSettingsErrors(t *testing.T) {
	fx := newFixture()
	fx.settings.err = settings.ErrMissingConfig
	if _, err := fx.svc.Create(context.Background(), megacable()); !errors.Is(err, settings.ErrMissingConfig) {
		t.Errorf("err = %v, want ErrMissingConfig", err)
	}
}

func TestFlagsAndSortByDueDate(t *testing.T) {
	fx := newFixture()
	mk := func(name, due string, lead int) {
		in := megacable()
		in.Name, in.NextDueDate, in.ReminderLeadDays = name, due, &lead
		fx.create(t, in)
	}
	mk("later", "2026-11-30", 3)   // created first, due last
	mk("soon", "2026-10-12", 3)    // 2 days: due soon
	mk("today", "2026-10-10", 0)   // due today: due soon, not overdue
	mk("overdue", "2026-10-01", 3) // past: overdue, not due soon
	mk("outside", "2026-10-14", 3) // 4 days with lead 3: not due soon
	list, err := fx.svc.List(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	got := map[string][3]any{}
	for _, s := range list {
		names = append(names, s.Bill.Name)
		got[s.Bill.Name] = [3]any{s.Overdue, s.DueSoon, s.DaysUntilDue}
	}
	if strings.Join(names, ",") != "overdue,today,soon,outside,later" {
		t.Errorf("order = %v", names)
	}
	want := map[string][3]any{
		"overdue": {true, false, -9}, "today": {false, true, 0}, "soon": {false, true, 2},
		"outside": {false, false, 4}, "later": {false, false, 51},
	}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("%s = %v, want %v", name, got[name], w)
		}
	}
}

func TestListHidesInactiveUnlessAsked(t *testing.T) {
	fx := newFixture()
	a := fx.create(t, megacable())
	fx.create(t, megacable())
	if err := fx.svc.Deactivate(context.Background(), a.Bill.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := fx.svc.List(context.Background(), false); len(list) != 1 {
		t.Errorf("active list = %d, want 1", len(list))
	}
	if list, _ := fx.svc.List(context.Background(), true); len(list) != 2 {
		t.Errorf("full list = %d, want 2", len(list))
	}
}

func TestPayFixedBillDefaults(t *testing.T) {
	fx := newFixture()
	b := fx.create(t, megacable()).Bill
	res, err := fx.svc.Pay(context.Background(), b.ID, bills.PaymentInput{})
	if err != nil {
		t.Fatalf("Pay: %v", err)
	}
	if len(fx.expenses.created) != 1 {
		t.Fatalf("expenses created = %d, want 1", len(fx.expenses.created))
	}
	in := fx.expenses.created[0]
	if in.Date != "2026-10-10" || in.Description != "Megacable" || in.Category != "Servicios" || in.Currency != "MXN" || !in.Amount.Equal(*dec("550")) {
		t.Errorf("expense input = %+v", in)
	}
	if in.PaymentMethod != "" || in.ExchangeRate != nil {
		t.Errorf("payment method and rate must be left to the expenses defaults: %+v", in)
	}
	r := fx.repo.resolved[0]
	if r.Status != bills.StatusPaid || r.PaidOn != "2026-10-10" || !r.AmountPaid.Equal(*dec("550")) || r.ExpenseID == nil || *r.ExpenseID != res.Expense.ID || r.NextDueDate != "2026-11-01" {
		t.Errorf("resolution = %+v", r)
	}
	if res.Bill.Pending == nil || res.Bill.Pending.DueDate != "2026-11-01" || res.Bill.NextDueDate != "2026-11-01" {
		t.Errorf("next occurrence = %+v", res.Bill.Pending)
	}
	if res.Paid.Status != bills.StatusPaid || res.Paid.ExpenseID == nil || *res.Paid.ExpenseID != res.Expense.ID || res.Paid.DueDate != "2026-10-01" {
		t.Errorf("paid occurrence = %+v", res.Paid)
	}
	if res.Overdue {
		t.Error("the next occurrence is not overdue")
	}
}

func TestPayOverrides(t *testing.T) {
	fx := newFixture()
	b := fx.create(t, megacable()).Bill
	_, err := fx.svc.Pay(context.Background(), b.ID, bills.PaymentInput{
		Date: "2026-10-03", Amount: dec("499.99"), Category: "Hogar", Description: "descuento",
	})
	if err != nil {
		t.Fatalf("Pay: %v", err)
	}
	in := fx.expenses.created[0]
	if in.Date != "2026-10-03" || in.Category != "Hogar" || in.Description != "descuento" || !in.Amount.Equal(*dec("499.99")) {
		t.Errorf("expense input = %+v", in)
	}
}

func TestPayVariableBillNeedsAnAmount(t *testing.T) {
	fx := newFixture()
	in := megacable()
	in.Name, in.Amount = "Luz", nil
	b := fx.create(t, in).Bill
	if _, err := fx.svc.Pay(context.Background(), b.ID, bills.PaymentInput{}); !errors.Is(err, bills.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
	if len(fx.expenses.created) != 0 {
		t.Error("no expense may be created without an amount")
	}
	if _, err := fx.svc.Pay(context.Background(), b.ID, bills.PaymentInput{Amount: dec("212.40")}); err != nil {
		t.Fatalf("Pay with amount: %v", err)
	}
}

func TestPayUSDBillUsesItsCurrency(t *testing.T) {
	fx := newFixture()
	in := megacable()
	in.Name, in.Currency, in.Amount = "Claude", "USD", dec("20")
	b := fx.create(t, in).Bill
	if _, err := fx.svc.Pay(context.Background(), b.ID, bills.PaymentInput{}); err != nil {
		t.Fatal(err)
	}
	if got := fx.expenses.created[0]; got.Currency != "USD" || !got.Amount.Equal(*dec("20")) {
		t.Errorf("expense input = %+v", got)
	}
	if fx.repo.resolved[0].Currency != "USD" {
		t.Errorf("resolution currency = %s", fx.repo.resolved[0].Currency)
	}
}

func TestPayGeneratesTheFollowingOccurrencesWithoutDrifting(t *testing.T) {
	fx := newFixture()
	in := megacable()
	in.NextDueDate = "2027-01-31"
	b := fx.create(t, in).Bill
	var got []string
	for i := 0; i < 3; i++ {
		res, err := fx.svc.Pay(context.Background(), b.ID, bills.PaymentInput{})
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, res.Bill.Pending.DueDate)
	}
	if strings.Join(got, ",") != "2027-02-28,2027-03-31,2027-04-30" {
		t.Errorf("dates = %v", got)
	}
}

func TestPayFailureRemovesTheExpense(t *testing.T) {
	fx := newFixture()
	b := fx.create(t, megacable()).Bill
	fx.repo.resolveErr = errors.New("boom")
	if _, err := fx.svc.Pay(context.Background(), b.ID, bills.PaymentInput{}); err == nil || err.Error() != "boom" {
		t.Fatalf("err = %v, want boom", err)
	}
	if len(fx.expenses.created) != 1 || len(fx.expenses.deleted) != 1 || fx.expenses.deleted[0] != 101 {
		t.Errorf("created %d, deleted %v; the created expense must be deleted", len(fx.expenses.created), fx.expenses.deleted)
	}
	if len(fx.repo.history[b.ID]) != 0 {
		t.Error("nothing may be recorded")
	}
}

func TestPayTwiceRemovesTheSecondExpense(t *testing.T) {
	fx := newFixture()
	b := fx.create(t, megacable()).Bill
	fx.repo.resolveErr = bills.ErrNotPending // the occurrence was resolved by a concurrent request
	if _, err := fx.svc.Pay(context.Background(), b.ID, bills.PaymentInput{}); !errors.Is(err, bills.ErrNotPending) {
		t.Fatalf("err = %v, want ErrNotPending", err)
	}
	if len(fx.expenses.deleted) != 1 {
		t.Errorf("deleted = %v, want the duplicate expense removed", fx.expenses.deleted)
	}
}

func TestPayExpenseFailureChangesNothing(t *testing.T) {
	fx := newFixture()
	b := fx.create(t, megacable()).Bill
	fx.expenses.createErr = ledger.ErrInvalid
	if _, err := fx.svc.Pay(context.Background(), b.ID, bills.PaymentInput{Category: "Nope"}); !errors.Is(err, ledger.ErrInvalid) {
		t.Fatalf("err = %v, want ledger.ErrInvalid", err)
	}
	if len(fx.repo.resolved) != 0 || len(fx.expenses.deleted) != 0 {
		t.Error("nothing may be resolved or deleted")
	}
}

func TestPayRejections(t *testing.T) {
	fx := newFixture()
	if _, err := fx.svc.Pay(context.Background(), 99, bills.PaymentInput{}); !errors.Is(err, bills.ErrNotFound) {
		t.Errorf("unknown: err = %v", err)
	}
	b := fx.create(t, megacable()).Bill
	if _, err := fx.svc.Pay(context.Background(), b.ID, bills.PaymentInput{Date: "nope"}); !errors.Is(err, bills.ErrInvalidInput) {
		t.Errorf("bad date: err = %v", err)
	}
	if err := fx.svc.Deactivate(context.Background(), b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.svc.Pay(context.Background(), b.ID, bills.PaymentInput{}); !errors.Is(err, bills.ErrInactive) {
		t.Errorf("inactive: err = %v", err)
	}
	if len(fx.expenses.created) != 0 {
		t.Error("no expense may be created for a rejected payment")
	}
}

func TestSkipRegistersNoExpenseAndKeepsHistory(t *testing.T) {
	fx := newFixture()
	b := fx.create(t, megacable()).Bill
	st, err := fx.svc.Skip(context.Background(), b.ID)
	if err != nil {
		t.Fatalf("Skip: %v", err)
	}
	if len(fx.expenses.created) != 0 {
		t.Error("skip must not register an expense")
	}
	if st.Bill.Pending == nil || st.Bill.Pending.DueDate != "2026-11-01" {
		t.Errorf("next = %+v", st.Bill.Pending)
	}
	detail, err := fx.svc.Get(context.Background(), b.ID)
	if err != nil || len(detail.History) != 1 || detail.History[0].Status != bills.StatusSkipped || detail.History[0].DueDate != "2026-10-01" {
		t.Errorf("history = %+v, %v", detail.History, err)
	}
	if _, err := fx.svc.Skip(context.Background(), 99); !errors.Is(err, bills.ErrNotFound) {
		t.Errorf("unknown: err = %v", err)
	}
	if err := fx.svc.Deactivate(context.Background(), b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.svc.Skip(context.Background(), b.ID); !errors.Is(err, bills.ErrInactive) {
		t.Errorf("inactive: err = %v", err)
	}
}

func TestOverdueStaysPendingUntilResolved(t *testing.T) {
	fx := newFixture()
	b := fx.create(t, megacable()).Bill // due 2026-10-01, today 2026-10-10
	detail, _ := fx.svc.Get(context.Background(), b.ID)
	if !detail.Overdue || detail.Bill.Pending.Status != bills.StatusPending {
		t.Errorf("detail = %+v", detail.Status)
	}
	again, _ := fx.svc.Get(context.Background(), b.ID)
	if again.Bill.Pending.ID != detail.Bill.Pending.ID {
		t.Error("an overdue occurrence must not be replaced automatically")
	}
}

func TestUpdateKeepsTheAnchorWhenTheDueDateIsUnchanged(t *testing.T) {
	fx := newFixture()
	in := megacable()
	in.NextDueDate = "2026-12-31"
	b := fx.create(t, in).Bill
	// Pay once: the next occurrence is 2027-01-31, then again: 2027-02-28.
	for i := 0; i < 2; i++ {
		if _, err := fx.svc.Pay(context.Background(), b.ID, bills.PaymentInput{}); err != nil {
			t.Fatal(err)
		}
	}
	cur, _ := fx.svc.Get(context.Background(), b.ID)
	if cur.Bill.NextDueDate != "2027-02-28" || cur.Bill.AnchorDay != 31 {
		t.Fatalf("setup: %+v", cur.Bill)
	}

	edit := megacable()
	edit.Name, edit.NextDueDate = "Megacable Plus", "2027-02-28" // form resends the current due date
	st, err := fx.svc.Update(context.Background(), b.ID, edit)
	if err != nil {
		t.Fatal(err)
	}
	if st.Bill.AnchorDay != 31 {
		t.Errorf("anchor = %d, want 31 kept", st.Bill.AnchorDay)
	}

	edit.NextDueDate = "2027-03-15" // the user really moves it
	st, err = fx.svc.Update(context.Background(), b.ID, edit)
	if err != nil {
		t.Fatal(err)
	}
	if st.Bill.AnchorDay != 15 || st.Bill.Pending.DueDate != "2027-03-15" {
		t.Errorf("after move: anchor %d pending %+v", st.Bill.AnchorDay, st.Bill.Pending)
	}
}

func TestUpdateRejectionsAndReactivation(t *testing.T) {
	fx := newFixture()
	b := fx.create(t, megacable()).Bill
	if _, err := fx.svc.Update(context.Background(), 99, megacable()); !errors.Is(err, bills.ErrNotFound) {
		t.Errorf("unknown: err = %v", err)
	}
	bad := megacable()
	bad.Category = "Nope"
	if _, err := fx.svc.Update(context.Background(), b.ID, bad); !errors.Is(err, bills.ErrInvalidInput) {
		t.Errorf("bad category: err = %v", err)
	}
	if err := fx.svc.Deactivate(context.Background(), b.ID); err != nil {
		t.Fatal(err)
	}
	st, err := fx.svc.Update(context.Background(), b.ID, megacable()) // Active: true
	if err != nil || !st.Bill.Active {
		t.Errorf("reactivate: %+v, %v", st.Bill, err)
	}
	if err := fx.svc.Deactivate(context.Background(), 99); !errors.Is(err, bills.ErrNotFound) {
		t.Errorf("deactivate unknown: err = %v", err)
	}
}
