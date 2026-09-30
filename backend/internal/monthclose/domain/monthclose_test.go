package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	dashboard "github.com/valium69mg/finances-app/backend/internal/dashboard/domain"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	monthclose "github.com/valium69mg/finances-app/backend/internal/monthclose/domain"
	savings "github.com/valium69mg/finances-app/backend/internal/savings/domain"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain/settingstest"
	taxfiling "github.com/valium69mg/finances-app/backend/internal/taxfiling/domain"
)

var d = settingstest.D

func budget(name string, kind ledger.Kind, amount string) dashboard.Budget {
	b := dashboard.Budget{Name: name, Kind: kind}
	if amount != "" {
		v := d(amount)
		b.Amount = &v
	}
	return b
}

func mov(date string, kind ledger.Kind, category, amount string) ledger.Movement {
	return ledger.Movement{Date: date, Kind: kind, Category: category, AmountMXN: d(amount)}
}

// closeBudgets: four expense categories of 1,000, one without a budget and
// three savings categories, so every figure can be checked by hand.
func closeBudgets() []dashboard.Budget {
	return []dashboard.Budget{
		budget("Sueldo", ledger.KindIncome, ""),
		budget("Vivienda", ledger.KindExpense, "1000"),
		budget("Servicios", ledger.KindExpense, "1000"),
		budget("Mandado", ledger.KindExpense, "1000"),
		budget("Salud", ledger.KindExpense, "1000"),
		budget("Ocio", ledger.KindExpense, ""),
		budget("Fondo de emergencia", ledger.KindSavings, "1000"),
		budget("Inversiones", ledger.KindSavings, "0"),
	}
}

// octoberMovements has income 10,000, expenses 4,000 and savings 2,000, so the
// leftover is 4,000.
func octoberMovements() []ledger.Movement {
	return []ledger.Movement{
		mov("2026-10-01", ledger.KindIncome, "Sueldo", "10000"),
		mov("2026-10-02", ledger.KindExpense, "Vivienda", "1200"),  // exactly +20%: no hint, over budget
		mov("2026-10-03", ledger.KindExpense, "Servicios", "1201"), // +20.1%: hint, over budget
		mov("2026-10-04", ledger.KindExpense, "Mandado", "799"),    // -20.1%: hint, under
		mov("2026-10-05", ledger.KindExpense, "Salud", "800"),      // exactly -20%: no hint
		mov("2026-10-06", ledger.KindSavings, "Fondo de emergencia", "1500"),
		mov("2026-10-07", ledger.KindSavings, "Inversiones", "500"), // zero budget: no hint
	}
}

func input(monthly []ledger.Movement, accumulated string, paused bool) monthclose.Input {
	return monthclose.Input{
		Period:            "2026-10",
		Budgets:           closeBudgets(),
		Monthly:           monthly,
		Emergency:         savings.EmergencyStatus{Accumulated: d(accumulated), Goal: d("24000")},
		InvestmentsPaused: paused,
		FilingStatus:      taxfiling.PaymentPending,
	}
}

func TestComputeTotalsAndCategories(t *testing.T) {
	got := monthclose.Compute(input(octoberMovements(), "22500", false))

	// income 10,000 - expenses 4,000 - savings 2,000 = 4,000.
	for name, pair := range map[string][2]decimal.Decimal{
		"income":    {got.Income, d("10000")},
		"expenses":  {got.Expenses, d("4000")},
		"savings":   {got.Savings, d("2000")},
		"available": {got.Available, d("4000")},
	} {
		if !pair[0].Equal(pair[1]) {
			t.Errorf("%s = %s, want %s", name, pair[0], pair[1])
		}
	}
	if got.Period != "2026-10" || got.FilingStatus != taxfiling.PaymentPending || !got.ClosedAt.IsZero() {
		t.Errorf("header = %+v", got)
	}
	if !got.Emergency.Accumulated.Equal(d("22500")) || !got.Emergency.Goal.Equal(d("24000")) {
		t.Errorf("emergency = %+v", got.Emergency)
	}

	// Gasto rows only, in budget order; the category without budget has nil
	// budget and remaining and is never over budget.
	wantNames := []string{"Vivienda", "Servicios", "Mandado", "Salud", "Ocio"}
	if len(got.Categories) != len(wantNames) {
		t.Fatalf("categories = %+v", got.Categories)
	}
	for i, n := range wantNames {
		if got.Categories[i].Name != n {
			t.Errorf("category %d = %s, want %s", i, got.Categories[i].Name, n)
		}
	}
	over := map[string]bool{"Vivienda": true, "Servicios": true}
	for _, c := range got.Categories {
		if c.OverBudget != over[c.Name] {
			t.Errorf("%s over budget = %v", c.Name, c.OverBudget)
		}
	}
	viv := got.Categories[0]
	if !viv.Spent.Equal(d("1200")) || viv.Budget == nil || !viv.Budget.Equal(d("1000")) || viv.Remaining == nil || !viv.Remaining.Equal(d("-200")) {
		t.Errorf("Vivienda = %+v", viv)
	}
	ocio := got.Categories[4]
	if ocio.Budget != nil || ocio.Remaining != nil || ocio.OverBudget {
		t.Errorf("Ocio = %+v", ocio)
	}
}

