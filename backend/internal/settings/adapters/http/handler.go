// Package settingshttp exposes the settings use cases over HTTP. Every route
// sits behind the authentication middleware passed to Register. Money and
// rates travel as decimal strings.
package settingshttp

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/shopspring/decimal"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/httpmw"
	"github.com/valium69mg/finances-app/backend/internal/settings/app"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

const maxBodyBytes = 1 << 20

// Service is the set of use cases the handlers need.
type Service interface {
	Get(ctx context.Context) (domain.Config, error)
	UpdateGeneral(ctx context.Context, g domain.General) error
	UpdateCategories(ctx context.Context, cats []domain.Category) error
	UpdateClients(ctx context.Context, clients []domain.Client) error
	UpdateInstruments(ctx context.Context, instruments []domain.Instrument, byCategory map[string]string) error
	UpdateBrackets(ctx context.Context, brackets []domain.Bracket) error
	UpdatePaymentMethods(ctx context.Context, methods []string) error
	UpdateIssuer(ctx context.Context, issuer domain.Issuer) error
	UpdatePause(ctx context.Context, plan *domain.PausePlan) error
	MonthBudgets(ctx context.Context, month string) ([]app.CategoryBudget, error)
}

// Handler serves the /settings routes.
type Handler struct {
	svc    Service
	logger *slog.Logger
}

// New builds a Handler. A nil logger selects slog.Default().
func New(svc Service, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{svc: svc, logger: logger}
}

// Register mounts the settings routes on mux, each wrapped by requireAuth.
func (h *Handler) Register(mux httpmw.Router, requireAuth func(http.Handler) http.Handler) {
	route := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, requireAuth(fn))
	}
	route("GET /settings", h.getAll)
	route("GET /settings/general", h.getGeneral)
	route("PUT /settings/general", h.putGeneral)
	route("GET /settings/categories", h.getCategories)
	route("PUT /settings/categories", h.putCategories)
	route("GET /settings/clients", h.getClients)
	route("PUT /settings/clients", h.putClients)
	route("GET /settings/instruments", h.getInstruments)
	route("PUT /settings/instruments", h.putInstruments)
	route("GET /settings/brackets", h.getBrackets)
	route("PUT /settings/brackets", h.putBrackets)
	route("GET /settings/payment-methods", h.getPaymentMethods)
	route("PUT /settings/payment-methods", h.putPaymentMethods)
	route("GET /settings/issuer", h.getIssuer)
	route("PUT /settings/issuer", h.putIssuer)
	route("GET /settings/investment-pause", h.getPause)
	route("PUT /settings/investment-pause", h.putPause)
	route("DELETE /settings/investment-pause", h.deletePause)
	route("GET /settings/budgets", h.getBudgets)
}

// --- DTOs ---------------------------------------------------------------

type weightDTO struct {
	Key   string          `json:"key"`
	Value decimal.Decimal `json:"value"`
}

type generalDTO struct {
	SalaryUSD                 decimal.Decimal            `json:"salary_usd"`
	FXRateApplied             decimal.Decimal            `json:"fx_rate_applied"`
	MorseFeeRate              decimal.Decimal            `json:"morse_fee_rate"`
	EmergencyMonths           decimal.Decimal            `json:"emergency_months"`
	ExtraIncomeEstimateMXN    decimal.Decimal            `json:"extra_income_estimate_mxn"`
	BudgetIncludesExtraIncome bool                       `json:"budget_includes_extra_income"`
	CycleStartDay             int                        `json:"cycle_start_day"`
	ExtraIncomeSplit          map[string]decimal.Decimal `json:"extra_income_split"`
	InvestmentAllocation      []weightDTO                `json:"investment_allocation"`
}

type categoryDTO struct {
	Name     string           `json:"name"`
	Kind     string           `json:"kind"`
	Budget   *decimal.Decimal `json:"budget"`
	Includes string           `json:"includes"`
	Keywords []string         `json:"keywords"`
}

type clientDTO struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Type          string          `json:"type"`
	Currency      string          `json:"currency"`
	IVARate       decimal.Decimal `json:"iva_rate"`
	RFC           string          `json:"rfc"`
	Regimen       string          `json:"regimen"`
	UsoCFDI       string          `json:"uso_cfdi"`
	RetISRRate    decimal.Decimal `json:"ret_isr_rate"`
	RetIVARate    decimal.Decimal `json:"ret_iva_rate"`
	Concepto      string          `json:"concepto"`
	ClaveProdServ string          `json:"clave_prod_serv"`
	ClaveUnidad   string          `json:"clave_unidad"`
	Address       string          `json:"address"`
	TaxResidence  string          `json:"tax_residence"`
	Contract      string          `json:"contract"`
	RealPayer     string          `json:"real_payer"`
	PostalCode    string          `json:"postal_code"`
}

