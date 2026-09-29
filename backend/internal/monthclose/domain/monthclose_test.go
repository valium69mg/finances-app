package domain_test

import (
	"testing"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	monthclose "github.com/valium69mg/finances-app/backend/internal/monthclose/domain"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain/settingstest"
)

var d = settingstest.D

func budget(name string, kind ledger.Kind, amount string) settings.Category {
	b := d(amount)
	return settings.Category{Name: name, Kind: kind, Budget: &b}
}

// closeConfig keeps the real tax parameters but uses a small category set so
// every expected number can be checked by hand: four expense categories of
// 1,000 (emergency goal = 6 * 4,000 = 24,000) and two savings categories.
func closeConfig() settings.Config {
	cfg := settingstest.RealConfig()
	cfg.Categories = []settings.Category{
		{Name: "Sueldo", Kind: ledger.KindIncome},
		budget("Vivienda", ledger.KindExpense, "1000"),
		budget("Servicios", ledger.KindExpense, "1000"),
		budget("Mandado", ledger.KindExpense, "1000"),
		budget("Salud", ledger.KindExpense, "1000"),
		budget("Fondo de emergencia", ledger.KindSavings, "1000"),
		budget("Inversiones", ledger.KindSavings, "0"),
	}
	return cfg
}

func mov(date string, kind ledger.Kind, category, amount string) ledger.Movement {
	return ledger.Movement{Date: date, Kind: kind, Category: category, AmountMXN: d(amount)}
}

// octoberMovements has income 10,000, expenses 4,000 and savings 2,000, so the
// leftover is 4,000. fundBefore is the emergency fund saved in September.
func octoberMovements(fundBefore string) []ledger.Movement {
	return []ledger.Movement{
		mov("2026-10-01", ledger.KindIncome, "Sueldo", "10000"),
		mov("2026-10-02", ledger.KindExpense, "Vivienda", "1200"),  // exactly +20%: no hint
		mov("2026-10-03", ledger.KindExpense, "Servicios", "1201"), // +20.1%: hint
		mov("2026-10-04", ledger.KindExpense, "Mandado", "799"),    // -20.1%: hint
		mov("2026-10-05", ledger.KindExpense, "Salud", "800"),      // exactly -20%: no hint
		mov("2026-10-06", ledger.KindSavings, "Fondo de emergencia", "1500"),
		mov("2026-10-07", ledger.KindSavings, "Inversiones", "500"), // zero budget: no hint
		mov("2026-09-15", ledger.KindSavings, "Fondo de emergencia", fundBefore),
	}
}

func TestCloseOverBudget(t *testing.T) {
	res, err := monthclose.Close(closeConfig(), octoberMovements("21000"), "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	// Strictly above a non-zero budget, expense rows only: Vivienda 1,200 and
	// Servicios 1,201; Mandado and Salud are under, and savings rows never count.
	want := []string{"Vivienda", "Servicios"}
	if len(res.OverBudget) != len(want) {
		t.Fatalf("over budget = %+v, want %v", res.OverBudget, want)
	}
	for i, name := range want {
		if res.OverBudget[i].Name != name {
			t.Errorf("over budget %d = %s, want %s", i, res.OverBudget[i].Name, name)
		}
	}
}

func TestCloseAdjustments(t *testing.T) {
	res, err := monthclose.Close(closeConfig(), octoberMovements("21000"), "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	// Only deviations strictly above 20% are flagged: Vivienda (+20%) and Salud
	// (-20%) sit exactly on the boundary and Inversiones has a zero budget.
	want := []struct {
		name, budget, real, deviation string
		kind                          ledger.Kind
	}{
		{"Servicios", "1000", "1201", "20.1", ledger.KindExpense},
		{"Mandado", "1000", "799", "-20.1", ledger.KindExpense},
		{"Fondo de emergencia", "1000", "1500", "50", ledger.KindSavings},
	}
	if len(res.Adjustments) != len(want) {
		t.Fatalf("adjustments = %+v, want %d", res.Adjustments, len(want))
	}
	for i, w := range want {
		g := res.Adjustments[i]
		if g.Name != w.name || g.Kind != w.kind || !g.Budget.Equal(d(w.budget)) ||
			!g.Real.Equal(d(w.real)) || !g.DeviationPct.Equal(d(w.deviation)) {
			t.Errorf("adjustment %d = %+v, want %+v", i, g, w)
		}
	}
}

func TestCloseSuggestion(t *testing.T) {
	tests := []struct {
		name       string
		fundBefore string
		toFund     string
		toInvest   string
	}{
		// Accumulated 22,500 of 24,000: room 1,500, so 1,500 to the fund and 2,500 to investments.
		{"emergency fund has room", "21000", "1500", "2500"},
		// Accumulated 24,500 >= 24,000: no room, everything goes to investments.
		{"emergency fund goal reached", "23000", "0", "4000"},
		// Accumulated 1,500 of 24,000: room 22,500 exceeds the leftover, so it all goes to the fund.
		{"fund far from goal", "0", "4000", "0"},
	}
	for _, tc := range tests {
		res, err := monthclose.Close(closeConfig(), octoberMovements(tc.fundBefore), "2026-10")
		if err != nil {
			t.Fatal(err)
		}
		if !res.Available.Equal(d("4000")) || res.Suggestion == nil {
			t.Fatalf("%s: available %s suggestion %v", tc.name, res.Available, res.Suggestion)
		}
		s := res.Suggestion
		if !s.ToEmergencyFund.Equal(d(tc.toFund)) || !s.ToInvestments.Equal(d(tc.toInvest)) || !s.EmergencyGoal.Equal(d("24000")) {
			t.Errorf("%s: suggestion = %+v", tc.name, *s)
		}
	}
}

func TestCloseNoSuggestionWithoutLeftover(t *testing.T) {
	// Income 1,000 against 4,000 of expenses and 2,000 of savings: negative leftover.
	movements := []ledger.Movement{
		mov("2026-10-01", ledger.KindIncome, "Sueldo", "1000"),
		mov("2026-10-02", ledger.KindExpense, "Vivienda", "4000"),
		mov("2026-10-03", ledger.KindSavings, "Inversiones", "2000"),
	}
	res, err := monthclose.Close(closeConfig(), movements, "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Available.Equal(d("-5000")) || res.Suggestion != nil {
		t.Errorf("available %s suggestion %v, want -5000 and nil", res.Available, res.Suggestion)
	}

	// A leftover of exactly zero also suggests nothing.
	movements = []ledger.Movement{
		mov("2026-10-01", ledger.KindIncome, "Sueldo", "1000"),
		mov("2026-10-02", ledger.KindExpense, "Vivienda", "1000"),
	}
	res, err = monthclose.Close(closeConfig(), movements, "2026-10")
	if err != nil || res.Suggestion != nil || len(res.OverBudget) != 0 {
		t.Errorf("zero leftover: suggestion %v over %v err %v", res.Suggestion, res.OverBudget, err)
	}
}

func TestCloseFailsWithoutConfig(t *testing.T) {
	if _, err := monthclose.Close(settings.Config{}, nil, "2026-10"); err == nil {
		t.Error("Close with an empty config should fail")
	}
}
