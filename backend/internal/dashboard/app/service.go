// Package app holds the dashboard use case: the read-only monthly summary that
// replaces the /finanzas command of fin.py. It composes the settings, income,
// savings and tax filing modules through small ports and duplicates none of
// their rules.
package app

import (
	"context"
	"fmt"
	"time"

	billsapp "github.com/valium69mg/finances-app/backend/internal/bills/app"
	bills "github.com/valium69mg/finances-app/backend/internal/bills/domain"
	dashboard "github.com/valium69mg/finances-app/backend/internal/dashboard/domain"
	futuredomain "github.com/valium69mg/finances-app/backend/internal/futureexpenses/domain"
	incomeapp "github.com/valium69mg/finances-app/backend/internal/income/app"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	savings "github.com/valium69mg/finances-app/backend/internal/savings/domain"
	settingsapp "github.com/valium69mg/finances-app/backend/internal/settings/app"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
	taxfiling "github.com/valium69mg/finances-app/backend/internal/taxfiling/domain"
)

// Settings is the part of the settings module the dashboard consumes.
type Settings interface {
	Get(ctx context.Context) (settings.Config, error)
	// MonthBudgets resolves every category budget of a month, with the computed
	// tax budgets and the investment pause overrides applied.
	MonthBudgets(ctx context.Context, month string) ([]settingsapp.CategoryBudget, error)
}

// Income is the part of the income module the dashboard consumes: the month
// total and the RESICO ISR estimate.
type Income interface {
	MonthSummary(ctx context.Context, month string) (incomeapp.Summary, error)
}

// Filings is the part of the tax filing module the dashboard consumes: the
// payment state of the filing of a month and the state of the previous period.
type Filings interface {
	MonthStatus(ctx context.Context, month string) (taxfiling.MonthStatus, error)
}

// Future is the part of the future expenses module the dashboard consumes: the
// plan of the active items with the free balance.
type Future interface {
	Overview(ctx context.Context) (futuredomain.Plan, error)
}

// Bills is the part of the bills module the dashboard consumes: the active
// bills with their pending occurrence and due flags.
type Bills interface {
	List(ctx context.Context, includeInactive bool) ([]billsapp.Status, error)
}

const (
	// UpcomingDays is how far ahead the upcoming bills card looks.
	UpcomingDays = 14
	// RecentLimit is how many movements the recent list holds.
	RecentLimit = 8
)

// Movements is the read side of the shared movement storage.
type Movements interface {
	ListByRange(ctx context.Context, from, to string, kind ledger.Kind, limit int) ([]ledger.Movement, error)
	ListAllByKind(ctx context.Context, kind ledger.Kind) ([]ledger.Movement, error)
}

// Service implements the dashboard use case.
type Service struct {
	movements Movements
	settings  Settings
	income    Income
	filings   Filings
	bills     Bills
	future    Future
	now       func() time.Time
}

// NewService builds a Service. A nil now selects time.Now.
func NewService(movements Movements, settings Settings, income Income, filings Filings, bills Bills, future Future, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{movements: movements, settings: settings, income: income, filings: filings, bills: bills, future: future, now: now}
}

