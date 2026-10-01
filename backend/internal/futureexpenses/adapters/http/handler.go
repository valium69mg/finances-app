// Package futureexpenseshttp exposes the future expenses use cases over HTTP.
// Every route sits behind the authentication middleware passed to Register.
// Money travels as decimal strings, dates as YYYY-MM-DD.
package futureexpenseshttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/futureexpenses/app"
	domain "github.com/valium69mg/finances-app/backend/internal/futureexpenses/domain"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/httpjson"
	"github.com/valium69mg/finances-app/backend/internal/platform/httpmw"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

// Service is the set of use cases the handlers need.
type Service interface {
	Create(ctx context.Context, in domain.Input) (domain.Planned, error)
	Get(ctx context.Context, id int) (domain.Planned, error)
	Update(ctx context.Context, id int, in domain.Input) (domain.Planned, error)
	Delete(ctx context.Context, id int) error
	List(ctx context.Context) (app.Listing, error)
	Contribute(ctx context.Context, id int, in app.SavingInput) (domain.Planned, ledger.Movement, error)
	Assign(ctx context.Context, id int, in app.AssignInput) (domain.Planned, error)
	Pay(ctx context.Context, id int, in app.PayInput) (domain.Planned, ledger.Movement, error)
}

// Handler serves the /future-expenses routes.
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
	route("POST /future-expenses", h.create)
	route("GET /future-expenses", h.list)
	route("GET /future-expenses/{id}", h.get)
	route("PUT /future-expenses/{id}", h.update)
	route("DELETE /future-expenses/{id}", h.delete)
	route("POST /future-expenses/{id}/savings", h.contribute)
	route("POST /future-expenses/{id}/assign", h.assign)
	route("POST /future-expenses/{id}/pay", h.pay)
}

// --- DTOs ---------------------------------------------------------------

// itemRequest is the body of POST and PUT: name, target_amount (MXN, at most 2
// decimals) and due_date are required.
type itemRequest struct {
	Name         string          `json:"name"`
	TargetAmount decimal.Decimal `json:"target_amount"`
	DueDate      string          `json:"due_date"`
}

// savingRequest is the body of POST /future-expenses/{id}/savings. date
// defaults to today and description to "Ahorro para <name>".
type savingRequest struct {
	Amount      decimal.Decimal `json:"amount"`
	Date        string          `json:"date"`
	Description string          `json:"description"`
}

// assignRequest is the body of POST /future-expenses/{id}/assign.
type assignRequest struct {
	Amount decimal.Decimal `json:"amount"`
	Date   string          `json:"date"`
}

// payRequest is the body of POST /future-expenses/{id}/pay. Every field is
// optional: date defaults to today, amount (what was actually paid) to the
// target and category to the one inferred from the name, else Otros.
type payRequest struct {
	Date     string           `json:"date"`
	Amount   *decimal.Decimal `json:"amount"`
	Category string           `json:"category"`
}

// itemDTO is an item with its plan. saved is the net of the savings linked to
// it; remaining, suggested_monthly and cycles_left are zero for a paid item,
// which carries paid_at, amount_paid and expense_movement_id instead.
type itemDTO struct {
	ID                int              `json:"id"`
	Name              string           `json:"name"`
	TargetAmount      decimal.Decimal  `json:"target_amount"`
	DueDate           string           `json:"due_date"`
	Status            string           `json:"status"`
	Saved             decimal.Decimal  `json:"saved"`
	Remaining         decimal.Decimal  `json:"remaining"`
	SuggestedMonthly  decimal.Decimal  `json:"suggested_monthly"`
	CyclesLeft        int              `json:"cycles_left"`
	PaidAt            *string          `json:"paid_at"`
	AmountPaid        *decimal.Decimal `json:"amount_paid"`
	ExpenseMovementID *int             `json:"expense_movement_id"`
	CreatedAt         string           `json:"created_at"`
	UpdatedAt         string           `json:"updated_at"`
}

type totalsDTO struct {
	Target           decimal.Decimal `json:"target"`
	Saved            decimal.Decimal `json:"saved"`
	Remaining        decimal.Decimal `json:"remaining"`
	SuggestedMonthly decimal.Decimal `json:"suggested_monthly"`
}

// listDTO is the body of GET /future-expenses: the active items (earliest due
// date first) with their totals, the paid ones (latest first) and the free
// balance, the Gastos futuros savings linked to no item.
type listDTO struct {
	Active      []itemDTO       `json:"active"`
	Paid        []itemDTO       `json:"paid"`
	Totals      totalsDTO       `json:"totals"`
	FreeBalance decimal.Decimal `json:"free_balance"`
}

// movementDTO is a movement registered by a saving or a payment.
type movementDTO struct {
	ID          int             `json:"id"`
	Date        string          `json:"date"`
	Description string          `json:"description"`
	Category    string          `json:"category"`
	Amount      decimal.Decimal `json:"amount"`
	AmountMXN   decimal.Decimal `json:"amount_mxn"`
}

