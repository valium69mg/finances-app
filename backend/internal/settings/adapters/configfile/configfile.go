// Package configfile reads the legacy ~/finances/config.json (Spanish keys)
// into the settings domain. It is only used by the one-time import.
package configfile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/shopspring/decimal"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

// Numbers are read as json.Number so decimals keep their literal text and never
// pass through a float64.
type file struct {
	SalaryUSD                 *json.Number           `json:"salary_usd"`
	FXRateApplied             *json.Number           `json:"fx_rate_applied"`
	MorseFeeRate              *json.Number           `json:"morse_fee_rate"`
	EmergencyMonths           *json.Number           `json:"emergency_months"`
	ExtraIncomeEstimateMXN    json.Number            `json:"extra_income_estimate_mxn"`
	BudgetIncludesExtraIncome bool                   `json:"budget_includes_extra_income"`
	ExtraIncomeSplit          map[string]json.Number `json:"extra_income_split"`
	Instruments               []struct {
		ID       string `json:"id"`
		Name     string `json:"nombre"`
		Type     string `json:"tipo"`
		Platform string `json:"plataforma"`
	} `json:"instrumentos"`
	InstrumentByCategory map[string]string      `json:"instrumento_por_categoria"`
	InvestmentAllocation map[string]json.Number `json:"asignacion_inversiones"`
	Issuer               struct {
		RFC        string `json:"rfc"`
		Name       string `json:"nombre"`
		Regimen    string `json:"regimen"`
		PostalCode string `json:"cp"`
		Note       string `json:"nota"`
	} `json:"emisor"`
	Clients []struct {
		ID            string      `json:"id"`
		Name          string      `json:"nombre"`
		Type          string      `json:"tipo"`
		RFC           string      `json:"rfc"`
		Regimen       string      `json:"regimen"`
		UsoCFDI       string      `json:"uso_cfdi"`
		Currency      string      `json:"moneda"`
		IVARate       json.Number `json:"iva_rate"`
		RetISRRate    json.Number `json:"ret_isr_rate"`
		RetIVARate    json.Number `json:"ret_iva_rate"`
		Concepto      string      `json:"concepto"`
		ClaveProdServ string      `json:"clave_prod_serv"`
		ClaveUnidad   string      `json:"clave_unidad"`
		Address       string      `json:"domicilio"`
		TaxResidence  string      `json:"residencia_fiscal"`
		Contract      string      `json:"contrato"`
		RealPayer     string      `json:"pagador_real"`
	} `json:"clientes"`
	Brackets []struct {
		Upper json.Number `json:"upper"`
		Rate  json.Number `json:"rate"`
	} `json:"resico_brackets"`
	PaymentMethods []string `json:"payment_methods"`
	Categories     []struct {
		Name     string       `json:"name"`
		Type     string       `json:"type"`
		Budget   *json.Number `json:"budget"`
		Includes string       `json:"includes"`
		Keywords []string     `json:"keywords"`
	} `json:"categories"`
	Pause *struct {
		Months     []string               `json:"meses"`
		NormalBudg json.Number            `json:"budget_normal"`
		Resume     string                 `json:"reanudar"`
		Plan       map[string]json.Number `json:"gastos_futuros_plan"`
		Note       string                 `json:"nota"`
	} `json:"inversiones_pausa"`
}

