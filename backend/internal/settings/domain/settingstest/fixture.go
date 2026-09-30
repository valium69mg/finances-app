// Package settingstest provides the real-numbers configuration fixture used by
// the domain tests of every module. It is pure data and only meant for tests.
package settingstest

import (
	"github.com/shopspring/decimal"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

// D parses a decimal literal and panics when it is invalid.
func D(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func ptr(s string) *decimal.Decimal {
	d := D(s)
	return &d
}

func cat(name string, kind ledger.Kind, budget *decimal.Decimal, keywords ...string) settings.Category {
	return settings.Category{Name: name, Kind: kind, Budget: budget, Keywords: keywords}
}

// RealPause mirrors the inversiones_pausa block of the current ~/finances/config.json.
func RealPause() settings.PausePlan {
	return settings.PausePlan{
		Months:       []string{"2026-10", "2026-11", "2026-12", "2027-01"},
		NormalBudget: D("5000"),
		ResumeMonth:  "2027-02",
		FutureExpensesPlan: map[string]decimal.Decimal{
			"2026-10": D("12000"),
			"2026-11": D("12000"),
			"2026-12": D("12000"),
			"2027-01": D("10000"),
		},
	}
}

// RealConfig mirrors the values of the current ~/finances/config.json.
func RealConfig() settings.Config {
	return settings.Config{
		SalaryUSD:                 ptr("3500"),
		FXRateApplied:             ptr("17.74"),
		MorseFeeRate:              ptr("0.001"),
		EmergencyMonths:           ptr("6"),
		ExtraIncomeEstimateMXN:    D("35000"),
		BudgetIncludesExtraIncome: false,
		ExtraIncomeSplit: map[string]decimal.Decimal{
			"sat_reserve_rate":     D("0.165"),
			"fondo_emergencia":     D("0.5"),
			"inversiones":          D("0.35"),
			"aguinaldo_vacaciones": D("0.15"),
		},
		InvestmentAllocation: []settings.Weight{{Key: "voo", Value: D("1")}},
		Instruments: []settings.Instrument{
			{ID: "liquidez-gbm", Name: "Liquidez diaria GBM (p.ej. Smart Cash)", Type: "liquidez", Platform: "GBM"},
			{ID: "cetes-28", Name: "CETES 28 días", Type: "deuda", Platform: "GBM"},
			{ID: "cetes-91", Name: "CETES 91 días", Type: "deuda", Platform: "GBM"},
			{ID: "voo", Name: "VOO — Vanguard S&P 500 (SIC)", Type: "renta_variable", Platform: "GBM"},
			{ID: "vxus", Name: "VXUS — Vanguard Total International (SIC)", Type: "renta_variable", Platform: "GBM"},
		},
		InstrumentByCategory: map[string]string{
			"Reserva SAT":            "liquidez-gbm",
			"Fondo de emergencia":    "cetes-28",
			"Aguinaldo y vacaciones": "cetes-28",
			"Inversiones":            "voo",
			"Gastos futuros":         "liquidez-gbm",
		},
		Clients: []settings.Client{
			{ID: "usa", Name: "Cliente USA", Currency: "USD", IVARate: D("0")},
			{ID: "b", Name: "Cliente B", Currency: "MXN", IVARate: D("0.16")},
		},
		Brackets: []settings.Bracket{
			{Upper: D("25000"), Rate: D("0.01")},
			{Upper: D("50000"), Rate: D("0.011")},
			{Upper: D("83333.33"), Rate: D("0.015")},
			{Upper: D("208333.33"), Rate: D("0.02")},
			{Upper: D("291666.67"), Rate: D("0.025")},
		},
		Categories: []settings.Category{
			cat("Sueldo", ledger.KindIncome, nil, "sueldo", "salario", "pago usa"),
			cat("Contrato extra", ledger.KindIncome, nil, "quincena", "cliente", "freelance", "contrato"),
			cat("Otros ingresos", ledger.KindIncome, nil),
			cat("Vivienda", ledger.KindExpense, ptr("3600"), "mantenimiento", "condominio", "predial", "renta"),
			cat("Servicios", ledger.KindExpense, ptr("2000"), "megacable", "internet", "agua", "luz", "cfe", "telcel", "celular", "gas"),
			cat("Mandado", ledger.KindExpense, ptr("10833"), "mandado", "super", "walmart", "soriana", "costco", "despensa"),
			cat("Comida fuera", ledger.KindExpense, ptr("2167"), "restaurante", "comida fuera", "rappi", "uber eats", "didi food", "cafe", "tacos"),
			cat("Transporte", ledger.KindExpense, ptr("1000"), "gasolina", "uber", "didi", "estacionamiento", "caseta"),
			cat("Salud", ledger.KindExpense, ptr("1758"), "gym", "gimnasio", "doctor", "farmacia", "medicina", "dentista"),
			cat("Suscripciones", ledger.KindExpense, ptr("869"), "claude", "netflix", "disney", "spotify", "youtube", "suscripcion"),
			cat("Educación", ledger.KindExpense, ptr("4000"), "escuela", "colegiatura", "curso"),
			cat("Mascota", ledger.KindExpense, ptr("600"), "mascota", "perro", "gato", "croquetas", "veterinario"),
			cat("Impuestos", ledger.KindExpense, nil, "isr", "sat", "impuesto"),
			cat("Comisiones", ledger.KindExpense, nil, "comision", "morse"),
			cat("Ocio", ledger.KindExpense, nil),
			cat("Ropa", ledger.KindExpense, nil),
			cat("Otros", ledger.KindExpense, nil),
			cat("Fondo de emergencia", ledger.KindSavings, ptr("15000"), "emergencia", "fondo"),
			cat("Inversiones", ledger.KindSavings, ptr("0"), "inversion", "cetes", "gbm", "ppr"),
			cat("Aguinaldo y vacaciones", ledger.KindSavings, ptr("3000"), "aguinaldo", "vacaciones", "viaje"),
			cat("Gastos futuros", ledger.KindSavings, ptr("12000"), "gastos futuros", "seguro", "predial", "apartado"),
			cat("Reserva SAT", ledger.KindSavings, nil, "reserva sat", "apartado sat", "impuestos b"),
		},
	}
}
