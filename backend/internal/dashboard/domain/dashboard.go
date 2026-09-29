// Package domain holds the pure monthly summary behind the dashboard: budget
// versus actual per category, totals and the client B / SAT reserve block.
package domain

import (
	"github.com/shopspring/decimal"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	savings "github.com/valium69mg/finances-app/backend/internal/savings/domain"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

var hundred = decimal.NewFromInt(100)

// Row is one configured category of the month. Budget is nil when the
// category has none. Diff (budget minus real) exists whenever there is a
// budget, including zero; Pct (real over budget, in percent) only for a
// non-zero budget.
type Row struct {
	Name   string
	Kind   ledger.Kind
	Budget *decimal.Decimal
	Real   decimal.Decimal
	Diff   *decimal.Decimal
	Pct    *decimal.Decimal
}

// Totals are the month totals by kind, over every movement of the month
// (including categories missing from the configuration).
type Totals struct {
	Income   decimal.Decimal
	Expenses decimal.Decimal
	Savings  decimal.Decimal
}

// ISR compares the tax the month's income would cause with the budget.
type ISR struct {
	Rate   decimal.Decimal
	Real   decimal.Decimal
	Budget decimal.Decimal
}

// MonthSplit is what the month put into each savings destination of the
// client B split.
type MonthSplit struct {
	EmergencyFund     decimal.Decimal
	Investments       decimal.Decimal
	AguinaldoVacation decimal.Decimal
}

// ClientB is the extra-income block: the month's deposits, the SAT reserve
// movement of the month and its all-time balance, and where the rest went.
type ClientB struct {
	MonthTotal     decimal.Decimal
	ReserveMonth   decimal.Decimal
	ReserveBalance decimal.Decimal
	MonthSplit     MonthSplit
}

// Summary is the monthly summary.
type Summary struct {
	Month     string
	Rows      []Row
	Totals    Totals
	Available decimal.Decimal
	ISR       ISR
	// Emergency is the all-time emergency fund status, not the month's.
	Emergency    savings.EmergencyStatus
	TaxBreakdown settings.TaxBudget
	ClientB      ClientB
}

// ComputeSummary builds the summary of a YYYY-MM month. Category rows follow
// the configuration order; the Impuestos and Comisiones budgets are the
// computed ones.
func ComputeSummary(cfg settings.Config, movements []ledger.Movement, month string) (Summary, error) {
	monthly := ledger.MonthMovements(movements, month)
	tb, err := cfg.TaxBudgetBreakdown()
	if err != nil {
		return Summary{}, err
	}
	overrides, err := cfg.BudgetOverrides()
	if err != nil {
		return Summary{}, err
	}

	rows := make([]Row, 0, len(cfg.Categories))
	for _, c := range cfg.Categories {
		budget := settings.CategoryBudget(c, overrides)
		real := ledger.SumBy(monthly, ledger.Filter{Kind: c.Kind, Category: c.Name})
		row := Row{Name: c.Name, Kind: c.Kind, Budget: budget, Real: real}
		if budget != nil {
			diff := budget.Sub(real)
			row.Diff = &diff
			if !budget.IsZero() {
				pct := real.Mul(hundred).DivRound(*budget, ledger.DivisionPrecision)
				row.Pct = &pct
			}
		}
		rows = append(rows, row)
	}

	totals := Totals{
		Income:   ledger.SumBy(monthly, ledger.Filter{Kind: ledger.KindIncome}),
		Expenses: ledger.SumBy(monthly, ledger.Filter{Kind: ledger.KindExpense}),
		Savings:  ledger.SumBy(monthly, ledger.Filter{Kind: ledger.KindSavings}),
	}

	rate, err := cfg.ResicoRateOrFirst(totals.Income)
	if err != nil {
		return Summary{}, err
	}
	emergency, err := savings.EmergencyFund(cfg, movements)
	if err != nil {
		return Summary{}, err
	}

	monthSavings := func(category string) decimal.Decimal {
		return ledger.SumBy(monthly, ledger.Filter{Kind: ledger.KindSavings, Category: category})
	}
	return Summary{
		Month:        month,
		Rows:         rows,
		Totals:       totals,
		Available:    totals.Income.Sub(totals.Expenses).Sub(totals.Savings),
		ISR:          ISR{Rate: rate, Real: totals.Income.Mul(rate), Budget: tb.TaxesBudget},
		Emergency:    emergency,
		TaxBreakdown: tb,
		ClientB: ClientB{
			MonthTotal:     ledger.SumBy(monthly, ledger.Filter{Kind: ledger.KindIncome, Category: ledger.CategoryExtraContract}),
			ReserveMonth:   monthSavings(ledger.CategorySATReserve),
			ReserveBalance: ledger.SumBy(movements, ledger.Filter{Kind: ledger.KindSavings, Category: ledger.CategorySATReserve}),
			MonthSplit: MonthSplit{
				EmergencyFund:     monthSavings(ledger.CategoryEmergencyFund),
				Investments:       monthSavings(ledger.CategoryInvestments),
				AguinaldoVacation: monthSavings(ledger.CategoryAguinaldoVacation),
			},
		},
	}, nil
}
