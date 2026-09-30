package domain

import (
	"github.com/shopspring/decimal"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
)

// PausePlan is the investment pause: while paused, the money that would be
// invested is redirected to the Gastos futuros sinking fund.
//
// Months and ResumeMonth use the YYYY-MM format, which compares correctly as
// text. FutureExpensesPlan holds the Gastos futuros budget of each paused month.
type PausePlan struct {
	Months             []string
	NormalBudget       decimal.Decimal
	ResumeMonth        string
	FutureExpensesPlan map[string]decimal.Decimal
	Note               string
}

// IsPaused reports whether the month is one of the paused months.
func (p PausePlan) IsPaused(month string) bool {
	for _, m := range p.Months {
		if m == month {
			return true
		}
	}
	return false
}

// BudgetOverrides returns the category budget overrides the pause implies for
// a month, ready to merge with the tax overrides and pass to CategoryBudget:
//
//   - paused month: Inversiones is 0 and Gastos futuros is the plan amount of
//     that month (left untouched when the plan has no entry for it);
//   - resume month and later: Inversiones returns to NormalBudget;
//   - any other month: no overrides.
//
// The extra-income split is deliberately not affected.
func (p PausePlan) BudgetOverrides(month string) map[string]decimal.Decimal {
	switch {
	case p.IsPaused(month):
		out := map[string]decimal.Decimal{ledger.CategoryInvestments: decimal.Zero}
		if plan, ok := p.FutureExpensesPlan[month]; ok {
			out[ledger.CategoryFutureExpenses] = plan
		}
		return out
	case p.ResumeMonth != "" && month >= p.ResumeMonth:
		return map[string]decimal.Decimal{ledger.CategoryInvestments: p.NormalBudget}
	default:
		return map[string]decimal.Decimal{}
	}
}

// MonthBudgetOverrides merges the computed Impuestos and Comisiones budgets
// with the pause overrides of the month. The pause never overrides those two
// categories, so the merge has no conflicts.
func (c Config) MonthBudgetOverrides(month string) (map[string]decimal.Decimal, error) {
	out, err := c.BudgetOverrides()
	if err != nil {
		return nil, err
	}
	if c.Pause != nil {
		for k, v := range c.Pause.BudgetOverrides(month) {
			out[k] = v
		}
	}
	return out, nil
}
