// Package domain holds the pure month close rules: the snapshot of a month
// (budget versus actual, totals, emergency fund progress), where to put the
// leftover money and the budget-adjustment hints. It reuses the dashboard row
// and totals definitions and duplicates none of them.
package domain

import (
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	dashboard "github.com/valium69mg/finances-app/backend/internal/dashboard/domain"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	savings "github.com/valium69mg/finances-app/backend/internal/savings/domain"
	taxfiling "github.com/valium69mg/finances-app/backend/internal/taxfiling/domain"
)

var (
	// ErrInvalidInput is wrapped by every rejected period.
	ErrInvalidInput = errors.New("invalid month close")
	// ErrNotFound is returned when a period has no stored close.
	ErrNotFound = errors.New("month close not found")
	// ErrAlreadyClosed is returned when the period already has a stored close.
	ErrAlreadyClosed = errors.New("month already closed")
)

var (
	hundred = decimal.NewFromInt(100)
	five    = decimal.NewFromInt(5)
)

// ValidatePeriod reports ErrInvalidInput unless period is a YYYY-MM month
// (the format is defined once, by ledger.IsMonth).
func ValidatePeriod(period string) error {
	if !ledger.IsMonth(period) {
		return fmt.Errorf("%w: period %q must be YYYY-MM", ErrInvalidInput, period)
	}
	return nil
}

// ValidateClosable additionally refuses a period after the current budget
// cycle: the current cycle may be closed (a mid-cycle snapshot), a future one
// may not. Cycle labels are YYYY-MM, so they compare as text.
func ValidateClosable(period string, now time.Time, cycle ledger.Cycle) error {
	if err := ValidatePeriod(period); err != nil {
		return err
	}
	if period > cycle.Current(now) {
		return fmt.Errorf("%w: period %s is in the future", ErrInvalidInput, period)
	}
	return nil
}

// Category is one Gasto category of the period against its budget.
// Budget and Remaining are nil when the category has no budget.
type Category struct {
	Name       string
	Budget     *decimal.Decimal
	Spent      decimal.Decimal
	Remaining  *decimal.Decimal
	OverBudget bool
}

// Suggestion says where to put a positive leftover: the emergency fund first,
// up to its remaining room, then the rest to Inversiones or, while the
// investment pause is on, to Gastos futuros (never both).
type Suggestion struct {
	ToEmergencyFund  decimal.Decimal
	ToInvestments    decimal.Decimal
	ToFutureExpenses decimal.Decimal
	// InvestmentsPaused tells that the month is a paused one, so the remainder
	// went to Gastos futuros instead of Inversiones.
	InvestmentsPaused bool
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

// Close is the month close: the immutable figures of a period as of the moment
// it was generated. ClosedAt is zero on a preview. FilingStatus is empty when
// it was not computed.
type Close struct {
	Period       string
	ClosedAt     time.Time
	Categories   []Category
	Income       decimal.Decimal
	Expenses     decimal.Decimal
	Savings      decimal.Decimal
	Available    decimal.Decimal
	Emergency    savings.EmergencyStatus
	Suggestion   *Suggestion
	Adjustments  []Adjustment
	FilingStatus taxfiling.PaymentStatus
}

// Input is everything the computation needs, already resolved by the caller.
type Input struct {
	Period string
	// Budgets are the category budgets resolved for the period (computed tax
	// budgets and pause overrides applied).
	Budgets []dashboard.Budget
	// Monthly are the movements of the period, of every kind.
	Monthly []ledger.Movement
	// Emergency is the emergency fund status as of the end of the period.
	Emergency savings.EmergencyStatus
	// InvestmentsPaused is true when the period is one of the paused months.
	InvestmentsPaused bool
	FilingStatus      taxfiling.PaymentStatus
}

// MovementsUpTo keeps the movements dated on or before end (a YYYY-MM-DD date,
// the last day of the period's cycle), so the emergency fund can be measured as
// of that period. Dates compare correctly as text.
func MovementsUpTo(movements []ledger.Movement, end string) []ledger.Movement {
	out := make([]ledger.Movement, 0, len(movements))
	for _, m := range movements {
		if m.Date <= end {
			out = append(out, m)
		}
	}
	return out
}

// hasBudget reports whether a row has a usable (non-nil, non-zero) budget.
func hasBudget(r dashboard.Row) bool {
	return r.Budget != nil && !r.Budget.IsZero()
}

// Compute builds the close of a period. The totals use the dashboard
// definitions: available = income - expenses - savings, savings net of
// withdrawals.
func Compute(in Input) Close {
	totals := dashboard.Totals{
		Income:   ledger.SumBy(in.Monthly, ledger.Filter{Kind: ledger.KindIncome}),
		Expenses: ledger.SumBy(in.Monthly, ledger.Filter{Kind: ledger.KindExpense}),
		Savings:  ledger.SumBy(in.Monthly, ledger.Filter{Kind: ledger.KindSavings}),
	}
	expenseRows := dashboard.RowsOfKind(ledger.KindExpense, in.Budgets, in.Monthly)
	savingsRows := dashboard.RowsOfKind(ledger.KindSavings, in.Budgets, in.Monthly)

	out := Close{
		Period:       in.Period,
		Categories:   make([]Category, len(expenseRows)),
		Income:       totals.Income,
		Expenses:     totals.Expenses,
		Savings:      totals.Savings,
		Available:    totals.Available(),
		Emergency:    in.Emergency,
		Adjustments:  []Adjustment{},
		FilingStatus: in.FilingStatus,
	}
	for i, r := range expenseRows {
		out.Categories[i] = Category{Name: r.Name, Budget: r.Budget, Spent: r.Real, Remaining: r.Diff, OverBudget: r.OverBudget()}
	}
	out.Suggestion = suggest(out.Available, in.Emergency, in.InvestmentsPaused)

	// Hints as fin.py: expense and savings categories with a budget.
	for _, r := range append(expenseRows, savingsRows...) {
		if !hasBudget(r) {
			continue
		}
		delta := r.Real.Sub(*r.Budget)
		// |real - budget| > 20% of |budget|, compared without dividing so the
		// exact 20% boundary is never crossed by rounding.
		if delta.Abs().Mul(five).GreaterThan(r.Budget.Abs()) {
			out.Adjustments = append(out.Adjustments, Adjustment{
				Name:         r.Name,
				Kind:         r.Kind,
				Budget:       *r.Budget,
				Real:         r.Real,
				DeviationPct: delta.Mul(hundred).DivRound(*r.Budget, ledger.DivisionPrecision),
			})
		}
	}
	return out
}

// suggest splits a positive leftover: first the room left to the emergency
// fund goal, then the rest to Inversiones, or to Gastos futuros while the
// investment pause is on (the PLAN pause rule: the money that would be invested
// goes to the sinking fund). It is nil without a positive leftover.
func suggest(available decimal.Decimal, em savings.EmergencyStatus, paused bool) *Suggestion {
	if !available.IsPositive() {
		return nil
	}
	room := decimal.Max(decimal.Zero, em.Goal.Sub(em.Accumulated))
	toFund := decimal.Min(available, room)
	rest := available.Sub(toFund)
	s := &Suggestion{ToEmergencyFund: toFund, ToInvestments: decimal.Zero, ToFutureExpenses: decimal.Zero, InvestmentsPaused: paused}
	if paused {
		s.ToFutureExpenses = rest
	} else {
		s.ToInvestments = rest
	}
	return s
}
