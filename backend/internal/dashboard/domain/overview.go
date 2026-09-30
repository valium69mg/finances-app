package domain

import (
	"github.com/shopspring/decimal"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	savings "github.com/valium69mg/finances-app/backend/internal/savings/domain"
	taxfiling "github.com/valium69mg/finances-app/backend/internal/taxfiling/domain"
)

// Budget is the budget of one category already resolved for the month (tax
// budgets and investment pause overrides applied); nil Amount means none.
type Budget struct {
	Name   string
	Kind   ledger.Kind
	Amount *decimal.Decimal
}

// TaxCard is the estimated RESICO ISR of the month and the rate it uses, plus
// the filing status of the period: the payment state of the filing of the
// month itself and whether the previous period still needs action (unfiled, or
// filed and not paid).
type TaxCard struct {
	Rate         decimal.Decimal
	EstimatedISR decimal.Decimal
	Filing       taxfiling.MonthStatus
}

// Overview is the dashboard of one budget cycle: the expense categories against their
// month budgets, the month totals, the all-time emergency fund and the tax
// card, which is nil when the tax settings are incomplete.
type Overview struct {
	Month string
	// PeriodStart and PeriodEnd are the first and last day (YYYY-MM-DD,
	// inclusive) of the budget cycle the figures cover.
	PeriodStart string
	PeriodEnd   string
	Rows        []Row // Gasto categories, in the order of the budgets
	Totals      Totals
	Available   decimal.Decimal
	Emergency   savings.EmergencyStatus
	Tax         *TaxCard
	// Cycle is where today sits in the displayed cycle.
	Cycle CycleProgress
	// Future is the plan of the expenses known in advance (yearly bills).
	Future FutureExpenses
	// Upcoming are the bills due within the next UpcomingDays days, overdue
	// ones included, earliest first.
	Upcoming []UpcomingBill
	// Recent are the latest movements of every kind, newest first.
	Recent []ledger.Movement
}

// ExpenseRows returns one row per Gasto budget, in order, with what the
// month's movements spent in it. Other kinds are skipped, and an expense in a
// category without a budget entry only counts in the totals.
func ExpenseRows(budgets []Budget, monthly []ledger.Movement) []Row {
	return RowsOfKind(ledger.KindExpense, budgets, monthly)
}

// RowsOfKind is ExpenseRows for any kind: one row per budget of that kind, in
// order, with what the month's movements of the kind put in it.
func RowsOfKind(kind ledger.Kind, budgets []Budget, monthly []ledger.Movement) []Row {
	rows := make([]Row, 0, len(budgets))
	for _, b := range budgets {
		if b.Kind != kind {
			continue
		}
		rows = append(rows, newRow(b.Name, b.Kind, b.Amount, monthly))
	}
	return rows
}
