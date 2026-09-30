// Package app holds the expenses use cases: registering, correcting and
// listing Gasto movements with budget feedback.
package app

import (
	"context"
	"errors"
	"fmt"
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
}

// NewService builds a Service. A nil now selects time.Now.
func NewService(repo ledgerapp.MovementRepo, settings Settings, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, settings: settings, now: now}
}

func catalogOf(cfg settings.Config) ledger.Catalog {
	cats := make([]ledger.CatalogCategory, len(cfg.Categories))
	for i, c := range cfg.Categories {
		cats[i] = ledger.CatalogCategory{Name: c.Name, Kind: c.Kind}
	}
	return ledger.Catalog{
		Categories:     cats,
		PaymentMethods: cfg.PaymentMethods,
		DefaultRate:    cfg.General().FXRateApplied,
	}
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
	}, catalogOf(cfg), s.now().Format("2006-01-02"))
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

// List returns the expenses of a YYYY-MM month (the current one when empty),
// newest first, at most limit of them (DefaultListLimit when not positive).
func (s *Service) List(ctx context.Context, month string, limit int) ([]ledger.Movement, error) {
	if month == "" {
		month = ledger.CurrentMonth(s.now())
	}
	if _, err := time.Parse("2006-01", month); err != nil {
		return nil, fmt.Errorf("%w: month %q must be YYYY-MM", ledger.ErrInvalid, month)
	}
	if limit <= 0 {
		limit = DefaultListLimit
	}
	return s.repo.ListByMonth(ctx, month, ledger.KindExpense, limit)
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

// result attaches the budget feedback to a saved expense.
func (s *Service) result(ctx context.Context, m ledger.Movement) (Result, error) {
	fb, err := s.feedback(ctx, m)
	if errors.Is(err, settings.ErrMissingConfig) {
		return Result{Movement: m}, nil
	}
	if err != nil {
		return Result{}, err
	}
	return Result{Movement: m, Feedback: &fb}, nil
}

func (s *Service) feedback(ctx context.Context, m ledger.Movement) (BudgetFeedback, error) {
	month := ledger.MonthOf(m.Date)
	budgets, err := s.settings.MonthBudgets(ctx, month)
	if err != nil {
		return BudgetFeedback{}, err
	}
	movements, err := s.repo.ListByMonth(ctx, month, ledger.KindExpense, 0)
	if err != nil {
		return BudgetFeedback{}, err
	}
	fb := BudgetFeedback{
		Month:    month,
		Category: m.Category,
		Spent:    ledger.SumBy(movements, ledger.Filter{Kind: ledger.KindExpense, Category: m.Category}),
	}
	for _, b := range budgets {
		if b.Name != m.Category || b.Budget == nil || !b.Budget.IsPositive() {
			continue
		}
		budget := *b.Budget
		remaining := budget.Sub(fb.Spent)
		fb.Budget, fb.Remaining = &budget, &remaining
		fb.OverBudget = fb.Spent.GreaterThan(budget)
	}
	return fb, nil
}
