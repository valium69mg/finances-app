package domain

import (
	"errors"
	"fmt"
	"strings"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
)

// ErrInvalidValuation is wrapped by ValidateValuation; the message says what is wrong.
var ErrInvalidValuation = errors.New("invalid valuation")

// ValidateValuation checks a valuation before it is stored: a valid
// YYYY-MM-DD date, a non-empty instrument and a positive value.
func ValidateValuation(v ledger.Valuation) error {
	if _, err := ledger.ParseDate(v.Date); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidValuation, err)
	}
	if strings.TrimSpace(v.Instrument) == "" {
		return fmt.Errorf("%w: instrument is required", ErrInvalidValuation)
	}
	if !v.ValueMXN.IsPositive() {
		return fmt.Errorf("%w: value must be greater than zero", ErrInvalidValuation)
	}
	return nil
}
