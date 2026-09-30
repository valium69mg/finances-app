package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	invoices "github.com/valium69mg/finances-app/backend/internal/invoices/domain"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

// MaxFolioLength bounds the folio of the SAT acuse.
const MaxFolioLength = 64

// PaymentStatus is the payment state of the filing of a period. The values are
// catalog data in Spanish, like the invoice states.
type PaymentStatus string

const (
	// PaymentNone means the period has no registered filing.
	PaymentNone PaymentStatus = "ninguna"
	// PaymentPending means the filing is registered and its payment is not.
	PaymentPending PaymentStatus = "pendiente"
	// PaymentPaid means the payment of the filing is recorded.
	PaymentPaid PaymentStatus = "pagada"
)

// IsValid reports whether s is a status a filing can have (never PaymentNone).
func (s PaymentStatus) IsValid() bool { return s == PaymentPending || s == PaymentPaid }

// Payment is the recorded payment of a filing to the SAT.
type Payment struct {
	Date    string // YYYY-MM-DD
	ISRPaid decimal.Decimal
	IVAPaid decimal.Decimal
}

// Total is the ISR plus the IVA paid.
func (p Payment) Total() decimal.Decimal { return p.ISRPaid.Add(p.IVAPaid) }

// Filing is the record of a declaration that was filed with the SAT. The
// amounts are the ones computed when it was registered; Payment stays nil
// until the payment is recorded. ExpenseMovementID links the Impuestos expense
// the user chose to record for the payment.
type Filing struct {
	Period          string
	FilingDate      string // YYYY-MM-DD
	IncomeCollected decimal.Decimal
	ISRRate         decimal.Decimal
	ISRAccrued      decimal.Decimal
	ISRWithheld     decimal.Decimal
	// ISRDue is the ISR to pay according to the declaration.
	ISRDue         decimal.Decimal
	IVATransferred decimal.Decimal
	IVAWithheld    decimal.Decimal
	IVACreditable  decimal.Decimal
	// IVADue is the IVA balance of the declaration; negative means in favor.
	IVADue            decimal.Decimal
	Folio             string
	Payment           *Payment
	ExpenseMovementID *int
	// InvoiceIDs are the invoices that carry this period as declaration_period.
	InvoiceIDs []int
	CreatedAt  time.Time
}

// PaymentStatus is pending until the payment is recorded.
func (f Filing) PaymentStatus() PaymentStatus {
	if f.Payment == nil {
		return PaymentPending
	}
	return PaymentPaid
}

// TotalToPay is the ISR due plus the IVA due when it is not in favor.
func (f Filing) TotalToPay() decimal.Decimal {
	return f.ISRDue.Add(decimal.Max(decimal.Zero, f.IVADue))
}

// FindFiling returns the filing of a period, if any.
func FindFiling(filings []Filing, period string) (Filing, bool) {
	for _, f := range filings {
		if f.Period == period {
			return f, true
		}
	}
	return Filing{}, false
}

// PaymentInput is what the user reports when paying a filing.
type PaymentInput struct {
	Date    string // YYYY-MM-DD
	ISRPaid decimal.Decimal
	IVAPaid decimal.Decimal
}

