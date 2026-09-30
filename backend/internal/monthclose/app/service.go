// Package app holds the month close use cases: previewing the close of a
// period, storing it as an immutable snapshot and reading or discarding the
// stored ones. It replaces the /cierre-mes command of fin.py and composes the
// settings, ledger and tax filing modules through small ports, duplicating none
// of their rules.
package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	dashboard "github.com/valium69mg/finances-app/backend/internal/dashboard/domain"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	monthclose "github.com/valium69mg/finances-app/backend/internal/monthclose/domain"
	savings "github.com/valium69mg/finances-app/backend/internal/savings/domain"
)

// Service implements the month close use cases.
type Service struct {
	repo      Repo
	movements Movements
	settings  Settings
	filings   Filings
	now       func() time.Time
	logger    *slog.Logger
}

// NewService builds a Service. A nil now selects time.Now and a nil logger
// slog.Default().
func NewService(repo Repo, movements Movements, settings Settings, filings Filings, now func() time.Time, logger *slog.Logger) *Service {
	if now == nil {
		now = time.Now
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{repo: repo, movements: movements, settings: settings, filings: filings, now: now, logger: logger}
}

// Preview is a close computed now, never stored, together with the stored close
// of the same period when there is one (so the caller can show both).
type Preview struct {
	Close    monthclose.Close
	Existing *monthclose.Close
}

// previousMonth is the month before the current one, the default period.
func (s *Service) previousMonth() string {
	first := s.now().UTC()
	first = time.Date(first.Year(), first.Month(), 1, 0, 0, 0, 0, time.UTC)
	return ledger.CurrentMonth(first.AddDate(0, -1, 0))
}

// compute builds the close of a YYYY-MM period from the current data. It wraps
// settings.ErrMissingConfig when the budgets or the emergency fund goal cannot
// be computed.
func (s *Service) compute(ctx context.Context, period string) (monthclose.Close, error) {
	resolved, err := s.settings.MonthBudgets(ctx, period)
	if err != nil {
		return monthclose.Close{}, err
	}
	budgets := make([]dashboard.Budget, len(resolved))
	for i, b := range resolved {
		budgets[i] = dashboard.Budget{Name: b.Name, Kind: ledger.Kind(b.Kind), Amount: b.Budget}
	}
	cfg, err := s.settings.Get(ctx)
	if err != nil {
		return monthclose.Close{}, err
	}

	var monthly []ledger.Movement
	for _, kind := range []ledger.Kind{ledger.KindIncome, ledger.KindExpense, ledger.KindSavings} {
		rows, err := s.movements.ListByMonth(ctx, period, kind, 0)
		if err != nil {
			return monthclose.Close{}, err
		}
		monthly = append(monthly, rows...)
	}

	// The emergency fund is measured as of the end of the period, so closing an
	// older month is not distorted by later contributions.
	allSavings, err := s.movements.ListAllByKind(ctx, ledger.KindSavings)
	if err != nil {
		return monthclose.Close{}, err
	}
	emergency, err := savings.EmergencyFund(cfg, monthclose.MovementsUpTo(allSavings, period))
	if err != nil {
		return monthclose.Close{}, err
	}

	status, err := s.filings.MonthStatus(ctx, period)
	if err != nil {
		return monthclose.Close{}, err
	}
	return monthclose.Compute(monthclose.Input{
		Period:            period,
		Budgets:           budgets,
		Monthly:           monthly,
		Emergency:         emergency,
		InvestmentsPaused: cfg.Pause != nil && cfg.Pause.IsPaused(period),
		FilingStatus:      status.Payment,
	}), nil
}

// Preview computes the close of a period (the previous month when empty)
// without storing anything, and returns the stored close of that period if any.
func (s *Service) Preview(ctx context.Context, period string) (Preview, error) {
	if period == "" {
		period = s.previousMonth()
	}
	if err := monthclose.ValidatePeriod(period); err != nil {
		return Preview{}, err
	}
	c, err := s.compute(ctx, period)
	if err != nil {
		return Preview{}, err
	}
	out := Preview{Close: c}
	existing, err := s.repo.Get(ctx, period)
	switch {
	case err == nil:
		out.Existing = &existing
	case !errors.Is(err, monthclose.ErrNotFound):
		return Preview{}, err
	}
	return out, nil
}

// Create computes the close of a period and stores it as a snapshot. The
// current month may be closed, a future one may not
// (monthclose.ErrInvalidInput), and a period with a stored close fails with
// monthclose.ErrAlreadyClosed: delete it first to generate it again.
func (s *Service) Create(ctx context.Context, period string) (monthclose.Close, error) {
	if err := monthclose.ValidateClosable(period, s.now()); err != nil {
		return monthclose.Close{}, err
	}
	c, err := s.compute(ctx, period)
	if err != nil {
		return monthclose.Close{}, err
	}
	c.ClosedAt = s.now().UTC().Truncate(time.Microsecond)
	saved, err := s.repo.Create(ctx, c)
	if err != nil {
		return monthclose.Close{}, err
	}
	s.logger.Info("month closed", "period", period)
	return saved, nil
}

// List returns the stored closes, newest period first.
func (s *Service) List(ctx context.Context) ([]monthclose.Close, error) {
	return s.repo.List(ctx)
}

// Get returns the stored close of a period (monthclose.ErrNotFound when none).
func (s *Service) Get(ctx context.Context, period string) (monthclose.Close, error) {
	if err := monthclose.ValidatePeriod(period); err != nil {
		return monthclose.Close{}, err
	}
	return s.repo.Get(ctx, period)
}

// Delete discards the stored close of a period so it can be generated again
// after corrections. Only the snapshot is removed; movements are never touched.
func (s *Service) Delete(ctx context.Context, period string) error {
	if err := monthclose.ValidatePeriod(period); err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, period); err != nil {
		return err
	}
	s.logger.Info("month close deleted", "period", period)
	return nil
}
