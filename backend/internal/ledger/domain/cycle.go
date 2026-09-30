package domain

import (
	"fmt"
	"time"
)

const dateLayout = "2006-01-02"

// Cycle maps dates to personal budget cycles (pay cycles). A cycle keeps the
// YYYY-MM label of the month in which it ENDS, so every period key in the app
// (month closes, pause plan, budgets, ?month= params) keeps its format.
//
// StartDay selects the rule:
//   - 0: a cycle is the calendar month (the default and the legacy behavior).
//   - 1..31: cycle M starts on day StartDay of month M-1, clamped to that
//     month's length (31 means its last day), and ends the day before cycle
//     M+1 starts, inclusive.
//
// Example with StartDay 31: cycle 2026-10 runs from 2026-09-30 to 2026-10-30
// and 2026-10-31 already belongs to cycle 2026-11. February clamps to 28/29.
//
// Only personal views use it. Fiscal periods (invoices, tax filing) stay on
// calendar months through MonthOf and friends.
//
// Case table shared with the frontend parity test (input -> result):
//
//	StartDay 31: Of 2026-09-29 -> 2026-09     Of 2026-09-30 -> 2026-10
//	StartDay 31: Of 2026-10-30 -> 2026-10     Of 2026-10-31 -> 2026-11
//	StartDay 31: Of 2026-12-31 -> 2027-01     Of 2027-01-30 -> 2027-01
//	StartDay 31: Of 2027-02-27 -> 2027-02     Of 2027-02-28 -> 2027-03 (non-leap clamp)
//	StartDay 31: Of 2028-02-28 -> 2028-02     Of 2028-02-29 -> 2028-03 (leap clamp)
//	StartDay 31: Range 2026-10 -> 2026-09-30..2026-10-30
//	StartDay 31: Range 2026-11 -> 2026-10-31..2026-11-29
//	StartDay 31: Range 2027-01 -> 2026-12-31..2027-01-30
//	StartDay 31: Range 2027-03 -> 2027-02-28..2027-03-30
//	StartDay 31: Range 2028-03 -> 2028-02-29..2028-03-30
//	StartDay 15: Of 2026-10-14 -> 2026-10     Of 2026-10-15 -> 2026-11
//	StartDay 15: Range 2026-10 -> 2026-09-15..2026-10-14
//	StartDay 1:  Of 2026-10-01 -> 2026-11     Range 2026-10 -> 2026-09-01..2026-09-30 (day 1 ends before the label month)
//	StartDay 0:  Of 2026-10-31 -> 2026-10     Range 2026-02 -> 2026-02-01..2026-02-28
//	StartDay 0:  Range 2028-02 -> 2028-02-01..2028-02-29
//
// Current(now) is Of(now's date); Previous(label) is the month before label.
type Cycle struct {
	StartDay int
}

// Valid reports whether StartDay is in 0..31.
func (c Cycle) Valid() bool { return c.StartDay >= 0 && c.StartDay <= 31 }

// startIn returns the day of the given year and month on which a cycle starts.
func (c Cycle) startIn(year int, month time.Month) time.Time {
	last := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
	day := c.StartDay
	if day > last {
		day = last
	}
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// Of returns the label of the cycle a YYYY-MM-DD date belongs to. A date that
// does not parse falls back to its YYYY-MM prefix, like MonthOf.
func (c Cycle) Of(date string) string {
	if c.StartDay == 0 {
		return MonthOf(date)
	}
	t, err := time.Parse(dateLayout, date)
	if err != nil {
		return MonthOf(date)
	}
	// Cycle M+1 starts inside month M, so a date on or after that day already
	// belongs to the next label.
	if !t.Before(c.startIn(t.Year(), t.Month())) {
		t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	}
	return t.Format("2006-01")
}

// Range returns the first and last day (both inclusive, YYYY-MM-DD) of the
// cycle with the given YYYY-MM label.
func (c Cycle) Range(label string) (from, to string, err error) {
	end, err := time.Parse("2006-01", label)
	if err != nil {
		return "", "", fmt.Errorf("invalid month %q, use YYYY-MM", label)
	}
	if c.StartDay == 0 {
		return end.Format(dateLayout), end.AddDate(0, 1, -1).Format(dateLayout), nil
	}
	prev := end.AddDate(0, -1, 0)
	start := c.startIn(prev.Year(), prev.Month())
	next := c.startIn(end.Year(), end.Month())
	return start.Format(dateLayout), next.AddDate(0, 0, -1).Format(dateLayout), nil
}

// Current returns the label of the cycle containing the given instant.
func (c Cycle) Current(now time.Time) string {
	return c.Of(now.Format(dateLayout))
}

// Previous returns the label of the cycle before the given one (January rolls
// to the previous December).
func (c Cycle) Previous(label string) (string, error) {
	return PreviousMonth(label)
}