// NewPayment validates the input: a real date and amounts that are not
// negative and fit the stored precision. Amounts are rounded to cents.
func NewPayment(in PaymentInput) (Payment, error) {
	if _, err := ledger.ParseDate(in.Date); err != nil {
		return Payment{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	// Range before any rounding or comparison: see ledger.CheckAmount.
	if checkMoney(in.ISRPaid) != nil || checkMoney(in.IVAPaid) != nil {
		return Payment{}, fmt.Errorf("%w: paid amounts are out of range, they must be below %s", ErrInvalidInput, ledger.MaxAmount)
	}
	p := Payment{Date: in.Date, ISRPaid: in.ISRPaid.Round(2), IVAPaid: in.IVAPaid.Round(2)}
	if p.ISRPaid.IsNegative() || p.IVAPaid.IsNegative() {
		return Payment{}, fmt.Errorf("%w: paid amounts cannot be negative", ErrInvalidInput)
	}
	if p.ISRPaid.GreaterThanOrEqual(ledger.MaxAmount) || p.IVAPaid.GreaterThanOrEqual(ledger.MaxAmount) {
		return Payment{}, fmt.Errorf("%w: amount is too large", ErrInvalidInput)
	}
	return p, nil
}

// FilingInput is what the user reports when registering a filed declaration.
// The declaration figures are computed, never typed. Payment is nil when the
// payment is still pending.
type FilingInput struct {
	Period        string
	Date          string // YYYY-MM-DD
	Folio         string
	IVACreditable decimal.Decimal
	Payment       *PaymentInput
}

// NewFiling registers a filing from the computed declaration of the period. It
// fails with ErrAlreadyFiled when the period has a filing. A period without
// issued invoices can still be filed (a declaration with zeros).
func NewFiling(cfg settings.Config, all []invoices.Invoice, filings []Filing, in FilingInput) (Filing, error) {
	if err := ValidatePeriod(in.Period); err != nil {
		return Filing{}, err
	}
	if _, filed := FindFiling(filings, in.Period); filed {
		return Filing{}, fmt.Errorf("%w: %s", ErrAlreadyFiled, in.Period)
	}
	if _, err := ledger.ParseDate(in.Date); err != nil {
		return Filing{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	folio := strings.TrimSpace(in.Folio)
	if len(folio) > MaxFolioLength {
		return Filing{}, fmt.Errorf("%w: the folio is longer than %d characters", ErrInvalidInput, MaxFolioLength)
	}

	// The creditable IVA is rounded below, so its range is checked first.
	if err := checkMoney(in.IVACreditable); err != nil {
		return Filing{}, fmt.Errorf("%w: creditable IVA must be between 0 and %s", ErrInvalidInput, ledger.MaxAmount)
	}
	decl, err := ComputeDeclaration(cfg, all, in.Period, in.IVACreditable.Round(2))
	if err != nil {
		return Filing{}, err
	}
	ids := make([]int, len(decl.Invoices))
	for i, inv := range decl.Invoices {
		ids[i] = inv.ID
	}
	f := Filing{
		Period:          in.Period,
		FilingDate:      in.Date,
		IncomeCollected: decl.IncomeCollected,
		ISRRate:         decl.ISRRate,
		ISRAccrued:      decl.ISRAccrued,
		ISRWithheld:     decl.ISRWithheld,
		ISRDue:          decl.ISRToPay,
		IVATransferred:  decl.IVATransferred,
		IVAWithheld:     decl.IVAWithheld,
		IVACreditable:   decl.IVACreditable,
		IVADue:          decl.IVAPayable,
		Folio:           folio,
		InvoiceIDs:      ids,
	}
	if in.Payment != nil {
		p, err := NewPayment(*in.Payment)
		if err != nil {
			return Filing{}, err
		}
		f.Payment = &p
	}
	return f, nil
}

// Pay records the payment of a pending filing. A paid filing cannot be paid
// again (ErrAlreadyPaid): the record is append-only once the money moved.
func (f Filing) Pay(in PaymentInput) (Filing, error) {
	if f.Payment != nil {
		return f, fmt.Errorf("%w: %s", ErrAlreadyPaid, f.Period)
	}
	p, err := NewPayment(in)
	if err != nil {
		return f, err
	}
	f.Payment = &p
	return f, nil
}

// PaymentExpenseDescription is the description of the Impuestos expense that
// records the payment of a period.
func PaymentExpenseDescription(period string) string {
	return "Pago SAT ISR+IVA periodo " + period
}

// MonthStatus is what the dashboard shows about the filings around a month:
// the payment state of the filing of that period and whether the previous
// period still needs action.
type MonthStatus struct {
	Payment PaymentStatus
	// PreviousPeriod is the month before the requested one.
	PreviousPeriod string
	// PreviousPending is true when the previous period has issued invoices and
	// no filing, or has a filing whose payment is pending.
	PreviousPending bool
}

// StatusOf computes the MonthStatus of a YYYY-MM month.
func StatusOf(all []invoices.Invoice, filings []Filing, month string) (MonthStatus, error) {
	if err := ValidatePeriod(month); err != nil {
		return MonthStatus{}, err
	}
	prev, err := ledger.PreviousMonth(month)
	if err != nil {
		return MonthStatus{}, err
	}
	st := MonthStatus{Payment: PaymentNone, PreviousPeriod: prev}
	if f, ok := FindFiling(filings, month); ok {
		st.Payment = f.PaymentStatus()
	}
	if f, ok := FindFiling(filings, prev); ok {
		st.PreviousPending = f.PaymentStatus() == PaymentPending
		return st, nil
	}
	for _, inv := range all {
		if inv.Period == prev && inv.Status == invoices.StatusIssued {
			st.PreviousPending = true
			break
		}
	}
	return st, nil
}
