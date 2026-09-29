package domain_test

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	dashboard "github.com/valium69mg/finances-app/backend/internal/dashboard/domain"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain/settingstest"
)

var d = settingstest.D

func mov(date string, kind ledger.Kind, category, amount string) ledger.Movement {
	return ledger.Movement{Date: date, Kind: kind, Category: category, AmountMXN: d(amount)}
}

func octoberMovements() []ledger.Movement {
	return []ledger.Movement{
		mov("2026-10-01", ledger.KindIncome, "Sueldo", "60020.2742"),
		mov("2026-10-31", ledger.KindIncome, "Contrato extra", "35000"),
		mov("2026-10-02", ledger.KindExpense, "Vivienda", "3600"),
		mov("2026-10-10", ledger.KindExpense, "Mandado", "12000"),
		mov("2026-10-15", ledger.KindExpense, "Impuestos", "900"),
		mov("2026-10-16", ledger.KindExpense, "Categoría rara", "100"), // not configured
		mov("2026-10-31", ledger.KindSavings, "Reserva SAT", "5775"),
		mov("2026-10-31", ledger.KindSavings, "Fondo de emergencia", "14612"),
		mov("2026-10-31", ledger.KindSavings, "Inversiones", "10229"),
		mov("2026-10-31", ledger.KindSavings, "Aguinaldo y vacaciones", "4384"),
		// Earlier month: excluded from the month, included in all-time balances.
		mov("2026-09-10", ledger.KindSavings, "Reserva SAT", "1000"),
		mov("2026-09-12", ledger.KindSavings, "Fondo de emergencia", "1000"),
		// Later month: excluded from the month.
		mov("2026-11-02", ledger.KindExpense, "Mandado", "500"),
	}
}

func TestComputeSummary(t *testing.T) {
	cfg := settingstest.RealConfig()
	s, err := dashboard.ComputeSummary(cfg, octoberMovements(), "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if s.Month != "2026-10" || len(s.Rows) != len(cfg.Categories) {
		t.Fatalf("month %s with %d rows", s.Month, len(s.Rows))
	}

	// Totals cover every movement of the month, configured category or not.
	// Income 95,020.2742; expenses 3,600 + 12,000 + 900 + 100; savings 35,000.
	if !s.Totals.Income.Equal(d("95020.2742")) || !s.Totals.Expenses.Equal(d("16600")) || !s.Totals.Savings.Equal(d("35000")) {
		t.Errorf("totals = %+v", s.Totals)
	}
	if !s.Available.Equal(d("43420.2742")) {
		t.Errorf("Available = %s, want 43420.2742", s.Available)
	}
	// 95,020.2742 falls in the 2% bracket.
	if !s.ISR.Rate.Equal(d("0.02")) || !s.ISR.Real.Equal(d("1900.405484")) || !s.ISR.Budget.Equal(d("931.35")) {
		t.Errorf("ISR = %+v", s.ISR)
	}
	// Emergency fund is all-time: 14,612 + 1,000 of September.
	if !s.Emergency.Accumulated.Equal(d("15612")) || !s.Emergency.Goal.Equal(d("166922.64")) {
		t.Errorf("Emergency = %+v", s.Emergency)
	}

	b := s.ClientB
	if !b.MonthTotal.Equal(d("35000")) || !b.ReserveMonth.Equal(d("5775")) || !b.ReserveBalance.Equal(d("6775")) ||
		!b.MonthSplit.EmergencyFund.Equal(d("14612")) || !b.MonthSplit.Investments.Equal(d("10229")) ||
		!b.MonthSplit.AguinaldoVacation.Equal(d("4384")) {
		t.Errorf("ClientB = %+v", b)
	}
	if !s.TaxBreakdown.TaxesBudget.Equal(d("931.35")) {
		t.Errorf("TaxBreakdown.TaxesBudget = %s", s.TaxBreakdown.TaxesBudget)
	}
}

func TestSummaryRows(t *testing.T) {
	s, err := dashboard.ComputeSummary(settingstest.RealConfig(), octoberMovements(), "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]dashboard.Row{}
	for _, r := range s.Rows {
		byName[r.Name] = r
	}

	tests := []struct {
		name         string
		budget, real string
		diff, pct    string // empty means nil
	}{
		{"Vivienda", "3600", "3600", "0", "100"},                 // exactly on budget
		{"Mandado", "10833", "12000", "-1167", "110.7726391581"}, // over budget
		{"Impuestos", "931.35", "900", "31.35", "96.6339185054"}, // computed budget replaces the nil one
		{"Servicios", "2000", "0", "2000", "0"},                  // no movements
		{"Inversiones", "0", "10229", "-10229", ""},              // zero budget: diff yes, percentage no
		{"Reserva SAT", "", "5775", "", ""},                      // no budget at all
		{"Sueldo", "", "60020.2742", "", ""},                     // income categories have no budget
	}
	for _, tc := range tests {
		r := byName[tc.name]
		if !r.Real.Equal(d(tc.real)) {
			t.Errorf("%s real = %s, want %s", tc.name, r.Real, tc.real)
		}
		checkOptional(t, tc.name+" budget", r.Budget, tc.budget)
		checkOptional(t, tc.name+" diff", r.Diff, tc.diff)
		checkOptional(t, tc.name+" pct", r.Pct, tc.pct)
	}

	// Rows keep the configuration order.
	if s.Rows[0].Name != "Sueldo" || s.Rows[len(s.Rows)-1].Name != "Reserva SAT" {
		t.Errorf("row order: first %s, last %s", s.Rows[0].Name, s.Rows[len(s.Rows)-1].Name)
	}
}

// checkOptional asserts a nullable decimal: an empty want means nil.
func checkOptional(t *testing.T, label string, got *decimal.Decimal, want string) {
	t.Helper()
	switch {
	case want == "" && got != nil:
		t.Errorf("%s = %s, want nil", label, got)
	case want != "" && got == nil:
		t.Errorf("%s = nil, want %s", label, want)
	case want != "" && !got.Equal(d(want)):
		t.Errorf("%s = %s, want %s", label, got, want)
	}
}

func TestSummaryEmptyMonthAndErrors(t *testing.T) {
	cfg := settingstest.RealConfig()
	s, err := dashboard.ComputeSummary(cfg, nil, "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	// No income: the first bracket applies but there is nothing to tax.
	if !s.ISR.Rate.Equal(d("0.01")) || !s.ISR.Real.IsZero() || !s.Available.IsZero() {
		t.Errorf("empty month = %+v", s)
	}

	if _, err := dashboard.ComputeSummary(settings.Config{}, nil, "2026-10"); !errors.Is(err, settings.ErrMissingConfig) {
		t.Errorf("empty config error = %v, want ErrMissingConfig", err)
	}
}
