// Package domain holds the pure RESICO tax filing rules: the monthly
// declaration, due dates, pending periods and the SAT reserve withdrawal that
// registering a filing produces.
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

// ErrAlreadyFiled is returned when a period already has a registered filing.
var ErrAlreadyFiled = errors.New("period already filed")

// ErrNegativePayment is returned when a filing reports a negative ISR or IVA payment.
var ErrNegativePayment = errors.New("payment amount cannot be negative")

const paymentMethodTransfer = "Transferencia"

// DueDate returns the filing deadline of a YYYY-MM period: the 17th of the
// following month (December rolls to January). There is no holiday calendar.
func DueDate(period string) (string, error) {
	next, err := ledger.NextMonth(period)
	if err != nil {
		return "", err
	}
	return next + "-17", nil
}

// Declaration is the computed monthly declaration of a period. IVAPayable is
// negative when the balance is in the taxpayer's favor; it is never clamped.
type Declaration struct {
	Period          string
	Invoices        []invoices.Invoice
	IncomeCollected decimal.Decimal
	ISRRate         decimal.Decimal
	ISRAccrued      decimal.Decimal
	ISRWithheld     decimal.Decimal
	ISRToPay        decimal.Decimal
	IVATransferred  decimal.Decimal
	IVAWithheld     decimal.Decimal
	IVACreditable   decimal.Decimal
	IVAPayable      decimal.Decimal
	// ExportBase is the income taxed at 0% (client USA, export of services).
	ExportBase decimal.Decimal
	DueDate    string
}

func activeInvoices(all []invoices.Invoice, period string, clientID string) []invoices.Invoice {
	var out []invoices.Invoice
	for _, inv := range all {
		if inv.Period != period || !inv.Status.IsActive() {
			continue
		}
		if clientID != "" && inv.ClientID != clientID {
			continue
		}
		out = append(out, inv)
	}
	return out
}

