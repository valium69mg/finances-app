package domain_test

import (
	"errors"
	"testing"

	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain/settingstest"
)

func TestTaxBudgetBreakdownSalaryOnly(t *testing.T) {
	tb, err := settingstest.RealConfig().TaxBudgetBreakdown()
	if err != nil {
		t.Fatal(err)
	}
	// Salary 3500 USD * 17.74 = 62,090 falls in the 1.5% bracket.
	want := map[string]struct{ got, want string }{
		"salary":  {tb.SalaryEstMXN.String(), "62090"},
		"total":   {tb.TotalEst.String(), "62090"},
		"rate":    {tb.Rate.String(), "0.015"},
		"isr":     {tb.ISRToPay.String(), "931.35"},
		"iva":     {tb.IVAToPay.String(), "0"},
		"taxes":   {tb.TaxesBudget.String(), "931.35"},
		"fees":    {tb.FeesBudget.String(), "62.09"},
		"extra":   {tb.ExtraEstTotal.String(), "0"},
		"subtotl": {tb.ExtraEstSubtotal.String(), "0"},
	}
	for name, w := range want {
		if !d(w.got).Equal(d(w.want)) {
			t.Errorf("%s = %s, want %s", name, w.got, w.want)
		}
	}
}

func TestTaxBudgetBreakdownIncludingExtraIncome(t *testing.T) {
	cfg := settingstest.RealConfig()
	cfg.BudgetIncludesExtraIncome = true
	tb, err := cfg.TaxBudgetBreakdown()
	if err != nil {
		t.Fatal(err)
	}
	// 35,000 / 1.16 = 30,172.4137931034 (10 decimals); IVA is the difference.
	// Total 92,262.4137931034 moves to the 2% bracket.
	checks := []struct{ name, got, want string }{
		{"subtotal", tb.ExtraEstSubtotal.String(), "30172.4137931034"},
		{"iva", tb.ExtraEstIVA.String(), "4827.5862068966"},
		{"total", tb.TotalEst.String(), "92262.4137931034"},
		{"rate", tb.Rate.String(), "0.02"},
		{"isr", tb.ISRToPay.String(), "1845.248275862068"},
		{"taxes", tb.TaxesBudget.String(), "6672.834482758668"},
		{"fees", tb.FeesBudget.String(), "62.09"},
	}
	for _, c := range checks {
		if !d(c.got).Equal(d(c.want)) {
			t.Errorf("%s = %s, want %s", c.name, c.got, c.want)
		}
	}
}

func TestTaxBudgetBreakdownRequiresConfig(t *testing.T) {
	if _, err := (settings.Config{}).TaxBudgetBreakdown(); !errors.Is(err, settings.ErrMissingConfig) {
		t.Errorf("error = %v, want ErrMissingConfig", err)
	}
}

func TestBudgetOverrides(t *testing.T) {
	got, err := settingstest.RealConfig().BudgetOverrides()
	if err != nil {
		t.Fatal(err)
	}
	if !got["Impuestos"].Equal(d("931.35")) || !got["Comisiones"].Equal(d("62.09")) {
		t.Errorf("overrides = %v", got)
	}
}
