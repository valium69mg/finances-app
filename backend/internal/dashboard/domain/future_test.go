package domain_test

import (
	"testing"

	dashboard "github.com/valium69mg/finances-app/backend/internal/dashboard/domain"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
)

func TestPlanFutureExpenses(t *testing.T) {
	items := []dashboard.FutureItem{
		{Name: "Predial", DueDate: "2027-01-20", Target: d("10000")},
		{Name: "Seguro", DueDate: "2026-12-15", Target: d("36000")},
	}
	tests := []struct {
		name          string
		pool          string
		today         string
		startDay      int
		wantSaved     [2]string // Seguro, Predial
		wantSuggested [2]string
		wantCycles    [2]int
	}{
		{"empty pool", "0", "2026-09-30", 0, [2]string{"0", "0"}, [2]string{"12000", "2500"}, [2]int{3, 4}},
		{"negative pool saves nothing", "-50", "2026-09-30", 0, [2]string{"0", "0"}, [2]string{"12000", "2500"}, [2]int{3, 4}},
		{"pool fills the earliest first", "40000", "2026-09-30", 0, [2]string{"36000", "4000"}, [2]string{"0", "1500"}, [2]int{3, 4}},
		{"rounds up to the cent", "0.03", "2026-10-01", 0, [2]string{"0.03", "0"}, [2]string{"17999.99", "3333.34"}, [2]int{2, 3}},
		{"overdue falls back to one cycle", "0", "2027-02-01", 0, [2]string{"0", "0"}, [2]string{"36000", "10000"}, [2]int{1, 1}},
		// Day 20 pay cycle: 2026-10-25 already belongs to cycle 2026-11, the
		// insurance (Dec 15) to 2026-12 and the predial (Jan 20) to 2027-02.
		{"follows the pay cycle", "0", "2026-10-25", 20, [2]string{"0", "0"}, [2]string{"36000", "3333.34"}, [2]int{1, 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := dashboard.PlanFutureExpenses(ledger.Cycle{StartDay: tt.startDay}, tt.today, d(tt.pool), items)
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Items) != 2 || plan.Items[0].Name != "Seguro" || plan.Items[1].Name != "Predial" {
				t.Fatalf("items = %+v, want sorted by due date", plan.Items)
			}
			for i, it := range plan.Items {
				if !it.Saved.Equal(d(tt.wantSaved[i])) || !it.Suggested.Equal(d(tt.wantSuggested[i])) || it.CyclesLeft != tt.wantCycles[i] {
					t.Errorf("%s = saved %s suggested %s cycles %d, want %s %s %d",
						it.Name, it.Saved, it.Suggested, it.CyclesLeft, tt.wantSaved[i], tt.wantSuggested[i], tt.wantCycles[i])
				}
				if !it.Remaining.Equal(it.Target.Sub(it.Saved)) {
					t.Errorf("%s remaining %s", it.Name, it.Remaining)
				}
			}
			if !plan.Target.Equal(d("46000")) {
				t.Errorf("target = %s, want 46000", plan.Target)
			}
			sumSaved := plan.Items[0].Saved.Add(plan.Items[1].Saved)
			sumSuggested := plan.Items[0].Suggested.Add(plan.Items[1].Suggested)
			if !plan.Saved.Equal(sumSaved) || !plan.Remaining.Equal(plan.Target.Sub(sumSaved)) || !plan.Suggested.Equal(sumSuggested) {
				t.Errorf("totals = %+v", plan)
			}
		})
	}
}

func TestPlanFutureExpensesEmptyAndInvalid(t *testing.T) {
	plan, err := dashboard.PlanFutureExpenses(ledger.Cycle{}, "2026-10-01", d("100"), nil)
	if err != nil || len(plan.Items) != 0 || !plan.Target.IsZero() || !plan.Suggested.IsZero() {
		t.Errorf("plan = %+v, %v, want an empty plan", plan, err)
	}
}

func TestNewCycleProgress(t *testing.T) {
	tests := []struct {
		name, from, to, today string
		day, days             int
	}{
		{"first day", "2026-10-01", "2026-10-31", "2026-10-01", 1, 31},
		{"middle", "2026-10-01", "2026-10-31", "2026-10-15", 15, 31},
		{"last day", "2026-09-30", "2026-10-30", "2026-10-30", 31, 31},
		{"before the cycle", "2026-10-01", "2026-10-31", "2026-08-01", 0, 31},
		{"after the cycle", "2026-10-01", "2026-10-31", "2026-12-01", 31, 31},
		{"leap february", "2028-02-01", "2028-02-29", "2028-02-29", 29, 29},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := dashboard.NewCycleProgress(tt.from, tt.to, tt.today)
			if err != nil || got.Day != tt.day || got.Days != tt.days || got.Today != tt.today {
				t.Errorf("progress = %+v, %v, want day %d of %d", got, err, tt.day, tt.days)
			}
		})
	}
	if _, err := dashboard.NewCycleProgress("x", "2026-10-31", "2026-10-01"); err == nil {
		t.Error("want an error for a malformed date")
	}
}
