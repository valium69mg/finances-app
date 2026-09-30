package domain_test

import (
	"testing"

	"github.com/shopspring/decimal"

	dashboard "github.com/valium69mg/finances-app/backend/internal/dashboard/domain"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
)

func ptr(s string) *decimal.Decimal {
	v := d(s)
	return &v
}

func TestTotalsAvailable(t *testing.T) {
	tests := []struct {
		name string
		t    dashboard.Totals
		want string
	}{
		{"typical month", dashboard.Totals{Income: d("60000"), Expenses: d("20000"), Savings: d("15000")}, "25000"},
		{"overspent goes negative", dashboard.Totals{Income: d("1000"), Expenses: d("800"), Savings: d("500")}, "-300"},
		{"withdrawal adds money back", dashboard.Totals{Income: d("1000"), Expenses: d("200"), Savings: d("-300")}, "1100"},
		{"empty month", dashboard.Totals{}, "0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.t.Available(); !got.Equal(d(tt.want)) {
				t.Errorf("Available = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestExpenseRows(t *testing.T) {
	budgets := []dashboard.Budget{
		{Name: "Sueldo", Kind: ledger.KindIncome},
		{Name: "Renta", Kind: ledger.KindExpense, Amount: ptr("10000")},
		{Name: "Comida", Kind: ledger.KindExpense, Amount: ptr("3000")},
		{Name: "Extras", Kind: ledger.KindExpense},
		{Name: "Vacío", Kind: ledger.KindExpense, Amount: ptr("500")},
		{Name: "Inversiones", Kind: ledger.KindSavings, Amount: ptr("0")},
	}
	monthly := []ledger.Movement{
		mov("2026-10-01", ledger.KindExpense, "Renta", "10000"),
		mov("2026-10-02", ledger.KindExpense, "Comida", "1000.50"),
		mov("2026-10-03", ledger.KindExpense, "Comida", "2500"),
		mov("2026-10-04", ledger.KindExpense, "Extras", "80"),
		mov("2026-10-05", ledger.KindExpense, "Desconocida", "999"),
		mov("2026-10-06", ledger.KindSavings, "Comida", "7"), // other kind: ignored
	}
	rows := dashboard.ExpenseRows(budgets, monthly)
	if len(rows) != 4 {
		t.Fatalf("rows = %d, want the 4 Gasto budgets in order", len(rows))
	}
	byName := map[string]dashboard.Row{}
	for _, r := range rows {
		byName[r.Name] = r
	}

	if r := byName["Renta"]; !r.Real.Equal(d("10000")) || !r.Diff.IsZero() || r.OverBudget() {
		t.Errorf("exactly on budget: %+v", r)
	}
	if r := byName["Comida"]; !r.Real.Equal(d("3500.50")) || !r.Diff.Equal(d("-500.50")) || !r.OverBudget() {
		t.Errorf("over budget: %+v", r)
	}
	if r := byName["Extras"]; !r.Real.Equal(d("80")) || r.Budget != nil || r.Diff != nil || r.OverBudget() {
		t.Errorf("no budget: %+v", r)
	}
	if r := byName["Vacío"]; !r.Real.IsZero() || !r.Diff.Equal(d("500")) || r.OverBudget() {
		t.Errorf("nothing spent: %+v", r)
	}
	if rows[0].Name != "Renta" || rows[3].Name != "Vacío" {
		t.Errorf("order: %s ... %s", rows[0].Name, rows[3].Name)
	}
}

func TestExpenseRowsNoBudgets(t *testing.T) {
	if rows := dashboard.ExpenseRows(nil, []ledger.Movement{mov("2026-10-01", ledger.KindExpense, "A", "1")}); len(rows) != 0 {
		t.Errorf("rows = %v, want none", rows)
	}
}
