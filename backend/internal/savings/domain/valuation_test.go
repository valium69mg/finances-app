package domain_test

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	"github.com/valium69mg/finances-app/backend/internal/savings/domain"
)

func TestValidateValuation(t *testing.T) {
	ok := ledger.Valuation{Date: "2026-10-01", Instrument: "voo", ValueMXN: decimal.RequireFromString("100.50")}
	if err := domain.ValidateValuation(ok); err != nil {
		t.Fatalf("valid valuation rejected: %v", err)
	}
	for _, s := range []string{"0.005", "100.555", "999999999999.99", "999999999999.994"} {
		v := ledger.Valuation{Date: ok.Date, Instrument: "voo", ValueMXN: decimal.RequireFromString(s)}
		if err := domain.ValidateValuation(v); err != nil {
			t.Errorf("%s rejected: %v", s, err)
		}
	}
	bad := map[string]ledger.Valuation{
		"bad date":         {Date: "2026-13-01", Instrument: "voo", ValueMXN: ok.ValueMXN},
		"empty date":       {Instrument: "voo", ValueMXN: ok.ValueMXN},
		"empty instrument": {Date: ok.Date, Instrument: "  ", ValueMXN: ok.ValueMXN},
		"zero value":       {Date: ok.Date, Instrument: "voo", ValueMXN: decimal.Zero},
		"negative value":   {Date: ok.Date, Instrument: "voo", ValueMXN: decimal.RequireFromString("-1")},
		"rounds to zero":   {Date: ok.Date, Instrument: "voo", ValueMXN: decimal.RequireFromString("0.004")},
		"at the upper cap": {Date: ok.Date, Instrument: "voo", ValueMXN: decimal.RequireFromString("1e12")},
		"rounds to cap":    {Date: ok.Date, Instrument: "voo", ValueMXN: decimal.RequireFromString("999999999999.995")},
		"absurd exponent":  {Date: ok.Date, Instrument: "voo", ValueMXN: decimal.RequireFromString("1e999999999")},
		"tiny exponent":    {Date: ok.Date, Instrument: "voo", ValueMXN: decimal.RequireFromString("1e-999999999")},
	}
	for name, v := range bad {
		if err := domain.ValidateValuation(v); !errors.Is(err, domain.ErrInvalidValuation) {
			t.Errorf("%s: err = %v, want ErrInvalidValuation", name, err)
		}
	}
}
