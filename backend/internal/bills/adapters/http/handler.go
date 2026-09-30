// Package billshttp exposes the Bills & Subscriptions use cases over HTTP: the
// bills with their next occurrence, paying it (which registers an expense) and
// skipping it. Every route sits behind the authentication middleware passed to
// Register. Money travels as decimal strings, dates as YYYY-MM-DD.
package billshttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/bills/app"
	bills "github.com/valium69mg/finances-app/backend/internal/bills/domain"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/httpjson"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

// Service is the set of use cases the handlers need.
type Service interface {
	Create(ctx context.Context, in bills.Input) (app.Status, error)
	List(ctx context.Context, includeInactive bool) ([]app.Status, error)
	Get(ctx context.Context, id int) (app.Detail, error)
	Update(ctx context.Context, id int, in bills.Input) (app.Status, error)
	Deactivate(ctx context.Context, id int) error
	Pay(ctx context.Context, id int, in bills.PaymentInput) (app.PayResult, error)
	Skip(ctx context.Context, id int) (app.Status, error)
}

// Handler serves the /bills routes.
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

// Register mounts the bills routes on mux, each wrapped by requireAuth.
func (h *Handler) Register(mux *http.ServeMux, requireAuth func(http.Handler) http.Handler) {
	route := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, requireAuth(fn))
	}
	route("POST /bills", h.create)
	route("GET /bills", h.list)
	route("GET /bills/{id}", h.get)
	route("PUT /bills/{id}", h.update)
	route("DELETE /bills/{id}", h.deactivate)
	route("POST /bills/{id}/pay", h.pay)
	route("POST /bills/{id}/skip", h.skip)
}

// --- DTOs ---------------------------------------------------------------

// billRequest is the body of POST and PUT. name, category, recurrence and
// next_due_date are required; a missing or null amount makes a variable bill;
// currency defaults to MXN, reminder_lead_days to 3 and active to true. On PUT,
// next_due_date moves the pending occurrence (sending the current one changes
// nothing); recurrence and amount apply from the following occurrence.
type billRequest struct {
	Name             string           `json:"name"`
	Category         string           `json:"category"`
	Amount           *decimal.Decimal `json:"amount"`
	Currency         string           `json:"currency"`
	Recurrence       string           `json:"recurrence"`
	NextDueDate      string           `json:"next_due_date"`
	ReminderLeadDays *int             `json:"reminder_lead_days"`
	Active           *bool            `json:"active"`
	Notes            string           `json:"notes"`
}

func (r billRequest) toInput() bills.Input {
	active := true
	if r.Active != nil {
		active = *r.Active
	}
	return bills.Input{
		Name: r.Name, Category: r.Category, Amount: r.Amount, Currency: r.Currency, Recurrence: bills.Recurrence(r.Recurrence),
		NextDueDate: r.NextDueDate, ReminderLeadDays: r.ReminderLeadDays, Active: active, Notes: r.Notes,
	}
}

// payRequest is the body of POST /bills/{id}/pay. Every field is optional except
// amount for a bill without a fixed amount: date defaults to today, amount to
// the bill's, category to the bill's and description to the bill's name.
type payRequest struct {
	Date        string           `json:"date"`
	Amount      *decimal.Decimal `json:"amount"`
	Category    string           `json:"category"`
	Description string           `json:"description"`
}

type occurrenceDTO struct {
	ID                int              `json:"id"`
	DueDate           string           `json:"due_date"`
	Status            string           `json:"status"`
	PaidOn            *string          `json:"paid_on"`
	ExpenseMovementID *int             `json:"expense_movement_id"`
	AmountPaid        *decimal.Decimal `json:"amount_paid"`
	Currency          *string          `json:"currency"`
	ResolvedAt        *string          `json:"resolved_at"`
}

// billDTO is a bill with its pending occurrence and the flags computed for
// today. pending_occurrence is null only for a bill without one; days_until_due
// is negative when overdue. The reminder job of a later phase reads
// reminder_lead_days, due_soon and overdue.
type billDTO struct {
	ID                int              `json:"id"`
	Name              string           `json:"name"`
	Category          string           `json:"category"`
	Amount            *decimal.Decimal `json:"amount"`
	Currency          string           `json:"currency"`
	Recurrence        string           `json:"recurrence"`
	NextDueDate       string           `json:"next_due_date"`
	ReminderLeadDays  int              `json:"reminder_lead_days"`
	Active            bool             `json:"active"`
	Notes             string           `json:"notes"`
	CreatedAt         string           `json:"created_at"`
	PendingOccurrence *occurrenceDTO   `json:"pending_occurrence"`
	Overdue           bool             `json:"overdue"`
	DueSoon           bool             `json:"due_soon"`
	DaysUntilDue      *int             `json:"days_until_due"`
}

// billDetailDTO is a bill with its resolved history, newest first.
type billDetailDTO struct {
	billDTO
	History []occurrenceDTO `json:"history"`
}

type expenseDTO struct {
	ID          int             `json:"id"`
	Date        string          `json:"date"`
	Description string          `json:"description"`
	Category    string          `json:"category"`
	Currency    string          `json:"currency"`
	Amount      decimal.Decimal `json:"amount"`
	AmountMXN   decimal.Decimal `json:"amount_mxn"`
}

