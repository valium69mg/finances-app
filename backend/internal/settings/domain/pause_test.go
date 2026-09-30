package domain_test

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain/settingstest"
)

func budgetOf(t *testing.T, cfg domain.Config, month, category string) decimal.Decimal {
	t.Helper()
	overrides, err := cfg.MonthBudgetOverrides(month)
	if err != nil {
		t.Fatalf("MonthBudgetOverrides(%s): %v", month, err)
	}
	for _, c := range cfg.Categories {
		if c.Name == category {
			b := domain.CategoryBudget(c, overrides)
			if b == nil {
				t.Fatalf("%s has no budget in %s", category, month)
			}
			return *b
		}
	}
	t.Fatalf("category %q not found", category)
	return decimal.Zero
}

func TestPauseBudgets(t *testing.T) {
	cfg := settingstest.RealConfig()
	pause := settingstest.RealPause()
	cfg.Pause = &pause

	cases := []struct {
		month, category, want string
	}{
		{"2026-10", ledger.CategoryInvestments, "0"},
		{"2026-10", ledger.CategoryFutureExpenses, "12000"},
		{"2026-12", ledger.CategoryFutureExpenses, "12000"},
		{"2027-01", ledger.CategoryInvestments, "0"},
		{"2027-01", ledger.CategoryFutureExpenses, "10000"},
		{"2027-02", ledger.CategoryInvestments, "5000"},
		{"2027-06", ledger.CategoryInvestments, "5000"},
		// Not affected: the other categories keep their normal budget.
		{"2026-10", "Vivienda", "3600"},
		{"2027-02", "Vivienda", "3600"},
	}
	for _, tc := range cases {
		t.Run(tc.month+"/"+tc.category, func(t *testing.T) {
			got := budgetOf(t, cfg, tc.month, tc.category)
			if !got.Equal(settingstest.D(tc.want)) {
				t.Fatalf("budget = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestPauseBeforeStartHasNoOverrides(t *testing.T) {
	p := settingstest.RealPause()
	if got := p.BudgetOverrides("2026-09"); len(got) != 0 {
		t.Fatalf("overrides before the pause = %v, want none", got)
	}
}

func TestPauseKeepsTaxOverrides(t *testing.T) {
	cfg := settingstest.RealConfig()
	pause := settingstest.RealPause()
	cfg.Pause = &pause
	overrides, err := cfg.MonthBudgetOverrides("2026-10")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{ledger.CategoryTaxes, ledger.CategoryFees, ledger.CategoryInvestments, ledger.CategoryFutureExpenses} {
		if _, ok := overrides[name]; !ok {
			t.Errorf("missing override for %s", name)
		}
	}
}

func TestNoPauseMatchesTaxOverridesOnly(t *testing.T) {
	cfg := settingstest.RealConfig()
	got, err := cfg.MonthBudgetOverrides("2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("overrides = %v, want only Impuestos and Comisiones", got)
	}
}

func TestPauseValidate(t *testing.T) {
	if err := func() error { p := settingstest.RealPause(); return p.Validate() }(); err != nil {
		t.Fatalf("real pause should be valid: %v", err)
	}
	tests := map[string]func(p *domain.PausePlan){
		"no months":          func(p *domain.PausePlan) { p.Months = nil },
		"bad month":          func(p *domain.PausePlan) { p.Months[0] = "2026-13" },
		"unordered":          func(p *domain.PausePlan) { p.Months[1], p.Months[2] = p.Months[2], p.Months[1] },
		"resume too early":   func(p *domain.PausePlan) { p.ResumeMonth = "2027-01" },
		"bad resume":         func(p *domain.PausePlan) { p.ResumeMonth = "feb" },
		"negative normal":    func(p *domain.PausePlan) { p.NormalBudget = settingstest.D("-1") },
		"missing plan month": func(p *domain.PausePlan) { delete(p.FutureExpensesPlan, "2026-11") },
		"extra plan month":   func(p *domain.PausePlan) { p.FutureExpensesPlan["2030-01"] = settingstest.D("1") },
		"negative plan":      func(p *domain.PausePlan) { p.FutureExpensesPlan["2026-10"] = settingstest.D("-5") },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			p := settingstest.RealPause()
			mutate(&p)
			if err := p.Validate(); !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("Validate() = %v, want ErrInvalid", err)
			}
		})
	}
}