type instrumentDTO struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Platform string `json:"platform"`
}

type instrumentsDTO struct {
	Instruments []instrumentDTO   `json:"instruments"`
	ByCategory  map[string]string `json:"by_category"`
}

type bracketDTO struct {
	Upper decimal.Decimal `json:"upper"`
	Rate  decimal.Decimal `json:"rate"`
}

type issuerDTO struct {
	RFC        string `json:"rfc"`
	Name       string `json:"name"`
	Regimen    string `json:"regimen"`
	PostalCode string `json:"postal_code"`
	Note       string `json:"note"`
}

type pauseDTO struct {
	Months       []string                   `json:"months"`
	NormalBudget decimal.Decimal            `json:"normal_budget"`
	ResumeMonth  string                     `json:"resume_month"`
	Plan         map[string]decimal.Decimal `json:"future_expenses_plan"`
	Note         string                     `json:"note"`
}

type budgetDTO struct {
	Name   string           `json:"name"`
	Kind   string           `json:"kind"`
	Budget *decimal.Decimal `json:"budget"`
}

type allDTO struct {
	General         generalDTO     `json:"general"`
	Categories      []categoryDTO  `json:"categories"`
	Clients         []clientDTO    `json:"clients"`
	Instruments     instrumentsDTO `json:"instruments"`
	Brackets        []bracketDTO   `json:"brackets"`
	PaymentMethods  []string       `json:"payment_methods"`
	Issuer          *issuerDTO     `json:"issuer"`
	InvestmentPause *pauseDTO      `json:"investment_pause"`
}

// --- domain -> DTO ------------------------------------------------------

func toGeneralDTO(c domain.Config) generalDTO {
	g := c.General()
	split := g.ExtraIncomeSplit
	if split == nil {
		split = map[string]decimal.Decimal{}
	}
	alloc := make([]weightDTO, len(g.InvestmentAllocation))
	for i, w := range g.InvestmentAllocation {
		alloc[i] = weightDTO{Key: w.Key, Value: w.Value}
	}
	return generalDTO{
		SalaryUSD: g.SalaryUSD, FXRateApplied: g.FXRateApplied, MorseFeeRate: g.MorseFeeRate,
		EmergencyMonths: g.EmergencyMonths, ExtraIncomeEstimateMXN: g.ExtraIncomeEstimateMXN,
		BudgetIncludesExtraIncome: g.BudgetIncludesExtraIncome, CycleStartDay: g.CycleStartDay,
		ExtraIncomeSplit: split, InvestmentAllocation: alloc,
	}
}

func toCategoryDTOs(cats []domain.Category) []categoryDTO {
	out := make([]categoryDTO, len(cats))
	for i, c := range cats {
		kw := c.Keywords
		if kw == nil {
			kw = []string{}
		}
		out[i] = categoryDTO{Name: c.Name, Kind: string(c.Kind), Budget: c.Budget, Includes: c.Includes, Keywords: kw}
	}
	return out
}

func toClientDTOs(clients []domain.Client) []clientDTO {
	out := make([]clientDTO, len(clients))
	for i, c := range clients {
		out[i] = clientDTO{
			ID: c.ID, Name: c.Name, Type: c.Type, Currency: c.Currency, IVARate: c.IVARate, RFC: c.RFC,
			Regimen: c.Regimen, UsoCFDI: c.UsoCFDI, RetISRRate: c.RetISRRate, RetIVARate: c.RetIVARate,
			Concepto: c.Concepto, ClaveProdServ: c.ClaveProdServ, ClaveUnidad: c.ClaveUnidad, Address: c.Address,
			TaxResidence: c.TaxResidence, Contract: c.Contract, RealPayer: c.RealPayer, PostalCode: c.PostalCode,
		}
	}
	return out
}

func toInstrumentsDTO(c domain.Config) instrumentsDTO {
	out := instrumentsDTO{Instruments: make([]instrumentDTO, len(c.Instruments)), ByCategory: c.InstrumentByCategory}
	if out.ByCategory == nil {
		out.ByCategory = map[string]string{}
	}
	for i, in := range c.Instruments {
		out.Instruments[i] = instrumentDTO{ID: in.ID, Name: in.Name, Type: in.Type, Platform: in.Platform}
	}
	return out
}

func toBracketDTOs(brackets []domain.Bracket) []bracketDTO {
	out := make([]bracketDTO, len(brackets))
	for i, b := range brackets {
		out[i] = bracketDTO{Upper: b.Upper, Rate: b.Rate}
	}
	return out
}

func toPaymentMethods(methods []string) []string {
	if methods == nil {
		return []string{}
	}
	return methods
}

