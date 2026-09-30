package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/dashboard/app"
	incomeapp "github.com/valium69mg/finances-app/backend/internal/income/app"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
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

type fakeIncome struct {
	summary incomeapp.Summary
	err     error
	month   string
}

func (f *fakeIncome) MonthSummary(_ context.Context, month string) (incomeapp.Summary, error) {
	f.month = month
	return f.summary, f.err
}

type fakeFilings struct {
	status taxfiling.MonthStatus
	err    error
	month  string
	calls  int
}

func (f *fakeFilings) MonthStatus(_ context.Context, month string) (taxfiling.MonthStatus, error) {
	f.month = month
	f.calls++
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

func mv(date string, kind ledger.Kind, category, amount string) ledger.Movement {
	return ledger.Movement{Date: date, Kind: kind, Category: category, AmountMXN: d(amount)}
}

func newService() (*app.Service, *fakeSettings, *fakeIncome, *fakeMovements) {
	svc, st, inc, mvs, _ := newServiceWithFilings()
	return svc, st, inc, mvs
}

func newServiceWithFilings() (*app.Service, *fakeSettings, *fakeIncome, *fakeMovements, *fakeFilings) {
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
	inc := &fakeIncome{summary: incomeapp.Summary{
		MonthTotalMXN: d("50000"),
		Resico:        &incomeapp.ResicoEstimate{Rate: d("0.011"), EstimatedISR: d("550")},
	}}
	mvs := &fakeMovements{rows: []ledger.Movement{
		mv("2026-10-02", ledger.KindExpense, "Vivienda", "3600"),
		mv("2026-10-05", ledger.KindExpense, "Mandado", "1500"),
		mv("2026-10-06", ledger.KindExpense, "Ocio", "200"),
		mv("2026-09-30", ledger.KindExpense, "Mandado", "9999"), // another month
		mv("2026-10-07", ledger.KindSavings, "Fondo de emergencia", "4000"),
		mv("2026-10-08", ledger.KindSavings, "Inversiones", "1000"),
		mv("2026-09-01", ledger.KindSavings, "Fondo de emergencia", "6000"), // counts only towards the fund
		mv("2026-10-03", ledger.KindIncome, "Sueldo", "50000"),              // income comes from the income port
	}}
	now := func() time.Time { return time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC) }
	filings := &fakeFilings{status: taxfiling.MonthStatus{Payment: taxfiling.PaymentNone, PreviousPeriod: "2026-09"}}
	return app.NewService(mvs, st, inc, filings, now), st, inc, mvs, filings
}

func TestMonthComposesTheSummary(t *testing.T) {
	svc, st, inc, mvs := newService()
	got, err := svc.Month(context.Background(), "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if got.Month != "2026-10" || st.gotMonth != "2026-10" || inc.month != "2026-10" {
		t.Errorf("month not propagated: %q, %q, %q", got.Month, st.gotMonth, inc.month)
	}

	// Only Gasto categories, in configuration order, with month movements only.
	if len(got.Rows) != 3 {
		t.Fatalf("rows = %+v, want the 3 Gasto ones", got.Rows)
	}
	vivienda, mandado, ocio := got.Rows[0], got.Rows[1], got.Rows[2]
	if vivienda.Name != "Vivienda" || !vivienda.Real.Equal(d("3600")) || vivienda.OverBudget() || !vivienda.Diff.IsZero() {
		t.Errorf("Vivienda = %+v", vivienda)
	}
	if !mandado.Real.Equal(d("1500")) || !mandado.OverBudget() || !mandado.Diff.Equal(d("-500")) {
		t.Errorf("Mandado = %+v", mandado)
	}
	if ocio.Budget != nil || ocio.Diff != nil || ocio.OverBudget() || !ocio.Real.Equal(d("200")) {
		t.Errorf("Ocio = %+v", ocio)
	}

	if !got.Totals.Income.Equal(d("50000")) || !got.Totals.Expenses.Equal(d("5300")) || !got.Totals.Savings.Equal(d("5000")) {
		t.Errorf("totals = %+v", got.Totals)
	}
	// Available = income - expenses - savings, as fin.py.
	if !got.Available.Equal(d("39700")) {
		t.Errorf("available = %s, want 39700", got.Available)
	}

	// The fund is the all-time balance against the goal the savings domain computes.
	all, _ := mvs.ListAllByKind(context.Background(), ledger.KindSavings)
	want, err := savings.EmergencyFund(st.cfg, all)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Emergency.Accumulated.Equal(d("10000")) || !got.Emergency.Goal.Equal(want.Goal) {
		t.Errorf("emergency = %+v, want accumulated 10000 goal %s", got.Emergency, want.Goal)
	}

	if got.Tax == nil || !got.Tax.Rate.Equal(d("0.011")) || !got.Tax.EstimatedISR.Equal(d("550")) {
		t.Errorf("tax = %+v", got.Tax)
	}
}

func TestMonthDefaultsToTheCurrentMonth(t *testing.T) {
	svc, st, _, _ := newService()
	got, err := svc.Month(context.Background(), "")
	if err != nil || got.Month != "2026-10" || st.gotMonth != "2026-10" {
		t.Errorf("Month(\"\") = %q, %v (settings month %q)", got.Month, err, st.gotMonth)
	}
}

func TestMonthRejectsAMalformedMonth(t *testing.T) {
	svc, _, _, _ := newService()
	for _, month := range []string{"2026-13", "202610", "hoy", "2026-1"} {
		if _, err := svc.Month(context.Background(), month); !errors.Is(err, ledger.ErrInvalid) {
			t.Errorf("Month(%q) err = %v, want ErrInvalid", month, err)
		}
	}
}

