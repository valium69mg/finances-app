package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/futureexpenses/domain"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestValidate(t *testing.T) {
	ok, err := domain.Validate(domain.Input{Name: "  Laptop ", Target: d("8000"), DueDate: "2027-01-20"})
	if err != nil || ok.Name != "Laptop" || !ok.Target.Equal(d("8000")) || ok.DueDate != "2027-01-20" {
		t.Fatalf("Validate = %+v, %v", ok, err)
	}
	tests := []struct {
		name string
		in   domain.Input
	}{
		{"empty name", domain.Input{Name: "  ", Target: d("1"), DueDate: "2027-01-20"}},
		{"long name", domain.Input{Name: strings.Repeat("x", 121), Target: d("1"), DueDate: "2027-01-20"}},
		{"zero target", domain.Input{Name: "a", Target: d("0"), DueDate: "2027-01-20"}},
		{"negative target", domain.Input{Name: "a", Target: d("-5"), DueDate: "2027-01-20"}},
		{"three decimals", domain.Input{Name: "a", Target: d("1.001"), DueDate: "2027-01-20"}},
		{"absurd exponent", domain.Input{Name: "a", Target: d("1e999999"), DueDate: "2027-01-20"}},
		{"too large", domain.Input{Name: "a", Target: d("1000000000000"), DueDate: "2027-01-20"}},
		{"bad date", domain.Input{Name: "a", Target: d("1"), DueDate: "20/01/2027"}},
		{"empty date", domain.Input{Name: "a", Target: d("1")}},
		{"impossible date", domain.Input{Name: "a", Target: d("1"), DueDate: "2027-02-30"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := domain.Validate(tt.in); !errors.Is(err, domain.ErrInvalidInput) {
				t.Errorf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func item(id int, name, due, target, saved string) domain.FutureExpense {
	return domain.FutureExpense{ID: id, Name: name, DueDate: due, Target: d(target), Saved: d(saved), Status: domain.StatusActive}
}

func TestBuildPlan(t *testing.T) {
	tests := []struct {
		name          string
		saved         [2]string // Viaje, Laptop
		today         string
		startDay      int
		wantSuggested [2]string
		wantCycles    [2]int
	}{
		{"nothing saved", [2]string{"0", "0"}, "2026-09-30", 0, [2]string{"8000", "2000"}, [2]int{3, 4}},
		{"saved is per item", [2]string{"24000", "4000"}, "2026-09-30", 0, [2]string{"0", "1000"}, [2]int{3, 4}},
		{"rounds up to the cent", [2]string{"0.03", "0"}, "2026-10-01", 0, [2]string{"11999.99", "2666.67"}, [2]int{2, 3}},
		{"overdue falls back to one cycle", [2]string{"0", "0"}, "2027-02-01", 0, [2]string{"24000", "8000"}, [2]int{1, 1}},
		{"saved beyond the target needs nothing", [2]string{"30000", "0"}, "2026-09-30", 0, [2]string{"0", "2000"}, [2]int{3, 4}},
		// Day 20 pay cycle: 2026-10-25 already belongs to cycle 2026-11, the
		// trip (Dec 15) to 2026-12 and the laptop (Jan 20) to 2027-02.
		{"follows the pay cycle", [2]string{"0", "0"}, "2026-10-25", 20, [2]string{"24000", "2666.67"}, [2]int{1, 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := []domain.FutureExpense{
				item(1, "Laptop", "2027-01-20", "8000", tt.saved[1]),
				item(2, "Viaje", "2026-12-15", "24000", tt.saved[0]),
				{ID: 3, Name: "Paid", DueDate: "2026-10-01", Target: d("5"), Status: domain.StatusPaid},
			}
			plan, err := domain.BuildPlan(ledger.Cycle{StartDay: tt.startDay}, tt.today, items, d("123.45"))
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Items) != 2 || plan.Items[0].Name != "Viaje" || plan.Items[1].Name != "Laptop" {
				t.Fatalf("items = %+v, want the active ones sorted by due date", plan.Items)
			}
			for i, it := range plan.Items {
				if !it.Suggested.Equal(d(tt.wantSuggested[i])) || it.CyclesLeft != tt.wantCycles[i] {
					t.Errorf("%s = suggested %s cycles %d, want %s %d", it.Name, it.Suggested, it.CyclesLeft, tt.wantSuggested[i], tt.wantCycles[i])
				}
				if it.Remaining.IsNegative() {
					t.Errorf("%s remaining %s is negative", it.Name, it.Remaining)
				}
			}
			sumSuggested := plan.Items[0].Suggested.Add(plan.Items[1].Suggested)
			if !plan.Target.Equal(d("32000")) || !plan.Suggested.Equal(sumSuggested) || !plan.FreeBalance.Equal(d("123.45")) {
				t.Errorf("totals = %+v", plan)
			}
		})
	}
}

func TestBuildPlanEmpty(t *testing.T) {
	plan, err := domain.BuildPlan(ledger.Cycle{}, "2026-10-01", nil, decimal.Zero)
	if err != nil || plan.Items == nil || len(plan.Items) != 0 || !plan.Target.IsZero() || !plan.Suggested.IsZero() {
		t.Errorf("plan = %+v, %v, want an empty plan with a non-nil list", plan, err)
	}
}

func TestPlanItemPaidHasNoPlan(t *testing.T) {
	p, err := domain.PlanItem(ledger.Cycle{}, "2026-10-01", domain.FutureExpense{Status: domain.StatusPaid, Target: d("100")})
	if err != nil || !p.Remaining.IsZero() || !p.Suggested.IsZero() || p.CyclesLeft != 0 {
		t.Errorf("paid plan = %+v, %v", p, err)
	}
}

func TestRelease(t *testing.T) {
	tests := []struct {
		name, saved, paid, linked, remainder string
	}{
		{"saved exceeds paid", "500", "400", "500", "100"},
		{"saved equals paid", "400", "400", "400", "0"},
		{"saved below paid", "300", "400", "300", "0"},
		{"nothing saved", "0", "400", "0", "0"},
		{"negative saved releases nothing", "-20", "400", "0", "0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			linked, remainder := domain.Release(d(tt.saved), d(tt.paid))
			if !linked.Equal(d(tt.linked)) || !remainder.Equal(d(tt.remainder)) {
				t.Errorf("Release = %s, %s, want %s, %s", linked, remainder, tt.linked, tt.remainder)
			}
		})
	}
}