// ComputeDeclaration computes the monthly RESICO declaration from the active
// (prepared or issued) invoices of the period.
//
// Like the original CLI, IVA and retentions of USD invoices are converted
// with the invoice exchange rate while SubtotalMXN is summed as is, and the
// export base is the subtotal of client "usa".
func ComputeDeclaration(cfg settings.Config, all []invoices.Invoice, period string, ivaCreditable decimal.Decimal) (Declaration, error) {
	due, err := DueDate(period)
	if err != nil {
		return Declaration{}, err
	}
	included := activeInvoices(all, period, "")

	incomeCollected := decimal.Zero
	isrWithheld, ivaTransferred, ivaWithheld := decimal.Zero, decimal.Zero, decimal.Zero
	exportBase := decimal.Zero
	for _, inv := range included {
		incomeCollected = incomeCollected.Add(inv.SubtotalMXN)
		isrWithheld = isrWithheld.Add(income.ToMXN(&inv.ISRWithheld, inv.Currency, inv.ExchangeRate))
		ivaTransferred = ivaTransferred.Add(income.ToMXN(&inv.IVA, inv.Currency, inv.ExchangeRate))
		ivaWithheld = ivaWithheld.Add(income.ToMXN(&inv.IVAWithheld, inv.Currency, inv.ExchangeRate))
		if inv.ClientID == settings.ClientUSA {
			exportBase = exportBase.Add(inv.SubtotalMXN)
		}
	}

	rate, err := cfg.ResicoRateOrFirst(incomeCollected)
	if err != nil {
		return Declaration{}, err
	}
	accrued := incomeCollected.Mul(rate)

	return Declaration{
		Period:          period,
		Invoices:        included,
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

// AOnlyISR returns the ISR that client USA alone would owe for the period, at
// its own bracket (client A has IVA 0).
func AOnlyISR(cfg settings.Config, all []invoices.Invoice, period string) (decimal.Decimal, error) {
	incomeA := decimal.Zero
	for _, inv := range activeInvoices(all, period, settings.ClientUSA) {
		incomeA = incomeA.Add(inv.SubtotalMXN)
	}
	rate, err := cfg.ResicoRateOrFirst(incomeA)
	if err != nil {
		return decimal.Zero, err
	}
	return incomeA.Mul(rate), nil
}

// Filing is the record of a declaration that was filed with the SAT.
type Filing struct {
	Period          string
	FilingDate      string
	IncomeCollected decimal.Decimal
	ISRRate         decimal.Decimal
	ISRAccrued      decimal.Decimal
	ISRWithheld     decimal.Decimal
	ISRPaid         decimal.Decimal
	IVATransferred  decimal.Decimal
	IVAWithheld     decimal.Decimal
	IVACreditable   decimal.Decimal
	IVAPaid         decimal.Decimal
	Folio           string
	InvoiceIDs      []int
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

// PendingPeriod is a period with active invoices and no registered filing.
type PendingPeriod struct {
	Period  string
	DueDate string
	Overdue bool
}

// PendingPeriods lists, in ascending order, the periods that have active
// invoices but no filing. A period is overdue when today (YYYY-MM-DD) is
// later than its due date.
func PendingPeriods(all []invoices.Invoice, filings []Filing, today string) ([]PendingPeriod, error) {
	filed := map[string]bool{}
	for _, f := range filings {
		filed[f.Period] = true
	}
	seen := map[string]bool{}
	var periods []string
	for _, inv := range all {
		if inv.Status.IsActive() && !filed[inv.Period] && !seen[inv.Period] {
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

// FilingInput is what the user reports when registering a filed declaration.
type FilingInput struct {
	Period  string
	Date    string // YYYY-MM-DD
	ISRPaid decimal.Decimal
	IVAPaid decimal.Decimal
	Folio   string
}

// Registration is the outcome of registering a filing: the filing record, the
// invoices marked as declared, the movements to record and the SAT reserve
// accounting behind them.
type Registration struct {
	Filing       Filing
	Invoices     []invoices.Invoice
	NewMovements []ledger.Movement

	TotalPaid decimal.Decimal
	// AOnlyEstimate is the ISR client A alone would owe; the rest of the
	// payment (BPortionNeeded) is attributable to client B.
	AOnlyEstimate  decimal.Decimal
	BPortionNeeded decimal.Decimal
	// ReserveWithdrawal is taken from the SAT reserve to cover BPortionNeeded,
	// limited by the reserve balance.
	ReserveWithdrawal decimal.Decimal
	// BudgetCovered is the part of the payment covered by the regular
	// Impuestos budget (TotalPaid minus ReserveWithdrawal).
	BudgetCovered decimal.Decimal
	// Shortfall is the part of BPortionNeeded the reserve could not cover.
	Shortfall        decimal.Decimal
	RemainingReserve decimal.Decimal
}

// RegisterFiling registers a filed declaration. It fails when the period is
// already filed or when the ISR or IVA payment is negative. The payment becomes an Impuestos expense; if client B's
// share of it can be covered from the SAT reserve, a negative Reserva SAT
// savings movement records the withdrawal. Movement IDs are left for the
// caller to assign.
func RegisterFiling(cfg settings.Config, all []invoices.Invoice, movements []ledger.Movement, filings []Filing, in FilingInput) (Registration, error) {
	if _, filed := FindFiling(filings, in.Period); filed {
		return Registration{}, fmt.Errorf("%w: %s", ErrAlreadyFiled, in.Period)
	}
	if in.ISRPaid.IsNegative() || in.IVAPaid.IsNegative() {
		return Registration{}, ErrNegativePayment
	}
	if _, err := ledger.ParseDate(in.Date); err != nil {
		return Registration{}, err
	}

	decl, err := ComputeDeclaration(cfg, all, in.Period, decimal.Zero)
	if err != nil {
		return Registration{}, err
	}
	ids := make([]int, len(decl.Invoices))
	for i, inv := range decl.Invoices {
		ids[i] = inv.ID
	}
	filing := Filing{
		Period:          in.Period,
		FilingDate:      in.Date,
		IncomeCollected: decl.IncomeCollected,
		ISRRate:         decl.ISRRate,
		ISRAccrued:      decl.ISRAccrued,
		ISRWithheld:     decl.ISRWithheld,
		ISRPaid:         in.ISRPaid,
		IVATransferred:  decl.IVATransferred,
		IVAWithheld:     decl.IVAWithheld,
		IVACreditable:   decimal.Zero,
		IVAPaid:         in.IVAPaid,
		Folio:           in.Folio,
		InvoiceIDs:      ids,
	}

	declared := invoices.MarkDeclared(all, in.Period)

	totalPaid := in.ISRPaid.Add(in.IVAPaid)
	aOnly, err := AOnlyISR(cfg, declared, in.Period)
	if err != nil {
		return Registration{}, err
	}
	bPortion := decimal.Max(decimal.Zero, totalPaid.Sub(aOnly))
	reserveBalance := ledger.SumBy(movements, ledger.Filter{Kind: ledger.KindSavings, Category: ledger.CategorySATReserve})
	withdrawal := decimal.Min(bPortion, decimal.Max(decimal.Zero, reserveBalance))

	newMovements := []ledger.Movement{{
		Date:          in.Date,
		Description:   "Pago SAT ISR+IVA periodo " + in.Period,
		Category:      ledger.CategoryTaxes,
		Kind:          ledger.KindExpense,
		PaymentMethod: paymentMethodTransfer,
		Currency:      ledger.CurrencyMXN,
		Amount:        totalPaid,
		AmountMXN:     totalPaid,
	}}
	if withdrawal.IsPositive() {
		newMovements = append(newMovements, ledger.Movement{
			Date:          in.Date,
			Description:   "Uso de reserva SAT periodo " + in.Period,
			Category:      ledger.CategorySATReserve,
			Kind:          ledger.KindSavings,
			PaymentMethod: paymentMethodTransfer,
			Currency:      ledger.CurrencyMXN,
			Amount:        withdrawal.Neg(),
			AmountMXN:     withdrawal.Neg(),
		})
	}

	return Registration{
		Filing:            filing,
		Invoices:          declared,
		NewMovements:      newMovements,
		TotalPaid:         totalPaid,
		AOnlyEstimate:     aOnly,
		BPortionNeeded:    bPortion,
		ReserveWithdrawal: withdrawal,
		BudgetCovered:     totalPaid.Sub(withdrawal),
		Shortfall:         decimal.Max(decimal.Zero, bPortion.Sub(withdrawal)),
		RemainingReserve:  reserveBalance.Sub(withdrawal),
	}, nil
}