func toIssuerDTO(i domain.Issuer) *issuerDTO {
	if i.IsZero() {
		return nil
	}
	return &issuerDTO{RFC: i.RFC, Name: i.Name, Regimen: i.Regimen, PostalCode: i.PostalCode, Note: i.Note}
}

func toPauseDTO(p *domain.PausePlan) *pauseDTO {
	if p == nil {
		return nil
	}
	return &pauseDTO{Months: p.Months, NormalBudget: p.NormalBudget, ResumeMonth: p.ResumeMonth, Plan: p.FutureExpensesPlan, Note: p.Note}
}

// --- DTO -> domain ------------------------------------------------------

func (d generalDTO) toDomain() domain.General {
	alloc := make([]domain.Weight, len(d.InvestmentAllocation))
	for i, w := range d.InvestmentAllocation {
		alloc[i] = domain.Weight{Key: w.Key, Value: w.Value}
	}
	return domain.General{
		SalaryUSD: d.SalaryUSD, FXRateApplied: d.FXRateApplied, MorseFeeRate: d.MorseFeeRate,
		EmergencyMonths: d.EmergencyMonths, ExtraIncomeEstimateMXN: d.ExtraIncomeEstimateMXN,
		BudgetIncludesExtraIncome: d.BudgetIncludesExtraIncome, CycleStartDay: d.CycleStartDay,
		ExtraIncomeSplit: d.ExtraIncomeSplit, InvestmentAllocation: alloc,
	}
}

func categoriesFromDTO(in []categoryDTO) []domain.Category {
	out := make([]domain.Category, len(in))
	for i, c := range in {
		out[i] = domain.Category{Name: c.Name, Kind: ledger.Kind(c.Kind), Budget: c.Budget, Includes: c.Includes, Keywords: c.Keywords}
	}
	return out
}

func clientsFromDTO(in []clientDTO) []domain.Client {
	out := make([]domain.Client, len(in))
	for i, c := range in {
		out[i] = domain.Client{
			ID: c.ID, Name: c.Name, Currency: c.Currency, IVARate: c.IVARate, Type: c.Type, RFC: c.RFC,
			Regimen: c.Regimen, UsoCFDI: c.UsoCFDI, RetISRRate: c.RetISRRate, RetIVARate: c.RetIVARate,
			Concepto: c.Concepto, ClaveProdServ: c.ClaveProdServ, ClaveUnidad: c.ClaveUnidad, Address: c.Address,
			TaxResidence: c.TaxResidence, Contract: c.Contract, RealPayer: c.RealPayer, PostalCode: c.PostalCode,
		}
	}
	return out
}

func bracketsFromDTO(in []bracketDTO) []domain.Bracket {
	out := make([]domain.Bracket, len(in))
	for i, b := range in {
		out[i] = domain.Bracket{Upper: b.Upper, Rate: b.Rate}
	}
	return out
}

// --- handlers -----------------------------------------------------------

func (h *Handler) getAll(w http.ResponseWriter, r *http.Request) {
	h.read(w, r, "get", func(c domain.Config) any {
		return allDTO{
			General: toGeneralDTO(c), Categories: toCategoryDTOs(c.Categories), Clients: toClientDTOs(c.Clients),
			Instruments: toInstrumentsDTO(c), Brackets: toBracketDTOs(c.Brackets),
			PaymentMethods: toPaymentMethods(c.PaymentMethods), Issuer: toIssuerDTO(c.Issuer),
			InvestmentPause: toPauseDTO(c.Pause),
		}
	})
}

func (h *Handler) getGeneral(w http.ResponseWriter, r *http.Request) {
	h.read(w, r, "get general", func(c domain.Config) any { return toGeneralDTO(c) })
}

func (h *Handler) getCategories(w http.ResponseWriter, r *http.Request) {
	h.read(w, r, "get categories", func(c domain.Config) any { return toCategoryDTOs(c.Categories) })
}

func (h *Handler) getClients(w http.ResponseWriter, r *http.Request) {
	h.read(w, r, "get clients", func(c domain.Config) any { return toClientDTOs(c.Clients) })
}

func (h *Handler) getInstruments(w http.ResponseWriter, r *http.Request) {
	h.read(w, r, "get instruments", func(c domain.Config) any { return toInstrumentsDTO(c) })
}

func (h *Handler) getBrackets(w http.ResponseWriter, r *http.Request) {
	h.read(w, r, "get brackets", func(c domain.Config) any { return toBracketDTOs(c.Brackets) })
}

func (h *Handler) getPaymentMethods(w http.ResponseWriter, r *http.Request) {
	h.read(w, r, "get payment methods", func(c domain.Config) any { return toPaymentMethods(c.PaymentMethods) })
}

