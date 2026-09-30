// Package domain holds the pure settings model (categories, clients,
// instruments, tax parameters) and the lookups built on it.
package domain

import (
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
)

// Client ID used by the foreign (USD, export of services) client.
const ClientUSA = "usa"

// Bracket is one RESICO bracket: incomes up to Upper (inclusive) pay Rate.
type Bracket struct {
	Upper decimal.Decimal
	Rate  decimal.Decimal
}

// Weight is the share of a split assigned to a key (for example an instrument
// ID). Order matters: it breaks ties when distributing rounding remainders.
type Weight struct {
	Key   string
	Value decimal.Decimal
}

// Category is an income, expense or savings category. A nil Budget means no
// budget; a zero Budget is also treated as "no budget" by percentage and
// deviation calculations.
type Category struct {
	Name     string
	Kind     ledger.Kind
	Budget   *decimal.Decimal
	Keywords []string
	// Includes is a free-text note describing what the budget covers.
	Includes string
}

// Client is a billed customer. The invoicing fields feed the CFDI checklist.
type Client struct {
	ID       string
	Name     string
	Currency string
	IVARate  decimal.Decimal

	Type          string
	RFC           string
	Regimen       string
	UsoCFDI       string
	RetISRRate    decimal.Decimal
	RetIVARate    decimal.Decimal
	Concepto      string
	ClaveProdServ string
	ClaveUnidad   string
	Address       string
	TaxResidence  string
	Contract      string
	RealPayer     string
}

// Issuer is the taxpayer that issues the invoices.
type Issuer struct {
	RFC        string
	Name       string
	Regimen    string
	PostalCode string
	Note       string
}

// IsZero reports whether no issuer has been configured.
func (i Issuer) IsZero() bool { return i == Issuer{} }

// General holds the scalar tax and budget parameters plus the extra-income
// split and the investment allocation. Unlike Config, every value is required.
type General struct {
	SalaryUSD                 decimal.Decimal
	FXRateApplied             decimal.Decimal
	MorseFeeRate              decimal.Decimal
	EmergencyMonths           decimal.Decimal
	ExtraIncomeEstimateMXN    decimal.Decimal
	BudgetIncludesExtraIncome bool
	ExtraIncomeSplit          map[string]decimal.Decimal
	InvestmentAllocation      []Weight
}

// General returns the general section of the configuration. Absent required
// values read as zero; use Validate first when that matters.
func (c Config) General() General {
	zero := func(p *decimal.Decimal) decimal.Decimal {
		if p == nil {
			return decimal.Zero
		}
		return *p
	}
	return General{
		SalaryUSD:                 zero(c.SalaryUSD),
		FXRateApplied:             zero(c.FXRateApplied),
		MorseFeeRate:              zero(c.MorseFeeRate),
		EmergencyMonths:           zero(c.EmergencyMonths),
		ExtraIncomeEstimateMXN:    c.ExtraIncomeEstimateMXN,
		BudgetIncludesExtraIncome: c.BudgetIncludesExtraIncome,
		ExtraIncomeSplit:          c.ExtraIncomeSplit,
		InvestmentAllocation:      c.InvestmentAllocation,
	}
}

// Instrument is an investment or savings instrument.
type Instrument struct {
	ID       string
	Name     string
	Type     string
	Platform string
}

// Config is the pure view of the user's settings. The pointer fields are
// required by the tax calculations; Validate reports the missing ones.
type Config struct {
	SalaryUSD       *decimal.Decimal
	FXRateApplied   *decimal.Decimal
	MorseFeeRate    *decimal.Decimal
	EmergencyMonths *decimal.Decimal

	ExtraIncomeEstimateMXN    decimal.Decimal
	BudgetIncludesExtraIncome bool

	// ExtraIncomeSplit holds optional rates keyed as in the configuration
	// (sat_reserve_rate, fondo_emergencia, inversiones, aguinaldo_vacaciones).
	ExtraIncomeSplit map[string]decimal.Decimal

	// InvestmentAllocation holds the weight of each instrument for the
	// investments category (asignacion_inversiones).
	InvestmentAllocation []Weight

	Instruments          []Instrument
	InstrumentByCategory map[string]string
	Clients              []Client
	Brackets             []Bracket
	Categories           []Category

	Issuer         Issuer
	PaymentMethods []string
	// Pause is the investment pause plan; nil means no pause is configured.
	Pause *PausePlan
}

