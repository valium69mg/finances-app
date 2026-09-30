package domain

import (
	"errors"

	"github.com/shopspring/decimal"
)

// ErrAmountOutOfRange is returned by CheckAmount. Callers wrap it in their own
// invalid-input error so it maps to their 400 code.
var ErrAmountOutOfRange = errors.New("amount out of range")

// The stored money columns are numeric(14,2): whole cents below MaxAmount. The
// exponent bounds reject absurd inputs such as "1e999999999" before any
// arithmetic could try to scale them: shopspring accepts any int32 exponent
// and compares, rounds and adds by rescaling to a common exponent, which for
// such a value allocates enormous numbers (or overflows).
const (
	maxAmountExponent = 12
	minAmountExponent = -32
)

// MaxAmount is the exclusive upper bound of any stored amount (numeric(14,2)
// holds up to 999,999,999,999.99).
var MaxAmount = decimal.New(1, 12)

// CheckAmount reports ErrAmountOutOfRange unless the magnitude of v is below
// MaxAmount and its exponent is sane. The sign is not checked: callers decide
// whether zero or negative values are allowed. It must run BEFORE any
// comparison, rounding or arithmetic on an untrusted decimal, because the
// exponent check is what keeps those operations cheap.
func CheckAmount(v decimal.Decimal) error {
	if exp := v.Exponent(); exp > maxAmountExponent || exp < minAmountExponent {
		return ErrAmountOutOfRange
	}
	if v.Abs().Cmp(MaxAmount) >= 0 {
		return ErrAmountOutOfRange
	}
	return nil
}
