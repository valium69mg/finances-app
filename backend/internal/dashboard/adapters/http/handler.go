// Package dashboardhttp exposes the read-only dashboard over HTTP. The route
// sits behind the authentication middleware passed to Register. Money and
// rates travel as decimal strings.
package dashboardhttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/shopspring/decimal"

	dashboard "github.com/valium69mg/finances-app/backend/internal/dashboard/domain"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/httpjson"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

// Service is the use case the handler needs.
type Service interface {
	Month(ctx context.Context, month string) (dashboard.Overview, error)
}

// Handler serves the /dashboard route.
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

// Register mounts the dashboard route on mux, wrapped by requireAuth.
func (h *Handler) Register(mux *http.ServeMux, requireAuth func(http.Handler) http.Handler) {
	mux.Handle("GET /dashboard", requireAuth(http.HandlerFunc(h.get)))
}

// --- DTOs ---------------------------------------------------------------

// categoryDTO is one Gasto category. Budget and Remaining are null when the
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

// taxDTO is the estimated RESICO ISR of the month. It has no payment status.
type taxDTO struct {
	Rate         decimal.Decimal `json:"rate"`
	EstimatedISR decimal.Decimal `json:"estimated_isr"`
}

// dashboardDTO is the body of GET /dashboard. Tax is null when the tax
// settings are incomplete.
type dashboardDTO struct {
	Month      string          `json:"month"`
	Categories []categoryDTO   `json:"categories"`
	Income     decimal.Decimal `json:"income"`
	Expenses   decimal.Decimal `json:"expenses"`
	Savings    decimal.Decimal `json:"savings"`
	Available  decimal.Decimal `json:"available"`
	Emergency  emergencyDTO    `json:"emergency"`
	Tax        *taxDTO         `json:"tax"`
}

func toDTO(d dashboard.Overview) dashboardDTO {
	out := dashboardDTO{
		Month:      d.Month,
		Categories: make([]categoryDTO, len(d.Rows)),
		Income:     d.Totals.Income,
		Expenses:   d.Totals.Expenses,
		Savings:    d.Totals.Savings,
		Available:  d.Available,
		Emergency:  emergencyDTO{Accumulated: d.Emergency.Accumulated, Goal: d.Emergency.Goal},
	}
	for i, r := range d.Rows {
		out.Categories[i] = categoryDTO{Category: r.Name, Spent: r.Real, Budget: r.Budget, Remaining: r.Diff, OverBudget: r.OverBudget()}
	}
	if d.Tax != nil {
		out.Tax = &taxDTO{Rate: d.Tax.Rate, EstimatedISR: d.Tax.EstimatedISR}
	}
	return out
}

// --- handlers -----------------------------------------------------------

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	d, err := h.svc.Month(r.Context(), r.URL.Query().Get("month"))
	if err != nil {
		h.fail(w, err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toDTO(d))
}

func (h *Handler) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ledger.ErrInvalid):
		httpjson.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_dashboard", "message": err.Error()})
	case errors.Is(err, settings.ErrMissingConfig):
		httpjson.WriteJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "settings_incomplete", "message": err.Error()})
	default:
		h.logger.Error("dashboard request failed", "error", err)
		httpjson.WriteError(w, http.StatusInternalServerError, "internal_error")
	}
}
