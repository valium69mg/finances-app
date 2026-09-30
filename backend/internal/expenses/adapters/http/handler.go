// Package expenseshttp exposes the expenses use cases over HTTP. Every route
// sits behind the authentication middleware passed to Register. Money and
// rates travel as decimal strings.
package expenseshttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/expenses/app"
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

// Handler serves the /expenses routes.
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

// Register mounts the expenses routes on mux, each wrapped by requireAuth.
func (h *Handler) Register(mux *http.ServeMux, requireAuth func(http.Handler) http.Handler) {
	route := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, requireAuth(fn))
	}
	route("POST /expenses", h.create)
	route("GET /expenses", h.list)
	route("GET /expenses/infer-category", h.inferCategory)
	route("PUT /expenses/{id}", h.update)
	route("DELETE /expenses/{id}", h.delete)
}

// --- DTOs ---------------------------------------------------------------

// expenseRequest is the body of POST and PUT. Only amount is required: an
// empty category is inferred from the description, the payment method defaults
// to Débito, the currency to MXN, the date to today and the USD rate to the
// configured fx_rate_applied.
type expenseRequest struct {
	Date          string           `json:"date"`
	Description   string           `json:"description"`
	Category      string           `json:"category"`
	PaymentMethod string           `json:"payment_method"`
	Currency      string           `json:"currency"`
	Amount        decimal.Decimal  `json:"amount"`
	ExchangeRate  *decimal.Decimal `json:"exchange_rate"`
}

type expenseDTO struct {
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

type budgetDTO struct {
	Month      string           `json:"month"`
	Category   string           `json:"category"`
	Budget     *decimal.Decimal `json:"budget"`
	Spent      decimal.Decimal  `json:"spent"`
	Remaining  *decimal.Decimal `json:"remaining"`
	OverBudget bool             `json:"over_budget"`
}

type resultDTO struct {
	Expense expenseDTO `json:"expense"`
	Budget  *budgetDTO `json:"budget"`
}

type inferDTO struct {
	Category *string `json:"category"`
}

func (r expenseRequest) toInput() app.Input {
	return app.Input{
		Date: r.Date, Description: r.Description, Category: r.Category, PaymentMethod: r.PaymentMethod,
		Currency: r.Currency, Amount: r.Amount, ExchangeRate: r.ExchangeRate,
	}
}

func toExpenseDTO(m ledger.Movement) expenseDTO {
	return expenseDTO{
		ID: m.ID, Date: m.Date, Description: m.Description, Category: m.Category, PaymentMethod: m.PaymentMethod,
		Currency: m.Currency, Amount: m.Amount, ExchangeRate: m.ExchangeRate, AmountMXN: m.AmountMXN,
	}
}

func toResultDTO(r app.Result) resultDTO {
	out := resultDTO{Expense: toExpenseDTO(r.Movement)}
	if fb := r.Feedback; fb != nil {
		out.Budget = &budgetDTO{
			Month: fb.Month, Category: fb.Category, Budget: fb.Budget, Spent: fb.Spent,
			Remaining: fb.Remaining, OverBudget: fb.OverBudget,
		}
	}
	return out
}

// --- handlers -----------------------------------------------------------

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req expenseRequest
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
	var req expenseRequest
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
	out := make([]expenseDTO, len(movements))
	for i, m := range movements {
		out[i] = toExpenseDTO(m)
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
		httpjson.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_expense", "message": err.Error()})
	case errors.Is(err, ledger.ErrNotFound):
		httpjson.WriteError(w, http.StatusNotFound, "not_found")
	default:
		h.logger.Error("expenses request failed", "op", op, "error", err)
		httpjson.WriteError(w, http.StatusInternalServerError, "internal_error")
	}
}
