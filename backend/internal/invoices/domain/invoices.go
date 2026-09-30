// Package domain holds the pure invoice rules: amount math per client type,
// preparation defaults, duplicate detection and the status lifecycle.
package domain

import (
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

// Status is the lifecycle state of an invoice.
type Status string

const (
	StatusPrepared  Status = "preparada"
	StatusIssued    Status = "emitida"
	StatusCancelled Status = "cancelada"
)

// IsActive reports whether the invoice counts toward the tax declaration
// (prepared or issued; cancelled invoices never do).
func (s Status) IsActive() bool {
	return s == StatusPrepared || s == StatusIssued
}

// MaxAmount is the exclusive upper bound of any stored invoice amount (the
// NUMERIC(14,2) columns hold up to 999,999,999,999.99).
var MaxAmount = decimal.New(1, 12)

// Errors returned by the invoice rules.
var (
	ErrUnknownClient = errors.New("unknown client")
	ErrInvalidInput  = errors.New("invalid invoice input")
	ErrCancelled     = errors.New("invoice is cancelled")
)

// Rounded returns the amounts rounded to cents, the precision that is stored
// (as for movements, amount_mxn is rounded to cents). When the invoice carries
// IVA the IVA is total minus the rounded subtotal, so subtotal + IVA always
// equals the total to the cent.
func (a Amounts) Rounded() Amounts {
	a.Subtotal = a.Subtotal.Round(2)
	a.SubtotalMXN = a.SubtotalMXN.Round(2)
	a.Total = a.Total.Round(2)
	a.ExpectedDepositMXN = a.ExpectedDepositMXN.Round(2)
	a.ISRWithheld = a.ISRWithheld.Round(2)
	a.IVAWithheld = a.IVAWithheld.Round(2)
	if a.IVA.IsZero() {
		return a
	}
	a.IVA = a.Total.Sub(a.Subtotal)
	return a
}

// Validate reports ErrInvalidInput unless the total and subtotals are
// positive and every amount is below MaxAmount. Call it on rounded amounts.
func (a Amounts) Validate() error {
	if !a.Total.IsPositive() || !a.Subtotal.IsPositive() || !a.SubtotalMXN.IsPositive() {
		return fmt.Errorf("%w: amounts must be greater than zero", ErrInvalidInput)
	}
	for _, v := range []decimal.Decimal{a.Total, a.Subtotal, a.SubtotalMXN, a.ExpectedDepositMXN, a.IVA} {
		if v.GreaterThanOrEqual(MaxAmount) {
			return fmt.Errorf("%w: amount is too large", ErrInvalidInput)
		}
	}
	return nil
}

// Amounts are the computed money fields of an invoice. Subtotal and Total are
// in the invoice currency; the other fields are in pesos.
type Amounts struct {
	Subtotal           decimal.Decimal
	SubtotalMXN        decimal.Decimal
	IVA                decimal.Decimal
	ISRWithheld        decimal.Decimal
	IVAWithheld        decimal.Decimal
	Total              decimal.Decimal
	ExpectedDepositMXN decimal.Decimal
}

// Invoice is an invoice with its computed amounts and lifecycle state.
type Invoice struct {
	ID                int
	ClientID          string
	CollectionDate    string // YYYY-MM-DD
	Period            string // YYYY-MM of the collection date
	Currency          string
	ExchangeRate      *decimal.Decimal
	Status            Status
	UUID              string
	MovementID        *int
	DeclarationPeriod string
	CreatedAt         time.Time
	Amounts
}

// ComputeUSAInvoice computes an export-of-services invoice: IVA 0% and no
// retentions. The total stays in USD; the expected deposit is in pesos.
func ComputeUSAInvoice(subtotalUSD, exchangeRate decimal.Decimal) Amounts {
	subtotalMXN := subtotalUSD.Mul(exchangeRate)
	return Amounts{
		Subtotal:           subtotalUSD,
		SubtotalMXN:        subtotalMXN,
		Total:              subtotalUSD,
		ExpectedDepositMXN: subtotalMXN,
	}
}

// ComputeClientBInvoice computes a global public-in-general invoice: the
// amount received includes IVA and there are no retentions. The subtotal is
// total/(1+ivaRate) at ledger.DivisionPrecision decimals.
func ComputeClientBInvoice(total, ivaRate decimal.Decimal) Amounts {
	subtotal := total.DivRound(decimal.NewFromInt(1).Add(ivaRate), ledger.DivisionPrecision)
	return Amounts{
		Subtotal:           subtotal,
		SubtotalMXN:        subtotal,
		IVA:                total.Sub(subtotal),
		Total:              total,
		ExpectedDepositMXN: total,
	}
}

// PrepareInput is the user input to prepare an invoice. USA clients take
// Subtotal (USD) and ExchangeRate; every other client takes Amount, the total
// received with IVA included.
type PrepareInput struct {
	ClientID     string
	Date         string
	Subtotal     *decimal.Decimal
	Amount       *decimal.Decimal
	ExchangeRate *decimal.Decimal
}

// Prepared is the result of preparing an invoice: the new invoice (without
// ID) and the IDs of existing invoices that look like duplicates.
type Prepared struct {
	Invoice    Invoice
	Duplicates []int
}

// Prepare validates the input, applies the defaults (USA subtotal defaults to
// salary_usd, exchange rate to fx_rate_applied; a missing or zero rate counts
// as absent) and computes the amounts. Existing non-cancelled invoices of the
// same client and collection date are reported as possible duplicates but
// never block the preparation.
func Prepare(cfg settings.Config, existing []Invoice, in PrepareInput) (Prepared, error) {
	client, ok := cfg.FindClient(in.ClientID)
	if !ok {
		return Prepared{}, fmt.Errorf("%w: %q", ErrUnknownClient, in.ClientID)
	}
	if _, err := ledger.ParseDate(in.Date); err != nil {
		return Prepared{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	inv := Invoice{
		ClientID:       in.ClientID,
		CollectionDate: in.Date,
		Period:         ledger.MonthOf(in.Date),
		Status:         StatusPrepared,
	}

	if in.ClientID == settings.ClientUSA {
		if in.Amount != nil {
			return Prepared{}, fmt.Errorf("%w: client %s takes a subtotal, not an amount", ErrInvalidInput, in.ClientID)
		}
		subtotal, rate, err := usaDefaults(cfg, in)
		if err != nil {
			return Prepared{}, err
		}
		if !subtotal.IsPositive() {
			return Prepared{}, fmt.Errorf("%w: subtotal must be greater than zero", ErrInvalidInput)
		}
		if rate.IsNegative() {
			return Prepared{}, fmt.Errorf("%w: exchange rate must be greater than zero", ErrInvalidInput)
		}
		inv.Currency = ledger.CurrencyUSD
		inv.ExchangeRate = &rate
		inv.Amounts = ComputeUSAInvoice(subtotal, rate)
	} else {
		if in.Subtotal != nil {
			return Prepared{}, fmt.Errorf("%w: client %s takes the total received (IVA included), not a subtotal", ErrInvalidInput, in.ClientID)
		}
		if in.Amount == nil {
			return Prepared{}, fmt.Errorf("%w: client %s requires the total received (IVA included)", ErrInvalidInput, in.ClientID)
		}
		if !in.Amount.IsPositive() {
			return Prepared{}, fmt.Errorf("%w: amount must be greater than zero", ErrInvalidInput)
		}
		inv.Currency = ledger.CurrencyMXN
		inv.Amounts = ComputeClientBInvoice(*in.Amount, client.IVARate)
	}

	var dups []int
	for _, e := range existing {
		if e.ClientID == in.ClientID && e.CollectionDate == in.Date && e.Status != StatusCancelled {
			dups = append(dups, e.ID)
		}
	}
	return Prepared{Invoice: inv, Duplicates: dups}, nil
}

func usaDefaults(cfg settings.Config, in PrepareInput) (subtotal, rate decimal.Decimal, err error) {
	if in.Subtotal != nil {
		subtotal = *in.Subtotal
	} else {
		if cfg.SalaryUSD == nil {
			return subtotal, rate, fmt.Errorf("%w: salary_usd", settings.ErrMissingConfig)
		}
		subtotal = *cfg.SalaryUSD
	}
	if in.ExchangeRate != nil && !in.ExchangeRate.IsZero() {
		rate = *in.ExchangeRate
	} else {
		if cfg.FXRateApplied == nil {
			return subtotal, rate, fmt.Errorf("%w: fx_rate_applied", settings.ErrMissingConfig)
		}
		rate = *cfg.FXRateApplied
	}
	return subtotal, rate, nil
}

// MarkIssued returns the invoice as issued with its fiscal UUID. A cancelled
// invoice cannot be issued; issuing an already issued one replaces the UUID.
func (i Invoice) MarkIssued(uuid string) (Invoice, error) {
	if i.Status == StatusCancelled {
		return i, fmt.Errorf("%w: #%d", ErrCancelled, i.ID)
	}
	i.Status = StatusIssued
	i.UUID = uuid
	return i, nil
}

// Cancel returns the invoice as cancelled. Like the original CLI it is
// allowed from any state, including issued and already cancelled.
func (i Invoice) Cancel() Invoice {
	i.Status = StatusCancelled
	return i
}

// MarkDeclared returns a copy of the invoices where every active invoice of
// the period records the declaration that included it.
func MarkDeclared(invoices []Invoice, period string) []Invoice {
	out := make([]Invoice, len(invoices))
	copy(out, invoices)
	for i := range out {
		if out[i].Period == period && out[i].Status.IsActive() {
			out[i].DeclarationPeriod = period
		}
	}
	return out
}
