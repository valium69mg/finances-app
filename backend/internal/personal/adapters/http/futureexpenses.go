package personalhttp

import (
	"net/http"

	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/personal/app"
	"github.com/valium69mg/finances-app/backend/internal/personal/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/httpjson"
)

// The /personal/future-expenses routes mirror the owner's /future-expenses.

// futureRequest is the body of POST and PUT: name, target_amount (MXN, at most
// 2 decimals) and due_date are required.
type futureRequest struct {
	Name         string          `json:"name"`
	TargetAmount decimal.Decimal `json:"target_amount"`
	DueDate      string          `json:"due_date"`
}

// futureSavingRequest is the body of POST .../{id}/savings. date defaults to
// today and description to "Ahorro para <name>".
type futureSavingRequest struct {
	Amount      decimal.Decimal `json:"amount"`
	Date        string          `json:"date"`
	Description string          `json:"description"`
}

// futureAssignRequest is the body of POST .../{id}/assign.
type futureAssignRequest struct {
	Amount decimal.Decimal `json:"amount"`
	Date   string          `json:"date"`
}

// futurePayRequest is the body of POST .../{id}/pay. category_id (one of her
// Gasto categories) is required; date defaults to today and amount (what was
// actually paid) to the target.
type futurePayRequest struct {
	Date       string           `json:"date"`
	Amount     *decimal.Decimal `json:"amount"`
	CategoryID int              `json:"category_id"`
}

// futureDTO is an item with its plan. saved is the net of the savings linked
// to it; remaining, suggested_monthly and cycles_left are zero for a paid
// item, which carries paid_at, amount_paid and expense_movement_id instead.
type futureDTO struct {
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

type futureTotalsDTO struct {
	Target           decimal.Decimal `json:"target"`
	Saved            decimal.Decimal `json:"saved"`
	Remaining        decimal.Decimal `json:"remaining"`
	SuggestedMonthly decimal.Decimal `json:"suggested_monthly"`
}

// futureListDTO is the body of GET /personal/future-expenses: the active items
// (earliest due date first) with their totals, the paid ones (latest first)
// and the free balance, her Gastos futuros savings linked to no item.
type futureListDTO struct {
	Active      []futureDTO     `json:"active"`
	Paid        []futureDTO     `json:"paid"`
	Totals      futureTotalsDTO `json:"totals"`
	FreeBalance decimal.Decimal `json:"free_balance"`
}

type futureSavingResultDTO struct {
	Item   futureDTO   `json:"item"`
	Saving movementDTO `json:"saving"`
}

type futurePayResultDTO struct {
	Item    futureDTO   `json:"item"`
	Expense movementDTO `json:"expense"`
}

func toFutureDTO(p domain.FuturePlanned) futureDTO {
	dto := futureDTO{
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

func toFutureDTOs(list []domain.FuturePlanned) []futureDTO {
	out := make([]futureDTO, len(list))
	for i, p := range list {
		out[i] = toFutureDTO(p)
	}
	return out
}

func (r futureRequest) input() domain.FutureInput {
	return domain.FutureInput{Name: r.Name, Target: r.TargetAmount, DueDate: r.DueDate}
}

func (h *Handler) createFuture(w http.ResponseWriter, r *http.Request) {
	var req futureRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	p, err := h.svc.CreateFutureExpense(r.Context(), actor(r), req.input())
	if err != nil {
		h.fail(w, "create future expense", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, toFutureDTO(p))
}

func (h *Handler) listFuture(w http.ResponseWriter, r *http.Request) {
	l, err := h.svc.FutureExpenses(r.Context(), actor(r))
	if err != nil {
		h.fail(w, "list future expenses", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, futureListDTO{
		Active: toFutureDTOs(l.Plan.Items), Paid: toFutureDTOs(l.Paid), FreeBalance: l.Plan.FreeBalance,
		Totals: futureTotalsDTO{Target: l.Plan.Target, Saved: l.Plan.Saved, Remaining: l.Plan.Remaining, SuggestedMonthly: l.Plan.Suggested},
	})
}

func (h *Handler) getFuture(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	p, err := h.svc.FutureExpense(r.Context(), actor(r), id)
	if err != nil {
		h.fail(w, "get future expense", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toFutureDTO(p))
}

func (h *Handler) updateFuture(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	var req futureRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	p, err := h.svc.UpdateFutureExpense(r.Context(), actor(r), id, req.input())
	if err != nil {
		h.fail(w, "update future expense", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toFutureDTO(p))
}

// deleteFuture is DELETE /personal/future-expenses/{id}: the savings linked to
// the item are unlinked (they return to the free balance), never deleted.
func (h *Handler) deleteFuture(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteFutureExpense(r.Context(), actor(r), id); err != nil {
		h.fail(w, "delete future expense", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) contributeFuture(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	var req futureSavingRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	p, m, err := h.svc.ContributeFuture(r.Context(), actor(r), id, app.FutureSavingInput{Amount: req.Amount, Date: req.Date, Description: req.Description})
	if err != nil {
		h.fail(w, "contribute to future expense", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, futureSavingResultDTO{Item: toFutureDTO(p), Saving: toMovementDTO(m)})
}

func (h *Handler) assignFuture(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	var req futureAssignRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	p, err := h.svc.AssignFuture(r.Context(), actor(r), id, app.FutureAssignInput{Amount: req.Amount, Date: req.Date})
	if err != nil {
		h.fail(w, "assign to future expense", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toFutureDTO(p))
}

func (h *Handler) payFuture(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	var req futurePayRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	p, m, err := h.svc.PayFuture(r.Context(), actor(r), id, app.FuturePayInput{Date: req.Date, Amount: req.Amount, CategoryID: req.CategoryID})
	if err != nil {
		h.fail(w, "pay future expense", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, futurePayResultDTO{Item: toFutureDTO(p), Expense: toMovementDTO(m)})
}
