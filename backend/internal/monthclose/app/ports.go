package app

import (
	"context"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	settingsapp "github.com/valium69mg/finances-app/backend/internal/settings/app"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
	taxfiling "github.com/valium69mg/finances-app/backend/internal/taxfiling/domain"

	monthclose "github.com/valium69mg/finances-app/backend/internal/monthclose/domain"
)

// Settings is the part of the settings module the month close consumes.
type Settings interface {
	Get(ctx context.Context) (settings.Config, error)
	// MonthBudgets resolves every category budget of a month, with the computed
	// tax budgets and the investment pause overrides applied.
	MonthBudgets(ctx context.Context, month string) ([]settingsapp.CategoryBudget, error)
}

// Movements is the read side of the shared movement storage.
type Movements interface {
	ListByMonth(ctx context.Context, month string, kind ledger.Kind, limit int) ([]ledger.Movement, error)
	ListAllByKind(ctx context.Context, kind ledger.Kind) ([]ledger.Movement, error)
}

// Filings is the part of the tax filing module the close consumes: the payment
// state of the filing of a month.
type Filings interface {
	MonthStatus(ctx context.Context, month string) (taxfiling.MonthStatus, error)
}

// Repo persists the stored closes of the month_closes table.
type Repo interface {
	// Create stores the close as is. It returns monthclose.ErrAlreadyClosed when
	// the period already has one.
	Create(ctx context.Context, c monthclose.Close) (monthclose.Close, error)
	// Get returns monthclose.ErrNotFound when the period has no close.
	Get(ctx context.Context, period string) (monthclose.Close, error)
	// List returns every close, newest period first.
	List(ctx context.Context) ([]monthclose.Close, error)
	// Delete returns monthclose.ErrNotFound when the period has no close.
	Delete(ctx context.Context, period string) error
}
