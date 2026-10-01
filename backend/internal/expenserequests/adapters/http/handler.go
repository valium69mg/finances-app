// Package expenserequestshttp exposes the expense requests over HTTP. Every
// route sits behind the authentication middleware passed to Register. The
// create, list, cancel and categories routes are on the household allowlist
// (internal/auth/adapters/http); the budget check, approve, reject and revert
// routes are owner-only by default deny, and the service checks the role again.
// Money travels as decimal strings, dates as YYYY-MM-DD.
package expenserequestshttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/expenserequests/app"
	"github.com/valium69mg/finances-app/backend/internal/expenserequests/domain"
	expensesapp "github.com/valium69mg/finances-app/backend/internal/expenses/app"
	futuredomain "github.com/valium69mg/finances-app/backend/internal/futureexpenses/domain"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/httpjson"
	"github.com/valium69mg/finances-app/backend/internal/platform/httpmw"
	"github.com/valium69mg/finances-app/backend/internal/platform/session"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

// Service is the set of use cases the handlers need.
type Service interface {
	Create(ctx context.Context, actor session.Identity, in domain.Input) (domain.Request, error)
	List(ctx context.Context, actor session.Identity, status string) ([]domain.Request, error)
	Categories(ctx context.Context, actor session.Identity) ([]string, error)
	Cancel(ctx context.Context, actor session.Identity, id int) (domain.Request, error)
	BudgetCheck(ctx context.Context, actor session.Identity, id int, category, date string) (domain.BudgetCheck, error)
	Approve(ctx context.Context, actor session.Identity, id int, in app.ApproveInput) (app.ApproveResult, error)
	Reject(ctx context.Context, actor session.Identity, id int, comment string) (domain.Request, error)
	Revert(ctx context.Context, actor session.Identity, id int) (domain.Request, error)
}

// Handler serves the /expense-requests routes.
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

// Register mounts the routes on mux, each wrapped by requireAuth.
func (h *Handler) Register(mux httpmw.Router, requireAuth func(http.Handler) http.Handler) {
	route := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, requireAuth(fn))
	}
	route("POST /expense-requests", h.create)
	route("GET /expense-requests", h.list)
	route("GET /expense-requests/categories", h.categories)
	route("POST /expense-requests/{id}/cancel", h.cancel)
	route("GET /expense-requests/{id}/budget-check", h.budgetCheck)
	route("POST /expense-requests/{id}/approve", h.approve)
	route("POST /expense-requests/{id}/reject", h.reject)
	route("POST /expense-requests/{id}/revert", h.revert)
}

// --- DTOs ---------------------------------------------------------------

// createRequest is the body of POST /expense-requests: amount (MXN, at most 2
// decimals) and description are required; suggested_category (a Gasto
// category) is optional and date defaults to today.
type createRequest struct {
	Amount            decimal.Decimal `json:"amount"`
	Description       string          `json:"description"`
	SuggestedCategory string          `json:"suggested_category"`
	Date              string          `json:"date"`
}

// approveRequest is the body of POST /expense-requests/{id}/approve. For the
// destination "gasto": category (required), date (defaults to the date of the
// request) and payment_method (defaults to Débito). For "gasto_futuro":
// due_date (required).
type approveRequest struct {
	Destination   domain.Destination `json:"destination"`
	Category      string             `json:"category"`
	Date          string             `json:"date"`
	PaymentMethod string             `json:"payment_method"`
	DueDate       string             `json:"due_date"`
}

type rejectRequest struct {
	Comment string `json:"comment"`
}

