package personalhttp

import (
	"net/http"

	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/personal/app"
	"github.com/valium69mg/finances-app/backend/internal/personal/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/httpjson"
)

// The /personal/wishlist routes. The status is derived by the domain from the
// link of the item; it is never part of a request body.

// wishRequest is the body of POST and PUT: name, price (MXN, at most 2
// decimals), category_id (one of her Gasto categories) and priority (alta,
// media or baja; media when empty) are the item; store_url (http or https) and
// note are optional.
type wishRequest struct {
	Name       string          `json:"name"`
	Price      decimal.Decimal `json:"price"`
	CategoryID int             `json:"category_id"`
	StoreURL   string          `json:"store_url"`
	Priority   string          `json:"priority"`
	Note       string          `json:"note"`
}

// wishFutureRequest is the body of POST .../convert/future-expense: due_date is
// required, target_amount defaults to the price.
type wishFutureRequest struct {
	TargetAmount *decimal.Decimal `json:"target_amount"`
	DueDate      string           `json:"due_date"`
}

// wishExpenseRequest is the body of POST .../convert/expense: amount defaults
// to the price, date to today and category_id to the category of the item.
type wishExpenseRequest struct {
	Amount     *decimal.Decimal `json:"amount"`
	Date       string           `json:"date"`
	CategoryID int              `json:"category_id"`
}

// wishRequestRequest is the body of POST .../convert/request: category is the
// name of an OWNER Gasto category (required); amount defaults to the price and
// date to today.
type wishRequestRequest struct {
	Amount   *decimal.Decimal `json:"amount"`
	Date     string           `json:"date"`
	Category string           `json:"category"`
}

// conversionDTO is what an item became: type is gasto_futuro, gasto or
// peticion, target_id the id of that row and request_state (only for a
// peticion) its current state.
type conversionDTO struct {
	Type         string  `json:"type"`
	TargetID     int     `json:"target_id"`
	RequestState *string `json:"request_state"`
}

// wishDTO is an item with its derived status (activa or convertida) and the
// conversion, null while the item is active with no link. A rejected or
// cancelled request leaves the status activa but still reports the conversion
// (with its state) until the item is converted again.
type wishDTO struct {
	ID         int             `json:"id"`
	Name       string          `json:"name"`
	Price      decimal.Decimal `json:"price"`
	CategoryID int             `json:"category_id"`
	Category   string          `json:"category"`
	StoreURL   *string         `json:"store_url"`
	Priority   string          `json:"priority"`
	Note       *string         `json:"note"`
	Status     string          `json:"status"`
	Conversion *conversionDTO  `json:"conversion"`
	CreatedAt  string          `json:"created_at"`
	UpdatedAt  string          `json:"updated_at"`
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func toWishDTO(w domain.WishItem) wishDTO {
	dto := wishDTO{
		ID: w.ID, Name: w.Name, Price: w.Price, CategoryID: w.CategoryID, Category: w.CategoryName,
		StoreURL: optional(w.StoreURL), Priority: string(w.Priority), Note: optional(w.Note), Status: string(w.Status()),
		CreatedAt: w.CreatedAt.UTC().Format(timeFormat), UpdatedAt: w.UpdatedAt.UTC().Format(timeFormat),
	}
	if w.Link != nil {
		dto.Conversion = &conversionDTO{Type: string(w.Link.Type), TargetID: w.Link.TargetID}
		if w.Link.Type == domain.ConvertedRequest {
			dto.Conversion.RequestState = optional(w.Link.RequestState)
		}
	}
	return dto
}

func (r wishRequest) input() domain.WishInput {
	return domain.WishInput{Name: r.Name, Price: r.Price, CategoryID: r.CategoryID, StoreURL: r.StoreURL, Priority: r.Priority, Note: r.Note}
}

func (h *Handler) listWish(w http.ResponseWriter, r *http.Request) {
	got, err := h.svc.Wishlist(r.Context(), actor(r), targetUser(r))
	if err != nil {
		h.fail(w, "list wishlist", err)
		return
	}
	out := make([]wishDTO, len(got))
	for i, it := range got {
		out[i] = toWishDTO(it)
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) createWish(w http.ResponseWriter, r *http.Request) {
	var req wishRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	it, err := h.svc.CreateWish(r.Context(), actor(r), req.input())
	if err != nil {
		h.fail(w, "create wishlist item", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, toWishDTO(it))
}

func (h *Handler) updateWish(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	var req wishRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	it, err := h.svc.UpdateWish(r.Context(), actor(r), id, req.input())
	if err != nil {
		h.fail(w, "update wishlist item", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toWishDTO(it))
}

// deleteWish is DELETE /personal/wishlist/{id}: only the item goes, never what
// it converted into.
func (h *Handler) deleteWish(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteWish(r.Context(), actor(r), id); err != nil {
		h.fail(w, "delete wishlist item", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) convertWishToFuture(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	var req wishFutureRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	it, err := h.svc.ConvertWishToFuture(r.Context(), actor(r), id, app.WishFutureInput{TargetAmount: req.TargetAmount, DueDate: req.DueDate})
	if err != nil {
		h.fail(w, "convert wishlist item to future expense", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toWishDTO(it))
}

func (h *Handler) convertWishToExpense(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	var req wishExpenseRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	it, err := h.svc.ConvertWishToExpense(r.Context(), actor(r), id, app.WishExpenseInput{Amount: req.Amount, Date: req.Date, CategoryID: req.CategoryID})
	if err != nil {
		h.fail(w, "convert wishlist item to expense", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toWishDTO(it))
}

func (h *Handler) convertWishToRequest(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	var req wishRequestRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	it, err := h.svc.ConvertWishToRequest(r.Context(), actor(r), id, app.WishRequestInput{Amount: req.Amount, Date: req.Date, Category: req.Category})
	if err != nil {
		h.fail(w, "convert wishlist item to request", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toWishDTO(it))
}
