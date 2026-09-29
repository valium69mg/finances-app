package domain_test

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	savings "github.com/valium69mg/finances-app/backend/internal/savings/domain"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain/settingstest"
)

var d = settingstest.D

func saving(date, category, instrument, amount string) ledger.Movement {
	return ledger.Movement{Date: date, Kind: ledger.KindSavings, Category: category, Instrument: instrument, AmountMXN: d(amount)}
}

func TestEmergencyFund(t *testing.T) {
	cfg := settingstest.RealConfig()
	movements := []ledger.Movement{
		saving("2026-10-05", "Fondo de emergencia", "cetes-28", "14612"),
		saving("2026-11-05", "Fondo de emergencia", "cetes-28", "500"),
		saving("2026-11-20", "Fondo de emergencia", "cetes-28", "-112"), // withdrawal
		saving("2026-11-05", "Inversiones", "voo", "10229"),
	}
	got, err := savings.EmergencyFund(cfg, movements)
	if err != nil {
		t.Fatal(err)
	}
	// Goal: 6 * (26,827 configured budgets + 931.35 Impuestos + 62.09 Comisiones).
	if !got.Accumulated.Equal(d("15000")) {
		t.Errorf("Accumulated = %s, want 15000", got.Accumulated)
	}
	if !got.Goal.Equal(d("166922.64")) {
		t.Errorf("Goal = %s, want 166922.64", got.Goal)
	}

	cfg.EmergencyMonths = nil
	if _, err := savings.EmergencyFund(cfg, nil); !errors.Is(err, settings.ErrMissingConfig) {
		t.Errorf("missing emergency_months error = %v", err)
	}
}

func TestSplitByWeights(t *testing.T) {
	tests := []struct {
		name    string
		total   string
		weights []settings.Weight
		want    []string
	}{
		{
			"single weight takes everything", "1234.56",
			[]settings.Weight{{Key: "voo", Value: d("1")}},
			[]string{"1234.56"},
		},
		{
			// 3.5 -> 4 and 3.5 -> 4 (half-even), 3 -> 3: sum 11, so the first largest weight gives back 1.
			"tie between largest weights uses the first", "10",
			[]settings.Weight{{Key: "a", Value: d("0.35")}, {Key: "b", Value: d("0.35")}, {Key: "c", Value: d("0.3")}},
			[]string{"3", "4", "3"},
		},
		{
			"largest weight absorbs the remainder", "10",
			[]settings.Weight{{Key: "a", Value: d("0.3")}, {Key: "b", Value: d("0.35")}, {Key: "c", Value: d("0.35")}},
			[]string{"3", "3", "4"},
		},
	}
	for _, tc := range tests {
		got := savings.SplitByWeights(d(tc.total), tc.weights)
		if len(got) != len(tc.want) {
			t.Fatalf("%s: %d shares, want %d", tc.name, len(got), len(tc.want))
		}
		sum := decimal.Zero
		for i, s := range got {
			sum = sum.Add(s.Amount)
			if s.Key != tc.weights[i].Key || !s.Amount.Equal(d(tc.want[i])) {
				t.Errorf("%s: share %d = %+v, want %s %s", tc.name, i, s, tc.weights[i].Key, tc.want[i])
			}
		}
		if !sum.Equal(d(tc.total)) {
			t.Errorf("%s: shares sum to %s, want %s", tc.name, sum, tc.total)
		}
	}
	if got := savings.SplitByWeights(d("100"), nil); got != nil {
		t.Errorf("no weights = %v, want nil", got)
	}
}

func TestLatestValuation(t *testing.T) {
	vals := []ledger.Valuation{
		{Date: "2026-10-01", Instrument: "voo", ValueMXN: d("10000")},
		{Date: "2026-11-01", Instrument: "voo", ValueMXN: d("11000")},
		{Date: "2026-11-01", Instrument: "voo", ValueMXN: d("11100")}, // same date, listed last
		{Date: "2026-12-01", Instrument: "vxus", ValueMXN: d("500")},
	}
	got, ok := savings.LatestValuation(vals, "voo")
	if !ok || !got.ValueMXN.Equal(d("11100")) || got.Date != "2026-11-01" {
		t.Errorf("LatestValuation(voo) = %+v, %v", got, ok)
	}
	if _, ok := savings.LatestValuation(vals, "cetes-28"); ok {
		t.Error("LatestValuation(cetes-28) should not be found")
	}
}