type requestDTO struct {
	ID                    int             `json:"id"`
	RequesterEmail        string          `json:"requester_email"`
	Amount                decimal.Decimal `json:"amount"`
	Description           string          `json:"description"`
	SuggestedCategory     *string         `json:"suggested_category"`
	ExpenseDate           string          `json:"expense_date"`
	Status                domain.Status   `json:"status"`
	DecisionComment       *string         `json:"decision_comment"`
	DecidedAt             *time.Time      `json:"decided_at"`
	ResultKind            *string         `json:"result_kind"`
	ResultMovementID      *int            `json:"result_movement_id"`
	ResultFutureExpenseID *int            `json:"result_future_expense_id"`
	CreatedAt             time.Time       `json:"created_at"`
	RevertCount           int             `json:"revert_count"`
	RevertedAt            *time.Time      `json:"reverted_at"`
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func toDTO(r domain.Request) requestDTO {
	return requestDTO{
		ID: r.ID, RequesterEmail: r.RequesterEmail, Amount: r.Amount, Description: r.Description,
		SuggestedCategory: optional(r.SuggestedCategory), ExpenseDate: r.ExpenseDate, Status: r.Status,
		DecisionComment: optional(r.DecisionComment), DecidedAt: r.DecidedAt, ResultKind: optional(string(r.ResultKind)),
		ResultMovementID: r.ResultMovementID, ResultFutureExpenseID: r.ResultFutureExpenseID, CreatedAt: r.CreatedAt,
		RevertCount: r.RevertCount, RevertedAt: r.RevertedAt,
	}
}

// budgetCheckDTO: every figure is a decimal string with 2 decimals. budget,
// remaining, projected_remaining and fits are null when the category has no
// budget ("Sin presupuesto"); over_by is the positive excess, else "0.00".
type budgetCheckDTO struct {
	Category           string  `json:"category"`
	Budget             *string `json:"budget"`
	Spent              string  `json:"spent"`
	Remaining          *string `json:"remaining"`
	Amount             string  `json:"amount"`
	ProjectedSpent     string  `json:"projected_spent"`
	ProjectedRemaining *string `json:"projected_remaining"`
	Fits               *bool   `json:"fits"`
	OverBy             string  `json:"over_by"`
}

func fixed(d decimal.Decimal) string { return d.StringFixed(2) }

func fixedPtr(d *decimal.Decimal) *string {
	if d == nil {
		return nil
	}
	s := fixed(*d)
	return &s
}

func toBudgetCheckDTO(c domain.BudgetCheck) budgetCheckDTO {
	return budgetCheckDTO{
		Category: c.Category, Budget: fixedPtr(c.Budget), Spent: fixed(c.Spent), Remaining: fixedPtr(c.Remaining),
		Amount: fixed(c.Amount), ProjectedSpent: fixed(c.ProjectedSpent), ProjectedRemaining: fixedPtr(c.ProjectedRemaining),
		Fits: c.Fits, OverBy: fixed(c.OverBy),
	}
}

// budgetFeedbackDTO echoes the feedback of the expenses module after the Gasto
// was registered (the same shape as POST /expenses).
type budgetFeedbackDTO struct {
	Month      string           `json:"month"`
	Category   string           `json:"category"`
	Budget     *decimal.Decimal `json:"budget"`
	Spent      decimal.Decimal  `json:"spent"`
	Remaining  *decimal.Decimal `json:"remaining"`
	OverBudget bool             `json:"over_budget"`
}

type approveDTO struct {
	Request requestDTO         `json:"request"`
	Budget  *budgetFeedbackDTO `json:"budget"`
}

func toApproveDTO(r app.ApproveResult) approveDTO {
	out := approveDTO{Request: toDTO(r.Request)}
	if fb := r.Feedback; fb != nil {
		out.Budget = feedbackDTO(*fb)
	}
	return out
}

func feedbackDTO(fb expensesapp.BudgetFeedback) *budgetFeedbackDTO {
	return &budgetFeedbackDTO{Month: fb.Month, Category: fb.Category, Budget: fb.Budget, Spent: fb.Spent, Remaining: fb.Remaining, OverBudget: fb.OverBudget}
}

// --- handlers -----------------------------------------------------------

func actor(r *http.Request) session.Identity {
	id, _ := session.From(r.Context())
	return id
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	got, err := h.svc.Create(r.Context(), actor(r), domain.Input{
		Amount: req.Amount, Description: req.Description, SuggestedCategory: req.SuggestedCategory, Date: req.Date,
	})
	if err != nil {
		h.fail(w, "create", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, toDTO(got))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	got, err := h.svc.List(r.Context(), actor(r), r.URL.Query().Get("status"))
	if err != nil {
		h.fail(w, "list", err)
		return
	}
	out := make([]requestDTO, len(got))
	for i, req := range got {
		out[i] = toDTO(req)
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) categories(w http.ResponseWriter, r *http.Request) {
	names, err := h.svc.Categories(r.Context(), actor(r))
	if err != nil {
		h.fail(w, "categories", err)
		return
	}
	if names == nil {
		names = []string{}
	}
	httpjson.WriteJSON(w, http.StatusOK, names)
}

func (h *Handler) cancel(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	got, err := h.svc.Cancel(r.Context(), actor(r), id)
	if err != nil {
		h.fail(w, "cancel", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toDTO(got))
}

func (h *Handler) budgetCheck(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	got, err := h.svc.BudgetCheck(r.Context(), actor(r), id, q.Get("category"), q.Get("date"))
	if err != nil {
		h.fail(w, "budget check", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpjson.WriteJSON(w, http.StatusOK, toBudgetCheckDTO(got))
}

func (h *Handler) approve(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	var req approveRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	got, err := h.svc.Approve(r.Context(), actor(r), id, app.ApproveInput{
		Destination: req.Destination, Category: req.Category, Date: req.Date, PaymentMethod: req.PaymentMethod, DueDate: req.DueDate,
	})
	if err != nil {
		h.fail(w, "approve", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toApproveDTO(got))
}

func (h *Handler) reject(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	var req rejectRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	got, err := h.svc.Reject(r.Context(), actor(r), id, req.Comment)
	if err != nil {
		h.fail(w, "reject", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toDTO(got))
}

func (h *Handler) revert(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	got, err := h.svc.Revert(r.Context(), actor(r), id)
	if err != nil {
		h.fail(w, "revert", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toDTO(got))
}

// fail maps the use case errors to the JSON envelope; every unmapped error is
// logged and answered with a generic internal_error.
func (h *Handler) fail(w http.ResponseWriter, op string, err error) {
	reply := func(status int, code string) {
		httpjson.WriteJSON(w, status, map[string]string{"error": code, "message": err.Error()})
	}
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		reply(http.StatusBadRequest, "invalid_expense_request")
	case errors.Is(err, futuredomain.ErrInvalidInput):
		reply(http.StatusBadRequest, "invalid_future_expense")
	case errors.Is(err, ledger.ErrInvalid):
		reply(http.StatusBadRequest, "invalid_expense")
	case errors.Is(err, domain.ErrForbidden):
		httpjson.WriteError(w, http.StatusForbidden, "forbidden")
	case errors.Is(err, domain.ErrNotFound):
		httpjson.WriteError(w, http.StatusNotFound, "not_found")
	case errors.Is(err, domain.ErrInvalidState):
		httpjson.WriteError(w, http.StatusConflict, "invalid_state")
	case errors.Is(err, domain.ErrFutureExpensePaid):
		reply(http.StatusConflict, "future_expense_paid")
	case errors.Is(err, domain.ErrRateLimited):
		w.Header().Set("Retry-After", "3600")
		httpjson.WriteError(w, http.StatusTooManyRequests, "rate_limited")
	case errors.Is(err, settings.ErrMissingConfig):
		reply(http.StatusUnprocessableEntity, "settings_incomplete")
	default:
		h.logger.Error("expense requests request failed", "op", op, "error", err)
		httpjson.WriteError(w, http.StatusInternalServerError, "internal_error")
	}
}
