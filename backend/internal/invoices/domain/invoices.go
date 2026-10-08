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

// MaxAmount is the exclusive upper bound of any stored invoice amount (the
// NUMERIC(14,2) columns hold up to 999,999,999,999.99).
var MaxAmount = ledger.MaxAmount

// Errors returned by the invoice rules.
var (
	ErrUnknownClient = errors.New("unknown client")
	ErrInvalidInput  = errors.New("invalid invoice input")
	ErrCancelled     = errors.New("invoice is cancelled")
)

// Rounded returns the amounts rounded to cents, the precision that is stored
// (as for movements, amount_mxn is rounded to cents). When the invoice carries
// IVA, the rounded IVA is kept as computed and only corrected when
// subtotal + IVA - withholdings would miss the total, so the invariant holds to
// the cent.
func (a Amounts) Rounded() Amounts {
	a.Subtotal = a.Subtotal.Round(2)
	a.SubtotalMXN = a.SubtotalMXN.Round(2)
	a.Total = a.Total.Round(2)
	a.ExpectedDepositMXN = a.ExpectedDepositMXN.Round(2)
	a.ISRWithheld = a.ISRWithheld.Round(2)
	a.IVAWithheld = a.IVAWithheld.Round(2)
	a.IVA = a.IVA.Round(2)
	if a.IVA.IsZero() {
		return a
	}
	if !a.Subtotal.Add(a.IVA).Sub(a.ISRWithheld).Sub(a.IVAWithheld).Equal(a.Total) {
		a.IVA = a.Total.Sub(a.Subtotal).Add(a.ISRWithheld).Add(a.IVAWithheld)
	}
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

// maxSubtotalAdjust bounds the search for a subtotal whose rounded taxes add up
// to the net amount exactly, in cents on each side of the first estimate.
const maxSubtotalAdjust = 5

// ComputeClientInvoice computes the invoice of a non-USA client from the net
// amount received: the CFDI total after IVA is added and the retentions are
// taken off. With rates iva, retISR and retIVA the subtotal is
// net / (1 + iva - retISR - retIVA) and each tax is the subtotal times its
// rate, all rounded to cents. When the rounded taxes miss the net amount by a
// cent the subtotal moves by cents until subtotal + IVA - retentions equals the
// net exactly; if no subtotal does, the remainder goes to the IVA. The result
// is already in cents. With all rates at zero the subtotal is the net amount.
// The caller must make sure 1 + iva - retISR - retIVA is positive; otherwise
// ok is false.
func ComputeClientInvoice(net, ivaRate, retISRRate, retIVARate decimal.Decimal) (a Amounts, ok bool) {
	factor := decimal.NewFromInt(1).Add(ivaRate).Sub(retISRRate).Sub(retIVARate)
	if !factor.IsPositive() {
		return Amounts{}, false
	}
	net = net.Round(2)
	taxes := func(subtotal decimal.Decimal) (iva, isr, retIVA decimal.Decimal) {
		return subtotal.Mul(ivaRate).Round(2), subtotal.Mul(retISRRate).Round(2), subtotal.Mul(retIVARate).Round(2)
	}
	build := func(subtotal, iva, isr, retIVA decimal.Decimal) Amounts {
		return Amounts{
			Subtotal: subtotal, SubtotalMXN: subtotal, IVA: iva, ISRWithheld: isr, IVAWithheld: retIVA,
			Total: net, ExpectedDepositMXN: net,
		}
	}
	first := net.DivRound(factor, ledger.DivisionPrecision).Round(2)
	cent := decimal.New(1, -2)
	for step := int64(0); step <= maxSubtotalAdjust; step++ {
		for _, sign := range []int64{1, -1} {
			if step == 0 && sign == -1 {
				continue
			}
			subtotal := first.Add(cent.Mul(decimal.NewFromInt(step * sign)))
			iva, isr, retIVA := taxes(subtotal)
			if subtotal.Add(iva).Sub(isr).Sub(retIVA).Equal(net) {
				return build(subtotal, iva, isr, retIVA), true
			}
		}
	}
	_, isr, retIVA := taxes(first)
	return build(first, net.Sub(first).Add(isr).Add(retIVA), isr, retIVA), true
}

// PrepareInput is the user input to prepare an invoice. USA clients take
// Subtotal (USD) and ExchangeRate; every other client takes Amount, the net
// amount received (IVA included, retentions already taken off).
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

	// Range before any comparison or arithmetic on the user decimals (a huge
	// exponent would make them allocate enormous numbers); see ledger.CheckAmount.
	for name, v := range map[string]*decimal.Decimal{"subtotal": in.Subtotal, "amount": in.Amount, "exchange rate": in.ExchangeRate} {
		if v == nil {
			continue
		}
		if err := ledger.CheckAmount(*v); err != nil {
			return Prepared{}, fmt.Errorf("%w: %s is out of range, it must be below %s", ErrInvalidInput, name, MaxAmount)
		}
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
			return Prepared{}, fmt.Errorf("%w: client %s takes the net amount received, not a subtotal", ErrInvalidInput, in.ClientID)
		}
		if in.Amount == nil {
			return Prepared{}, fmt.Errorf("%w: client %s requires the net amount received", ErrInvalidInput, in.ClientID)
		}
		if !in.Amount.IsPositive() {
			return Prepared{}, fmt.Errorf("%w: amount must be greater than zero", ErrInvalidInput)
		}
		inv.Currency = ledger.CurrencyMXN
		amounts, ok := ComputeClientInvoice(*in.Amount, client.IVARate, client.RetISRRate, client.RetIVARate)
		if !ok {
			return Prepared{}, fmt.Errorf("%w: the IVA and retention rates of client %s leave no taxable base", ErrInvalidInput, in.ClientID)
		}
		inv.Amounts = amounts
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
