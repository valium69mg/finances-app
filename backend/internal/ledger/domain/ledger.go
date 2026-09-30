// Package domain holds the pure ledger model: movements, valuations and the
// helpers every other module uses to slice and sum them.
package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/shopspring/decimal"
	"golang.org/x/text/unicode/norm"
)

// DivisionPrecision is the number of decimals kept by every division whose
// result is not exact (invoice IVA split, percentages, deviations).
const DivisionPrecision int32 = 10

// Kind is the type of a movement or category.
type Kind string

const (
	KindIncome  Kind = "Ingreso"
	KindExpense Kind = "Gasto"
	KindSavings Kind = "Ahorro"
)

// Currency codes handled by the app.
const (
	CurrencyUSD = "USD"
	CurrencyMXN = "MXN"
)

// Category names the domain logic depends on. They match the seeded configuration.
const (
	CategoryTaxes             = "Impuestos"
	CategoryFees              = "Comisiones"
	CategoryExtraContract     = "Contrato extra"
	CategoryEmergencyFund     = "Fondo de emergencia"
	CategoryInvestments       = "Inversiones"
	CategoryAguinaldoVacation = "Aguinaldo y vacaciones"
	CategoryFutureExpenses    = "Gastos futuros"
	CategorySATReserve        = "Reserva SAT"
)

// Movement is one income, expense or savings entry. AmountMXN is stored, never
// recomputed from Amount and ExchangeRate.
type Movement struct {
	ID            int
	Date          string // YYYY-MM-DD
	Description   string
	Category      string
	Instrument    string
	Kind          Kind
	PaymentMethod string
	Currency      string
	Amount        decimal.Decimal
	ExchangeRate  *decimal.Decimal
	AmountMXN     decimal.Decimal
	// TransferID (a UUID) links the two legs of a savings transfer; it is empty
	// for every other movement.
	TransferID string
}

// Valuation is a manually recorded value of an instrument on a date.
type Valuation struct {
	Date       string // YYYY-MM-DD
	Instrument string
	ValueMXN   decimal.Decimal
	Note       string
}

// Filter selects movements for SumBy. An empty field matches everything.
type Filter struct {
	Kind     Kind
	Category string
}

// MonthOf returns the YYYY-MM prefix of a YYYY-MM-DD date.
func MonthOf(date string) string {
	if len(date) < 7 {
		return date
	}
	return date[:7]
}

// ParseDate validates a YYYY-MM-DD date and returns it unchanged.
func ParseDate(s string) (string, error) {
	if _, err := time.Parse("2006-01-02", s); err != nil {
		return "", fmt.Errorf("invalid date %q, use YYYY-MM-DD", s)
	}
	return s, nil
}

var monthPattern = regexp.MustCompile(`^[0-9]{4}-(0[1-9]|1[0-2])$`)

// IsMonth reports whether s is a YYYY-MM month (month 01 to 12, zero padded).
// It is the single definition of the month/period format: every module that
// validates a period wraps it in its own invalid-input error, and the SQL CHECK
// constraints stay as the last line of defence.
func IsMonth(s string) bool { return monthPattern.MatchString(s) }

// CurrentMonth returns the YYYY-MM of the given instant.
func CurrentMonth(now time.Time) string {
	return now.Format("2006-01")
}

// PreviousMonth returns the month before a YYYY-MM month (January rolls to the previous December).
func PreviousMonth(month string) (string, error) {
	t, err := time.Parse("2006-01", month)
	if err != nil {
		return "", fmt.Errorf("invalid month %q, use YYYY-MM", month)
	}
	return t.AddDate(0, -1, 0).Format("2006-01"), nil
}

// NextMonth returns the month after a YYYY-MM month (December rolls to the next January).
func NextMonth(month string) (string, error) {
	t, err := time.Parse("2006-01", month)
	if err != nil {
		return "", fmt.Errorf("invalid month %q, use YYYY-MM", month)
	}
	return t.AddDate(0, 1, 0).Format("2006-01"), nil
}

// Normalize lowercases, trims and strips accents so text comparisons ignore them.
func Normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range norm.NFKD.String(s) {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// MonthMovements returns the movements dated in the given YYYY-MM month, in order.
func MonthMovements(movements []Movement, month string) []Movement {
	var out []Movement
	for _, m := range movements {
		if MonthOf(m.Date) == month {
			out = append(out, m)
		}
	}
	return out
}

// SumBy sums AmountMXN over the movements matching the filter (negative
// savings withdrawals subtract).
func SumBy(movements []Movement, f Filter) decimal.Decimal {
	total := decimal.Zero
	for _, m := range movements {
		if f.Kind != "" && m.Kind != f.Kind {
			continue
		}
		if f.Category != "" && m.Category != f.Category {
			continue
		}
		total = total.Add(m.AmountMXN)
	}
	return total
}
