// Package app holds the expenses use cases: registering, correcting and
// listing Gasto movements with budget feedback.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/shopspring/decimal"

	ledgerapp "github.com/valium69mg/finances-app/backend/internal/ledger/app"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	settingsapp "github.com/valium69mg/finances-app/backend/internal/settings/app"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

// DefaultListLimit is the number of expenses listed when no limit is given.
const DefaultListLimit = 20

// Settings is the part of the settings module the expenses use cases consume.
type Settings interface {
	Get(ctx context.Context) (settings.Config, error)
	MonthBudgets(ctx context.Context, month string) ([]settingsapp.CategoryBudget, error)
}

// Input is the data of a new or updated expense. Empty optional fields take
// the defaults of ledger.NewMovement; an empty Category is inferred from the
// description.
type Input struct {
	Date          string
	Description   string
	Category      string
	PaymentMethod string
	Currency      string
	Amount        decimal.Decimal
	ExchangeRate  *decimal.Decimal
}

// BudgetFeedback compares the spending of a category in the month of a
// movement with its budget. Budget and Remaining are nil when the category has
// no budget (a nil or zero configured budget).
type BudgetFeedback struct {
	Month      string
	Category   string
	Budget     *decimal.Decimal
	Spent      decimal.Decimal
	Remaining  *decimal.Decimal
	OverBudget bool
}

// Result is a saved expense with its budget feedback. Feedback is nil when the
// month's budgets cannot be resolved because the tax settings are incomplete.
type Result struct {
	Movement ledger.Movement
	Feedback *BudgetFeedback
}

// Service implements the expenses use cases.
type Service struct {
	repo     ledgerapp.MovementRepo
	settings Settings
	now      func() time.Time
	logger   *slog.Logger
}

// NewService builds a Service. A nil now selects time.Now.
func NewService(repo ledgerapp.MovementRepo, settings Settings, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, settings: settings, now: now, logger: slog.Default()}
}

// WithLogger sets the logger used to report a budget feedback that could not be
// computed. A nil logger keeps slog.Default().
func (s *Service) WithLogger(l *slog.Logger) *Service {
	if l != nil {
		s.logger = l
	}
	return s
}

// build validates the input into a Gasto movement, inferring the category when
// it is empty.
func (s *Service) build(ctx context.Context, in Input) (ledger.Movement, error) {
	cfg, err := s.settings.Get(ctx)
	if err != nil {
		return ledger.Movement{}, err
	}
	category := in.Category
	if category == "" {
		inferred, ok := cfg.InferCategory(in.Description, ledger.KindExpense)
		if !ok {
			return ledger.Movement{}, fmt.Errorf("%w: could not infer the category from the description, choose one", ledger.ErrInvalid)
		}
		category = inferred
	}
	return ledger.NewMovement(ledger.MovementInput{
		Date: in.Date, Description: in.Description, Category: category, Kind: ledger.KindExpense,
		PaymentMethod: in.PaymentMethod, Currency: in.Currency, Amount: in.Amount, ExchangeRate: in.ExchangeRate,
	}, cfg.Catalog(), s.now().Format("2006-01-02"))
}

// Build validates the input into a Gasto movement without storing it, so
// modules that write the expense atomically with their own rows reuse the
// category, payment method and amount rules of this module.
func (s *Service) Build(ctx context.Context, in Input) (ledger.Movement, error) {
	return s.build(ctx, in)
}

// Create registers an expense and returns it with the budget feedback of its month.
func (s *Service) Create(ctx context.Context, in Input) (Result, error) {
	m, err := s.build(ctx, in)
	if err != nil {
		return Result{}, err
	}
	saved, err := s.repo.Create(ctx, m)
	if err != nil {
		return Result{}, err
	}
	return s.result(ctx, saved)
}

// Update replaces the expense with the given ID. Movements of another kind are
// reported as not found: this module only manages expenses.
func (s *Service) Update(ctx context.Context, id int, in Input) (Result, error) {
	if _, err := s.expense(ctx, id); err != nil {
		return Result{}, err
	}
	m, err := s.build(ctx, in)
	if err != nil {
		return Result{}, err
	}
	m.ID = id
	if err := s.repo.Update(ctx, m); err != nil {
		return Result{}, err
	}
	return s.result(ctx, m)
}

