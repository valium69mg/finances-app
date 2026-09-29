// Package domain holds the pure savings rules: emergency fund status, weighted
// splits and the portfolio (contributions versus manually recorded valuations).
package domain

import (
	"fmt"

	"github.com/shopspring/decimal"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

// Labels of the synthetic portfolio rows.
const (
	// NoInstrument groups savings movements that name no instrument.
	NoInstrument = "(sin instrumento)"
	// UnknownInstrumentType is the type of rows whose instrument is not in the configuration.
	UnknownInstrumentType = "desconocido"
	// UnknownPlatform is the platform of rows whose instrument is not in the configuration.
	UnknownPlatform = "-"
)

// DestinationCategories are the savings categories reported by destination.
var DestinationCategories = []string{
	ledger.CategorySATReserve,
	ledger.CategoryEmergencyFund,
	ledger.CategoryAguinaldoVacation,
	ledger.CategoryFutureExpenses,
	ledger.CategoryInvestments,
}

var hundred = decimal.NewFromInt(100)

// EmergencyStatus is the emergency fund progress: the all-time balance (net
// of withdrawals) against its goal.
type EmergencyStatus struct {
	Accumulated decimal.Decimal
	Goal        decimal.Decimal
}

// EmergencyFund returns the emergency fund status. The goal is the configured
// number of months times the monthly expense budgets, with the computed
// Impuestos and Comisiones budgets overriding the configured ones and
// categories without a budget counting as zero.
func EmergencyFund(cfg settings.Config, movements []ledger.Movement) (EmergencyStatus, error) {
	if cfg.EmergencyMonths == nil {
		return EmergencyStatus{}, fmt.Errorf("%w: emergency_months", settings.ErrMissingConfig)
	}
	overrides, err := cfg.BudgetOverrides()
	if err != nil {
		return EmergencyStatus{}, err
	}
	monthlyBudget := decimal.Zero
	for _, c := range cfg.CategoriesByKind(ledger.KindExpense) {
		if b := settings.CategoryBudget(c, overrides); b != nil {
			monthlyBudget = monthlyBudget.Add(*b)
		}
	}
	return EmergencyStatus{
		Accumulated: ledger.SumBy(movements, ledger.Filter{Kind: ledger.KindSavings, Category: ledger.CategoryEmergencyFund}),
		Goal:        cfg.EmergencyMonths.Mul(monthlyBudget),
	}, nil
}

// Share is one part of a weighted split.
type Share struct {
	Key    string
	Amount decimal.Decimal
}

// SplitByWeights rounds each share half-even to whole pesos; the rounding
// remainder is absorbed by the largest-weight key (the first one on ties).
// It returns nil when there are no weights.
func SplitByWeights(total decimal.Decimal, weights []settings.Weight) []Share {
	if len(weights) == 0 {
		return nil
	}
	shares := make([]Share, len(weights))
	sum := decimal.Zero
	biggest := 0
	for i, w := range weights {
		amount := total.Mul(w.Value).RoundBank(0)
		shares[i] = Share{Key: w.Key, Amount: amount}
		sum = sum.Add(amount)
		if w.Value.GreaterThan(weights[biggest].Value) {
			biggest = i
		}
	}
	shares[biggest].Amount = shares[biggest].Amount.Add(total.Sub(sum))
	return shares
}

// LatestValuation returns the most recent valuation of an instrument; on
// equal dates the last one listed wins.
func LatestValuation(valuations []ledger.Valuation, instrumentID string) (ledger.Valuation, bool) {
	var best ledger.Valuation
	found := false
	for _, v := range valuations {
		if v.Instrument != instrumentID {
			continue
		}
		if !found || v.Date >= best.Date {
			best, found = v, true
		}
	}
	return best, found
}

// PortfolioRow is one instrument of the portfolio. Unvalued rows use the
// contributed amount as their value.
type PortfolioRow struct {
	ID          string
	Name        string
	Type        string
	Platform    string
	Contributed decimal.Decimal
	Value       decimal.Decimal
	ValueDate   string // empty when unvalued
	Unvalued    bool
	Gain        decimal.Decimal
	GainPct     decimal.Decimal
	PctOfTotal  decimal.Decimal
}

// TypeTotal aggregates the rows of one instrument type.
type TypeTotal struct {
	Type        string
	Contributed decimal.Decimal
	Value       decimal.Decimal
}

// DestinationTotal is the all-time savings balance of one destination category.
type DestinationTotal struct {
	Category string
	Balance  decimal.Decimal
}

// Portfolio is the savings and investments picture by instrument.
type Portfolio struct {
	Rows             []PortfolioRow
	TotalContributed decimal.Decimal
	TotalValue       decimal.Decimal
	ByType           []TypeTotal // first-appearance order
	ByDestination    []DestinationTotal
	Emergency        EmergencyStatus
}

// ComputePortfolio builds the portfolio from all-time savings movements and
// the manually recorded valuations. Configured instruments come first, in
// configuration order; movements naming an unknown instrument (or none) are
// appended in order of first appearance.
func ComputePortfolio(cfg settings.Config, movements []ledger.Movement, valuations []ledger.Valuation) (Portfolio, error) {
	var order []string
	contributed := map[string]decimal.Decimal{}
	for _, m := range movements {
		if m.Kind != ledger.KindSavings {
			continue
		}
		key := m.Instrument
		if key == "" {
			key = NoInstrument
		}
		if _, seen := contributed[key]; !seen {
			order = append(order, key)
		}
		contributed[key] = contributed[key].Add(m.AmountMXN)
	}

	var rows []PortfolioRow
	for _, in := range cfg.Instruments {
		amount := contributed[in.ID]
		delete(contributed, in.ID)
		row := PortfolioRow{ID: in.ID, Name: in.Name, Type: in.Type, Platform: in.Platform, Contributed: amount}
		if val, ok := LatestValuation(valuations, in.ID); ok {
			row.Value, row.ValueDate = val.ValueMXN, val.Date
		} else {
			row.Value, row.Unvalued = amount, true
		}
		row.Gain = row.Value.Sub(amount)
		if !amount.IsZero() {
			row.GainPct = row.Gain.Mul(hundred).DivRound(amount, ledger.DivisionPrecision)
		}
		rows = append(rows, row)
	}
	for _, key := range order {
		amount, ok := contributed[key]
		if !ok {
			continue // a configured instrument, already reported
		}
		rows = append(rows, PortfolioRow{
			ID: key, Name: key, Type: UnknownInstrumentType, Platform: UnknownPlatform,
			Contributed: amount, Value: amount, Unvalued: true,
		})
	}

	p := Portfolio{Rows: rows}
	for _, r := range rows {
		p.TotalContributed = p.TotalContributed.Add(r.Contributed)
		p.TotalValue = p.TotalValue.Add(r.Value)
	}
	typeIndex := map[string]int{}
	for i := range p.Rows {
		r := &p.Rows[i]
		if !p.TotalValue.IsZero() {
			r.PctOfTotal = r.Value.Mul(hundred).DivRound(p.TotalValue, ledger.DivisionPrecision)
		}
		idx, ok := typeIndex[r.Type]
		if !ok {
			idx = len(p.ByType)
			typeIndex[r.Type] = idx
			p.ByType = append(p.ByType, TypeTotal{Type: r.Type})
		}
		p.ByType[idx].Contributed = p.ByType[idx].Contributed.Add(r.Contributed)
		p.ByType[idx].Value = p.ByType[idx].Value.Add(r.Value)
	}

	for _, cat := range DestinationCategories {
		p.ByDestination = append(p.ByDestination, DestinationTotal{
			Category: cat,
			Balance:  ledger.SumBy(movements, ledger.Filter{Kind: ledger.KindSavings, Category: cat}),
		})
	}

	em, err := EmergencyFund(cfg, movements)
	if err != nil {
		return Portfolio{}, err
	}
	p.Emergency = em
	return p, nil
}