// Month builds the dashboard of a YYYY-MM budget cycle (the current one when
// empty); the movements are those dated inside the cycle range. The tax card
// stays fiscal: the cycle label is passed as the calendar month. It fails with settings.ErrMissingConfig when the budgets or the emergency
// fund goal cannot be computed; an incomplete RESICO configuration only leaves
// the tax card nil.
func (s *Service) Month(ctx context.Context, month string) (dashboard.Overview, error) {
	cfg, err := s.settings.Get(ctx)
	if err != nil {
		return dashboard.Overview{}, err
	}
	month, from, to, budgets, err := s.cycleBudgets(ctx, cfg, month)
	if err != nil {
		return dashboard.Overview{}, err
	}

	expenses, err := s.movements.ListByRange(ctx, from, to, ledger.KindExpense, 0)
	if err != nil {
		return dashboard.Overview{}, err
	}
	saved, err := s.movements.ListByRange(ctx, from, to, ledger.KindSavings, 0)
	if err != nil {
		return dashboard.Overview{}, err
	}
	summary, err := s.income.MonthSummary(ctx, month)
	if err != nil {
		return dashboard.Overview{}, err
	}

	allSavings, err := s.movements.ListAllByKind(ctx, ledger.KindSavings)
	if err != nil {
		return dashboard.Overview{}, err
	}
	emergency, err := savings.EmergencyFund(cfg, allSavings)
	if err != nil {
		return dashboard.Overview{}, err
	}

	totals := dashboard.Totals{
		Income:   summary.MonthTotalMXN,
		Expenses: ledger.SumBy(expenses, ledger.Filter{Kind: ledger.KindExpense}),
		Savings:  ledger.SumBy(saved, ledger.Filter{Kind: ledger.KindSavings}),
	}
	today := s.now().Format(bills.DateLayout)
	progress, err := dashboard.NewCycleProgress(from, to, today)
	if err != nil {
		return dashboard.Overview{}, err
	}
	statuses, err := s.bills.List(ctx, false)
	if err != nil {
		return dashboard.Overview{}, err
	}
	plan, err := s.future.Overview(ctx)
	if err != nil {
		return dashboard.Overview{}, err
	}
	recent, err := s.movements.ListByRange(ctx, recentFrom, recentTo, "", RecentLimit)
	if err != nil {
		return dashboard.Overview{}, err
	}

	out := dashboard.Overview{
		Month:       month,
		PeriodStart: from,
		PeriodEnd:   to,
		Rows:        dashboard.ExpenseRows(budgets, expenses),
		Totals:      totals,
		Available:   totals.Available(),
		Emergency:   emergency,
		Cycle:       progress,
		Future:      plan,
		Upcoming:    upcomingBills(statuses),
		Recent:      recent,
	}
	if summary.Resico != nil {
		status, err := s.filings.MonthStatus(ctx, month)
		if err != nil {
			return dashboard.Overview{}, err
		}
		out.Tax = &dashboard.TaxCard{Rate: summary.Resico.Rate, EstimatedISR: summary.Resico.EstimatedISR, Filing: status}
	}
	return out, nil
}

// Budget builds the reduced dashboard of the household role: only the cycle
// and the per-category budget rows (what was spent against each budget). It
// never reads the income, savings, tax, bills, future expenses or recent
// movements, so nothing else can leak through the payload.
func (s *Service) Budget(ctx context.Context, month string) (dashboard.BudgetView, error) {
	cfg, err := s.settings.Get(ctx)
	if err != nil {
		return dashboard.BudgetView{}, err
	}
	month, from, to, budgets, err := s.cycleBudgets(ctx, cfg, month)
	if err != nil {
		return dashboard.BudgetView{}, err
	}
	expenses, err := s.movements.ListByRange(ctx, from, to, ledger.KindExpense, 0)
	if err != nil {
		return dashboard.BudgetView{}, err
	}
	return dashboard.BudgetView{Month: month, PeriodStart: from, PeriodEnd: to, Rows: dashboard.ExpenseRows(budgets, expenses)}, nil
}

// cycleBudgets resolves the budget cycle of month (the current one when empty)
// and the budgets of that month.
func (s *Service) cycleBudgets(ctx context.Context, cfg settings.Config, month string) (resolvedMonth, from, to string, budgets []dashboard.Budget, err error) {
	cycle := cfg.Cycle()
	if month == "" {
		month = cycle.Current(s.now())
	}
	from, to, err = cycle.Range(month)
	if err != nil {
		return "", "", "", nil, fmt.Errorf("%w: month %q must be YYYY-MM", ledger.ErrInvalid, month)
	}
	resolved, err := s.settings.MonthBudgets(ctx, month)
	if err != nil {
		return "", "", "", nil, err
	}
	budgets = make([]dashboard.Budget, len(resolved))
	for i, b := range resolved {
		budgets[i] = dashboard.Budget{Name: b.Name, Kind: ledger.Kind(b.Kind), Amount: b.Budget}
	}
	return month, from, to, budgets, nil
}

// Bounds of the recent movements query: every movement, whatever its date.
const (
	recentFrom = "1900-01-01"
	recentTo   = "9999-12-31"
)

// upcomingBills keeps the pending occurrences due within UpcomingDays days,
// overdue ones included. The bills arrive sorted by due date.
func upcomingBills(statuses []billsapp.Status) []dashboard.UpcomingBill {
	out := []dashboard.UpcomingBill{}
	for _, st := range statuses {
		b := st.Bill
		if !b.Active || b.Pending == nil || st.DaysUntilDue > UpcomingDays {
			continue
		}
		out = append(out, dashboard.UpcomingBill{
			ID: b.ID, Name: b.Name, Category: b.Category, Amount: b.Amount, Currency: b.Currency,
			DueDate: b.Pending.DueDate, DaysUntilDue: st.DaysUntilDue, Overdue: st.Overdue,
		})
	}
	return out
}