// payResultDTO is the body of a payment: the bill with its next occurrence, the
// occurrence just paid and the expense registered for it.
type payResultDTO struct {
	Bill    billDTO       `json:"bill"`
	Paid    occurrenceDTO `json:"paid_occurrence"`
	Expense expenseDTO    `json:"expense"`
}

const timeFormat = "2006-01-02T15:04:05Z"

func toOccurrenceDTO(o bills.Occurrence) occurrenceDTO {
	dto := occurrenceDTO{
		ID: o.ID, DueDate: o.DueDate, Status: string(o.Status), ExpenseMovementID: o.ExpenseID, AmountPaid: o.AmountPaid,
	}
	if o.PaidOn != "" {
		p := o.PaidOn
		dto.PaidOn = &p
	}
	if o.Currency != "" {
		c := o.Currency
		dto.Currency = &c
	}
	if o.ResolvedAt != nil {
		r := o.ResolvedAt.UTC().Format(timeFormat)
		dto.ResolvedAt = &r
	}
	return dto
}

func toBillDTO(s app.Status) billDTO {
	b := s.Bill
	dto := billDTO{
		ID: b.ID, Name: b.Name, Category: b.Category, Amount: b.Amount, Currency: b.Currency, Recurrence: string(b.Recurrence),
		NextDueDate: b.NextDueDate, ReminderLeadDays: b.ReminderLeadDays, Active: b.Active, Notes: b.Notes,
		CreatedAt: b.CreatedAt.UTC().Format(timeFormat), Overdue: s.Overdue, DueSoon: s.DueSoon,
	}
	if b.Pending != nil {
		o := toOccurrenceDTO(*b.Pending)
		days := s.DaysUntilDue
		dto.PendingOccurrence, dto.DaysUntilDue = &o, &days
	}
	return dto
}

func toOccurrenceDTOs(list []bills.Occurrence) []occurrenceDTO {
	out := make([]occurrenceDTO, len(list))
	for i, o := range list {
		out[i] = toOccurrenceDTO(o)
	}
	return out
}

// --- handlers -----------------------------------------------------------

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req billRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	st, err := h.svc.Create(r.Context(), req.toInput())
	if err != nil {
		h.fail(w, "create", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, toBillDTO(st))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	var includeInactive bool
	switch r.URL.Query().Get("include_inactive") {
	case "", "false":
	case "true":
		includeInactive = true
	default:
		httpjson.WriteError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	list, err := h.svc.List(r.Context(), includeInactive)
	if err != nil {
		h.fail(w, "list", err)
		return
	}
	out := make([]billDTO, len(list))
	for i, s := range list {
		out[i] = toBillDTO(s)
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	detail, err := h.svc.Get(r.Context(), id)
	if err != nil {
		h.fail(w, "get", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, billDetailDTO{billDTO: toBillDTO(detail.Status), History: toOccurrenceDTOs(detail.History)})
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	var req billRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	st, err := h.svc.Update(r.Context(), id, req.toInput())
	if err != nil {
		h.fail(w, "update", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toBillDTO(st))
}

// deactivate is DELETE /bills/{id}: the bill is turned off, never removed, so its
// history and the expenses of its payments stay.
func (h *Handler) deactivate(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Deactivate(r.Context(), id); err != nil {
		h.fail(w, "deactivate", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
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
	res, err := h.svc.Pay(r.Context(), id, bills.PaymentInput{
		Date: req.Date, Amount: req.Amount, Category: req.Category, Description: req.Description,
	})
	if err != nil {
		h.fail(w, "pay", err)
		return
	}
	m := res.Expense
	httpjson.WriteJSON(w, http.StatusOK, payResultDTO{
		Bill: toBillDTO(res.Status),
		Paid: toOccurrenceDTO(res.Paid),
		Expense: expenseDTO{
			ID: m.ID, Date: m.Date, Description: m.Description, Category: m.Category, Currency: m.Currency,
			Amount: m.Amount, AmountMXN: m.AmountMXN,
		},
	})
}

func (h *Handler) skip(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	st, err := h.svc.Skip(r.Context(), id)
	if err != nil {
		h.fail(w, "skip", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toBillDTO(st))
}

func (h *Handler) fail(w http.ResponseWriter, op string, err error) {
	reply := func(status int, code string) {
		httpjson.WriteJSON(w, status, map[string]string{"error": code, "message": err.Error()})
	}
	switch {
	case errors.Is(err, bills.ErrInvalidInput):
		reply(http.StatusBadRequest, "invalid_bill")
	case errors.Is(err, ledger.ErrInvalid):
		reply(http.StatusBadRequest, "invalid_expense")
	case errors.Is(err, bills.ErrNotFound):
		httpjson.WriteError(w, http.StatusNotFound, "not_found")
	case errors.Is(err, bills.ErrNotPending):
		reply(http.StatusConflict, "occurrence_resolved")
	case errors.Is(err, bills.ErrInactive):
		reply(http.StatusConflict, "bill_inactive")
	case errors.Is(err, settings.ErrMissingConfig):
		reply(http.StatusUnprocessableEntity, "settings_incomplete")
	default:
		h.logger.Error("bills request failed", "op", op, "error", err)
		httpjson.WriteError(w, http.StatusInternalServerError, "internal_error")
	}
}