func (h *Handler) getIssuer(w http.ResponseWriter, r *http.Request) {
	h.read(w, r, "get issuer", func(c domain.Config) any { return toIssuerDTO(c.Issuer) })
}

func (h *Handler) getPause(w http.ResponseWriter, r *http.Request) {
	h.read(w, r, "get investment pause", func(c domain.Config) any { return toPauseDTO(c.Pause) })
}

func (h *Handler) putGeneral(w http.ResponseWriter, r *http.Request) {
	var req generalDTO
	if !decode(w, r, &req) {
		return
	}
	h.write(w, "put general", h.svc.UpdateGeneral(r.Context(), req.toDomain()))
}

func (h *Handler) putCategories(w http.ResponseWriter, r *http.Request) {
	var req []categoryDTO
	if !decode(w, r, &req) {
		return
	}
	h.write(w, "put categories", h.svc.UpdateCategories(r.Context(), categoriesFromDTO(req)))
}

func (h *Handler) putClients(w http.ResponseWriter, r *http.Request) {
	var req []clientDTO
	if !decode(w, r, &req) {
		return
	}
	h.write(w, "put clients", h.svc.UpdateClients(r.Context(), clientsFromDTO(req)))
}

func (h *Handler) putInstruments(w http.ResponseWriter, r *http.Request) {
	var req instrumentsDTO
	if !decode(w, r, &req) {
		return
	}
	instruments := make([]domain.Instrument, len(req.Instruments))
	for i, in := range req.Instruments {
		instruments[i] = domain.Instrument{ID: in.ID, Name: in.Name, Type: in.Type, Platform: in.Platform}
	}
	h.write(w, "put instruments", h.svc.UpdateInstruments(r.Context(), instruments, req.ByCategory))
}

func (h *Handler) putBrackets(w http.ResponseWriter, r *http.Request) {
	var req []bracketDTO
	if !decode(w, r, &req) {
		return
	}
	h.write(w, "put brackets", h.svc.UpdateBrackets(r.Context(), bracketsFromDTO(req)))
}

func (h *Handler) putPaymentMethods(w http.ResponseWriter, r *http.Request) {
	var req []string
	if !decode(w, r, &req) {
		return
	}
	h.write(w, "put payment methods", h.svc.UpdatePaymentMethods(r.Context(), req))
}

func (h *Handler) putIssuer(w http.ResponseWriter, r *http.Request) {
	var req issuerDTO
	if !decode(w, r, &req) {
		return
	}
	h.write(w, "put issuer", h.svc.UpdateIssuer(r.Context(), domain.Issuer{
		RFC: req.RFC, Name: req.Name, Regimen: req.Regimen, PostalCode: req.PostalCode, Note: req.Note,
	}))
}

func (h *Handler) putPause(w http.ResponseWriter, r *http.Request) {
	var req pauseDTO
	if !decode(w, r, &req) {
		return
	}
	h.write(w, "put investment pause", h.svc.UpdatePause(r.Context(), &domain.PausePlan{
		Months: req.Months, NormalBudget: req.NormalBudget, ResumeMonth: req.ResumeMonth,
		FutureExpensesPlan: req.Plan, Note: req.Note,
	}))
}

func (h *Handler) deletePause(w http.ResponseWriter, r *http.Request) {
	h.write(w, "delete investment pause", h.svc.UpdatePause(r.Context(), nil))
}

func (h *Handler) getBudgets(w http.ResponseWriter, r *http.Request) {
	budgets, err := h.svc.MonthBudgets(r.Context(), r.URL.Query().Get("month"))
	if err != nil {
		h.fail(w, "get budgets", err)
		return
	}
	out := make([]budgetDTO, len(budgets))
	for i, b := range budgets {
		out[i] = budgetDTO{Name: b.Name, Kind: b.Kind, Budget: b.Budget}
	}
	writeJSON(w, http.StatusOK, out)
}

// read loads the configuration and writes the projection of it.
func (h *Handler) read(w http.ResponseWriter, r *http.Request, op string, project func(domain.Config) any) {
	cfg, err := h.svc.Get(r.Context())
	if err != nil {
		h.fail(w, op, err)
		return
	}
	writeJSON(w, http.StatusOK, project(cfg))
}

// write answers a mutation: 204 on success, the mapped error otherwise.
func (h *Handler) write(w http.ResponseWriter, op string, err error) {
	if err != nil {
		h.fail(w, op, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) fail(w http.ResponseWriter, op string, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalid):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_settings", "message": err.Error()})
	case errors.Is(err, domain.ErrMissingConfig):
		writeError(w, http.StatusConflict, "settings_incomplete")
	default:
		h.logger.Error("settings request failed", "op", op, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error")
	}
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