func TestComputePortfolio(t *testing.T) {
	cfg := settingstest.RealConfig()
	movements := []ledger.Movement{
		{Date: "2026-10-01", Kind: ledger.KindIncome, Category: "Sueldo", AmountMXN: d("60020.2742")}, // ignored
		saving("2026-10-05", "Reserva SAT", "liquidez-gbm", "5775"),
		saving("2026-10-20", "Reserva SAT", "liquidez-gbm", "-775"),
		saving("2026-10-05", "Fondo de emergencia", "cetes-28", "14612"),
		saving("2026-10-05", "Inversiones", "voo", "10229"),
		saving("2026-10-06", "Gastos futuros", "zzz", "100"),
		saving("2026-10-07", "Gastos futuros", "", "50"),
	}
	valuations := []ledger.Valuation{
		{Date: "2026-10-15", Instrument: "voo", ValueMXN: d("10500")},
		{Date: "2026-11-01", Instrument: "voo", ValueMXN: d("11000")},
	}

	p, err := savings.ComputePortfolio(cfg, movements, valuations)
	if err != nil {
		t.Fatal(err)
	}

	wantIDs := []string{"liquidez-gbm", "cetes-28", "cetes-91", "voo", "vxus", "zzz", savings.NoInstrument}
	if len(p.Rows) != len(wantIDs) {
		t.Fatalf("rows = %d, want %d", len(p.Rows), len(wantIDs))
	}
	for i, id := range wantIDs {
		if p.Rows[i].ID != id {
			t.Errorf("row %d = %s, want %s", i, p.Rows[i].ID, id)
		}
	}

	byID := map[string]savings.PortfolioRow{}
	for _, r := range p.Rows {
		byID[r.ID] = r
	}
	voo := byID["voo"]
	if !voo.Contributed.Equal(d("10229")) || !voo.Value.Equal(d("11000")) || voo.Unvalued || voo.ValueDate != "2026-11-01" {
		t.Errorf("voo = %+v", voo)
	}
	if !voo.Gain.Equal(d("771")) || !voo.GainPct.Equal(d("7.5373936846")) {
		t.Errorf("voo gain = %s (%s%%), want 771 (7.5373936846%%)", voo.Gain, voo.GainPct)
	}
	if !voo.PctOfTotal.Equal(d("35.7584032248")) {
		t.Errorf("voo share = %s, want 35.7584032248", voo.PctOfTotal)
	}
	liquidity := byID["liquidez-gbm"]
	if !liquidity.Contributed.Equal(d("5000")) || !liquidity.Value.Equal(d("5000")) || !liquidity.Unvalued || !liquidity.Gain.IsZero() {
		t.Errorf("liquidez-gbm = %+v", liquidity)
	}
	if !liquidity.PctOfTotal.Equal(d("16.2538196476")) {
		t.Errorf("liquidez-gbm share = %s, want 16.2538196476", liquidity.PctOfTotal)
	}
	if empty := byID["vxus"]; !empty.Contributed.IsZero() || !empty.GainPct.IsZero() {
		t.Errorf("vxus = %+v", empty)
	}
	unknown := byID["zzz"]
	if unknown.Type != savings.UnknownInstrumentType || unknown.Platform != savings.UnknownPlatform ||
		!unknown.Value.Equal(d("100")) || !unknown.Unvalued || !unknown.Gain.IsZero() {
		t.Errorf("zzz = %+v", unknown)
	}

	if !p.TotalContributed.Equal(d("29991")) || !p.TotalValue.Equal(d("30762")) {
		t.Errorf("totals = %s / %s, want 29991 / 30762", p.TotalContributed, p.TotalValue)
	}

	wantTypes := []savings.TypeTotal{
		{Type: "liquidez", Contributed: d("5000"), Value: d("5000")},
		{Type: "deuda", Contributed: d("14612"), Value: d("14612")},
		{Type: "renta_variable", Contributed: d("10229"), Value: d("11000")},
		{Type: "desconocido", Contributed: d("150"), Value: d("150")},
	}
	if len(p.ByType) != len(wantTypes) {
		t.Fatalf("types = %+v", p.ByType)
	}
	for i, w := range wantTypes {
		g := p.ByType[i]
		if g.Type != w.Type || !g.Contributed.Equal(w.Contributed) || !g.Value.Equal(w.Value) {
			t.Errorf("type %d = %+v, want %+v", i, g, w)
		}
	}

	wantDest := map[string]string{
		"Reserva SAT": "5000", "Fondo de emergencia": "14612", "Aguinaldo y vacaciones": "0",
		"Gastos futuros": "150", "Inversiones": "10229",
	}
	if len(p.ByDestination) != len(savings.DestinationCategories) {
		t.Fatalf("destinations = %+v", p.ByDestination)
	}
	for _, dt := range p.ByDestination {
		if !dt.Balance.Equal(d(wantDest[dt.Category])) {
			t.Errorf("destination %s = %s, want %s", dt.Category, dt.Balance, wantDest[dt.Category])
		}
	}

	if !p.Emergency.Accumulated.Equal(d("14612")) || !p.Emergency.Goal.Equal(d("166922.64")) {
		t.Errorf("emergency = %+v", p.Emergency)
	}
}
