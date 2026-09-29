package domain

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
)

// ClientB is the ID of the domestic (public in general, IVA included) client.
const ClientB = "b"

var defaultClientBIVARate = decimal.RequireFromString("0.16")

// TaxBudget is the budget-derived estimate behind the Impuestos and
// Comisiones budgets. Budget planning relies only on the client A salary by
// default; client B's estimated deposit is folded in only when
// BudgetIncludesExtraIncome is set.
type TaxBudget struct {
	IncludeExtraIncome bool
	SalaryEstMXN       decimal.Decimal
	ExtraEstTotal      decimal.Decimal
	ExtraEstSubtotal   decimal.Decimal
	ExtraEstIVA        decimal.Decimal
	TotalEst           decimal.Decimal
	Rate               decimal.Decimal
	ISRToPay           decimal.Decimal
	IVAToPay           decimal.Decimal
	TaxesBudget        decimal.Decimal
	FeesBudget         decimal.Decimal
}

// TaxBudgetBreakdown computes the Impuestos and Comisiones budgets. Divisions
// keep ledger.DivisionPrecision decimals; nothing else is rounded.
func (c Config) TaxBudgetBreakdown() (TaxBudget, error) {
	var missing []string
	if c.SalaryUSD == nil {
		missing = append(missing, "salary_usd")
	}
	if c.FXRateApplied == nil {
		missing = append(missing, "fx_rate_applied")
	}
	if c.MorseFeeRate == nil {
		missing = append(missing, "morse_fee_rate")
	}
	if len(c.Brackets) == 0 {
		missing = append(missing, "resico_brackets")
	}
	if len(missing) > 0 {
		return TaxBudget{}, fmt.Errorf("%w: %s", ErrMissingConfig, strings.Join(missing, ", "))
	}

	salaryEst := c.SalaryUSD.Mul(*c.FXRateApplied)

	extraTotal := decimal.Zero
	if c.BudgetIncludesExtraIncome {
		extraTotal = c.ExtraIncomeEstimateMXN
	}
	ivaRate := defaultClientBIVARate
	if b, ok := c.FindClient(ClientB); ok {
		ivaRate = b.IVARate
	}
	extraSubtotal := decimal.Zero
	if !ivaRate.IsZero() && !extraTotal.IsZero() {
		extraSubtotal = extraTotal.DivRound(decimal.NewFromInt(1).Add(ivaRate), ledger.DivisionPrecision)
	}
	extraIVA := extraTotal.Sub(extraSubtotal)

	totalEst := salaryEst.Add(extraSubtotal)
	rate, err := c.ResicoRate(totalEst)
	if err != nil {
		return TaxBudget{}, err
	}
	isr := totalEst.Mul(rate)

	return TaxBudget{
		IncludeExtraIncome: c.BudgetIncludesExtraIncome,
		SalaryEstMXN:       salaryEst,
		ExtraEstTotal:      extraTotal,
		ExtraEstSubtotal:   extraSubtotal,
		ExtraEstIVA:        extraIVA,
		TotalEst:           totalEst,
		Rate:               rate,
		ISRToPay:           isr,
		IVAToPay:           extraIVA,
		TaxesBudget:        isr.Add(extraIVA),
		FeesBudget:         salaryEst.Mul(*c.MorseFeeRate),
	}, nil
}

// BudgetOverrides returns the computed Impuestos and Comisiones budgets keyed
// by category name, ready to pass to CategoryBudget.
func (c Config) BudgetOverrides() (map[string]decimal.Decimal, error) {
	tb, err := c.TaxBudgetBreakdown()
	if err != nil {
		return nil, err
	}
	return map[string]decimal.Decimal{
		ledger.CategoryTaxes: tb.TaxesBudget,
		ledger.CategoryFees:  tb.FeesBudget,
	}, nil
}
