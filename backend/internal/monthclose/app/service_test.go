package app_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	"github.com/valium69mg/finances-app/backend/internal/monthclose/app"
	monthclose "github.com/valium69mg/finances-app/backend/internal/monthclose/domain"
	savings "github.com/valium69mg/finances-app/backend/internal/savings/domain"
	settingsapp "github.com/valium69mg/finances-app/backend/internal/settings/app"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain/settingstest"
	taxfiling "github.com/valium69mg/finances-app/backend/internal/taxfiling/domain"
)

var d = settingstest.D

func ptr(s string) *decimal.Decimal {
	v := d(s)
	return &v
}

type fakeSettings struct {
	cfg        settings.Config
	budgets    []settingsapp.CategoryBudget
	budgetsErr error
	gotMonth   string
}

func (f *fakeSettings) Get(context.Context) (settings.Config, error) { return f.cfg, nil }
func (f *fakeSettings) MonthBudgets(_ context.Context, month string) ([]settingsapp.CategoryBudget, error) {
	f.gotMonth = month
	return f.budgets, f.budgetsErr
}

type fakeFilings struct {
	status taxfiling.MonthStatus
	err    error
	month  string
}

func (f *fakeFilings) MonthStatus(_ context.Context, month string) (taxfiling.MonthStatus, error) {
	f.month = month
	return f.status, f.err
}

type fakeMovements struct{ rows []ledger.Movement }

func (f *fakeMovements) ListByMonth(_ context.Context, month string, kind ledger.Kind, _ int) ([]ledger.Movement, error) {
	var out []ledger.Movement
	for _, m := range f.rows {
		if ledger.MonthOf(m.Date) == month && m.Kind == kind {
			out = append(out, m)
		}
	}
	return out, nil
}
func (f *fakeMovements) ListAllByKind(_ context.Context, kind ledger.Kind) ([]ledger.Movement, error) {
	var out []ledger.Movement
	for _, m := range f.rows {
		if m.Kind == kind {
			out = append(out, m)
		}
	}
	return out, nil
}

type fakeRepo struct {
	closes map[string]monthclose.Close
	err    error
}

func (f *fakeRepo) Create(_ context.Context, c monthclose.Close) (monthclose.Close, error) {
	if f.err != nil {
		return monthclose.Close{}, f.err
	}
	if _, ok := f.closes[c.Period]; ok {
		return monthclose.Close{}, monthclose.ErrAlreadyClosed
	}
	f.closes[c.Period] = c
	return c, nil
}
func (f *fakeRepo) Get(_ context.Context, period string) (monthclose.Close, error) {
	c, ok := f.closes[period]
	if !ok {
		return monthclose.Close{}, monthclose.ErrNotFound
	}
	return c, nil
}
func (f *fakeRepo) List(context.Context) ([]monthclose.Close, error) {
	out := []monthclose.Close{}
	for _, c := range f.closes {
		out = append(out, c)
	}
	return out, nil
}
func (f *fakeRepo) Delete(_ context.Context, period string) error {
	if _, ok := f.closes[period]; !ok {
		return monthclose.ErrNotFound
	}
	delete(f.closes, period)
	return nil
}

func mv(date string, kind ledger.Kind, category, amount string) ledger.Movement {
	return ledger.Movement{Date: date, Kind: kind, Category: category, AmountMXN: d(amount)}
}

type fixture struct {
	svc      *app.Service
	repo     *fakeRepo
	settings *fakeSettings
	mvs      *fakeMovements
	filings  *fakeFilings
}