func TestComputeAdjustments(t *testing.T) {
	got := monthclose.Compute(input(octoberMovements(), "22500", false))
	// Only deviations strictly above 20% are flagged: Vivienda (+20%) and Salud
	// (-20%) sit exactly on the boundary, Ocio has no budget and Inversiones a
	// zero budget.
	want := []struct {
		name, budget, real, deviation string
		kind                          ledger.Kind
	}{
		{"Servicios", "1000", "1201", "20.1", ledger.KindExpense},
		{"Mandado", "1000", "799", "-20.1", ledger.KindExpense},
		{"Fondo de emergencia", "1000", "1500", "50", ledger.KindSavings},
	}
	if len(got.Adjustments) != len(want) {
		t.Fatalf("adjustments = %+v, want %d", got.Adjustments, len(want))
	}
	for i, w := range want {
		g := got.Adjustments[i]
		if g.Name != w.name || g.Kind != w.kind || !g.Budget.Equal(d(w.budget)) ||
			!g.Real.Equal(d(w.real)) || !g.DeviationPct.Equal(d(w.deviation)) {
			t.Errorf("adjustment %d = %+v, want %+v", i, g, w)
		}
	}
}

func TestComputeExactly20PercentIsNotAHint(t *testing.T) {
	monthly := []ledger.Movement{
		mov("2026-10-02", ledger.KindExpense, "Vivienda", "1200"),
		mov("2026-10-03", ledger.KindExpense, "Salud", "800"),
	}
	in := input(monthly, "0", false)
	in.Budgets = []dashboard.Budget{budget("Vivienda", ledger.KindExpense, "1000"), budget("Salud", ledger.KindExpense, "1000")}
	if got := monthclose.Compute(in); len(got.Adjustments) != 0 {
		t.Errorf("adjustments = %+v, want none at exactly +-20%%", got.Adjustments)
	}
}

func TestComputeSuggestion(t *testing.T) {
	// income 10,000 - expenses 4,000 - savings 2,000 = 4,000 leftover.
	tests := []struct {
		name        string
		accumulated string
		paused      bool
		toFund      string
		toInvest    string
		toFuture    string
	}{
		// Room 1,500: 1,500 to the fund, the rest to investments.
		{"fund has room", "22500", false, "1500", "2500", "0"},
		// Goal reached (24,500 >= 24,000): no room, all to investments.
		{"fund complete", "24500", false, "0", "4000", "0"},
		// Room 22,500 exceeds the leftover: all to the fund.
		{"fund far from goal", "1500", false, "4000", "0", "0"},
		// Paused: the fund is still first, the remainder goes to Gastos futuros.
		{"paused fund has room", "22500", true, "1500", "0", "2500"},
		{"paused fund complete", "24500", true, "0", "0", "4000"},
		{"paused fund far from goal", "1500", true, "4000", "0", "0"},
		// Exactly the goal is complete (room 0).
		{"fund exactly at goal", "24000", false, "0", "4000", "0"},
	}
	for _, tc := range tests {
		got := monthclose.Compute(input(octoberMovements(), tc.accumulated, tc.paused))
		s := got.Suggestion
		if s == nil {
			t.Fatalf("%s: no suggestion", tc.name)
		}
		if !s.ToEmergencyFund.Equal(d(tc.toFund)) || !s.ToInvestments.Equal(d(tc.toInvest)) ||
			!s.ToFutureExpenses.Equal(d(tc.toFuture)) || s.InvestmentsPaused != tc.paused {
			t.Errorf("%s: suggestion = %+v", tc.name, *s)
		}
	}
}

