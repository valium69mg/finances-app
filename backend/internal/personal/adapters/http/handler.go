// Package personalhttp exposes the personal finances use cases over HTTP. Every
// route sits behind the authentication middleware passed to Register and is on
// the household allowlist; the service scopes every row to the session user.
// Money travels as decimal strings, dates as YYYY-MM-DD.
package personalhttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/shopspring/decimal"

	requestsdomain "github.com/valium69mg/finances-app/backend/internal/expenserequests/domain"
	"github.com/valium69mg/finances-app/backend/internal/personal/app"
	"github.com/valium69mg/finances-app/backend/internal/personal/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/httpjson"
	"github.com/valium69mg/finances-app/backend/internal/platform/httpmw"
	"github.com/valium69mg/finances-app/backend/internal/platform/session"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

// Service is the set of use cases the handlers need.
type Service interface {
	Categories(ctx context.Context, actor session.Identity, kind string) ([]domain.Category, error)
	CreateCategory(ctx context.Context, actor session.Identity, name string) (domain.Category, error)
	RenameCategory(ctx context.Context, actor session.Identity, id int, name string) (domain.Category, error)
	Movements(ctx context.Context, actor session.Identity, in app.ListInput) ([]domain.Movement, error)
	CreateMovement(ctx context.Context, actor session.Identity, in domain.MovementInput) (domain.Movement, error)
	UpdateMovement(ctx context.Context, actor session.Identity, id int, in domain.MovementInput) (domain.Movement, error)
	DeleteMovement(ctx context.Context, actor session.Identity, id int) error
	Period(ctx context.Context, actor session.Identity, month string) (domain.Period, error)
	Dashboard(ctx context.Context, actor session.Identity, month string) (domain.Dashboard, error)

	CreateFutureExpense(ctx context.Context, actor session.Identity, in domain.FutureInput) (domain.FuturePlanned, error)
	FutureExpense(ctx context.Context, actor session.Identity, id int) (domain.FuturePlanned, error)
	UpdateFutureExpense(ctx context.Context, actor session.Identity, id int, in domain.FutureInput) (domain.FuturePlanned, error)
	DeleteFutureExpense(ctx context.Context, actor session.Identity, id int) error
	FutureExpenses(ctx context.Context, actor session.Identity) (app.FutureListing, error)
	ContributeFuture(ctx context.Context, actor session.Identity, id int, in app.FutureSavingInput) (domain.FuturePlanned, domain.Movement, error)
	AssignFuture(ctx context.Context, actor session.Identity, id int, in app.FutureAssignInput) (domain.FuturePlanned, error)
	PayFuture(ctx context.Context, actor session.Identity, id int, in app.FuturePayInput) (domain.FuturePlanned, domain.Movement, error)

	Wishlist(ctx context.Context, actor session.Identity) ([]domain.WishItem, error)
	CreateWish(ctx context.Context, actor session.Identity, in domain.WishInput) (domain.WishItem, error)
	UpdateWish(ctx context.Context, actor session.Identity, id int, in domain.WishInput) (domain.WishItem, error)
	DeleteWish(ctx context.Context, actor session.Identity, id int) error
	ConvertWishToFuture(ctx context.Context, actor session.Identity, id int, in app.WishFutureInput) (domain.WishItem, error)
	ConvertWishToExpense(ctx context.Context, actor session.Identity, id int, in app.WishExpenseInput) (domain.WishItem, error)
	ConvertWishToRequest(ctx context.Context, actor session.Identity, id int, in app.WishRequestInput) (domain.WishItem, error)
}

// Handler serves the /personal routes.
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

// Register mounts the routes on mux, each wrapped by requireAuth. Every pattern
// here must also be on the household allowlist (auth handler) and in
// cmd/api/routes_test.go.
func (h *Handler) Register(mux httpmw.Router, requireAuth func(http.Handler) http.Handler) {
	route := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, requireAuth(fn))
	}
	route("GET /personal/dashboard", h.dashboard)
	route("GET /personal/period", h.period)
	route("GET /personal/categories", h.categories)
	route("POST /personal/categories", h.createCategory)
	route("PUT /personal/categories/{id}", h.renameCategory)
	route("GET /personal/movements", h.movements)
	route("POST /personal/movements", h.createMovement)
	route("PUT /personal/movements/{id}", h.updateMovement)
	route("DELETE /personal/movements/{id}", h.deleteMovement)
	route("POST /personal/future-expenses", h.createFuture)
	route("GET /personal/future-expenses", h.listFuture)
	route("GET /personal/future-expenses/{id}", h.getFuture)
	route("PUT /personal/future-expenses/{id}", h.updateFuture)
	route("DELETE /personal/future-expenses/{id}", h.deleteFuture)
	route("POST /personal/future-expenses/{id}/savings", h.contributeFuture)
	route("POST /personal/future-expenses/{id}/assign", h.assignFuture)
	route("POST /personal/future-expenses/{id}/pay", h.payFuture)
	route("GET /personal/wishlist", h.listWish)
	route("POST /personal/wishlist", h.createWish)
	route("PUT /personal/wishlist/{id}", h.updateWish)
	route("DELETE /personal/wishlist/{id}", h.deleteWish)
	route("POST /personal/wishlist/{id}/convert/future-expense", h.convertWishToFuture)
	route("POST /personal/wishlist/{id}/convert/expense", h.convertWishToExpense)
	route("POST /personal/wishlist/{id}/convert/request", h.convertWishToRequest)
}

