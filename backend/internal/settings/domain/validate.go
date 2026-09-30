package domain

import (
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
)

// ErrNotFound is returned by repositories when a record does not exist.
var ErrNotFound = errors.New("not found")

// ErrInvalid is wrapped by the section validators; the message says what is wrong.
var ErrInvalid = errors.New("invalid settings")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// validRate reports whether d is a rate between 0 and 1. The range guard comes
// first: comparing a huge-exponent decimal would allocate enormous numbers.
func validRate(d decimal.Decimal) bool {
	return ledger.CheckAmount(d) == nil && !d.IsNegative() && d.LessThanOrEqual(decimal.NewFromInt(1))
}

// checkMoney rejects an amount outside the stored range (see ledger.CheckAmount).
func checkMoney(name string, v decimal.Decimal) error {
	if err := ledger.CheckAmount(v); err != nil {
		return invalid("%s is out of range, it must be below %s", name, ledger.MaxAmount)
	}
	return nil
}

// ValidateMonth requires the YYYY-MM format.
func ValidateMonth(month string) error {
	if !ledger.IsMonth(month) {
		return invalid("month %q must be YYYY-MM", month)
	}
	return nil
}

// Validate checks the general section.
func (g General) Validate() error {
	for name, v := range map[string]decimal.Decimal{
		"salary_usd":                g.SalaryUSD,
		"fx_rate_applied":           g.FXRateApplied,
		"emergency_months":          g.EmergencyMonths,
		"extra_income_estimate_mxn": g.ExtraIncomeEstimateMXN,
	} {
		if err := checkMoney(name, v); err != nil {
			return err
		}
		if v.IsNegative() {
			return invalid("%s must not be negative", name)
		}
	}
	if !g.FXRateApplied.IsPositive() {
		return invalid("fx_rate_applied must be positive")
	}
	if !validRate(g.MorseFeeRate) {
		return invalid("morse_fee_rate must be between 0 and 1")
	}
	for k, v := range g.ExtraIncomeSplit {
		if !validRate(v) {
			return invalid("extra_income_split.%s must be between 0 and 1", k)
		}
	}
	seen := map[string]bool{}
	for _, w := range g.InvestmentAllocation {
		if w.Key == "" {
			return invalid("investment_allocation has an empty instrument")
		}
		if seen[w.Key] {
			return invalid("investment_allocation repeats %q", w.Key)
		}
		seen[w.Key] = true
		if !validRate(w.Value) {
			return invalid("investment_allocation.%s must be between 0 and 1", w.Key)
		}
	}
	return nil
}

// ValidateCategories checks names (non-empty, unique), kinds and budgets.
func ValidateCategories(cats []Category) error {
	seen := map[string]bool{}
	for _, c := range cats {
		if strings.TrimSpace(c.Name) == "" {
			return invalid("category name is required")
		}
		if seen[c.Name] {
			return invalid("category %q is repeated", c.Name)
		}
		seen[c.Name] = true
		switch c.Kind {
		case ledger.KindIncome, ledger.KindExpense, ledger.KindSavings:
		default:
			return invalid("category %q has unknown kind %q", c.Name, c.Kind)
		}
		if c.Budget != nil {
			if err := checkMoney("category budget", *c.Budget); err != nil {
				return err
			}
			if c.Budget.IsNegative() {
				return invalid("category %q budget must not be negative", c.Name)
			}
		}
	}
	return nil
}

// ValidateClients checks IDs (non-empty, unique), currency and rates.
func ValidateClients(clients []Client) error {
	seen := map[string]bool{}
	for _, c := range clients {
		if strings.TrimSpace(c.ID) == "" {
			return invalid("client id is required")
		}
		if seen[c.ID] {
			return invalid("client %q is repeated", c.ID)
		}
		seen[c.ID] = true
		if strings.TrimSpace(c.Name) == "" {
			return invalid("client %q needs a name", c.ID)
		}
		if c.Currency != ledger.CurrencyUSD && c.Currency != ledger.CurrencyMXN {
			return invalid("client %q has unknown currency %q", c.ID, c.Currency)
		}
		for name, r := range map[string]decimal.Decimal{
			"iva_rate": c.IVARate, "ret_isr_rate": c.RetISRRate, "ret_iva_rate": c.RetIVARate,
		} {
			if !validRate(r) {
				return invalid("client %q %s must be between 0 and 1", c.ID, name)
			}
		}
	}
	return nil
}