func TestComputeNoSuggestionWithoutLeftover(t *testing.T) {
	negative := []ledger.Movement{
		mov("2026-10-01", ledger.KindIncome, "Sueldo", "1000"),
		mov("2026-10-02", ledger.KindExpense, "Vivienda", "4000"),
		mov("2026-10-03", ledger.KindSavings, "Inversiones", "2000"),
	}
	got := monthclose.Compute(input(negative, "0", false))
	if !got.Available.Equal(d("-5000")) || got.Suggestion != nil {
		t.Errorf("available %s suggestion %v, want -5000 and nil", got.Available, got.Suggestion)
	}

	zero := []ledger.Movement{
		mov("2026-10-01", ledger.KindIncome, "Sueldo", "1000"),
		mov("2026-10-02", ledger.KindExpense, "Vivienda", "1000"),
	}
	if got := monthclose.Compute(input(zero, "0", false)); got.Suggestion != nil {
		t.Errorf("zero leftover suggestion = %+v", got.Suggestion)
	}
}

func TestComputeWithdrawalsCountAsNegativeSavings(t *testing.T) {
	monthly := []ledger.Movement{
		mov("2026-10-01", ledger.KindIncome, "Sueldo", "1000"),
		mov("2026-10-02", ledger.KindSavings, "Inversiones", "-300"),
	}
	got := monthclose.Compute(input(monthly, "0", false))
	if !got.Savings.Equal(d("-300")) || !got.Available.Equal(d("1300")) {
		t.Errorf("savings %s available %s, want -300 and 1300", got.Savings, got.Available)
	}
}

func TestComputeCategoriesWithoutBudgetsHaveNoHints(t *testing.T) {
	in := input([]ledger.Movement{mov("2026-10-02", ledger.KindExpense, "Ocio", "5000")}, "0", false)
	in.Budgets = []dashboard.Budget{budget("Ocio", ledger.KindExpense, ""), budget("Ropa", ledger.KindExpense, "")}
	got := monthclose.Compute(in)
	if len(got.Adjustments) != 0 || got.Categories[0].OverBudget {
		t.Errorf("hints %+v over %v, want none for categories without budget", got.Adjustments, got.Categories[0].OverBudget)
	}
}

func TestMovementsUpTo(t *testing.T) {
	all := []ledger.Movement{
		mov("2026-09-30", ledger.KindSavings, "Fondo de emergencia", "1"),
		mov("2026-10-31", ledger.KindSavings, "Fondo de emergencia", "2"),
		mov("2026-11-01", ledger.KindSavings, "Fondo de emergencia", "4"),
	}
	got := monthclose.MovementsUpTo(all, "2026-10")
	if len(got) != 2 || !ledger.SumBy(got, ledger.Filter{Kind: ledger.KindSavings}).Equal(d("3")) {
		t.Errorf("up to 2026-10 = %+v", got)
	}
}

func TestValidatePeriod(t *testing.T) {
	for _, p := range []string{"2026-10", "1999-01", "2026-12"} {
		if err := monthclose.ValidatePeriod(p); err != nil {
			t.Errorf("%q: %v", p, err)
		}
	}
	for _, p := range []string{"", "2026-13", "2026-00", "2026-1", "26-10", "2026-10-01", "abcd-ef"} {
		if err := monthclose.ValidatePeriod(p); !errors.Is(err, monthclose.ErrInvalidInput) {
			t.Errorf("%q: err = %v, want ErrInvalidInput", p, err)
		}
	}
}

func TestValidateClosable(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	for _, p := range []string{"2026-09", "2026-10", "2020-01"} {
		if err := monthclose.ValidateClosable(p, now); err != nil {
			t.Errorf("%q: %v", p, err)
		}
	}
	for _, p := range []string{"2026-11", "2027-01"} {
		if err := monthclose.ValidateClosable(p, now); !errors.Is(err, monthclose.ErrInvalidInput) {
			t.Errorf("%q: err = %v, want ErrInvalidInput", p, err)
		}
	}
}