// --- DTOs ---------------------------------------------------------------

// categoryRequest is the body of POST and PUT /personal/categories.
type categoryRequest struct {
	Name string `json:"name"`
}

// movementRequest is the body of POST and PUT /personal/movements. amount is a
// decimal in MXN with at most 2 decimals (negative only for an Ahorro
// withdrawal); an empty date means today.
type movementRequest struct {
	Kind        string          `json:"kind"`
	CategoryID  int             `json:"category_id"`
	Amount      decimal.Decimal `json:"amount"`
	Description string          `json:"description"`
	Date        string          `json:"date"`
}

// categoryDTO is a category. fixed categories (Ingreso and Ahorro) cannot be renamed.
type categoryDTO struct {
	ID    int    `json:"id"`
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	Fixed bool   `json:"fixed"`
}

type movementDTO struct {
	ID          int             `json:"id"`
	Kind        string          `json:"kind"`
	CategoryID  int             `json:"category_id"`
	Category    string          `json:"category"`
	Amount      decimal.Decimal `json:"amount"`
	Description string          `json:"description"`
	Date        string          `json:"date"`
	// FutureExpenseID is the future expense an Ahorro feeds, null otherwise. A
	// manual update never changes it.
	FutureExpenseID *int   `json:"future_expense_id"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

const timeFormat = "2006-01-02T15:04:05Z"

func toCategoryDTO(c domain.Category) categoryDTO {
	return categoryDTO{ID: c.ID, Kind: string(c.Kind), Name: c.Name, Fixed: !c.Kind.Editable()}
}

func toMovementDTO(m domain.Movement) movementDTO {
	return movementDTO{
		ID: m.ID, Kind: string(m.Kind), CategoryID: m.CategoryID, Category: m.CategoryName, Amount: m.Amount,
		Description: m.Description, Date: m.Date, FutureExpenseID: m.FutureExpenseID,
		CreatedAt: m.CreatedAt.UTC().Format(timeFormat), UpdatedAt: m.UpdatedAt.UTC().Format(timeFormat),
	}
}

func (r movementRequest) input() domain.MovementInput {
	return domain.MovementInput{Kind: r.Kind, CategoryID: r.CategoryID, Amount: r.Amount, Description: r.Description, Date: r.Date}
}

func actor(r *http.Request) session.Identity {
	id, _ := session.From(r.Context())
	return id
}

// --- handlers -----------------------------------------------------------

func (h *Handler) categories(w http.ResponseWriter, r *http.Request) {
	got, err := h.svc.Categories(r.Context(), actor(r), r.URL.Query().Get("kind"))
	if err != nil {
		h.fail(w, "categories", err)
		return
	}
	out := make([]categoryDTO, len(got))
	for i, c := range got {
		out[i] = toCategoryDTO(c)
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) createCategory(w http.ResponseWriter, r *http.Request) {
	var req categoryRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	c, err := h.svc.CreateCategory(r.Context(), actor(r), req.Name)
	if err != nil {
		h.fail(w, "create category", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, toCategoryDTO(c))
}

func (h *Handler) renameCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	var req categoryRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	c, err := h.svc.RenameCategory(r.Context(), actor(r), id, req.Name)
	if err != nil {
		h.fail(w, "rename category", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toCategoryDTO(c))
}

func (h *Handler) movements(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	in := app.ListInput{Month: q.Get("month"), Kind: q.Get("kind")}
	if raw := q.Get("category_id"); raw != "" {
		id, err := strconv.Atoi(raw)
		if err != nil || id <= 0 {
			httpjson.WriteError(w, http.StatusBadRequest, "invalid_request")
			return
		}
		in.CategoryID = id
	}
	got, err := h.svc.Movements(r.Context(), actor(r), in)
	if err != nil {
		h.fail(w, "movements", err)
		return
	}
	out := make([]movementDTO, len(got))
	for i, m := range got {
		out[i] = toMovementDTO(m)
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) createMovement(w http.ResponseWriter, r *http.Request) {
	var req movementRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	m, err := h.svc.CreateMovement(r.Context(), actor(r), req.input())
	if err != nil {
		h.fail(w, "create movement", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, toMovementDTO(m))
}

func (h *Handler) updateMovement(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	var req movementRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	m, err := h.svc.UpdateMovement(r.Context(), actor(r), id, req.input())
	if err != nil {
		h.fail(w, "update movement", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toMovementDTO(m))
}

func (h *Handler) deleteMovement(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteMovement(r.Context(), actor(r), id); err != nil {
		h.fail(w, "delete movement", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

// periodDTO is a pay cycle: the YYYY-MM label and its inclusive bounds.
type periodDTO struct {
	Month       string `json:"month"`
	PeriodStart string `json:"period_start"`
	PeriodEnd   string `json:"period_end"`
}

type categorySpendDTO struct {
	CategoryID int             `json:"category_id"`
	Category   string          `json:"category"`
	Spent      decimal.Decimal `json:"spent"`
}

// dashboardDTO is her summary of a pay cycle. savings is net of withdrawals and
// available = income - expenses - savings. categories lists only the Gasto
// categories with spending in the cycle, biggest first.
type dashboardDTO struct {
	periodDTO
	Income     decimal.Decimal    `json:"income"`
	Expenses   decimal.Decimal    `json:"expenses"`
	Savings    decimal.Decimal    `json:"savings"`
	Available  decimal.Decimal    `json:"available"`
	Categories []categorySpendDTO `json:"categories"`
}

func toPeriodDTO(p domain.Period) periodDTO {
	return periodDTO{Month: p.Month, PeriodStart: p.Start, PeriodEnd: p.End}
}

func (h *Handler) period(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.Period(r.Context(), actor(r), r.URL.Query().Get("month"))
	if err != nil {
		h.fail(w, "period", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toPeriodDTO(p))
}

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	got, err := h.svc.Dashboard(r.Context(), actor(r), r.URL.Query().Get("month"))
	if err != nil {
		h.fail(w, "dashboard", err)
		return
	}
	out := dashboardDTO{
		periodDTO: toPeriodDTO(got.Period), Income: got.Totals.Income, Expenses: got.Totals.Expenses,
		Savings: got.Totals.Savings, Available: got.Totals.Available(), Categories: make([]categorySpendDTO, len(got.Categories)),
	}
	for i, c := range got.Categories {
		out.Categories[i] = categorySpendDTO{CategoryID: c.CategoryID, Category: c.Category, Spent: c.Spent}
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) fail(w http.ResponseWriter, op string, err error) {
	reply := func(status int, code string) {
		httpjson.WriteJSON(w, status, map[string]string{"error": code, "message": err.Error()})
	}
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		reply(http.StatusBadRequest, "invalid_personal")
	case errors.Is(err, domain.ErrFutureInvalid):
		reply(http.StatusBadRequest, "invalid_future_expense")
	case errors.Is(err, domain.ErrWishInvalid):
		reply(http.StatusBadRequest, "invalid_wishlist_item")
	case errors.Is(err, domain.ErrAlreadyConverted):
		httpjson.WriteError(w, http.StatusConflict, "already_converted")
	case errors.Is(err, requestsdomain.ErrInvalidInput):
		reply(http.StatusBadRequest, "invalid_expense_request")
	case errors.Is(err, requestsdomain.ErrRateLimited):
		w.Header().Set("Retry-After", "3600")
		httpjson.WriteError(w, http.StatusTooManyRequests, "rate_limited")
	case errors.Is(err, domain.ErrAlreadyPaid):
		reply(http.StatusConflict, "already_paid")
	case errors.Is(err, domain.ErrInsufficientFreeBalance):
		reply(http.StatusConflict, "insufficient_free_balance")
	case errors.Is(err, domain.ErrForbidden):
		httpjson.WriteError(w, http.StatusForbidden, "forbidden")
	case errors.Is(err, domain.ErrNotFound):
		httpjson.WriteError(w, http.StatusNotFound, "not_found")
	case errors.Is(err, domain.ErrCategoryFixed):
		reply(http.StatusConflict, "category_fixed")
	case errors.Is(err, domain.ErrDuplicateCategory):
		reply(http.StatusConflict, "category_exists")
	case errors.Is(err, settings.ErrMissingConfig):
		reply(http.StatusUnprocessableEntity, "settings_incomplete")
	default:
		h.logger.Error("personal request failed", "op", op, "error", err)
		httpjson.WriteError(w, http.StatusInternalServerError, "internal_error")
	}
}
