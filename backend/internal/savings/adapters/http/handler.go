// Package savingshttp exposes the savings use cases over HTTP. Every route sits
// behind the authentication middleware passed to Register. Money and rates
// travel as decimal strings.
package savingshttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/shopspring/decimal"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/httpjson"
	"github.com/valium69mg/finances-app/backend/internal/platform/httpmw"
	"github.com/valium69mg/finances-app/backend/internal/savings/app"
	savings "github.com/valium69mg/finances-app/backend/internal/savings/domain"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

const maxListLimit = 200

// Service is the set of use cases the handlers need.
type Service interface {
	CreateSaving(ctx context.Context, in app.Input) (ledger.Movement, error)
	UpdateSaving(ctx context.Context, id int, in app.Input) (ledger.Movement, error)
	DeleteSaving(ctx context.Context, id int) error
	ListSavings(ctx context.Context, month string, limit int) ([]ledger.Movement, error)
	Transfer(ctx context.Context, in app.TransferInput) (app.Transfer, error)
	AddValuation(ctx context.Context, in app.ValuationInput) (ledger.Valuation, error)
	ListValuations(ctx context.Context) ([]ledger.Valuation, error)
	Portfolio(ctx context.Context) (savings.Portfolio, error)
}

// Handler serves the /savings routes.
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

// Register mounts the savings routes on mux, each wrapped by requireAuth.
func (h *Handler) Register(mux httpmw.Router, requireAuth func(http.Handler) http.Handler) {
	route := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, requireAuth(fn))
	}
	route("POST /savings", h.create)
	route("GET /savings", h.list)
	route("PUT /savings/{id}", h.update)
	route("DELETE /savings/{id}", h.delete)
	route("POST /savings/transfers", h.transfer)
	route("POST /savings/valuations", h.addValuation)
	route("GET /savings/valuations", h.listValuations)
	route("GET /savings/portfolio", h.portfolio)
}

// --- DTOs ---------------------------------------------------------------

// savingRequest is the body of POST and PUT /savings. Only amount is required
// (negative for a withdrawal): an empty category is inferred from the
// description, the instrument defaults to the one configured for the category,
// the payment method to Transferencia, the currency to MXN, the date to today
// and the USD rate to the configured fx_rate_applied.
type savingRequest struct {
	Date          string           `json:"date"`
	Description   string           `json:"description"`
	Category      string           `json:"category"`
	Instrument    string           `json:"instrument"`
	PaymentMethod string           `json:"payment_method"`
	Currency      string           `json:"currency"`
	Amount        decimal.Decimal  `json:"amount"`
	ExchangeRate  *decimal.Decimal `json:"exchange_rate"`
}

type transferRequest struct {
	From        string          `json:"from"`
	To          string          `json:"to"`
	Amount      decimal.Decimal `json:"amount"`
	Date        string          `json:"date"`
	Description string          `json:"description"`
	Category    string          `json:"category"`
}

type valuationRequest struct {
	Date       string          `json:"date"`
	Instrument string          `json:"instrument"`
	ValueMXN   decimal.Decimal `json:"value_mxn"`
	Note       string          `json:"note"`
}

type savingDTO struct {
	ID            int              `json:"id"`
	Date          string           `json:"date"`
	Description   string           `json:"description"`
	Category      string           `json:"category"`
	Instrument    string           `json:"instrument"`
	PaymentMethod string           `json:"payment_method"`
	Currency      string           `json:"currency"`
	Amount        decimal.Decimal  `json:"amount"`
	ExchangeRate  *decimal.Decimal `json:"exchange_rate"`
	AmountMXN     decimal.Decimal  `json:"amount_mxn"`
	// TransferID is shared by the two legs of a transfer and null otherwise.
	TransferID *string `json:"transfer_id"`
	// FutureExpenseID is the future expense the saving feeds, null otherwise.
	FutureExpenseID *int `json:"future_expense_id"`
}

type transferDTO struct {
	Out savingDTO `json:"out"`
	In  savingDTO `json:"in"`
}

type valuationDTO struct {
	Date       string          `json:"date"`
	Instrument string          `json:"instrument"`
	ValueMXN   decimal.Decimal `json:"value_mxn"`
	Note       string          `json:"note"`
}

type portfolioRowDTO struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Type        string          `json:"type"`
	Platform    string          `json:"platform"`
	Contributed decimal.Decimal `json:"contributed"`
	Value       decimal.Decimal `json:"value"`
	ValueDate   *string         `json:"value_date"` // null when unvalued
	Unvalued    bool            `json:"unvalued"`
	Gain        decimal.Decimal `json:"gain"`
	GainPct     decimal.Decimal `json:"gain_pct"`
	PctOfTotal  decimal.Decimal `json:"pct_of_total"`
}

type typeTotalDTO struct {
	Type        string          `json:"type"`
	Contributed decimal.Decimal `json:"contributed"`
	Value       decimal.Decimal `json:"value"`
}

type destinationDTO struct {
	Category string          `json:"category"`
	Balance  decimal.Decimal `json:"balance"`
}

type emergencyDTO struct {
	Accumulated decimal.Decimal `json:"accumulated"`
	Goal        decimal.Decimal `json:"goal"`
}

type portfolioDTO struct {
	Rows             []portfolioRowDTO `json:"rows"`
	TotalContributed decimal.Decimal   `json:"total_contributed"`
	TotalValue       decimal.Decimal   `json:"total_value"`
	ByType           []typeTotalDTO    `json:"by_type"`
	ByDestination    []destinationDTO  `json:"by_destination"`
	Emergency        emergencyDTO      `json:"emergency"`
}