// Parse maps the contents of config.json to the domain. Unknown keys are
// ignored (the file carries notes and metadata the app does not model).
func Parse(data []byte) (domain.Config, error) {
	var f file
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&f); err != nil {
		return domain.Config{}, fmt.Errorf("decode config: %w", err)
	}

	var perr error
	num := func(field string, n json.Number) decimal.Decimal {
		if n == "" {
			return decimal.Zero
		}
		d, err := decimal.NewFromString(n.String())
		if err != nil && perr == nil {
			perr = fmt.Errorf("%s: %w", field, err)
		}
		return d
	}
	numPtr := func(field string, n *json.Number) *decimal.Decimal {
		if n == nil {
			return nil
		}
		d := num(field, *n)
		return &d
	}

	cfg := domain.Config{
		SalaryUSD:                 numPtr("salary_usd", f.SalaryUSD),
		FXRateApplied:             numPtr("fx_rate_applied", f.FXRateApplied),
		MorseFeeRate:              numPtr("morse_fee_rate", f.MorseFeeRate),
		EmergencyMonths:           numPtr("emergency_months", f.EmergencyMonths),
		ExtraIncomeEstimateMXN:    num("extra_income_estimate_mxn", f.ExtraIncomeEstimateMXN),
		BudgetIncludesExtraIncome: f.BudgetIncludesExtraIncome,
		InstrumentByCategory:      f.InstrumentByCategory,
		PaymentMethods:            f.PaymentMethods,
		Issuer: domain.Issuer{
			RFC: f.Issuer.RFC, Name: f.Issuer.Name, Regimen: f.Issuer.Regimen,
			PostalCode: f.Issuer.PostalCode, Note: f.Issuer.Note,
		},
	}

	cfg.ExtraIncomeSplit = make(map[string]decimal.Decimal, len(f.ExtraIncomeSplit))
	for k, v := range f.ExtraIncomeSplit {
		cfg.ExtraIncomeSplit[k] = num("extra_income_split."+k, v)
	}

	// JSON objects are unordered: sort the allocation keys for a stable result.
	keys := make([]string, 0, len(f.InvestmentAllocation))
	for k := range f.InvestmentAllocation {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		cfg.InvestmentAllocation = append(cfg.InvestmentAllocation,
			domain.Weight{Key: k, Value: num("asignacion_inversiones."+k, f.InvestmentAllocation[k])})
	}

	for _, in := range f.Instruments {
		cfg.Instruments = append(cfg.Instruments,
			domain.Instrument{ID: in.ID, Name: in.Name, Type: in.Type, Platform: in.Platform})
	}
	for _, c := range f.Clients {
		cfg.Clients = append(cfg.Clients, domain.Client{
			ID: c.ID, Name: c.Name, Currency: c.Currency, IVARate: num("iva_rate", c.IVARate),
			Type: c.Type, RFC: c.RFC, Regimen: c.Regimen, UsoCFDI: c.UsoCFDI,
			RetISRRate: num("ret_isr_rate", c.RetISRRate), RetIVARate: num("ret_iva_rate", c.RetIVARate),
			Concepto: c.Concepto, ClaveProdServ: c.ClaveProdServ, ClaveUnidad: c.ClaveUnidad,
			Address: c.Address, TaxResidence: c.TaxResidence, Contract: c.Contract, RealPayer: c.RealPayer,
		})
	}
	for _, b := range f.Brackets {
		cfg.Brackets = append(cfg.Brackets, domain.Bracket{Upper: num("resico_brackets.upper", b.Upper), Rate: num("resico_brackets.rate", b.Rate)})
	}
	for _, c := range f.Categories {
		cfg.Categories = append(cfg.Categories, domain.Category{
			Name: c.Name, Kind: ledger.Kind(c.Type), Budget: numPtr("category "+c.Name+" budget", c.Budget),
			Includes: c.Includes, Keywords: c.Keywords,
		})
	}
	if p := f.Pause; p != nil {
		plan := domain.PausePlan{
			Months: p.Months, NormalBudget: num("inversiones_pausa.budget_normal", p.NormalBudg),
			ResumeMonth: p.Resume, Note: p.Note, FutureExpensesPlan: map[string]decimal.Decimal{},
		}
		for m, v := range p.Plan {
			plan.FutureExpensesPlan[m] = num("inversiones_pausa.gastos_futuros_plan."+m, v)
		}
		cfg.Pause = &plan
	}
	if perr != nil {
		return domain.Config{}, perr
	}
	return cfg, nil
}