// ValidateInstruments checks instrument IDs and that every category default
// points to an existing instrument.
func ValidateInstruments(instruments []Instrument, byCategory map[string]string) error {
	ids := map[string]bool{}
	for _, in := range instruments {
		if strings.TrimSpace(in.ID) == "" {
			return invalid("instrument id is required")
		}
		if ids[in.ID] {
			return invalid("instrument %q is repeated", in.ID)
		}
		ids[in.ID] = true
		if strings.TrimSpace(in.Name) == "" {
			return invalid("instrument %q needs a name", in.ID)
		}
	}
	for cat, id := range byCategory {
		if !ids[id] {
			return invalid("default instrument %q for %q does not exist", id, cat)
		}
	}
	return nil
}

// ValidateBrackets requires at least one bracket, strictly increasing upper
// bounds and rates between 0 and 1.
func ValidateBrackets(brackets []Bracket) error {
	if len(brackets) == 0 {
		return invalid("at least one bracket is required")
	}
	prev := decimal.Zero
	for i, b := range brackets {
		if err := checkMoney("bracket upper bound", b.Upper); err != nil {
			return err
		}
		if !b.Upper.GreaterThan(prev) {
			return invalid("bracket %d upper bound must be greater than the previous one", i+1)
		}
		if !validRate(b.Rate) {
			return invalid("bracket %d rate must be between 0 and 1", i+1)
		}
		prev = b.Upper
	}
	return nil
}

// ValidatePaymentMethods requires non-empty, unique names.
func ValidatePaymentMethods(methods []string) error {
	seen := map[string]bool{}
	for _, m := range methods {
		if strings.TrimSpace(m) == "" {
			return invalid("payment method name is required")
		}
		if seen[m] {
			return invalid("payment method %q is repeated", m)
		}
		seen[m] = true
	}
	return nil
}

// Validate checks the issuer: an RFC and a name are required.
func (i Issuer) Validate() error {
	if strings.TrimSpace(i.RFC) == "" || strings.TrimSpace(i.Name) == "" {
		return invalid("issuer rfc and name are required")
	}
	return nil
}

// Validate checks the pause plan: valid, strictly ascending months, a resume
// month after the last paused month and a non-negative plan entry for every
// paused month (and no entry for any other).
func (p PausePlan) Validate() error {
	if len(p.Months) == 0 {
		return invalid("pause needs at least one month")
	}
	prev := ""
	for _, m := range p.Months {
		if !ledger.IsMonth(m) {
			return invalid("pause month %q must be YYYY-MM", m)
		}
		if m <= prev {
			return invalid("pause months must be strictly ascending")
		}
		prev = m
	}
	if !ledger.IsMonth(p.ResumeMonth) {
		return invalid("resume month %q must be YYYY-MM", p.ResumeMonth)
	}
	if p.ResumeMonth <= prev {
		return invalid("resume month must be after the last paused month")
	}
	if err := checkMoney("normal budget", p.NormalBudget); err != nil {
		return err
	}
	if p.NormalBudget.IsNegative() {
		return invalid("normal budget must not be negative")
	}
	for _, m := range p.Months {
		amount, ok := p.FutureExpensesPlan[m]
		if !ok {
			return invalid("plan is missing paused month %s", m)
		}
		if err := checkMoney("plan amount", amount); err != nil {
			return err
		}
		if amount.IsNegative() {
			return invalid("plan amount of %s must not be negative", m)
		}
	}
	for m := range p.FutureExpensesPlan {
		if !p.IsPaused(m) {
			return invalid("plan has month %s which is not paused", m)
		}
	}
	return nil
}
