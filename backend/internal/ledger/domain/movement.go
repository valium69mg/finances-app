package domain

import (
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

// ErrInvalid is wrapped by movement validation; the message says what is wrong.
var ErrInvalid = errors.New("invalid movement")

// ErrNotFound is returned by repositories when a movement does not exist.
var ErrNotFound = errors.New("movement not found")

// DefaultExpensePaymentMethod is the payment method of an expense that names none.
const DefaultExpensePaymentMethod = "Débito"

// DefaultOtherPaymentMethod is the payment method of an income or savings
// entry that names none.
const DefaultOtherPaymentMethod = "Transferencia"

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// CatalogCategory is a category name with its kind.
type CatalogCategory struct {
	Name string
	Kind Kind
}

// Catalog is the slice of the settings that movement validation depends on. It
// keeps the ledger domain independent of the settings module.
type Catalog struct {
	Categories     []CatalogCategory
	PaymentMethods []string
	// DefaultRate is the fx_rate_applied used for USD movements without a rate.
	// Zero means none is configured.
	DefaultRate decimal.Decimal
}

// FindCategory returns the canonical name of the category of the given kind
// matching name (ignoring case and accents).
func (c Catalog) FindCategory(name string, kind Kind) (string, bool) {
	n := Normalize(name)
	for _, cat := range c.Categories {
		if cat.Kind == kind && Normalize(cat.Name) == n {
			return cat.Name, true
		}
	}
	return "", false
}

// CategoryNames lists the category names of a kind in catalog order.
func (c Catalog) CategoryNames(kind Kind) []string {
	var out []string
	for _, cat := range c.Categories {
		if cat.Kind == kind {
			out = append(out, cat.Name)
		}
	}
	return out
}

// MovementInput is the raw data of a new or updated movement. Empty optional
// fields take the defaults of NewMovement.
type MovementInput struct {
	Date          string
	Description   string
	Category      string
	Instrument    string
	Kind          Kind
	PaymentMethod string
	Currency      string
	Amount        decimal.Decimal
	ExchangeRate  *decimal.Decimal
}

// AmountMXN converts an amount to MXN: MXN amounts pass through, USD amounts
// are multiplied by the rate and rounded to cents.
func AmountMXN(amount decimal.Decimal, currency string, rate *decimal.Decimal) decimal.Decimal {
	if currency == CurrencyUSD && rate != nil {
		return amount.Mul(*rate).Round(2)
	}
	return amount
}

// NewMovement validates the input against the catalog, applies the defaults
// (payment method by kind, currency MXN, date today, USD rate from the
// catalog) and computes AmountMXN. today is a YYYY-MM-DD date. The returned
// movement has no ID. Errors wrap ErrInvalid.
func NewMovement(in MovementInput, cat Catalog, today string) (Movement, error) {
	switch in.Kind {
	case KindIncome, KindExpense, KindSavings:
	default:
		return Movement{}, invalid("kind %q must be one of Ingreso, Gasto, Ahorro", in.Kind)
	}

	category, ok := cat.FindCategory(in.Category, in.Kind)
	if !ok {
		return Movement{}, invalid("%q is not a valid %s category, valid: %s",
			in.Category, in.Kind, strings.Join(cat.CategoryNames(in.Kind), ", "))
	}

	method := strings.TrimSpace(in.PaymentMethod)
	if method == "" {
		method = DefaultOtherPaymentMethod
		if in.Kind == KindExpense {
			method = DefaultExpensePaymentMethod
		}
	}
	if !containsString(cat.PaymentMethods, method) {
		return Movement{}, invalid("payment method %q is not valid, valid: %s", method, strings.Join(cat.PaymentMethods, ", "))
	}

	currency := strings.TrimSpace(in.Currency)
	if currency == "" {
		currency = CurrencyMXN
	}
	if currency != CurrencyMXN && currency != CurrencyUSD {
		return Movement{}, invalid("currency %q must be MXN or USD", currency)
	}

	date := strings.TrimSpace(in.Date)
	if date == "" {
		date = today
	}
	date, err := ParseDate(date)
	if err != nil {
		return Movement{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}

	if in.Amount.IsZero() {
		return Movement{}, invalid("amount must not be zero")
	}
	// Only savings may be negative (withdrawals); the movements table enforces
	// the same rule with a CHECK.
	switch {
	case in.Kind == KindExpense && !in.Amount.IsPositive():
		return Movement{}, invalid("amount of an expense must be greater than zero")
	case in.Kind == KindIncome && !in.Amount.IsPositive():
		return Movement{}, invalid("amount of an income must be greater than zero")
	}

	var rate *decimal.Decimal
	if currency == CurrencyUSD {
		r := cat.DefaultRate
		if in.ExchangeRate != nil {
			r = *in.ExchangeRate
		}
		if !r.IsPositive() {
			return Movement{}, invalid("USD movements need a positive exchange rate")
		}
		rate = &r
	}

	return Movement{
		Date:          date,
		Description:   strings.TrimSpace(in.Description),
		Category:      category,
		Instrument:    strings.TrimSpace(in.Instrument),
		Kind:          in.Kind,
		PaymentMethod: method,
		Currency:      currency,
		Amount:        in.Amount,
		ExchangeRate:  rate,
		AmountMXN:     AmountMXN(in.Amount, currency, rate),
	}, nil
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
