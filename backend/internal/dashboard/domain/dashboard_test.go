package domain_test

import (
	"testing"

	"github.com/shopspring/decimal"

	dashboard "github.com/valium69mg/finances-app/backend/internal/dashboard/domain"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain/settingstest"
)

var d = settingstest.D

func mov(date string, kind ledger.Kind, category, amount string) ledger.Movement {
	return ledger.Movement{Date: date, Kind: kind, Category: category, AmountMXN: d(amount)}
}

func TestRowMath(t *testing.T) {
	budgets := []dashboard.Budget{
		{Name: "Vivienda", Kind: ledger.KindExpense, Amount: ptr("3600")},
		{Name: "Mandado", Kind: ledger.KindExpense, Amount: ptr("10833")},
		{Name: "Servicios", Kind: ledger.KindExpense, Amount: ptr("2000")},
		{Name: "Ocio", Kind: ledger.KindExpense, Amount: ptr("0")},
		{Name: "Sin presupuesto", Kind: ledger.KindExpense},
	}
	movements := []ledger.Movement{
		mov("2026-10-02", ledger.KindExpense, "Vivienda", "3600"),
		mov("2026-10-10", ledger.KindExpense, "Mandado", "12000"),
		mov("2026-10-11", ledger.KindExpense, "Ocio", "50"),
		mov("2026-10-12", ledger.KindExpense, "Sin presupuesto", "75"),
	}
	byName := map[string]dashboard.Row{}
	for _, r := range dashboard.ExpenseRows(budgets, movements) {
		byName[r.Name] = r
	}

	tests := []struct {
		name         string
		budget, real string
		diff, pct    string // empty means nil
	}{
		{"Vivienda", "3600", "3600", "0", "100"},                 // exactly on budget
		{"Mandado", "10833", "12000", "-1167", "110.7726391581"}, // over budget
		{"Servicios", "2000", "0", "2000", "0"},                  // no movements
		{"Ocio", "0", "50", "-50", ""},                           // zero budget: diff yes, percentage no
		{"Sin presupuesto", "", "75", "", ""},                    // no budget at all
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

// An expense paid from the savings of a future expense counts only its
// uncovered part, and the settlement rows count nothing, while the available
// money stays what it was.
func TestRowsAndTotalsLeaveOutWhatSavingsCovered(t *testing.T) {
	budgets := []dashboard.Budget{{Name: "Hogar", Kind: ledger.KindExpense, Amount: ptr("1000")}}
	covered := mov("2026-10-05", ledger.KindExpense, "Hogar", "800")
	covered.CoveredBySavings = d("500")
	release := mov("2026-10-05", ledger.KindSavings, "Gastos futuros", "-500")
	release.SavingsSettlement = true
	month := []ledger.Movement{covered, mov("2026-10-06", ledger.KindExpense, "Hogar", "100"), release}

	rows := dashboard.ExpenseRows(budgets, month)
	if len(rows) != 1 || !rows[0].Real.Equal(d("400")) || rows[0].OverBudget() {
		t.Fatalf("rows = %+v, want Hogar spent 400 (800 - 500 covered + 100)", rows)
	}
	totals := dashboard.Totals{
		Income:   d("2000"),
		Expenses: ledger.SumKPI(month, ledger.Filter{Kind: ledger.KindExpense}),
		Savings:  ledger.SumKPI(month, ledger.Filter{Kind: ledger.KindSavings}),
	}
	old := dashboard.Totals{
		Income:   d("2000"),
		Expenses: ledger.SumBy(month, ledger.Filter{Kind: ledger.KindExpense}),
		Savings:  ledger.SumBy(month, ledger.Filter{Kind: ledger.KindSavings}),
	}
	if !totals.Expenses.Equal(d("400")) || !totals.Savings.IsZero() {
		t.Errorf("totals = %+v, want expenses 400 and savings 0", totals)
	}
	if !totals.Available().Equal(old.Available()) {
		t.Errorf("available changed: %s -> %s", old.Available(), totals.Available())
	}
}

func TestSplitSpendingSeparatesFixedFromVariable(t *testing.T) {
	budgets := []dashboard.Budget{
		{Name: "Renta", Kind: ledger.KindExpense, Amount: ptr("10000"), Fixed: true},
		{Name: "Luz", Kind: ledger.KindExpense, Amount: ptr("500"), Fixed: true},
		{Name: "Mandado", Kind: ledger.KindExpense, Amount: ptr("3000")},
		{Name: "Ocio", Kind: ledger.KindExpense}, // no budget: spending counts, budget does not
		{Name: "Inversiones", Kind: ledger.KindSavings, Amount: ptr("999")},
	}
	covered := mov("2026-10-02", ledger.KindExpense, "Renta", "12000") // rent over budget
	covered.CoveredBySavings = d("2000")                               // 2,000 of it came from savings
	month := []ledger.Movement{
		covered,
		mov("2026-10-03", ledger.KindExpense, "Mandado", "700"),
		mov("2026-10-04", ledger.KindExpense, "Ocio", "100"),
		mov("2026-10-05", ledger.KindSavings, "Inversiones", "50"),
	}
	rows := dashboard.ExpenseRows(budgets, month)
	got := dashboard.SplitSpending(rows)
	if !got.HasFixed || !got.Fixed.Spent.Equal(d("10000")) || !got.Fixed.Budget.Equal(d("10500")) {
		t.Errorf("fixed = %+v, want spent 10000 (12000 - 2000 covered) of 10500", got)
	}
	if !got.Variable.Spent.Equal(d("800")) || !got.Variable.Budget.Equal(d("3000")) {
		t.Errorf("variable = %+v, want spent 800 of 3000", got.Variable)
	}
}

func TestSplitSpendingWithoutFixedCategories(t *testing.T) {
	budgets := []dashboard.Budget{{Name: "Mandado", Kind: ledger.KindExpense, Amount: ptr("3000")}}
	got := dashboard.SplitSpending(dashboard.ExpenseRows(budgets, []ledger.Movement{mov("2026-10-03", ledger.KindExpense, "Mandado", "700")}))
	if got.HasFixed || !got.Fixed.Spent.IsZero() || !got.Fixed.Budget.IsZero() || !got.Variable.Spent.Equal(d("700")) {
		t.Errorf("spending = %+v", got)
	}
	// A fixed category with no budget and no spending still turns the split on.
	only := dashboard.SplitSpending(dashboard.ExpenseRows([]dashboard.Budget{{Name: "Renta", Kind: ledger.KindExpense, Fixed: true}}, nil))
	if !only.HasFixed {
		t.Error("a fixed category without movements must still set HasFixed")
	}
}
