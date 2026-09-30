package domain

import (
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
)

// ErrInvalidValuation is wrapped by ValidateValuation; the message says what is wrong.
var ErrInvalidValuation = errors.New("invalid valuation")

// The valuations table stores value_mxn as numeric(14,2): whole cents, below
// MaxValuation. The exponent bounds reject absurd inputs such as "1e999999999"
// before any arithmetic could try to scale them.
const (
	maxValuationExponent = 12
	minValuationExponent = -32
)

// MaxValuation is the exclusive upper bound of a valuation (numeric(14,2)).
var MaxValuation = decimal.New(1, 12)

// RoundValuation rounds a value to cents, half away from zero, the precision
// the valuations table stores. Callers validate first so the exponent is sane.
func RoundValuation(v decimal.Decimal) decimal.Decimal { return v.Round(2) }

// ValidateValuation checks a valuation before it is stored: a valid
// YYYY-MM-DD date, a non-empty instrument and a value that is at least one
// cent and below MaxValuation once rounded to cents.
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
	if exp := v.ValueMXN.Exponent(); exp > maxValuationExponent || exp < minValuationExponent {
		return fmt.Errorf("%w: value is out of range", ErrInvalidValuation)
	}
	rounded := RoundValuation(v.ValueMXN)
	if !rounded.IsPositive() {
		return fmt.Errorf("%w: value must be at least 0.01", ErrInvalidValuation)
	}
	if rounded.Cmp(MaxValuation) >= 0 {
		return fmt.Errorf("%w: value must be less than 1000000000000", ErrInvalidValuation)
	}
	return nil
}
