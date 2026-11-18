package domain

import "github.com/shopspring/decimal"

// Block is what a group of Gasto categories spent against what they budgeted.
type Block struct {
	Spent  decimal.Decimal
	Budget decimal.Decimal
}

// Spending splits the Gasto categories of the cycle into fixed ones (rent,
// services: paid about once, so a linear pace means nothing for them) and
// variable ones. Spent follows the monthly KPI rule (the part paid from future
// expense savings is not counted). Budget adds only the categories that have a
// budget; a category without one still counts its spending.
type Spending struct {
	Fixed    Block
	Variable Block
	// HasFixed is true when at least one Gasto category is marked fixed, even
	// if it has no budget or spending yet.
	HasFixed bool
}

// SplitSpending builds the Spending of the Gasto rows; other kinds are skipped.
func SplitSpending(rows []Row) Spending {
	var out Spending
	for _, r := range rows {
		if r.Kind != "Gasto" {
			continue
		}
		block := &out.Variable
		if r.Fixed {
			block = &out.Fixed
			out.HasFixed = true
		}
		block.Spent = block.Spent.Add(r.Real)
		if r.Budget != nil {
			block.Budget = block.Budget.Add(*r.Budget)
		}
	}
	return out
}