type savingResultDTO struct {
	Item   itemDTO     `json:"item"`
	Saving movementDTO `json:"saving"`
}

type payResultDTO struct {
	Item    itemDTO     `json:"item"`
	Expense movementDTO `json:"expense"`
}

const timeFormat = "2006-01-02T15:04:05Z"

func toItemDTO(p domain.Planned) itemDTO {
	dto := itemDTO{
		ID: p.ID, Name: p.Name, TargetAmount: p.Target, DueDate: p.DueDate, Status: string(p.Status), Saved: p.Saved,
		Remaining: p.Remaining, SuggestedMonthly: p.Suggested, CyclesLeft: p.CyclesLeft, AmountPaid: p.AmountPaid,
		ExpenseMovementID: p.ExpenseMovementID,
		CreatedAt:         p.CreatedAt.UTC().Format(timeFormat), UpdatedAt: p.UpdatedAt.UTC().Format(timeFormat),
	}
	if p.PaidAt != "" {
		paid := p.PaidAt
		dto.PaidAt = &paid
	}
	return dto
}

func toItemDTOs(list []domain.Planned) []itemDTO {
	out := make([]itemDTO, len(list))
	for i, p := range list {
		out[i] = toItemDTO(p)
	}
	return out
}

func toMovementDTO(m ledger.Movement) movementDTO {
	return movementDTO{ID: m.ID, Date: m.Date, Description: m.Description, Category: m.Category, Amount: m.Amount, AmountMXN: m.AmountMXN}
}

// --- handlers -----------------------------------------------------------

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req itemRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	p, err := h.svc.Create(r.Context(), domain.Input{Name: req.Name, Target: req.TargetAmount, DueDate: req.DueDate})
	if err != nil {
		h.fail(w, "create", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, toItemDTO(p))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	l, err := h.svc.List(r.Context())
	if err != nil {
		h.fail(w, "list", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, listDTO{
		Active: toItemDTOs(l.Plan.Items), Paid: toItemDTOs(l.Paid), FreeBalance: l.Plan.FreeBalance,
		Totals: totalsDTO{Target: l.Plan.Target, Saved: l.Plan.Saved, Remaining: l.Plan.Remaining, SuggestedMonthly: l.Plan.Suggested},
	})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	p, err := h.svc.Get(r.Context(), id)
	if err != nil {
		h.fail(w, "get", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toItemDTO(p))
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	var req itemRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	p, err := h.svc.Update(r.Context(), id, domain.Input{Name: req.Name, Target: req.TargetAmount, DueDate: req.DueDate})
	if err != nil {
		h.fail(w, "update", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toItemDTO(p))
}

// delete is DELETE /future-expenses/{id}: the savings linked to the item are
// unlinked (they return to the free balance), never deleted.
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

func (h *Handler) contribute(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	var req savingRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	p, m, err := h.svc.Contribute(r.Context(), id, app.SavingInput{Amount: req.Amount, Date: req.Date, Description: req.Description})
	if err != nil {
		h.fail(w, "contribute", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, savingResultDTO{Item: toItemDTO(p), Saving: toMovementDTO(m)})
}

func (h *Handler) assign(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	var req assignRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	p, err := h.svc.Assign(r.Context(), id, app.AssignInput{Amount: req.Amount, Date: req.Date})
	if err != nil {
		h.fail(w, "assign", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toItemDTO(p))
}

func (h *Handler) pay(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	var req payRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	p, m, err := h.svc.Pay(r.Context(), id, app.PayInput{Date: req.Date, Amount: req.Amount, Category: req.Category})
	if err != nil {
		h.fail(w, "pay", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, payResultDTO{Item: toItemDTO(p), Expense: toMovementDTO(m)})
}

func (h *Handler) fail(w http.ResponseWriter, op string, err error) {
	reply := func(status int, code string) {
		httpjson.WriteJSON(w, status, map[string]string{"error": code, "message": err.Error()})
	}
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		reply(http.StatusBadRequest, "invalid_future_expense")
	case errors.Is(err, ledger.ErrInvalid):
		reply(http.StatusBadRequest, "invalid_movement")
	case errors.Is(err, domain.ErrNotFound):
		httpjson.WriteError(w, http.StatusNotFound, "not_found")
	case errors.Is(err, domain.ErrAlreadyPaid):
		reply(http.StatusConflict, "already_paid")
	case errors.Is(err, domain.ErrInsufficientFreeBalance):
		reply(http.StatusConflict, "insufficient_free_balance")
	case errors.Is(err, settings.ErrMissingConfig):
		reply(http.StatusUnprocessableEntity, "settings_incomplete")
	default:
		h.logger.Error("future expenses request failed", "op", op, "error", err)
		httpjson.WriteError(w, http.StatusInternalServerError, "internal_error")
	}
}
