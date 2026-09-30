// Package app holds the dashboard use case: the read-only monthly summary that
// replaces the /finanzas command of fin.py. It composes the settings, income,
// savings and tax filing modules through small ports and duplicates none of
// their rules.
package app

import (
	"context"
	"fmt"
	"time"

	dashboard "github.com/valium69mg/finances-app/backend/internal/dashboard/domain"
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

// Movements is the read side of the shared movement storage.
type Movements interface {
	ListByMonth(ctx context.Context, month string, kind ledger.Kind, limit int) ([]ledger.Movement, error)
	ListAllByKind(ctx context.Context, kind ledger.Kind) ([]ledger.Movement, error)
}

// Service implements the dashboard use case.
type Service struct {
	movements Movements
	settings  Settings
	income    Income
	filings   Filings
	now       func() time.Time
}

// NewService builds a Service. A nil now selects time.Now.
func NewService(movements Movements, settings Settings, income Income, filings Filings, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{movements: movements, settings: settings, income: income, filings: filings, now: now}
}

// Month builds the dashboard of a YYYY-MM month (the current one when empty).
// It fails with settings.ErrMissingConfig when the budgets or the emergency
// fund goal cannot be computed; an incomplete RESICO configuration only leaves
// the tax card nil.
func (s *Service) Month(ctx context.Context, month string) (dashboard.Overview, error) {
	if month == "" {
		month = ledger.CurrentMonth(s.now())
	}
	if _, err := time.Parse("2006-01", month); err != nil {
		return dashboard.Overview{}, fmt.Errorf("%w: month %q must be YYYY-MM", ledger.ErrInvalid, month)
	}

	resolved, err := s.settings.MonthBudgets(ctx, month)
	if err != nil {
		return dashboard.Overview{}, err
	}
	budgets := make([]dashboard.Budget, len(resolved))
	for i, b := range resolved {
		budgets[i] = dashboard.Budget{Name: b.Name, Kind: ledger.Kind(b.Kind), Amount: b.Budget}
	}

	expenses, err := s.movements.ListByMonth(ctx, month, ledger.KindExpense, 0)
	if err != nil {
		return dashboard.Overview{}, err
	}
	saved, err := s.movements.ListByMonth(ctx, month, ledger.KindSavings, 0)
	if err != nil {
		return dashboard.Overview{}, err
	}
	summary, err := s.income.MonthSummary(ctx, month)
	if err != nil {
		return dashboard.Overview{}, err
	}

	cfg, err := s.settings.Get(ctx)
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
	out := dashboard.Overview{
		Month:     month,
		Rows:      dashboard.ExpenseRows(budgets, expenses),
		Totals:    totals,
		Available: totals.Available(),
		Emergency: emergency,
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