// Delete removes an expense. Movements of another kind are reported as not found.
func (s *Service) Delete(ctx context.Context, id int) error {
	if _, err := s.expense(ctx, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

// List returns the expenses of a YYYY-MM budget cycle (the current one when
// empty), newest first, at most limit of them (DefaultListLimit when not
// positive).
func (s *Service) List(ctx context.Context, month string, limit int) ([]ledger.Movement, error) {
	cfg, err := s.settings.Get(ctx)
	if err != nil {
		return nil, err
	}
	cycle := cfg.Cycle()
	if month == "" {
		month = cycle.Current(s.now())
	}
	from, to, err := cycle.Range(month)
	if err != nil {
		return nil, fmt.Errorf("%w: month %q must be YYYY-MM", ledger.ErrInvalid, month)
	}
	if limit <= 0 {
		limit = DefaultListLimit
	}
	return s.repo.ListByRange(ctx, from, to, ledger.KindExpense, limit)
}

// InferCategory suggests the expense category for a description.
func (s *Service) InferCategory(ctx context.Context, description string) (string, bool, error) {
	cfg, err := s.settings.Get(ctx)
	if err != nil {
		return "", false, err
	}
	name, ok := cfg.InferCategory(description, ledger.KindExpense)
	return name, ok, nil
}

func (s *Service) expense(ctx context.Context, id int) (ledger.Movement, error) {
	m, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return ledger.Movement{}, err
	}
	if m.Kind != ledger.KindExpense {
		return ledger.Movement{}, ledger.ErrNotFound
	}
	return m, nil
}

// result attaches the budget feedback to a saved expense. The expense is
// already persisted, so a feedback that cannot be computed never turns into an
// error: callers (bills, tax filing) would leave a saved expense unlinked and a
// retry would duplicate it. The feedback is omitted instead (a warning is
// logged unless the tax settings are simply incomplete).
func (s *Service) result(ctx context.Context, m ledger.Movement) (Result, error) {
	fb, err := s.feedback(ctx, m)
	if err != nil {
		if !errors.Is(err, settings.ErrMissingConfig) {
			s.logger.Warn("expense saved but its budget feedback could not be computed", "movement", m.ID, "error", err)
		}
		return Result{Movement: m}, nil
	}
	return Result{Movement: m, Feedback: &fb}, nil
}

func (s *Service) feedback(ctx context.Context, m ledger.Movement) (BudgetFeedback, error) {
	return s.BudgetFor(ctx, m.Category, m.Date)
}

// BudgetFor compares the spending of a category in the pay cycle that contains
// date (YYYY-MM-DD) with its budget of that cycle. The budgets are resolved
// exactly as the dashboard resolves them (month overrides and the investment
// pause plan included), so every screen shows the same numbers. Nothing is
// stored.
func (s *Service) BudgetFor(ctx context.Context, category, date string) (BudgetFeedback, error) {
	cfg, err := s.settings.Get(ctx)
	if err != nil {
		return BudgetFeedback{}, err
	}
	cycle := cfg.Cycle()
	month := cycle.Of(date)
	from, to, err := cycle.Range(month)
	if err != nil {
		return BudgetFeedback{}, err
	}
	budgets, err := s.settings.MonthBudgets(ctx, month)
	if err != nil {
		return BudgetFeedback{}, err
	}
	movements, err := s.repo.ListByRange(ctx, from, to, ledger.KindExpense, 0)
	if err != nil {
		return BudgetFeedback{}, err
	}
	fb := BudgetFeedback{
		Month:    month,
		Category: category,
		Spent:    ledger.SumBy(movements, ledger.Filter{Kind: ledger.KindExpense, Category: category}),
	}
	for _, b := range budgets {
		if b.Name != category || b.Budget == nil || !b.Budget.IsPositive() {
			continue
		}
		budget := *b.Budget
		remaining := budget.Sub(fb.Spent)
		fb.Budget, fb.Remaining = &budget, &remaining
		fb.OverBudget = fb.Spent.GreaterThan(budget)
	}
	return fb, nil
}
