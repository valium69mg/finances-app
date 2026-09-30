package domain

import (
	"testing"
	"time"
)

// The case tables below are the contract mirrored by the frontend parity test
// (frontend cycle.ts); the same rows are listed in the Cycle doc comment.

func TestCycleOf(t *testing.T) {
	tests := []struct {
		name  string
		start int
		date  string
		want  string
	}{
		{"31 day before the last day of September", 31, "2026-09-29", "2026-09"},
		{"31 last day of September opens October", 31, "2026-09-30", "2026-10"},
		{"31 October 30 still October", 31, "2026-10-30", "2026-10"},
		{"31 October 31 opens November", 31, "2026-10-31", "2026-11"},
		{"31 December 31 rolls to January", 31, "2026-12-31", "2027-01"},
		{"31 January 30 stays January", 31, "2027-01-30", "2027-01"},
		{"31 February non-leap before clamp", 31, "2027-02-27", "2027-02"},
		{"31 February non-leap clamp day", 31, "2027-02-28", "2027-03"},
		{"31 February leap before clamp", 31, "2028-02-28", "2028-02"},
		{"31 February leap clamp day", 31, "2028-02-29", "2028-03"},
		{"15 day before start", 15, "2026-10-14", "2026-10"},
		{"15 start day", 15, "2026-10-15", "2026-11"},
		{"1 first day opens next label", 1, "2026-10-01", "2026-11"},
		{"0 is the calendar month", 0, "2026-10-31", "2026-10"},
		{"0 first day", 0, "2026-10-01", "2026-10"},
		{"0 leap day", 0, "2028-02-29", "2028-02"},
	}
	for _, tc := range tests {
		if got := (Cycle{StartDay: tc.start}).Of(tc.date); got != tc.want {
			t.Errorf("%s: Cycle{%d}.Of(%s) = %q, want %q", tc.name, tc.start, tc.date, got, tc.want)
		}
	}
}

func TestCycleRange(t *testing.T) {
	tests := []struct {
		name     string
		start    int
		label    string
		from, to string
	}{
		{"31 October", 31, "2026-10", "2026-09-30", "2026-10-30"},
		{"31 November", 31, "2026-11", "2026-10-31", "2026-11-29"},
		{"31 January rollover", 31, "2027-01", "2026-12-31", "2027-01-30"},
		{"31 March after a non-leap February", 31, "2027-03", "2027-02-28", "2027-03-30"},
		{"31 March after a leap February", 31, "2028-03", "2028-02-29", "2028-03-30"},
		{"31 February non-leap", 31, "2027-02", "2027-01-31", "2027-02-27"},
		{"15 October", 15, "2026-10", "2026-09-15", "2026-10-14"},
		{"1 October", 1, "2026-10", "2026-09-01", "2026-09-30"},
		{"0 February non-leap", 0, "2026-02", "2026-02-01", "2026-02-28"},
		{"0 February leap", 0, "2028-02", "2028-02-01", "2028-02-29"},
		{"0 December", 0, "2026-12", "2026-12-01", "2026-12-31"},
	}
	for _, tc := range tests {
		from, to, err := (Cycle{StartDay: tc.start}).Range(tc.label)
		if err != nil || from != tc.from || to != tc.to {
			t.Errorf("%s: Cycle{%d}.Range(%s) = %s..%s, %v; want %s..%s", tc.name, tc.start, tc.label, from, to, err, tc.from, tc.to)
		}
	}
	if _, _, err := (Cycle{StartDay: 31}).Range("2026-13"); err == nil {
		t.Error("Range(2026-13) should fail")
	}
}

// TestCycleRangeOfRoundTrip proves Of and Range agree: the first and last day
// of a range belong to the label, the days just outside do not, and ranges of
// consecutive labels tile the calendar without gaps or overlaps.
func TestCycleRangeOfRoundTrip(t *testing.T) {
	for start := 0; start <= 31; start++ {
		c := Cycle{StartDay: start}
		label := "2025-12"
		prevTo := ""
		for i := 0; i < 26; i++ {
			from, to, err := c.Range(label)
			if err != nil {
				t.Fatal(err)
			}
			if got := c.Of(from); got != label {
				t.Errorf("start %d: Of(from %s) = %s, want %s", start, from, got, label)
			}
			if got := c.Of(to); got != label {
				t.Errorf("start %d: Of(to %s) = %s, want %s", start, to, got, label)
			}
			if prevTo != "" {
				want, _ := time.Parse(dateLayout, prevTo)
				if from != want.AddDate(0, 0, 1).Format(dateLayout) {
					t.Errorf("start %d: %s starts %s, previous ended %s", start, label, from, prevTo)
				}
			}
			prevTo = to
			next, err := NextMonth(label)
			if err != nil {
				t.Fatal(err)
			}
			label = next
		}
	}
}

func TestCycleCurrentAndPrevious(t *testing.T) {
	c := Cycle{StartDay: 31}
	if got := c.Current(time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)); got != "2026-10" {
		t.Errorf("Current(2026-09-30) = %q, want 2026-10", got)
	}
	if got := (Cycle{}).Current(time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)); got != "2026-09" {
		t.Errorf("calendar Current(2026-09-30) = %q, want 2026-09", got)
	}
	if got, err := c.Previous("2027-01"); err != nil || got != "2026-12" {
		t.Errorf("Previous(2027-01) = %q, %v; want 2026-12", got, err)
	}
	if _, err := c.Previous("nope"); err == nil {
		t.Error("Previous(nope) should fail")
	}
}

func TestCycleValid(t *testing.T) {
	for _, d := range []int{0, 1, 31} {
		if !(Cycle{StartDay: d}).Valid() {
			t.Errorf("StartDay %d should be valid", d)
		}
	}
	for _, d := range []int{-1, 32} {
		if (Cycle{StartDay: d}).Valid() {
			t.Errorf("StartDay %d should be invalid", d)
		}
	}
}
