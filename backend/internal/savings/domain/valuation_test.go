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
	bad := map[string]ledger.Valuation{
		"bad date":         {Date: "2026-13-01", Instrument: "voo", ValueMXN: ok.ValueMXN},
		"empty date":       {Instrument: "voo", ValueMXN: ok.ValueMXN},
		"empty instrument": {Date: ok.Date, Instrument: "  ", ValueMXN: ok.ValueMXN},
		"zero value":       {Date: ok.Date, Instrument: "voo", ValueMXN: decimal.Zero},
		"negative value":   {Date: ok.Date, Instrument: "voo", ValueMXN: decimal.RequireFromString("-1")},
	}
	for name, v := range bad {
		if err := domain.ValidateValuation(v); !errors.Is(err, domain.ErrInvalidValuation) {
			t.Errorf("%s: err = %v, want ErrInvalidValuation", name, err)
		}
	}
}
