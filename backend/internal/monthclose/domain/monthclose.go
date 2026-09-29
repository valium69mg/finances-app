// Package domain holds the pure month close rules: over-budget categories,
// where to put the leftover money and budget-adjustment hints.
package domain

import (
	"github.com/shopspring/decimal"

	dashboard "github.com/valium69mg/finances-app/backend/internal/dashboard/domain"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

var (
	hundred = decimal.NewFromInt(100)
	five    = decimal.NewFromInt(5)
)

// Suggestion says where to put a positive leftover: the emergency fund first
// (up to its remaining room), then investments.
type Suggestion struct {
	ToEmergencyFund decimal.Decimal
	ToInvestments   decimal.Decimal
	EmergencyGoal   decimal.Decimal
}

// Adjustment hints that a budget should change because the real amount
// deviated by more than 20% of it. DeviationPct is signed, in percent
// ((real - budget) / budget * 100, 10 decimals).
type Adjustment struct {
	Name         string
	Kind         ledger.Kind
	Budget       decimal.Decimal
	Real         decimal.Decimal
	DeviationPct decimal.Decimal
}

// Result is the month close: the summary it is based on plus the derived
// over-budget rows, leftover suggestion and budget adjustment hints.
type Result struct {
	Summary dashboard.Summary
	// OverBudget lists expense rows whose real amount exceeds a non-zero budget.
	OverBudget []dashboard.Row
	// Available is the real leftover of the month.
	Available decimal.Decimal
	// Suggestion is nil unless there is a positive leftover.
	Suggestion  *Suggestion
	Adjustments []Adjustment
}

// hasBudget reports whether a row has a usable (non-nil, non-zero) budget.
func hasBudget(r dashboard.Row) bool {
	return r.Budget != nil && !r.Budget.IsZero()
}

// Close computes the close of a YYYY-MM month.
func Close(cfg settings.Config, movements []ledger.Movement, month string) (Result, error) {
	summary, err := dashboard.ComputeSummary(cfg, movements, month)
	if err != nil {
		return Result{}, err
	}
	res := Result{Summary: summary, Available: summary.Available}

	for _, r := range summary.Rows {
		if r.Kind == ledger.KindExpense && hasBudget(r) && r.Real.GreaterThan(*r.Budget) {
			res.OverBudget = append(res.OverBudget, r)
		}
	}

	if res.Available.IsPositive() {
		em := summary.Emergency
		room := decimal.Max(decimal.Zero, em.Goal.Sub(em.Accumulated))
		toFund := decimal.Min(res.Available, room)
		res.Suggestion = &Suggestion{
			ToEmergencyFund: toFund,
			ToInvestments:   res.Available.Sub(toFund),
			EmergencyGoal:   em.Goal,
		}
	}

	for _, r := range summary.Rows {
		if (r.Kind != ledger.KindExpense && r.Kind != ledger.KindSavings) || !hasBudget(r) {
			continue
		}
		delta := r.Real.Sub(*r.Budget)
		// |real - budget| > 20% of |budget|, compared without dividing so the
		// exact 20% boundary is never crossed by rounding.
		if delta.Abs().Mul(five).GreaterThan(r.Budget.Abs()) {
			res.Adjustments = append(res.Adjustments, Adjustment{
				Name:         r.Name,
				Kind:         r.Kind,
				Budget:       *r.Budget,
				Real:         r.Real,
				DeviationPct: delta.Mul(hundred).DivRound(*r.Budget, ledger.DivisionPrecision),
			})
		}
	}
	return res, nil
}
