package domain_test

import (
	"testing"

	"github.com/shopspring/decimal"

	income "github.com/valium69mg/finances-app/backend/internal/income/domain"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain/settingstest"
)

var d = settingstest.D

func ptr(s string) *decimal.Decimal {
	v := d(s)
	return &v
}

func TestToMXN(t *testing.T) {
	tests := []struct {
		name     string
		value    *decimal.Decimal
		currency string
		rate     *decimal.Decimal
		want     string
	}{
		{"USD converts", ptr("3383.33"), "USD", ptr("17.74"), "60020.2742"},
		{"USD without rate counts as 1", ptr("100"), "USD", nil, "100"},
		{"USD with zero rate counts as 1", ptr("100"), "USD", ptr("0"), "100"},
		{"MXN ignores the rate", ptr("100"), "MXN", ptr("17.74"), "100"},
		{"missing value is zero", nil, "USD", ptr("17.74"), "0"},
	}
	for _, tc := range tests {
		if got := income.ToMXN(tc.value, tc.currency, tc.rate); !got.Equal(d(tc.want)) {
			t.Errorf("%s: ToMXN = %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestComputeExtraSplit(t *testing.T) {
	cfg := settingstest.RealConfig()
	tests := []struct {
		name                      string
		amount, acc, goal         string
		sat, fund, inv, aguinaldo string
		reached                   bool
	}{
		// 35,000: SAT 5,775; remainder 29,225 -> 14,612.5 (half-even 14,612), 10,228.75 (10,229), 4,383.75 (4,384).
		{"estimated client B deposit", "35000", "0", "166922.64", "5775", "14612", "10229", "4384", false},
		// 11: SAT 1.815 -> 2; remainder 9 -> 4.5 (4), 3.15 (3), 1.35 (1); diff 1 goes to the emergency fund.
		{"rounding remainder goes to the fund", "11", "0", "166922.64", "2", "5", "3", "1", false},
		// 1,000.50: shares 165, 418, 292, 125 sum to 1,000; the 0.50 diff goes to the fund.
		{"fractional amount", "1000.50", "0", "166922.64", "165", "418.5", "292", "125", false},
		// Goal reached: the fund share is redirected to investments.
		{"goal reached", "35000", "166922.64", "166922.64", "5775", "0", "24841", "4384", true},
		// A zero goal never counts as reached.
		{"zero goal", "35000", "0", "0", "5775", "14612", "10229", "4384", false},
	}
	for _, tc := range tests {
		got := income.ComputeExtraSplit(d(tc.amount), cfg, d(tc.acc), d(tc.goal))
		if !got.SATReserve.Equal(d(tc.sat)) || !got.EmergencyFund.Equal(d(tc.fund)) ||
			!got.Investments.Equal(d(tc.inv)) || !got.AguinaldoVacation.Equal(d(tc.aguinaldo)) ||
			got.GoalReached != tc.reached {
			t.Errorf("%s: got %+v", tc.name, got)
		}
		sum := got.SATReserve.Add(got.EmergencyFund).Add(got.Investments).Add(got.AguinaldoVacation)
		if !sum.Equal(d(tc.amount)) {
			t.Errorf("%s: shares sum to %s, want %s", tc.name, sum, tc.amount)
		}
	}
}

func TestComputeExtraSplitUsesConfiguredRates(t *testing.T) {
	cfg := settingstest.RealConfig()
	cfg.ExtraIncomeSplit = map[string]decimal.Decimal{"sat_reserve_rate": d("0.2")}
	got := income.ComputeExtraSplit(d("1000"), cfg, d("0"), d("0"))
	// SAT 200; remainder 800 at the default 50/35/15 -> 400, 280, 120.
	if !got.SATReserve.Equal(d("200")) || !got.EmergencyFund.Equal(d("400")) ||
		!got.Investments.Equal(d("280")) || !got.AguinaldoVacation.Equal(d("120")) {
		t.Errorf("got %+v", got)
	}
}
