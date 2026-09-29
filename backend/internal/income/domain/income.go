// Package domain holds the pure income rules: currency conversion and the
// automatic split of extra (client B) deposits.
package domain

import (
	"github.com/shopspring/decimal"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

// Default split rates, used when the configuration does not define them.
var (
	defaultSATReserveRate = decimal.RequireFromString("0.165")
	defaultEmergencyRate  = decimal.RequireFromString("0.5")
	defaultInvestmentRate = decimal.RequireFromString("0.35")
	defaultAguinaldoRate  = decimal.RequireFromString("0.15")
)

// ToMXN converts a value to pesos: USD values are multiplied by the exchange
// rate (a missing or zero rate counts as 1), any other currency is returned
// unchanged and a missing value counts as zero.
func ToMXN(value *decimal.Decimal, currency string, exchangeRate *decimal.Decimal) decimal.Decimal {
	v := decimal.Zero
	if value != nil {
		v = *value
	}
	if currency != ledger.CurrencyUSD {
		return v
	}
	rate := decimal.NewFromInt(1)
	if exchangeRate != nil && !exchangeRate.IsZero() {
		rate = *exchangeRate
	}
	return v.Mul(rate)
}

// ExtraSplit is the suggested split of an extra-income deposit. The shares
// always add up exactly to Amount.
type ExtraSplit struct {
	Amount            decimal.Decimal
	SATReserve        decimal.Decimal
	EmergencyFund     decimal.Decimal
	Investments       decimal.Decimal
	AguinaldoVacation decimal.Decimal
	GoalReached       bool
}

// ComputeExtraSplit splits a client B deposit: the SAT reserve comes off the
// top and the remainder goes to the emergency fund, investments and
// aguinaldo/vacation. Every share is rounded half-even to whole pesos and the
// rounding remainder is absorbed by the emergency fund. When the emergency
// fund already reached its goal, its share is redirected to investments.
func ComputeExtraSplit(amount decimal.Decimal, cfg settings.Config, emergencyAccumulated, emergencyGoal decimal.Decimal) ExtraSplit {
	satRate := cfg.SplitRate("sat_reserve_rate", defaultSATReserveRate)
	fundRate := cfg.SplitRate("fondo_emergencia", defaultEmergencyRate)
	investRate := cfg.SplitRate("inversiones", defaultInvestmentRate)
	aguinaldoRate := cfg.SplitRate("aguinaldo_vacaciones", defaultAguinaldoRate)

	sat := amount.Mul(satRate).RoundBank(0)
	remainder := amount.Sub(sat)
	fund := remainder.Mul(fundRate).RoundBank(0)
	investments := remainder.Mul(investRate).RoundBank(0)
	aguinaldo := remainder.Mul(aguinaldoRate).RoundBank(0)
	diff := amount.Sub(sat.Add(fund).Add(investments).Add(aguinaldo))
	fund = fund.Add(diff)

	goalReached := emergencyGoal.IsPositive() && emergencyAccumulated.GreaterThanOrEqual(emergencyGoal)
	if goalReached {
		investments = investments.Add(fund)
		fund = decimal.Zero
	}

	return ExtraSplit{
		Amount:            amount,
		SATReserve:        sat,
		EmergencyFund:     fund,
		Investments:       investments,
		AguinaldoVacation: aguinaldo,
		GoalReached:       goalReached,
	}
}