func TestMonthEmptyMonth(t *testing.T) {
	svc, _, inc, _ := newService()
	inc.summary = incomeapp.Summary{Resico: &incomeapp.ResicoEstimate{Rate: d("0.01"), EstimatedISR: d("0")}}
	got, err := svc.Month(context.Background(), "2027-02")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Totals.Income.IsZero() || !got.Totals.Expenses.IsZero() || !got.Totals.Savings.IsZero() || !got.Available.IsZero() {
		t.Errorf("empty month totals = %+v", got)
	}
	for _, c := range got.Rows {
		if !c.Real.IsZero() || c.OverBudget() {
			t.Errorf("empty month row = %+v", c)
		}
	}
}

func TestMonthWithoutTaxSettingsHasNoTaxCard(t *testing.T) {
	svc, _, inc, _, filings := newServiceWithFilings()
	inc.summary.Resico = nil
	got, err := svc.Month(context.Background(), "2026-10")
	if err != nil || got.Tax != nil {
		t.Errorf("tax = %+v, err = %v, want nil tax and no error", got.Tax, err)
	}
	if filings.calls != 0 {
		t.Error("the filings are only queried when there is a tax card")
	}
}

func TestMonthTaxCardCarriesTheFilingStatus(t *testing.T) {
	svc, _, _, _, filings := newServiceWithFilings()
	filings.status = taxfiling.MonthStatus{Payment: taxfiling.PaymentPaid, PreviousPeriod: "2026-09", PreviousPending: true}
	got, err := svc.Month(context.Background(), "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if filings.month != "2026-10" {
		t.Errorf("status queried for %q, want the requested month", filings.month)
	}
	// The ISR estimate and its rate are kept next to the status.
	if got.Tax == nil || !got.Tax.Rate.Equal(d("0.011")) || !got.Tax.EstimatedISR.Equal(d("550")) ||
		got.Tax.Filing.Payment != taxfiling.PaymentPaid || got.Tax.Filing.PreviousPeriod != "2026-09" || !got.Tax.Filing.PreviousPending {
		t.Errorf("tax = %+v", got.Tax)
	}
}

func TestMonthPropagatesFilingErrors(t *testing.T) {
	svc, _, _, _, filings := newServiceWithFilings()
	boom := errors.New("boom")
	filings.err = boom
	if _, err := svc.Month(context.Background(), "2026-10"); !errors.Is(err, boom) {
		t.Errorf("err = %v, want boom", err)
	}
}

func TestMonthSettingsIncomplete(t *testing.T) {
	t.Run("budgets", func(t *testing.T) {
		svc, st, _, _ := newService()
		st.budgetsErr = settings.ErrMissingConfig
		if _, err := svc.Month(context.Background(), "2026-10"); !errors.Is(err, settings.ErrMissingConfig) {
			t.Errorf("err = %v, want ErrMissingConfig", err)
		}
	})
	t.Run("emergency months", func(t *testing.T) {
		svc, st, _, _ := newService()
		st.cfg.EmergencyMonths = nil
		if _, err := svc.Month(context.Background(), "2026-10"); !errors.Is(err, settings.ErrMissingConfig) {
			t.Errorf("err = %v, want ErrMissingConfig", err)
		}
	})
}

func TestMonthPropagatesIncomeErrors(t *testing.T) {
	svc, _, inc, _ := newService()
	boom := errors.New("boom")
	inc.err = boom
	if _, err := svc.Month(context.Background(), "2026-10"); !errors.Is(err, boom) {
		t.Errorf("err = %v, want boom", err)
	}
}

func TestMonthFollowsTheConfiguredCycle(t *testing.T) {
	svc, st, _, mvs := newService()
	st.cfg.CycleStartDay = 31
	mvs.rows = append(mvs.rows,
		mv("2026-09-30", ledger.KindExpense, "Mandado", "300"), // first day of cycle 2026-10
		mv("2026-10-31", ledger.KindExpense, "Mandado", "700"), // belongs to cycle 2026-11
	)
	got, err := svc.Month(context.Background(), "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if got.PeriodStart != "2026-09-30" || got.PeriodEnd != "2026-10-30" {
		t.Errorf("period = %s..%s, want 2026-09-30..2026-10-30", got.PeriodStart, got.PeriodEnd)
	}
	// 3600 + 1500 + 200 of October plus the 9999 and 300 of 2026-09-30; the
	// 700 of 2026-10-31 is out.
	if !got.Totals.Expenses.Equal(d("15599")) {
		t.Errorf("expenses = %s, want 15599", got.Totals.Expenses)
	}
	// The tax card stays fiscal: the cycle label is used as the calendar month.
	if st.gotMonth != "2026-10" {
		t.Errorf("budgets resolved for %q", st.gotMonth)
	}

	// At 2026-10-15 the current cycle of a day-31 calendar is still 2026-10.
	if got, err := svc.Month(context.Background(), ""); err != nil || got.Month != "2026-10" {
		t.Errorf("Month(\"\") = %q, %v, want 2026-10", got.Month, err)
	}
}

func TestMonthDefaultPeriodIsTheCalendarMonth(t *testing.T) {
	svc, _, _, _ := newService()
	got, err := svc.Month(context.Background(), "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if got.PeriodStart != "2026-10-01" || got.PeriodEnd != "2026-10-31" {
		t.Errorf("period = %s..%s, want the calendar month", got.PeriodStart, got.PeriodEnd)
	}
}