// ErrMissingConfig is wrapped by Validate when a required key is absent.
var ErrMissingConfig = errors.New("missing required config")

// Validate reports the required keys that are absent.
func (c Config) Validate() error {
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
	if c.EmergencyMonths == nil {
		missing = append(missing, "emergency_months")
	}
	if len(c.Brackets) == 0 {
		missing = append(missing, "resico_brackets")
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: %s", ErrMissingConfig, strings.Join(missing, ", "))
	}
	return nil
}

// ResicoRate returns the rate of the first bracket whose upper bound is
// greater than or equal to the income; above the last bound it returns the last rate.
func (c Config) ResicoRate(income decimal.Decimal) (decimal.Decimal, error) {
	if len(c.Brackets) == 0 {
		return decimal.Zero, fmt.Errorf("%w: resico_brackets", ErrMissingConfig)
	}
	for _, b := range c.Brackets {
		if income.LessThanOrEqual(b.Upper) {
			return b.Rate, nil
		}
	}
	return c.Brackets[len(c.Brackets)-1].Rate, nil
}

// ResicoRateOrFirst is ResicoRate for a positive income and the first bracket's
// rate when there is no income (the rate applied to an empty month).
func (c Config) ResicoRateOrFirst(income decimal.Decimal) (decimal.Decimal, error) {
	if income.IsPositive() {
		return c.ResicoRate(income)
	}
	if len(c.Brackets) == 0 {
		return decimal.Zero, fmt.Errorf("%w: resico_brackets", ErrMissingConfig)
	}
	return c.Brackets[0].Rate, nil
}

// SplitRate returns the configured extra-income split rate or the default.
func (c Config) SplitRate(key string, def decimal.Decimal) decimal.Decimal {
	if r, ok := c.ExtraIncomeSplit[key]; ok {
		return r
	}
	return def
}

// FindClient returns the client with the given ID.
func (c Config) FindClient(id string) (Client, bool) {
	for _, cl := range c.Clients {
		if cl.ID == id {
			return cl, true
		}
	}
	return Client{}, false
}

// FindInstrument returns the instrument with the given ID.
func (c Config) FindInstrument(id string) (Instrument, bool) {
	for _, i := range c.Instruments {
		if i.ID == id {
			return i, true
		}
	}
	return Instrument{}, false
}

// CategoriesByKind returns the categories of a kind in configuration order.
func (c Config) CategoriesByKind(kind ledger.Kind) []Category {
	var out []Category
	for _, cat := range c.Categories {
		if cat.Kind == kind {
			out = append(out, cat)
		}
	}
	return out
}

// InferCategory returns the first category of the given kind (configuration
// order) with a keyword contained in the description, ignoring case and accents.
func (c Config) InferCategory(description string, kind ledger.Kind) (string, bool) {
	if description == "" {
		return "", false
	}
	nd := ledger.Normalize(description)
	for _, cat := range c.CategoriesByKind(kind) {
		for _, kw := range cat.Keywords {
			if strings.Contains(nd, ledger.Normalize(kw)) {
				return cat.Name, true
			}
		}
	}
	return "", false
}

// ResolveInstrument returns the instrument for a savings category: the
// explicit ID if valid, else the category default. An empty result with a nil
// error means no instrument applies.
func (c Config) ResolveInstrument(category, instrumentID string) (string, error) {
	if instrumentID != "" {
		if _, ok := c.FindInstrument(instrumentID); !ok {
			ids := make([]string, len(c.Instruments))
			for i, in := range c.Instruments {
				ids[i] = in.ID
			}
			return "", fmt.Errorf("instrument %q does not exist, valid: %s", instrumentID, strings.Join(ids, ", "))
		}
		return instrumentID, nil
	}
	def := c.InstrumentByCategory[category]
	if def != "" {
		if _, ok := c.FindInstrument(def); !ok {
			return "", fmt.Errorf("default instrument %q for %q does not exist in config", def, category)
		}
	}
	return def, nil
}

// CategoryBudget returns the budget of a category, with overrides (computed
// Impuestos and Comisiones budgets) taking precedence. Nil means no budget.
func CategoryBudget(cat Category, overrides map[string]decimal.Decimal) *decimal.Decimal {
	if v, ok := overrides[cat.Name]; ok {
		return &v
	}
	return cat.Budget
}
