// Package monthclosehttp exposes the month close over HTTP: the preview of a
// period, storing it as an immutable snapshot, the history and discarding a
// stored close. Every route sits behind the authentication middleware passed to
// Register. Money and percentages travel as decimal strings.
package monthclosehttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/shopspring/decimal"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	"github.com/valium69mg/finances-app/backend/internal/monthclose/app"
	monthclose "github.com/valium69mg/finances-app/backend/internal/monthclose/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/httpjson"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

// Service is the set of use cases the handlers need.
type Service interface {
	Preview(ctx context.Context, period string) (app.Preview, error)
	Create(ctx context.Context, period string) (monthclose.Close, error)
	List(ctx context.Context) ([]monthclose.Close, error)
	Get(ctx context.Context, period string) (monthclose.Close, error)
	Delete(ctx context.Context, period string) error
}

// Handler serves the /month-close routes.
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

// Register mounts the month close routes on mux, each wrapped by requireAuth.
func (h *Handler) Register(mux *http.ServeMux, requireAuth func(http.Handler) http.Handler) {
	route := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, requireAuth(fn))
	}
	route("GET /month-close/preview", h.preview)
	route("POST /month-close", h.create)
	route("GET /month-close", h.list)
	route("GET /month-close/{period}", h.get)
	route("DELETE /month-close/{period}", h.delete)
}

// --- DTOs ---------------------------------------------------------------

// categoryDTO is one Gasto category. budget and remaining are null when the
// category has no budget for the month.
type categoryDTO struct {
	Category   string           `json:"category"`
	Spent      decimal.Decimal  `json:"spent"`
	Budget     *decimal.Decimal `json:"budget"`
	Remaining  *decimal.Decimal `json:"remaining"`
	OverBudget bool             `json:"over_budget"`
}

type emergencyDTO struct {
	Accumulated decimal.Decimal `json:"accumulated"`
	Goal        decimal.Decimal `json:"goal"`
}

// suggestionDTO splits a positive leftover. The remainder after the emergency
// fund goes to investments, or to future_expenses while investments_paused.
type suggestionDTO struct {
	ToEmergencyFund   decimal.Decimal `json:"to_emergency_fund"`
	ToInvestments     decimal.Decimal `json:"to_investments"`
	ToFutureExpenses  decimal.Decimal `json:"to_future_expenses"`
	InvestmentsPaused bool            `json:"investments_paused"`
}

// adjustmentDTO hints a budget change: deviation_pct is signed, in percent.
type adjustmentDTO struct {
	Category     string          `json:"category"`
	Kind         string          `json:"kind"`
	Budget       decimal.Decimal `json:"budget"`
	Real         decimal.Decimal `json:"real"`
	DeviationPct decimal.Decimal `json:"deviation_pct"`
}

// closeDTO is a month close. closed_at is null on a preview and
// tax_filing_status (ninguna, pendiente, pagada) when it was not computed.
// suggestion is null without a positive leftover.
type closeDTO struct {
	Period          string          `json:"period"`
	ClosedAt        *string         `json:"closed_at"`
	Categories      []categoryDTO   `json:"categories"`
	Income          decimal.Decimal `json:"income"`
	Expenses        decimal.Decimal `json:"expenses"`
	Savings         decimal.Decimal `json:"savings"`
	Available       decimal.Decimal `json:"available"`
	Emergency       emergencyDTO    `json:"emergency"`
	Suggestion      *suggestionDTO  `json:"suggestion"`
	Adjustments     []adjustmentDTO `json:"adjustments"`
	TaxFilingStatus *string         `json:"tax_filing_status"`
}

// previewDTO is the body of GET /month-close/preview: the close computed now
// and the stored close of the period, if any.
type previewDTO struct {
	Preview  closeDTO  `json:"preview"`
	Existing *closeDTO `json:"existing"`
}

type createRequest struct {
	Period string `json:"period"`
}

func toCloseDTO(c monthclose.Close) closeDTO {
	out := closeDTO{
		Period:      c.Period,
		Categories:  make([]categoryDTO, len(c.Categories)),
		Income:      c.Income,
		Expenses:    c.Expenses,
		Savings:     c.Savings,
		Available:   c.Available,
		Emergency:   emergencyDTO{Accumulated: c.Emergency.Accumulated, Goal: c.Emergency.Goal},
		Adjustments: make([]adjustmentDTO, len(c.Adjustments)),
	}
	if !c.ClosedAt.IsZero() {
		s := c.ClosedAt.UTC().Format(time.RFC3339)
		out.ClosedAt = &s
	}
	if c.FilingStatus != "" {
		s := string(c.FilingStatus)
		out.TaxFilingStatus = &s
	}
	for i, x := range c.Categories {
		out.Categories[i] = categoryDTO{Category: x.Name, Spent: x.Spent, Budget: x.Budget, Remaining: x.Remaining, OverBudget: x.OverBudget}
	}
	if s := c.Suggestion; s != nil {
		out.Suggestion = &suggestionDTO{
			ToEmergencyFund: s.ToEmergencyFund, ToInvestments: s.ToInvestments,
			ToFutureExpenses: s.ToFutureExpenses, InvestmentsPaused: s.InvestmentsPaused,
		}
	}
	for i, a := range c.Adjustments {
		out.Adjustments[i] = adjustmentDTO{Category: a.Name, Kind: string(a.Kind), Budget: a.Budget, Real: a.Real, DeviationPct: a.DeviationPct}
	}
	return out
}

// --- handlers -----------------------------------------------------------

func (h *Handler) preview(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Preview(r.Context(), r.URL.Query().Get("period"))
	if err != nil {
		h.fail(w, "preview", err)
		return
	}
	out := previewDTO{Preview: toCloseDTO(res.Close)}
	if res.Existing != nil {
		e := toCloseDTO(*res.Existing)
		out.Existing = &e
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	c, err := h.svc.Create(r.Context(), req.Period)
	if err != nil {
		h.fail(w, "create", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, toCloseDTO(c))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.List(r.Context())
	if err != nil {
		h.fail(w, "list", err)
		return
	}
	out := make([]closeDTO, len(list))
	for i, c := range list {
		out[i] = toCloseDTO(c)
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.Get(r.Context(), r.PathValue("period"))
	if err != nil {
		h.fail(w, "get", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toCloseDTO(c))
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), r.PathValue("period")); err != nil {
		h.fail(w, "delete", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) fail(w http.ResponseWriter, op string, err error) {
	reply := func(status int, code string) {
		httpjson.WriteJSON(w, status, map[string]string{"error": code, "message": err.Error()})
	}
	switch {
	case errors.Is(err, monthclose.ErrInvalidInput), errors.Is(err, ledger.ErrInvalid), errors.Is(err, settings.ErrInvalid):
		reply(http.StatusBadRequest, "invalid_close")
	case errors.Is(err, monthclose.ErrAlreadyClosed):
		reply(http.StatusConflict, "already_closed")
	case errors.Is(err, monthclose.ErrNotFound):
		httpjson.WriteError(w, http.StatusNotFound, "not_found")
	case errors.Is(err, settings.ErrMissingConfig):
		reply(http.StatusUnprocessableEntity, "settings_incomplete")
	default:
		h.logger.Error("month close request failed", "op", op, "error", err)
		httpjson.WriteError(w, http.StatusInternalServerError, "internal_error")
	}
}
