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

func (f *fakeMovements) ListByRange(_ context.Context, from, to string, kind ledger.Kind, _ int) ([]ledger.Movement, error) {
	var out []ledger.Movement
	for _, m := range f.rows {
		if m.Date >= from && m.Date <= to && m.Kind == kind {
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

// The default period follows the service clock in the clock's own zone, like
// the tax filing preview and the closable check, not UTC: around a month
// boundary the two disagree.
func TestPreviewDefaultPeriodUsesTheClockZoneAtAMonthBoundary(t *testing.T) {
	for _, tc := range []struct {
		name string
		now  time.Time
		want string
	}{
		// 20:00 on Oct 31 in UTC-6 is already Nov 1 in UTC: local October, so previous is September.
		{"west of UTC, late on the last day", time.Date(2026, 10, 31, 20, 0, 0, 0, time.FixedZone("UTC-6", -6*3600)), "2026-09"},
		// 01:00 on Oct 1 in UTC+13 is still Sep 30 in UTC: local October, so previous is September.
		{"east of UTC, early on the first day", time.Date(2026, 10, 1, 1, 0, 0, 0, time.FixedZone("UTC+13", 13*3600)), "2026-09"},
		// 21:00 on Sep 30 in UTC-6 is Oct 1 in UTC: local September, so previous is August.
		{"west of UTC, evening before the new month", time.Date(2026, 9, 30, 21, 0, 0, 0, time.FixedZone("UTC-6", -6*3600)), "2026-08"},
		{"year boundary", time.Date(2027, 1, 1, 0, 30, 0, 0, time.FixedZone("UTC+13", 13*3600)), "2026-12"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newFixture()
			now := tc.now
			svc := app.NewService(fx.repo, fx.mvs, fx.settings, fx.filings, func() time.Time { return now }, slog.New(slog.NewTextHandler(io.Discard, nil)))
			res, err := svc.Preview(context.Background(), "")
			if err != nil {
				t.Fatal(err)
			}
			if res.Close.Period != tc.want {
				t.Errorf("default period = %s, want %s (clock %s)", res.Close.Period, tc.want, tc.now)
			}
		})
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

func TestCycleSelectsTheMovementsAndTheFundHorizon(t *testing.T) {
	fx := newFixture()
	fx.settings.cfg.CycleStartDay = 31
	fx.mvs.rows = append(fx.mvs.rows,
		mv("2026-09-30", ledger.KindExpense, "Mandado", "100"),             // first day of cycle 2026-10
		mv("2026-09-30", ledger.KindSavings, "Fondo de emergencia", "500"), // same day, counts from cycle 2026-10
	)

	// Default period: the cycle before the current one (2026-10), so 2026-09,
	// which ends on 2026-09-29 and leaves the 2026-09-30 movements out.
	res, err := fx.svc.Preview(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Close.Period != "2026-09" || !res.Close.Expenses.Equal(d("5300")) {
		t.Errorf("period %s expenses %s, want 2026-09 and 5300", res.Close.Period, res.Close.Expenses)
	}
	if !res.Close.Emergency.Accumulated.Equal(d("10000")) {
		t.Errorf("fund = %s, want 10000 as of 2026-09-29", res.Close.Emergency.Accumulated)
	}

	res, err = fx.svc.Preview(context.Background(), "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	// 2026-09-30..2026-10-30: the 2026-09-30 expense plus the 9,999 of 10-04.
	if !res.Close.Expenses.Equal(d("10099")) {
		t.Errorf("2026-10 expenses = %s, want 10099", res.Close.Expenses)
	}
	if !res.Close.Emergency.Accumulated.Equal(d("19500")) {
		t.Errorf("2026-10 fund = %s, want 19500 as of 2026-10-30", res.Close.Emergency.Accumulated)
	}
	// Filing is fiscal: the cycle label is passed as the calendar month.
	if fx.filings.month != "2026-10" {
		t.Errorf("filing month = %s, want 2026-10", fx.filings.month)
	}
}

func TestCreateClosableFollowsTheCycle(t *testing.T) {
	fx := newFixture()
	fx.settings.cfg.CycleStartDay = 31
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC) // already cycle 2026-11
	svc := app.NewService(fx.repo, fx.mvs, fx.settings, fx.filings, func() time.Time { return now }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := svc.Create(context.Background(), "2026-11"); err != nil {
		t.Errorf("closing the current cycle 2026-11: %v", err)
	}
	if _, err := svc.Create(context.Background(), "2026-12"); !errors.Is(err, monthclose.ErrInvalidInput) {
		t.Errorf("closing the future cycle 2026-12: err = %v, want ErrInvalidInput", err)
	}
}
