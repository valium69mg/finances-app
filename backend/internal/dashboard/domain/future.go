package domain

import (
	"fmt"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
)

// FutureItem is one planned future expense: a name, the day it is due and the
// amount it will cost. The dashboard derives them from the yearly fixed-amount
// MXN bills, which are the expenses known in advance.
type FutureItem struct {
	Name    string
	DueDate string // YYYY-MM-DD
	Target  decimal.Decimal
}

// FutureExpense is a FutureItem with what is saved towards it and what to put
// aside each cycle to have it on time. CyclesLeft is the number of pay cycles
// from the current one up to the cycle of the due date (at least 1, also when
// the expense is overdue), the divisor of Suggested.
type FutureExpense struct {
	Name       string
	DueDate    string
	Target     decimal.Decimal
	Saved      decimal.Decimal
	Remaining  decimal.Decimal
	Suggested  decimal.Decimal
	CyclesLeft int
}

// FutureExpenses is the plan of every future expense plus its totals.
type FutureExpenses struct {
	Items     []FutureExpense
	Target    decimal.Decimal
	Saved     decimal.Decimal
	Remaining decimal.Decimal
	Suggested decimal.Decimal
}

// PlanFutureExpenses spreads the Gastos futuros pool over the items, earliest
// due date first, each item taking at most its target (a negative or empty pool
// saves nothing). The suggested amount is what is still missing divided by the
// cycles left, rounded up to the cent so the plan never falls short. today is
// a YYYY-MM-DD date in the configured time zone.
func PlanFutureExpenses(cycle ledger.Cycle, today string, pool decimal.Decimal, items []FutureItem) (FutureExpenses, error) {
	sorted := append([]FutureItem(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].DueDate < sorted[j].DueDate })

	currentIdx, err := cycleIndex(cycle.Of(today))
	if err != nil {
		return FutureExpenses{}, err
	}
	plan := FutureExpenses{Items: make([]FutureExpense, 0, len(sorted))}
	left := pool
	if left.IsNegative() {
		left = decimal.Zero
	}
	for _, it := range sorted {
		dueIdx, err := cycleIndex(cycle.Of(it.DueDate))
		if err != nil {
			return FutureExpenses{}, err
		}
		cycles := dueIdx - currentIdx
		if cycles < 1 {
			cycles = 1
		}
		saved := decimal.Min(left, it.Target)
		left = left.Sub(saved)
		remaining := it.Target.Sub(saved)
		suggested := remaining.Div(decimal.NewFromInt(int64(cycles))).RoundCeil(2)
		plan.Items = append(plan.Items, FutureExpense{
			Name: it.Name, DueDate: it.DueDate, Target: it.Target, Saved: saved,
			Remaining: remaining, Suggested: suggested, CyclesLeft: cycles,
		})
		plan.Target = plan.Target.Add(it.Target)
		plan.Saved = plan.Saved.Add(saved)
		plan.Remaining = plan.Remaining.Add(remaining)
		plan.Suggested = plan.Suggested.Add(suggested)
	}
	return plan, nil
}

// cycleIndex turns a YYYY-MM cycle label into a month count so two labels can
// be subtracted.
func cycleIndex(label string) (int, error) {
	t, err := time.Parse("2006-01", label)
	if err != nil {
		return 0, fmt.Errorf("invalid cycle label %q", label)
	}
	return t.Year()*12 + int(t.Month()), nil
}

// CycleProgress places today inside the displayed cycle. Day counts from 1 on
// the first day of the cycle and is 0 before it starts; it stops at Days once
// the cycle is over. Days is the cycle length.
type CycleProgress struct {
	Today string
	Day   int
	Days  int
}

// NewCycleProgress computes the progress of today (YYYY-MM-DD) over the cycle
// that runs from..to, both inclusive.
func NewCycleProgress(from, to, today string) (CycleProgress, error) {
	start, err := time.Parse("2006-01-02", from)
	if err != nil {
		return CycleProgress{}, fmt.Errorf("invalid cycle start %q", from)
	}
	end, err := time.Parse("2006-01-02", to)
	if err != nil {
		return CycleProgress{}, fmt.Errorf("invalid cycle end %q", to)
	}
	now, err := time.Parse("2006-01-02", today)
	if err != nil {
		return CycleProgress{}, fmt.Errorf("invalid date %q", today)
	}
	days := int(end.Sub(start).Hours()/24) + 1
	day := int(now.Sub(start).Hours()/24) + 1
	switch {
	case day < 0:
		day = 0
	case day > days:
		day = days
	}
	return CycleProgress{Today: today, Day: day, Days: days}, nil
}

// UpcomingBill is a pending bill occurrence due soon (or already overdue).
// Amount is nil for a variable bill.
type UpcomingBill struct {
	ID           int
	Name         string
	Category     string
	Amount       *decimal.Decimal
	Currency     string
	DueDate      string
	DaysUntilDue int
	Overdue      bool
}
