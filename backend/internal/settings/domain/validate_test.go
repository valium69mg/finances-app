package domain_test

import (
	"errors"
	"testing"

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
		"instrument duplicate": domain.ValidateInstruments([]domain.Instrument{{ID: "a", Name: "A"}, {ID: "a", Name: "B"}}, nil),
		"unknown default":      domain.ValidateInstruments([]domain.Instrument{{ID: "a", Name: "A"}}, map[string]string{"Inversiones": "zzz"}),
		"no brackets":          domain.ValidateBrackets(nil),
		"brackets not rising":  domain.ValidateBrackets([]domain.Bracket{{Upper: d("100"), Rate: d("0.01")}, {Upper: d("100"), Rate: d("0.02")}}),
		"duplicate payment":    domain.ValidatePaymentMethods([]string{"Efectivo", "Efectivo"}),
		"issuer without rfc":   domain.Issuer{Name: "x"}.Validate(),
		"general fx zero":      domain.General{MorseFeeRate: d("0.001")}.Validate(),
		"general fee over one": domain.General{FXRateApplied: d("17"), MorseFeeRate: d("2")}.Validate(),
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
