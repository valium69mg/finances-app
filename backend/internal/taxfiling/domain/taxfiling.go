// Package domain holds the pure RESICO tax filing rules: the monthly
// declaration computed from the issued invoices of a period, its due date, the
// filing record with its payment state and the periods still to file.
package domain

import (
	"errors"
	"fmt"
	"sort"

	"github.com/shopspring/decimal"

	income "github.com/valium69mg/finances-app/backend/internal/income/domain"
	invoices "github.com/valium69mg/finances-app/backend/internal/invoices/domain"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

// Errors returned by the tax filing rules.
var (
	ErrInvalidInput = errors.New("invalid tax filing input")
	// ErrAlreadyFiled is returned when a period already has a registered filing.
	ErrAlreadyFiled = errors.New("period already filed")
	// ErrAlreadyPaid is returned when the payment of a filing is already recorded.
	ErrAlreadyPaid = errors.New("filing payment already recorded")
	// ErrFilingPaid is returned when a paid filing would be deleted.
	ErrFilingPaid = errors.New("a paid filing cannot be deleted")
	// ErrNotFound is returned when a period has no filing.
	ErrNotFound = errors.New("filing not found")
	// ErrInvoicesChanged is returned when the invoices of the period changed
	// between the computation and the registration.
	ErrInvoicesChanged = errors.New("the invoices of the period changed, compute the declaration again")
)

// ValidatePeriod reports ErrInvalidInput unless period is a YYYY-MM month
// (the format is defined once, by ledger.IsMonth).
func ValidatePeriod(period string) error {
	if !ledger.IsMonth(period) {
		return fmt.Errorf("%w: invalid period %q, use YYYY-MM", ErrInvalidInput, period)
	}
	return nil
}

// checkMoney is the range guard of every user decimal of this module. It runs
// before any comparison or rounding of the value (see ledger.CheckAmount).
func checkMoney(v decimal.Decimal) error { return ledger.CheckAmount(v) }

// DueDate returns the filing deadline of a YYYY-MM period: the 17th of the
// following month (December rolls to January), as the original CLI does. There
// is no holiday calendar, so the SAT may move it to the next business day.
func DueDate(period string) (string, error) {
	next, err := ledger.NextMonth(period)
	if err != nil {
		return "", err
	}
	return next + "-17", nil
}

// Declaration is the computed monthly declaration of a period. Every money
// field is rounded to cents. IVAPayable is negative when the balance is in the
// taxpayer's favor; it is never clamped.
type Declaration struct {
	Period string
	// Invoices are the issued (emitida) invoices of the period, by ID ascending.
	Invoices []invoices.Invoice
	// PreparedIDs are the invoices of the period still in state preparada. They
	// are not part of the declaration; the caller warns about them.
	PreparedIDs     []int
	IncomeCollected decimal.Decimal
	ISRRate         decimal.Decimal
	ISRAccrued      decimal.Decimal
	ISRWithheld     decimal.Decimal
	// ISRToPay is the accrued ISR minus the withheld ISR, never below zero.
	ISRToPay       decimal.Decimal
	IVATransferred decimal.Decimal
	IVAWithheld    decimal.Decimal
	IVACreditable  decimal.Decimal
	IVAPayable     decimal.Decimal
	// ExportBase is the income taxed at 0% (client USA, export of services).
	ExportBase decimal.Decimal
	DueDate    string
}

// TotalToPay is the ISR to pay plus the IVA to pay (an IVA balance in favor
// does not reduce the ISR).
func (d Declaration) TotalToPay() decimal.Decimal {
	return d.ISRToPay.Add(decimal.Max(decimal.Zero, d.IVAPayable))
}

func byID(list []invoices.Invoice) {
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
}

// ComputeDeclaration computes the monthly RESICO declaration from the issued
// invoices of the period. Prepared and cancelled invoices are never included;
// the prepared ones are reported in PreparedIDs.
//
// The rate is the RESICO bracket of the income collected (the first bracket
// when there is none). As in the original CLI, IVA and retentions of USD
// invoices are converted with the invoice exchange rate while SubtotalMXN is
// summed as stored, and the export base is the subtotal of client "usa".
func ComputeDeclaration(cfg settings.Config, all []invoices.Invoice, period string, ivaCreditable decimal.Decimal) (Declaration, error) {
	if err := ValidatePeriod(period); err != nil {
		return Declaration{}, err
	}
	if err := checkMoney(ivaCreditable); err != nil || ivaCreditable.IsNegative() {
		return Declaration{}, fmt.Errorf("%w: creditable IVA must be between 0 and %s", ErrInvalidInput, ledger.MaxAmount)
	}
	due, err := DueDate(period)
	if err != nil {
		return Declaration{}, err
	}

	var included []invoices.Invoice
	var prepared []int
	for _, inv := range all {
		if inv.Period != period {
			continue
		}
		switch inv.Status {
		case invoices.StatusIssued:
			included = append(included, inv)
		case invoices.StatusPrepared:
			prepared = append(prepared, inv.ID)
		}
	}
	byID(included)
	sort.Ints(prepared)

	incomeCollected, exportBase := decimal.Zero, decimal.Zero
	isrWithheld, ivaTransferred, ivaWithheld := decimal.Zero, decimal.Zero, decimal.Zero
	for _, inv := range included {
		incomeCollected = incomeCollected.Add(inv.SubtotalMXN)
		isrWithheld = isrWithheld.Add(income.ToMXN(&inv.ISRWithheld, inv.Currency, inv.ExchangeRate))
		ivaTransferred = ivaTransferred.Add(income.ToMXN(&inv.IVA, inv.Currency, inv.ExchangeRate))
		ivaWithheld = ivaWithheld.Add(income.ToMXN(&inv.IVAWithheld, inv.Currency, inv.ExchangeRate))
		if inv.ClientID == settings.ClientUSA {
			exportBase = exportBase.Add(inv.SubtotalMXN)
		}
	}
	incomeCollected, exportBase = incomeCollected.Round(2), exportBase.Round(2)
	isrWithheld, ivaTransferred, ivaWithheld = isrWithheld.Round(2), ivaTransferred.Round(2), ivaWithheld.Round(2)

	rate, err := cfg.ResicoRateOrFirst(incomeCollected)
	if err != nil {
		return Declaration{}, err
	}
	accrued := incomeCollected.Mul(rate).Round(2)

	return Declaration{
		Period:          period,
		Invoices:        included,
		PreparedIDs:     prepared,
		IncomeCollected: incomeCollected,
		ISRRate:         rate,
		ISRAccrued:      accrued,
		ISRWithheld:     isrWithheld,
		ISRToPay:        decimal.Max(decimal.Zero, accrued.Sub(isrWithheld)),
		IVATransferred:  ivaTransferred,
		IVAWithheld:     ivaWithheld,
		IVACreditable:   ivaCreditable,
		IVAPayable:      ivaTransferred.Sub(ivaWithheld).Sub(ivaCreditable),
		ExportBase:      exportBase,
		DueDate:         due,
	}, nil
}

// UnfiledInvoices returns the issued invoices that no filing includes although
// their period was already filed (declaration_period empty): typically an
// invoice issued after the filing was registered. Their income is not part of
// any saved declaration. Ordered by period, then ID, ascending. This app does
// not build a complementary declaration for them; it only surfaces them.
func UnfiledInvoices(all []invoices.Invoice, filings []Filing) []invoices.Invoice {
	filed := map[string]bool{}
	for _, f := range filings {
		filed[f.Period] = true
	}
	var out []invoices.Invoice
	for _, inv := range all {
		if inv.Status == invoices.StatusIssued && inv.DeclarationPeriod == "" && filed[inv.Period] {
			out = append(out, inv)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Period != out[j].Period {
			return out[i].Period < out[j].Period
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// PendingPeriod is a period with issued invoices and no registered filing.
type PendingPeriod struct {
	Period  string
	DueDate string
	Overdue bool
}

// PendingPeriods lists, in ascending order, the periods that have issued
// invoices but no filing. A period is overdue when today (YYYY-MM-DD) is later
// than its due date.
func PendingPeriods(all []invoices.Invoice, filings []Filing, today string) ([]PendingPeriod, error) {
	filed := map[string]bool{}
	for _, f := range filings {
		filed[f.Period] = true
	}
	seen := map[string]bool{}
	var periods []string
	for _, inv := range all {
		if inv.Status == invoices.StatusIssued && !filed[inv.Period] && !seen[inv.Period] {
			seen[inv.Period] = true
			periods = append(periods, inv.Period)
		}
	}
	sort.Strings(periods)

	out := make([]PendingPeriod, 0, len(periods))
	for _, p := range periods {
		due, err := DueDate(p)
		if err != nil {
			return nil, err
		}
		out = append(out, PendingPeriod{Period: p, DueDate: due, Overdue: today > due})
	}
	return out, nil
}
