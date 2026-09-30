// Package incomehttp exposes the income use cases over HTTP. Every route sits
// behind the authentication middleware passed to Register. Money and rates
// travel as decimal strings.
package incomehttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/income/app"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/httpjson"
)

const maxListLimit = 200

// Service is the set of use cases the handlers need.
type Service interface {
	Create(ctx context.Context, in app.Input) (app.Result, error)
	Update(ctx context.Context, id int, in app.Input) (app.Result, error)
	Delete(ctx context.Context, id int) error
	List(ctx context.Context, month string, limit int) ([]ledger.Movement, error)
	InferCategory(ctx context.Context, description string) (string, bool, error)
}

// Handler serves the /income routes.
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

// Register mounts the income routes on mux, each wrapped by requireAuth.
func (h *Handler) Register(mux *http.ServeMux, requireAuth func(http.Handler) http.Handler) {
	route := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, requireAuth(fn))
	}
	route("POST /income", h.create)
	route("GET /income", h.list)
	route("GET /income/infer-category", h.inferCategory)
	route("PUT /income/{id}", h.update)
	route("DELETE /income/{id}", h.delete)
}

// --- DTOs ---------------------------------------------------------------

// incomeRequest is the body of POST and PUT. Only amount is required: an empty
// category is inferred from the description, the payment method defaults to
// Transferencia, the currency to MXN, the date to today and the USD rate to the
// configured fx_rate_applied.
type incomeRequest struct {
	Date          string           `json:"date"`
	Description   string           `json:"description"`
	Category      string           `json:"category"`
	PaymentMethod string           `json:"payment_method"`
	Currency      string           `json:"currency"`
	Amount        decimal.Decimal  `json:"amount"`
	ExchangeRate  *decimal.Decimal `json:"exchange_rate"`
}

type incomeDTO struct {
	ID            int              `json:"id"`
	Date          string           `json:"date"`
	Description   string           `json:"description"`
	Category      string           `json:"category"`
	PaymentMethod string           `json:"payment_method"`
	Currency      string           `json:"currency"`
	Amount        decimal.Decimal  `json:"amount"`
	ExchangeRate  *decimal.Decimal `json:"exchange_rate"`
	AmountMXN     decimal.Decimal  `json:"amount_mxn"`
}

type resicoDTO struct {
	Rate          decimal.Decimal  `json:"rate"`
	EstimatedISR  decimal.Decimal  `json:"estimated_isr"`
	RateIncreased bool             `json:"rate_increased"`
	PreviousRate  *decimal.Decimal `json:"previous_rate"`
}

type summaryDTO struct {
	Month         string          `json:"month"`
	MonthTotalMXN decimal.Decimal `json:"month_total_mxn"`
	Resico        *resicoDTO      `json:"resico"`
}

type shareDTO struct {
	Instrument string          `json:"instrument"`
	Amount     decimal.Decimal `json:"amount"`
}

type splitDTO struct {
	SATReserve        decimal.Decimal `json:"sat_reserve"`
	EmergencyFund     decimal.Decimal `json:"emergency_fund"`
	Investments       decimal.Decimal `json:"investments"`
	AguinaldoVacation decimal.Decimal `json:"aguinaldo_vacation"`
	GoalReached       bool            `json:"goal_reached"`
	// InvestmentBreakdown divides investments by instrument; empty when no
	// investment allocation is configured.
	InvestmentBreakdown []shareDTO `json:"investment_breakdown"`
}

type resultDTO struct {
	Income  incomeDTO  `json:"income"`
	Summary summaryDTO `json:"summary"`
	Split   *splitDTO  `json:"split"`
}

type inferDTO struct {
	Category *string `json:"category"`
}

func (r incomeRequest) toInput() app.Input {
	return app.Input{
		Date: r.Date, Description: r.Description, Category: r.Category, PaymentMethod: r.PaymentMethod,
		Currency: r.Currency, Amount: r.Amount, ExchangeRate: r.ExchangeRate,
	}
}

func toIncomeDTO(m ledger.Movement) incomeDTO {
	return incomeDTO{
		ID: m.ID, Date: m.Date, Description: m.Description, Category: m.Category, PaymentMethod: m.PaymentMethod,
		Currency: m.Currency, Amount: m.Amount, ExchangeRate: m.ExchangeRate, AmountMXN: m.AmountMXN,
	}
}

func toResultDTO(r app.Result) resultDTO {
	out := resultDTO{
		Income:  toIncomeDTO(r.Movement),
		Summary: summaryDTO{Month: r.Summary.Month, MonthTotalMXN: r.Summary.MonthTotalMXN},
	}
	if e := r.Summary.Resico; e != nil {
		out.Summary.Resico = &resicoDTO{Rate: e.Rate, EstimatedISR: e.EstimatedISR, RateIncreased: e.RateIncreased, PreviousRate: e.PreviousRate}
	}
	if s := r.Split; s != nil {
		out.Split = &splitDTO{
			SATReserve: s.SATReserve, EmergencyFund: s.EmergencyFund, Investments: s.Investments,
			AguinaldoVacation: s.AguinaldoVacation, GoalReached: s.GoalReached,
			InvestmentBreakdown: make([]shareDTO, len(s.Breakdown)),
		}
		for i, sh := range s.Breakdown {
			out.Split.InvestmentBreakdown[i] = shareDTO{Instrument: sh.Key, Amount: sh.Amount}
		}
	}
	return out
}

// --- handlers -----------------------------------------------------------

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req incomeRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	res, err := h.svc.Create(r.Context(), req.toInput())
	if err != nil {
		h.fail(w, "create", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, toResultDTO(res))
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	var req incomeRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	res, err := h.svc.Update(r.Context(), id, req.toInput())
	if err != nil {
		h.fail(w, "update", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toResultDTO(res))
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		h.fail(w, "delete", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	limit, ok := httpjson.ListLimit(w, r, maxListLimit)
	if !ok {
		return
	}
	movements, err := h.svc.List(r.Context(), r.URL.Query().Get("month"), limit)
	if err != nil {
		h.fail(w, "list", err)
		return
	}
	out := make([]incomeDTO, len(movements))
	for i, m := range movements {
		out[i] = toIncomeDTO(m)
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) inferCategory(w http.ResponseWriter, r *http.Request) {
	name, ok, err := h.svc.InferCategory(r.Context(), r.URL.Query().Get("description"))
	if err != nil {
		h.fail(w, "infer category", err)
		return
	}
	out := inferDTO{}
	if ok {
		out.Category = &name
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) fail(w http.ResponseWriter, op string, err error) {
	switch {
	case errors.Is(err, ledger.ErrInvalid):
		httpjson.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_income", "message": err.Error()})
	case errors.Is(err, ledger.ErrNotFound):
		httpjson.WriteError(w, http.StatusNotFound, "not_found")
	default:
		h.logger.Error("income request failed", "op", op, "error", err)
		httpjson.WriteError(w, http.StatusInternalServerError, "internal_error")
	}
}
