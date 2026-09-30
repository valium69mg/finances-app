package domain

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

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
