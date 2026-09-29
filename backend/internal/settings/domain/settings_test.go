package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain/settingstest"
)

var d = settingstest.D

func TestResicoRateBoundaries(t *testing.T) {
	cfg := settingstest.RealConfig()
	tests := []struct{ income, want string }{
		{"0", "0.01"},
		{"25000", "0.01"}, // upper bound is inclusive
		{"25000.01", "0.011"},
		{"50000", "0.011"},
		{"62090", "0.015"}, // 3500 USD * 17.74
		{"83333.33", "0.015"},
		{"83333.34", "0.02"},
		{"208333.33", "0.02"},
		{"291666.67", "0.025"},
		{"500000", "0.025"}, // above the last bracket keeps the last rate
	}
	for _, tc := range tests {
		got, err := cfg.ResicoRate(d(tc.income))
		if err != nil || !got.Equal(d(tc.want)) {
			t.Errorf("ResicoRate(%s) = %s, %v; want %s", tc.income, got, err, tc.want)
		}
	}
	if _, err := (settings.Config{}).ResicoRate(d("1")); !errors.Is(err, settings.ErrMissingConfig) {
		t.Errorf("empty brackets error = %v, want ErrMissingConfig", err)
	}
}

func TestValidateReportsMissingKeys(t *testing.T) {
	if err := settingstest.RealConfig().Validate(); err != nil {
		t.Fatalf("real config should be valid: %v", err)
	}
	err := (settings.Config{}).Validate()
	if !errors.Is(err, settings.ErrMissingConfig) {
		t.Fatalf("Validate = %v, want ErrMissingConfig", err)
	}
	for _, key := range []string{"salary_usd", "fx_rate_applied", "morse_fee_rate", "emergency_months", "resico_brackets"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error %q does not mention %s", err, key)
		}
	}
}

func TestInferCategory(t *testing.T) {
	cfg := settingstest.RealConfig()
	tests := []struct {
		desc string
		kind ledger.Kind
		want string
		ok   bool
	}{
		{"Uber Eats cena", ledger.KindExpense, "Comida fuera", true}, // declared before Transporte's "uber"
		{"uber al aeropuerto", ledger.KindExpense, "Transporte", true},
		{"Predial 2026", ledger.KindExpense, "Vivienda", true},
		{"Predial 2026", ledger.KindSavings, "Gastos futuros", true}, // same keyword, different kind
		{"COLEGIATURA octubre", ledger.KindExpense, "Educación", true},
		{"Sueldo octubre", ledger.KindIncome, "Sueldo", true},
		{"algo sin palabra clave", ledger.KindExpense, "", false},
		{"", ledger.KindExpense, "", false},
	}
	for _, tc := range tests {
		got, ok := cfg.InferCategory(tc.desc, tc.kind)
		if got != tc.want || ok != tc.ok {
			t.Errorf("InferCategory(%q, %s) = %q, %v; want %q, %v", tc.desc, tc.kind, got, ok, tc.want, tc.ok)
		}
	}
}

func TestResolveInstrument(t *testing.T) {
	cfg := settingstest.RealConfig()

	if got, err := cfg.ResolveInstrument("Inversiones", "vxus"); err != nil || got != "vxus" {
		t.Errorf("explicit instrument = %q, %v; want vxus", got, err)
	}
	if got, err := cfg.ResolveInstrument("Inversiones", ""); err != nil || got != "voo" {
		t.Errorf("category default = %q, %v; want voo", got, err)
	}
	if got, err := cfg.ResolveInstrument("Otros", ""); err != nil || got != "" {
		t.Errorf("no mapping = %q, %v; want empty and nil error", got, err)
	}
	if _, err := cfg.ResolveInstrument("Inversiones", "nope"); err == nil {
		t.Error("unknown explicit instrument should fail")
	}
	cfg.InstrumentByCategory["Ocio"] = "ghost"
	if _, err := cfg.ResolveInstrument("Ocio", ""); err == nil {
		t.Error("default pointing to an unknown instrument should fail")
	}
}

func TestCategoryBudget(t *testing.T) {
	cfg := settingstest.RealConfig()
	byName := map[string]settings.Category{}
	for _, c := range cfg.Categories {
		byName[c.Name] = c
	}
	overrides := map[string]decimal.Decimal{"Impuestos": d("931.35")}

	if got := settings.CategoryBudget(byName["Impuestos"], overrides); got == nil || !got.Equal(d("931.35")) {
		t.Errorf("override should replace a nil budget, got %v", got)
	}
	if got := settings.CategoryBudget(byName["Vivienda"], overrides); got == nil || !got.Equal(d("3600")) {
		t.Errorf("Vivienda budget = %v, want 3600", got)
	}
	if got := settings.CategoryBudget(byName["Ocio"], overrides); got != nil {
		t.Errorf("Ocio budget = %v, want nil", got)
	}
}

func TestFindClientAndInstrument(t *testing.T) {
	cfg := settingstest.RealConfig()
	if c, ok := cfg.FindClient("b"); !ok || !c.IVARate.Equal(d("0.16")) {
		t.Errorf("FindClient(b) = %+v, %v", c, ok)
	}
	if _, ok := cfg.FindClient("zzz"); ok {
		t.Error("FindClient(zzz) should not be found")
	}
	if _, ok := cfg.FindInstrument("cetes-91"); !ok {
		t.Error("FindInstrument(cetes-91) should be found")
	}
}