// newFixture sets today to 2026-10-15, so the default period is 2026-09.
func newFixture() *fixture {
	st := &fakeSettings{
		cfg: settingstest.RealConfig(),
		budgets: []settingsapp.CategoryBudget{
			{Name: "Sueldo", Kind: "Ingreso"},
			{Name: "Vivienda", Kind: "Gasto", Budget: ptr("3600")},
			{Name: "Mandado", Kind: "Gasto", Budget: ptr("1000")},
			{Name: "Ocio", Kind: "Gasto"},
			{Name: "Inversiones", Kind: "Ahorro", Budget: ptr("0")},
		},
	}
	mvs := &fakeMovements{rows: []ledger.Movement{
		mv("2026-09-01", ledger.KindIncome, "Sueldo", "50000"),
		mv("2026-09-02", ledger.KindExpense, "Vivienda", "3600"),
		mv("2026-09-05", ledger.KindExpense, "Mandado", "1500"),
		mv("2026-09-06", ledger.KindExpense, "Ocio", "200"),
		mv("2026-09-07", ledger.KindSavings, "Fondo de emergencia", "4000"),
		mv("2026-08-01", ledger.KindSavings, "Fondo de emergencia", "6000"), // before: counts towards the fund
		mv("2026-10-03", ledger.KindSavings, "Fondo de emergencia", "9000"), // after: not as of 2026-09
		mv("2026-10-04", ledger.KindExpense, "Mandado", "9999"),             // another month
	}}
	repo := &fakeRepo{closes: map[string]monthclose.Close{}}
	filings := &fakeFilings{status: taxfiling.MonthStatus{Payment: taxfiling.PaymentPending, PreviousPeriod: "2026-08"}}
	now := func() time.Time { return time.Date(2026, 10, 15, 12, 0, 0, 123456789, time.UTC) }
	svc := app.NewService(repo, mvs, st, filings, now, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return &fixture{svc: svc, repo: repo, settings: st, mvs: mvs, filings: filings}
}

func TestPreviewDefaultsToThePreviousMonthAndComposesTheClose(t *testing.T) {
	fx := newFixture()
	res, err := fx.svc.Preview(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	c := res.Close
	if c.Period != "2026-09" || fx.settings.gotMonth != "2026-09" || fx.filings.month != "2026-09" {
		t.Errorf("period %s, budgets month %s, filing month %s; want 2026-09", c.Period, fx.settings.gotMonth, fx.filings.month)
	}
	// income 50,000 - expenses 5,300 - savings 4,000 = 40,700.
	if !c.Income.Equal(d("50000")) || !c.Expenses.Equal(d("5300")) || !c.Savings.Equal(d("4000")) || !c.Available.Equal(d("40700")) {
		t.Errorf("totals = %+v", c)
	}
	if len(c.Categories) != 3 || c.Categories[0].Name != "Vivienda" || c.Categories[1].Name != "Mandado" || !c.Categories[1].OverBudget {
		t.Errorf("categories = %+v", c.Categories)
	}
	if c.FilingStatus != taxfiling.PaymentPending || !c.ClosedAt.IsZero() || res.Existing != nil {
		t.Errorf("filing %q closedAt %v existing %v", c.FilingStatus, c.ClosedAt, res.Existing)
	}
	// Emergency fund as of the end of September: 6,000 + 4,000, not the 9,000 of October.
	if !c.Emergency.Accumulated.Equal(d("10000")) {
		t.Errorf("emergency accumulated = %s, want 10000", c.Emergency.Accumulated)
	}
	wantGoal, _ := savings.EmergencyFund(fx.settings.cfg, nil)
	if !c.Emergency.Goal.Equal(wantGoal.Goal) || !c.Emergency.Goal.IsPositive() {
		t.Errorf("goal = %s, want %s", c.Emergency.Goal, wantGoal.Goal)
	}
	// Room to the goal is larger than the leftover: everything to the fund, not paused.
	s := c.Suggestion
	if s == nil || !s.ToEmergencyFund.Equal(d("40700")) || s.InvestmentsPaused {
		t.Errorf("suggestion = %+v", s)
	}
	if len(fx.repo.closes) != 0 {
		t.Error("a preview must never persist")
	}
}

func TestPreviewOfAPausedMonthSuggestsFutureExpenses(t *testing.T) {
	fx := newFixture()
	pause := settingstest.RealPause()
	fx.settings.cfg.Pause = &pause
	fx.mvs.rows = append(fx.mvs.rows, mv("2026-10-01", ledger.KindIncome, "Sueldo", "100000"))

	res, err := fx.svc.Preview(context.Background(), "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	s := res.Close.Suggestion
	if s == nil || !s.InvestmentsPaused || !s.ToInvestments.IsZero() {
		t.Fatalf("suggestion = %+v", s)
	}

	res, err = fx.svc.Preview(context.Background(), "2026-09")
	if err != nil || res.Close.Suggestion.InvestmentsPaused {
		t.Errorf("2026-09 is not paused: %+v, %v", res.Close.Suggestion, err)
	}
}

func TestPreviewReturnsTheExistingClose(t *testing.T) {
	fx := newFixture()
	stored := monthclose.Close{Period: "2026-09", Income: d("1")}
	fx.repo.closes["2026-09"] = stored

	res, err := fx.svc.Preview(context.Background(), "2026-09")
	if err != nil {
		t.Fatal(err)
	}
	if res.Existing == nil || !res.Existing.Income.Equal(d("1")) || !res.Close.Income.Equal(d("50000")) {
		t.Errorf("existing = %+v close income = %s", res.Existing, res.Close.Income)
	}
}

func TestPreviewRejectsAMalformedPeriod(t *testing.T) {
	if _, err := newFixture().svc.Preview(context.Background(), "2026-13"); !errors.Is(err, monthclose.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestCreateStoresAnImmutableSnapshot(t *testing.T) {
	fx := newFixture()
	ctx := context.Background()
	saved, err := fx.svc.Create(ctx, "2026-09")
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 10, 15, 12, 0, 0, 123456000, time.UTC); !saved.ClosedAt.Equal(want) {
		t.Errorf("closed at = %v, want %v", saved.ClosedAt, want)
	}

	// Editing the movements afterwards never changes the stored close.
	fx.mvs.rows = append(fx.mvs.rows, mv("2026-09-20", ledger.KindExpense, "Ocio", "10000"))
	got, err := fx.svc.Get(ctx, "2026-09")
	if err != nil || !got.Expenses.Equal(d("5300")) {
		t.Errorf("stored expenses = %s, %v; want 5300", got.Expenses, err)
	}
	// A fresh preview does see the change, and reports the stored close.
	res, err := fx.svc.Preview(ctx, "2026-09")
	if err != nil || !res.Close.Expenses.Equal(d("15300")) || res.Existing == nil || !res.Existing.Expenses.Equal(d("5300")) {
		t.Errorf("preview = %+v, %v", res, err)
	}
}

func TestCreateTwiceIsAlreadyClosed(t *testing.T) {
	fx := newFixture()
	ctx := context.Background()
	if _, err := fx.svc.Create(ctx, "2026-09"); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.svc.Create(ctx, "2026-09"); !errors.Is(err, monthclose.ErrAlreadyClosed) {
		t.Errorf("err = %v, want ErrAlreadyClosed", err)
	}
}

func TestCreateRefusesTheFutureButNotTheCurrentMonth(t *testing.T) {
	fx := newFixture()
	ctx := context.Background()
	if _, err := fx.svc.Create(ctx, "2026-11"); !errors.Is(err, monthclose.ErrInvalidInput) {
		t.Errorf("future err = %v, want ErrInvalidInput", err)
	}
	if _, err := fx.svc.Create(ctx, ""); !errors.Is(err, monthclose.ErrInvalidInput) {
		t.Errorf("empty err = %v, want ErrInvalidInput", err)
	}
	if _, err := fx.svc.Create(ctx, "2026-10"); err != nil {
		t.Errorf("current month: %v", err)
	}
}

func TestSettingsIncomplete(t *testing.T) {
	ctx := context.Background()

	fx := newFixture()
	fx.settings.budgetsErr = settings.ErrMissingConfig
	if _, err := fx.svc.Preview(ctx, "2026-09"); !errors.Is(err, settings.ErrMissingConfig) {
		t.Errorf("budgets: err = %v, want ErrMissingConfig", err)
	}
	if _, err := fx.svc.Create(ctx, "2026-09"); !errors.Is(err, settings.ErrMissingConfig) || len(fx.repo.closes) != 0 {
		t.Errorf("create with incomplete budgets: err = %v, closes %d", err, len(fx.repo.closes))
	}

	fx = newFixture()
	fx.settings.cfg.EmergencyMonths = nil
	if _, err := fx.svc.Preview(ctx, "2026-09"); !errors.Is(err, settings.ErrMissingConfig) {
		t.Errorf("emergency goal: err = %v, want ErrMissingConfig", err)
	}
}

func TestFilingErrorsPropagate(t *testing.T) {
	fx := newFixture()
	boom := errors.New("boom")
	fx.filings.err = boom
	if _, err := fx.svc.Preview(context.Background(), "2026-09"); !errors.Is(err, boom) {
		t.Errorf("err = %v, want boom", err)
	}
}

func TestRepoErrorsPropagate(t *testing.T) {
	fx := newFixture()
	boom := errors.New("boom")
	fx.repo.err = boom
	if _, err := fx.svc.Create(context.Background(), "2026-09"); !errors.Is(err, boom) {
		t.Errorf("err = %v, want boom", err)
	}
}

func TestDeleteAllowsRegenerating(t *testing.T) {
	fx := newFixture()
	ctx := context.Background()
	if err := fx.svc.Delete(ctx, "2026-09"); !errors.Is(err, monthclose.ErrNotFound) {
		t.Errorf("delete unknown: %v, want ErrNotFound", err)
	}
	if err := fx.svc.Delete(ctx, "x"); !errors.Is(err, monthclose.ErrInvalidInput) {
		t.Errorf("delete malformed: %v, want ErrInvalidInput", err)
	}
	if _, err := fx.svc.Create(ctx, "2026-09"); err != nil {
		t.Fatal(err)
	}
	if err := fx.svc.Delete(ctx, "2026-09"); err != nil {
		t.Fatal(err)
	}
	if len(fx.mvs.rows) != 8 {
		t.Error("deleting a close must not touch the movements")
	}
	if _, err := fx.svc.Create(ctx, "2026-09"); err != nil {
		t.Errorf("regenerate: %v", err)
	}
}

func TestGetAndList(t *testing.T) {
	fx := newFixture()
	ctx := context.Background()
	if _, err := fx.svc.Get(ctx, "2026-09"); !errors.Is(err, monthclose.ErrNotFound) {
		t.Errorf("get unknown: %v", err)
	}
	if _, err := fx.svc.Get(ctx, "nope"); !errors.Is(err, monthclose.ErrInvalidInput) {
		t.Errorf("get malformed: %v", err)
	}
	if _, err := fx.svc.Create(ctx, "2026-09"); err != nil {
		t.Fatal(err)
	}
	list, err := fx.svc.List(ctx)
	if err != nil || len(list) != 1 {
		t.Errorf("list = %v, %v", list, err)
	}
}
