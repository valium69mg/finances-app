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

// taxDTO is the estimated RESICO ISR of the month plus its filing status.
// filing_status is the payment state of the filing of this month (ninguna when
// not filed yet, pendiente when filed and unpaid, pagada); previous_period is
// the month before and previous_period_pending tells whether it still needs
// action (unfiled with issued invoices, or filed and unpaid).
type taxDTO struct {
	Rate                  decimal.Decimal `json:"rate"`
	EstimatedISR          decimal.Decimal `json:"estimated_isr"`
	FilingStatus          string          `json:"filing_status"`
	PreviousPeriod        string          `json:"previous_period"`
	PreviousPeriodPending bool            `json:"previous_period_pending"`
}

// cycleDTO places today in the displayed cycle: day is 1 on its first day, 0
// before it starts and days once it is over.
type cycleDTO struct {
	Today string `json:"today"`
	Day   int    `json:"day"`
	Days  int    `json:"days"`
}

// futureExpenseDTO is one active future expense: what is saved towards it (the
// savings linked to it) and the amount to put aside each cycle
// (suggested_monthly) to have it on time.
type futureExpenseDTO struct {
	ID               int             `json:"id"`
	Name             string          `json:"name"`
	DueDate          string          `json:"due_date"`
	Target           decimal.Decimal `json:"target"`
	Saved            decimal.Decimal `json:"saved"`
	Remaining        decimal.Decimal `json:"remaining"`
	SuggestedMonthly decimal.Decimal `json:"suggested_monthly"`
	CyclesLeft       int             `json:"cycles_left"`
}

type futureExpensesDTO struct {
	Items            []futureExpenseDTO `json:"items"`
	Target           decimal.Decimal    `json:"target"`
	Saved            decimal.Decimal    `json:"saved"`
	Remaining        decimal.Decimal    `json:"remaining"`
	SuggestedMonthly decimal.Decimal    `json:"suggested_monthly"`
	// FreeBalance is the Gastos futuros savings linked to no item yet.
	FreeBalance decimal.Decimal `json:"free_balance"`
}

// upcomingBillDTO is a bill due soon; amount is null for a variable bill.
type upcomingBillDTO struct {
	ID           int              `json:"id"`
	Name         string           `json:"name"`
	Category     string           `json:"category"`
	Amount       *decimal.Decimal `json:"amount"`
	Currency     string           `json:"currency"`
	DueDate      string           `json:"due_date"`
	DaysUntilDue int              `json:"days_until_due"`
	Overdue      bool             `json:"overdue"`
}

// recentMovementDTO is one line of the mixed recent movements list.
type recentMovementDTO struct {
	ID          int             `json:"id"`
	Date        string          `json:"date"`
	Kind        string          `json:"kind"`
	Description string          `json:"description"`
	Category    string          `json:"category"`
	AmountMXN   decimal.Decimal `json:"amount_mxn"`
}

// dashboardDTO is the body of GET /dashboard. Tax is null when the tax
// settings are incomplete.
type dashboardDTO struct {
	Month       string          `json:"month"`
	PeriodStart string          `json:"period_start"`
	PeriodEnd   string          `json:"period_end"`
	Categories  []categoryDTO   `json:"categories"`
	Income      decimal.Decimal `json:"income"`
	Expenses    decimal.Decimal `json:"expenses"`
	Savings     decimal.Decimal `json:"savings"`
	Available   decimal.Decimal `json:"available"`
	Emergency   emergencyDTO    `json:"emergency"`
	Tax         *taxDTO         `json:"tax"`

	Cycle          cycleDTO            `json:"cycle"`
	FutureExpenses futureExpensesDTO   `json:"future_expenses"`
	UpcomingBills  []upcomingBillDTO   `json:"upcoming_bills"`
	Recent         []recentMovementDTO `json:"recent_movements"`
}

func toDTO(d dashboard.Overview) dashboardDTO {
	out := dashboardDTO{
		Month:       d.Month,
		PeriodStart: d.PeriodStart,
		PeriodEnd:   d.PeriodEnd,
		Categories:  make([]categoryDTO, len(d.Rows)),
		Income:      d.Totals.Income,
		Expenses:    d.Totals.Expenses,
		Savings:     d.Totals.Savings,
		Available:   d.Available,
		Emergency:   emergencyDTO{Accumulated: d.Emergency.Accumulated, Goal: d.Emergency.Goal},
		Cycle:       cycleDTO{Today: d.Cycle.Today, Day: d.Cycle.Day, Days: d.Cycle.Days},
		FutureExpenses: futureExpensesDTO{
			Items:  make([]futureExpenseDTO, len(d.Future.Items)),
			Target: d.Future.Target, Saved: d.Future.Saved, Remaining: d.Future.Remaining, SuggestedMonthly: d.Future.Suggested,
			FreeBalance: d.Future.FreeBalance,
		},
		UpcomingBills: make([]upcomingBillDTO, len(d.Upcoming)),
		Recent:        make([]recentMovementDTO, len(d.Recent)),
	}
	for i, f := range d.Future.Items {
		out.FutureExpenses.Items[i] = futureExpenseDTO{
			ID: f.ID, Name: f.Name, DueDate: f.DueDate, Target: f.Target, Saved: f.Saved, Remaining: f.Remaining,
			SuggestedMonthly: f.Suggested, CyclesLeft: f.CyclesLeft,
		}
	}
	for i, b := range d.Upcoming {
		out.UpcomingBills[i] = upcomingBillDTO{
			ID: b.ID, Name: b.Name, Category: b.Category, Amount: b.Amount, Currency: b.Currency,
			DueDate: b.DueDate, DaysUntilDue: b.DaysUntilDue, Overdue: b.Overdue,
		}
	}
	for i, m := range d.Recent {
		out.Recent[i] = recentMovementDTO{
			ID: m.ID, Date: m.Date, Kind: string(m.Kind), Description: m.Description, Category: m.Category, AmountMXN: m.AmountMXN,
		}
	}
	for i, r := range d.Rows {
		out.Categories[i] = categoryDTO{Category: r.Name, Spent: r.Real, Budget: r.Budget, Remaining: r.Diff, OverBudget: r.OverBudget()}
	}
	if d.Tax != nil {
		out.Tax = &taxDTO{
			Rate: d.Tax.Rate, EstimatedISR: d.Tax.EstimatedISR, FilingStatus: string(d.Tax.Filing.Payment),
			PreviousPeriod: d.Tax.Filing.PreviousPeriod, PreviousPeriodPending: d.Tax.Filing.PreviousPending,
		}
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
