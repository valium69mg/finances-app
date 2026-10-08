package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain/settingstest"
)

func TestRealConfigSectionsAreValid(t *testing.T) {
	cfg := settingstest.RealConfig()
	checks := map[string]error{
		"general":     cfg.General().Validate(),
		"categories":  domain.ValidateCategories(cfg.Categories),
		"clients":     domain.ValidateClients(cfg.Clients),
		"instruments": domain.ValidateInstruments(cfg.Instruments, cfg.InstrumentByCategory),
		"brackets":    domain.ValidateBrackets(cfg.Brackets),
	}
	for name, err := range checks {
		if err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestInvalidSections(t *testing.T) {
	d := settingstest.D
	neg, huge, over := d("-1"), d("1e2000000000"), d("10000000000000")
	tests := map[string]error{
		"empty category name":  domain.ValidateCategories([]domain.Category{{Name: " ", Kind: ledger.KindExpense}}),
		"duplicate category":   domain.ValidateCategories([]domain.Category{{Name: "A", Kind: ledger.KindExpense}, {Name: "A", Kind: ledger.KindExpense}}),
		"unknown kind":         domain.ValidateCategories([]domain.Category{{Name: "A", Kind: "Otro"}}),
		"negative budget":      domain.ValidateCategories([]domain.Category{{Name: "A", Kind: ledger.KindExpense, Budget: &neg}}),
		"client without id":    domain.ValidateClients([]domain.Client{{Name: "x", Currency: "MXN"}}),
		"client bad currency":  domain.ValidateClients([]domain.Client{{ID: "a", Name: "x", Currency: "EUR"}}),
		"client rate over one": domain.ValidateClients([]domain.Client{{ID: "a", Name: "x", Currency: "MXN", IVARate: d("1.5")}}),
		"client short postal":  domain.ValidateClients([]domain.Client{{ID: "a", Name: "x", Currency: "MXN", PostalCode: "6400"}}),
		"client letter postal": domain.ValidateClients([]domain.Client{{ID: "a", Name: "x", Currency: "MXN", PostalCode: "6400A"}}),
		"instrument duplicate": domain.ValidateInstruments([]domain.Instrument{{ID: "a", Name: "A"}, {ID: "a", Name: "B"}}, nil),
		"unknown default":      domain.ValidateInstruments([]domain.Instrument{{ID: "a", Name: "A"}}, map[string]string{"Inversiones": "zzz"}),
		"no brackets":          domain.ValidateBrackets(nil),
		"brackets not rising":  domain.ValidateBrackets([]domain.Bracket{{Upper: d("100"), Rate: d("0.01")}, {Upper: d("100"), Rate: d("0.02")}}),
		"duplicate payment":    domain.ValidatePaymentMethods([]string{"Efectivo", "Efectivo"}),
		"issuer without rfc":   domain.Issuer{Name: "x"}.Validate(),
		"general fx zero":      domain.General{MorseFeeRate: d("0.001")}.Validate(),
		"general fee over one": domain.General{FXRateApplied: d("17"), MorseFeeRate: d("2")}.Validate(),
		"cycle day negative":   domain.General{FXRateApplied: d("17"), MorseFeeRate: d("0.001"), CycleStartDay: -1}.Validate(),
		"cycle day above 31":   domain.General{FXRateApplied: d("17"), MorseFeeRate: d("0.001"), CycleStartDay: 32}.Validate(),
		// Huge exponents are rejected before any comparison rescales them.
		"general huge salary":  domain.General{SalaryUSD: d("1e2000000000"), FXRateApplied: d("17"), MorseFeeRate: d("0.001")}.Validate(),
		"general huge fx":      domain.General{FXRateApplied: d("1e999999999"), MorseFeeRate: d("0.001")}.Validate(),
		"huge budget":          domain.ValidateCategories([]domain.Category{{Name: "A", Kind: ledger.KindExpense, Budget: &huge}}),
		"huge client rate":     domain.ValidateClients([]domain.Client{{ID: "a", Name: "x", Currency: "MXN", IVARate: huge}}),
		"huge bracket bound":   domain.ValidateBrackets([]domain.Bracket{{Upper: huge, Rate: d("0.01")}}),
		"huge bracket rate":    domain.ValidateBrackets([]domain.Bracket{{Upper: d("100"), Rate: huge}}),
		"budget above maximum": domain.ValidateCategories([]domain.Category{{Name: "A", Kind: ledger.KindExpense, Budget: &over}}),
	}
	for name, err := range tests {
		if !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

func TestCycleStartDayBounds(t *testing.T) {
	for _, day := range []int{0, 1, 30, 31} {
		g := settingstest.RealConfig().General()
		g.CycleStartDay = day
		if err := g.Validate(); err != nil {
			t.Errorf("cycle_start_day %d: %v", day, err)
		}
	}
}

func general(split map[string]string, alloc ...domain.Weight) domain.General {
	g := settingstest.RealConfig().General()
	if split != nil {
		g.ExtraIncomeSplit = map[string]decimal.Decimal{}
		for k, v := range split {
			g.ExtraIncomeSplit[k] = d(v)
		}
	}
	if alloc != nil {
		g.InvestmentAllocation = alloc
	}
	return g
}

func TestSplitDestinationsMustAddUpToOneHundredPercent(t *testing.T) {
	valid := map[string]map[string]string{
		"realistic":              {"sat_reserve_rate": "0.165", "fondo_emergencia": "0.5", "inversiones": "0.35", "aguinaldo_vacaciones": "0.15"},
		"sat reserve is aside":   {"sat_reserve_rate": "0.9", "fondo_emergencia": "0.5", "inversiones": "0.35", "aguinaldo_vacaciones": "0.15"},
		"no sat reserve":         {"fondo_emergencia": "0.5", "inversiones": "0.35", "aguinaldo_vacaciones": "0.15"},
		"only defaults":          {},
		"partial equal defaults": {"inversiones": "0.35"},
		"custom 60-30-10":        {"fondo_emergencia": "0.6", "inversiones": "0.3", "aguinaldo_vacaciones": "0.1"},
	}
	for name, split := range valid {
		if err := general(split).Validate(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	invalid := map[string]map[string]string{
		"adds up to 99.9":              {"fondo_emergencia": "0.5", "inversiones": "0.35", "aguinaldo_vacaciones": "0.149"},
		"adds up to 110":               {"fondo_emergencia": "0.5", "inversiones": "0.35", "aguinaldo_vacaciones": "0.25"},
		"partial against the defaults": {"inversiones": "0.9"},
		"sat reserve above 100":        {"sat_reserve_rate": "1.1", "fondo_emergencia": "0.5", "inversiones": "0.35", "aguinaldo_vacaciones": "0.15"},
	}
	for name, split := range invalid {
		if err := general(split).Validate(); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

func TestInvestmentAllocationMustAddUpToOneHundredPercent(t *testing.T) {
	w := func(k, v string) domain.Weight { return domain.Weight{Key: k, Value: d(v)} }
	for name, alloc := range map[string][]domain.Weight{
		"realistic voo 1.0": {w("voo", "1.0")},
		"60-40":             {w("voo", "0.6"), w("vxus", "0.4")},
		"thirds are exact":  {w("a", "0.3333"), w("b", "0.3333"), w("c", "0.3334")},
	} {
		if err := general(nil, alloc...).Validate(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	for name, alloc := range map[string][]domain.Weight{
		"adds up to 90":  {w("voo", "0.5"), w("vxus", "0.4")},
		"adds up to 101": {w("voo", "0.6"), w("vxus", "0.41")},
		"single 0.99":    {w("voo", "0.99")},
	} {
		if err := general(nil, alloc...).Validate(); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	g := settingstest.RealConfig().General()
	g.InvestmentAllocation = nil
	if err := g.Validate(); err != nil {
		t.Errorf("no allocation is valid: %v", err)
	}
}

func TestBaseMonthlyIncome(t *testing.T) {
	tests := []struct {
		salary, fx, want string
		ok               bool
	}{
		{"3500", "17.74", "62090", true},
		{"3500.55", "17.743", "62110.26", true}, // 62110.25865 rounds to cents
		{"0", "17.74", "", false},
		{"3500", "0", "", false},
		{"", "", "", false},
		{"1e2000000000", "17", "", false}, // out of range, never multiplied
	}
	for _, tc := range tests {
		g := domain.General{}
		if tc.salary != "" {
			g.SalaryUSD = d(tc.salary)
		}
		if tc.fx != "" {
			g.FXRateApplied = d(tc.fx)
		}
		base, ok := g.BaseMonthlyIncome()
		if ok != tc.ok || (ok && !base.Equal(d(tc.want))) {
			t.Errorf("base(%s, %s) = %s, %v; want %s, %v", tc.salary, tc.fx, base, ok, tc.want, tc.ok)
		}
	}
}

func TestBudgetTotalCannotExceedBase(t *testing.T) {
	g := settingstest.RealConfig().General() // base 62,090.00
	cfg := settingstest.RealConfig()
	if err := domain.ValidateBudgetTotal(cfg.Categories, g); err != nil {
		t.Fatalf("realistic budgets (56,827 of 62,090) must pass: %v", err)
	}
	bud := func(name string, kind ledger.Kind, v string) domain.Category {
		b := d(v)
		return domain.Category{Name: name, Kind: kind, Budget: &b}
	}
	exact := []domain.Category{bud("A", ledger.KindExpense, "40000.50"), bud("B", ledger.KindSavings, "22089.50")}
	if err := domain.ValidateBudgetTotal(exact, g); err != nil {
		t.Errorf("exactly 100%% passes: %v", err)
	}
	over := []domain.Category{bud("A", ledger.KindExpense, "40000.50"), bud("B", ledger.KindSavings, "22089.51")}
	err := domain.ValidateBudgetTotal(over, g)
	if !errors.Is(err, domain.ErrInvalid) || !strings.Contains(err.Error(), "0.01 more") {
		t.Errorf("one cent over: err = %v", err)
	}
	// Income budgets do not count, and without a base the rule does not apply.
	income := []domain.Category{bud("Sueldo", ledger.KindIncome, "999999")}
	if err := domain.ValidateBudgetTotal(income, g); err != nil {
		t.Errorf("income budgets are ignored: %v", err)
	}
	if err := domain.ValidateBudgetTotal(over, domain.General{}); err != nil {
		t.Errorf("no base, no rule: %v", err)
	}
}
