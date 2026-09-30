package configfile_test

import (
	"testing"

	"github.com/valium69mg/finances-app/backend/internal/settings/adapters/configfile"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain/settingstest"
)

const sample = `{
  "salary_usd": 3500,
  "fx_rate_applied": 17.74,
  "morse_fee_rate": 0.001,
  "emergency_months": 6,
  "extra_income_estimate_mxn": 35000,
  "budget_includes_extra_income": false,
  "cycle_start_day": 31,
  "extra_income_split": {"sat_reserve_rate": 0.165, "inversiones": 0.35},
  "instrumentos": [{"id": "voo", "nombre": "VOO", "tipo": "renta_variable", "plataforma": "GBM"}],
  "instrumento_por_categoria": {"Inversiones": "voo"},
  "asignacion_inversiones": {"voo": 1.0},
  "emisor": {"rfc": "R", "nombre": "N", "regimen": "626", "cp": "76246", "nota": "x"},
  "clientes": [{"id": "b", "nombre": "PUBLICO", "tipo": "publico_general", "moneda": "MXN", "iva_rate": 0.16,
                "ret_isr_rate": 0, "ret_iva_rate": 0, "concepto": "c", "clave_prod_serv": "01010101", "clave_unidad": "ACT"}],
  "resico_brackets": [{"upper": 25000, "rate": 0.01}, {"upper": 83333.33, "rate": 0.015}],
  "payment_methods": ["Efectivo", "Débito"],
  "categories": [
    {"name": "Sueldo", "type": "Ingreso", "budget": null, "includes": "", "keywords": ["sueldo"]},
    {"name": "Vivienda", "type": "Gasto", "budget": 3600, "includes": "mantenimiento", "keywords": []},
    {"name": "Inversiones", "type": "Ahorro", "budget": 0, "includes": "", "keywords": []}
  ],
  "inversiones_pausa": {
    "meses": ["2026-10", "2027-01"], "budget_normal": 5000, "reanudar": "2027-02",
    "gastos_futuros_plan": {"2026-10": 12000, "2027-01": 10000}, "nota": "n"
  },
  "fx_rate_source": "ignored"
}`

func TestParse(t *testing.T) {
	cfg, err := configfile.Parse([]byte(sample))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.FXRateApplied.String() != "17.74" || cfg.MorseFeeRate.String() != "0.001" {
		t.Fatalf("decimals lost precision: fx=%s fee=%s", cfg.FXRateApplied, cfg.MorseFeeRate)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("parsed config should validate: %v", err)
	}
	if cfg.CycleStartDay != 31 {
		t.Fatalf("cycle_start_day = %d, want 31", cfg.CycleStartDay)
	}
	if len(cfg.Categories) != 3 || cfg.Categories[0].Budget != nil || cfg.Categories[1].Budget.String() != "3600" {
		t.Fatalf("categories = %+v", cfg.Categories)
	}
	if cfg.Categories[2].Budget == nil || !cfg.Categories[2].Budget.IsZero() {
		t.Fatal("a zero budget must stay zero, not nil")
	}
	if string(cfg.Categories[1].Kind) != "Gasto" || cfg.Categories[1].Includes != "mantenimiento" {
		t.Fatalf("category mapping: %+v", cfg.Categories[1])
	}
	if cfg.Clients[0].ClaveProdServ != "01010101" || cfg.Clients[0].IVARate.String() != "0.16" {
		t.Fatalf("client = %+v", cfg.Clients[0])
	}
	if cfg.Brackets[1].Upper.String() != "83333.33" {
		t.Fatalf("brackets = %+v", cfg.Brackets)
	}
	if cfg.Issuer.PostalCode != "76246" || len(cfg.PaymentMethods) != 2 {
		t.Fatalf("issuer/payment methods: %+v %v", cfg.Issuer, cfg.PaymentMethods)
	}
	if cfg.Pause == nil || cfg.Pause.ResumeMonth != "2027-02" || cfg.Pause.NormalBudget.String() != "5000" {
		t.Fatalf("pause = %+v", cfg.Pause)
	}
	if err := cfg.Pause.Validate(); err != nil {
		t.Fatal(err)
	}
	// The parsed pause must reproduce the hand-computed cases.
	want := settingstest.RealPause()
	if !cfg.Pause.BudgetOverrides("2027-01")["Gastos futuros"].Equal(want.FutureExpensesPlan["2027-01"]) {
		t.Fatalf("2027-01 Gastos futuros = %v", cfg.Pause.BudgetOverrides("2027-01"))
	}
}

func TestParseErrors(t *testing.T) {
	if _, err := configfile.Parse([]byte(`{not json`)); err == nil {
		t.Fatal("malformed JSON should fail")
	}
	if _, err := configfile.Parse([]byte(`{"salary_usd": "abc"}`)); err == nil {
		t.Fatal("non-numeric salary should fail")
	}
}