func (r savingRequest) toInput() app.Input {
	return app.Input{
		Date: r.Date, Description: r.Description, Category: r.Category, Instrument: r.Instrument,
		PaymentMethod: r.PaymentMethod, Currency: r.Currency, Amount: r.Amount, ExchangeRate: r.ExchangeRate,
	}
}

func toSavingDTO(m ledger.Movement) savingDTO {
	dto := savingDTO{
		ID: m.ID, Date: m.Date, Description: m.Description, Category: m.Category, Instrument: m.Instrument,
		PaymentMethod: m.PaymentMethod, Currency: m.Currency, Amount: m.Amount, ExchangeRate: m.ExchangeRate,
		AmountMXN: m.AmountMXN,
	}
	if m.FutureExpenseID != 0 {
		id := m.FutureExpenseID
		dto.FutureExpenseID = &id
	}
	if m.TransferID != "" {
		id := m.TransferID
		dto.TransferID = &id
	}
	return dto
}

func toValuationDTO(v ledger.Valuation) valuationDTO {
	return valuationDTO{Date: v.Date, Instrument: v.Instrument, ValueMXN: v.ValueMXN, Note: v.Note}
}

func toPortfolioDTO(p savings.Portfolio) portfolioDTO {
	out := portfolioDTO{
		Rows:             make([]portfolioRowDTO, len(p.Rows)),
		TotalContributed: p.TotalContributed,
		TotalValue:       p.TotalValue,
		ByType:           make([]typeTotalDTO, len(p.ByType)),
		ByDestination:    make([]destinationDTO, len(p.ByDestination)),
		Emergency:        emergencyDTO{Accumulated: p.Emergency.Accumulated, Goal: p.Emergency.Goal},
	}
	for i, r := range p.Rows {
		row := portfolioRowDTO{
			ID: r.ID, Name: r.Name, Type: r.Type, Platform: r.Platform, Contributed: r.Contributed, Value: r.Value,
			Unvalued: r.Unvalued, Gain: r.Gain, GainPct: r.GainPct, PctOfTotal: r.PctOfTotal,
		}
		if r.ValueDate != "" {
			date := r.ValueDate
			row.ValueDate = &date
		}
		out.Rows[i] = row
	}
	for i, t := range p.ByType {
		out.ByType[i] = typeTotalDTO{Type: t.Type, Contributed: t.Contributed, Value: t.Value}
	}
	for i, d := range p.ByDestination {
		out.ByDestination[i] = destinationDTO{Category: d.Category, Balance: d.Balance}
	}
	return out
}

// --- handlers -----------------------------------------------------------

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req savingRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	m, err := h.svc.CreateSaving(r.Context(), req.toInput())
	if err != nil {
		h.fail(w, "create", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, toSavingDTO(m))
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	var req savingRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	m, err := h.svc.UpdateSaving(r.Context(), id, req.toInput())
	if err != nil {
		h.fail(w, "update", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toSavingDTO(m))
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteSaving(r.Context(), id); err != nil {
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
	movements, err := h.svc.ListSavings(r.Context(), r.URL.Query().Get("month"), limit)
	if err != nil {
		h.fail(w, "list", err)
		return
	}
	out := make([]savingDTO, len(movements))
	for i, m := range movements {
		out[i] = toSavingDTO(m)
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) transfer(w http.ResponseWriter, r *http.Request) {
	var req transferRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	res, err := h.svc.Transfer(r.Context(), app.TransferInput{
		From: req.From, To: req.To, Amount: req.Amount, Date: req.Date, Description: req.Description, Category: req.Category,
	})
	if err != nil {
		h.fail(w, "transfer", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, transferDTO{Out: toSavingDTO(res.Out), In: toSavingDTO(res.In)})
}

func (h *Handler) addValuation(w http.ResponseWriter, r *http.Request) {
	var req valuationRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	v, err := h.svc.AddValuation(r.Context(), app.ValuationInput{
		Date: req.Date, Instrument: req.Instrument, ValueMXN: req.ValueMXN, Note: req.Note,
	})
	if err != nil {
		h.fail(w, "add valuation", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, toValuationDTO(v))
}

func (h *Handler) listValuations(w http.ResponseWriter, r *http.Request) {
	valuations, err := h.svc.ListValuations(r.Context())
	if err != nil {
		h.fail(w, "list valuations", err)
		return
	}
	out := make([]valuationDTO, len(valuations))
	for i, v := range valuations {
		out[i] = toValuationDTO(v)
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) portfolio(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.Portfolio(r.Context())
	if err != nil {
		h.fail(w, "portfolio", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toPortfolioDTO(p))
}

func (h *Handler) fail(w http.ResponseWriter, op string, err error) {
	switch {
	case errors.Is(err, ledger.ErrInvalid):
		httpjson.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_saving", "message": err.Error()})
	case errors.Is(err, savings.ErrInvalidValuation):
		httpjson.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_valuation", "message": err.Error()})
	case errors.Is(err, savings.ErrTransferLegLocked):
		httpjson.WriteJSON(w, http.StatusConflict, map[string]string{"error": "transfer_leg_locked", "message": err.Error()})
	case errors.Is(err, ledger.ErrNotFound):
		httpjson.WriteError(w, http.StatusNotFound, "not_found")
	case errors.Is(err, settings.ErrMissingConfig):
		httpjson.WriteJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "settings_incomplete", "message": err.Error()})
	default:
		h.logger.Error("savings request failed", "op", op, "error", err)
		httpjson.WriteError(w, http.StatusInternalServerError, "internal_error")
	}
}
