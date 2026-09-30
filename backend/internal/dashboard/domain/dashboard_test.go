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
