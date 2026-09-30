// Package app holds the future expenses use cases: managing the items the
// owner saves for, contributing to them, assigning the free balance and paying
// them. The Ahorro and Gasto movements are built through the savings and
// expenses services so their rules stay in one place; only the atomic payment
// is written by the repository.
package app

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	expensesapp "github.com/valium69mg/finances-app/backend/internal/expenses/app"
	domain "github.com/valium69mg/finances-app/backend/internal/futureexpenses/domain"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	savingsapp "github.com/valium69mg/finances-app/backend/internal/savings/app"
)

// Service implements the future expenses use cases.
type Service struct {
	repo     Repo
	savings  Savings
	expenses Expenses
	settings Settings
	now      func() time.Time
}

// NewService builds a Service. A nil now selects time.Now; the composition root
// passes a clock in TZ_NAME so "today" and the cycles never follow UTC.
func NewService(repo Repo, savings Savings, expenses Expenses, settings Settings, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, savings: savings, expenses: expenses, settings: settings, now: now}
}

func (s *Service) today() string { return s.now().Format(domain.DateLayout) }

// dateOrToday fills an empty date with today in the configured zone, so the
// savings rows never take the date of the process clock (UTC).
func (s *Service) dateOrToday(date string) string {
	if strings.TrimSpace(date) == "" {
		return s.today()
	}
	return date
}

// planned attaches the plan of one item (cycle and today in the configured zone).
func (s *Service) planned(ctx context.Context, it domain.FutureExpense) (domain.Planned, error) {
	cfg, err := s.settings.Get(ctx)
	if err != nil {
		return domain.Planned{}, err
	}
	return domain.PlanItem(cfg.Cycle(), s.today(), it)
}

// Create stores an active item.
func (s *Service) Create(ctx context.Context, in domain.Input) (domain.Planned, error) {
	v, err := domain.Validate(in)
	if err != nil {
		return domain.Planned{}, err
	}
	it, err := s.repo.Create(ctx, v)
	if err != nil {
		return domain.Planned{}, err
	}
	return s.planned(ctx, it)
}

// Get returns one item with its plan.
func (s *Service) Get(ctx context.Context, id int) (domain.Planned, error) {
	it, err := s.repo.Get(ctx, id)
	if err != nil {
		return domain.Planned{}, err
	}
	return s.planned(ctx, it)
}

// Update replaces the name, target and due date of an active item. A paid item
// is a record of what happened and cannot change (domain.ErrAlreadyPaid).
func (s *Service) Update(ctx context.Context, id int, in domain.Input) (domain.Planned, error) {
	v, err := domain.Validate(in)
	if err != nil {
		return domain.Planned{}, err
	}
	it, err := s.repo.Update(ctx, id, v)
	if err != nil {
		return domain.Planned{}, err
	}
	return s.planned(ctx, it)
}

// Delete removes an item of any status. Savings linked to it are not deleted:
// they become free balance again, so no money is orphaned. The Gasto of a paid
// item is never touched.
func (s *Service) Delete(ctx context.Context, id int) error {
	return s.repo.Delete(ctx, id)
}

// Listing is every item: the plan of the active ones plus the paid history.
type Listing struct {
	Plan domain.Plan
	Paid []domain.Planned
}

// List returns the plan of the active items (earliest due date first) and the
// paid items (most recently paid first).
func (s *Service) List(ctx context.Context) (Listing, error) {
	items, err := s.repo.List(ctx)
	if err != nil {
		return Listing{}, err
	}
	plan, err := s.plan(ctx, items)
	if err != nil {
		return Listing{}, err
	}
	out := Listing{Plan: plan, Paid: []domain.Planned{}}
	for _, it := range items {
		if it.Status == domain.StatusPaid {
			out.Paid = append(out.Paid, domain.Planned{FutureExpense: it})
		}
	}
	sort.SliceStable(out.Paid, func(i, j int) bool {
		if out.Paid[i].PaidAt != out.Paid[j].PaidAt {
			return out.Paid[i].PaidAt > out.Paid[j].PaidAt
		}
		return out.Paid[i].ID > out.Paid[j].ID
	})
	return out, nil
}

// Overview is the plan of the active items, what the dashboard shows.
func (s *Service) Overview(ctx context.Context) (domain.Plan, error) {
	items, err := s.repo.List(ctx)
	if err != nil {
		return domain.Plan{}, err
	}
	return s.plan(ctx, items)
}

func (s *Service) plan(ctx context.Context, items []domain.FutureExpense) (domain.Plan, error) {
	cfg, err := s.settings.Get(ctx)
	if err != nil {
		return domain.Plan{}, err
	}
	free, err := s.repo.FreeBalance(ctx)
	if err != nil {
		return domain.Plan{}, err
	}
	return domain.BuildPlan(cfg.Cycle(), s.today(), items, free)
}

// active loads an item that can still receive savings or be paid.
func (s *Service) active(ctx context.Context, id int) (domain.FutureExpense, error) {
	it, err := s.repo.Get(ctx, id)
	if err != nil {
		return domain.FutureExpense{}, err
	}
	if it.Status == domain.StatusPaid {
		return domain.FutureExpense{}, domain.ErrAlreadyPaid
	}
	return it, nil
}

// SavingInput is a contribution: Amount is MXN and greater than zero, Date
// defaults to today and Description to "Ahorro para <name>".
type SavingInput struct {
	Amount      decimal.Decimal
	Date        string
	Description string
}

// Contribute registers an Ahorro movement in the Gastos futuros category linked
// to the item, through the savings service (instrument, payment method and
// range rules included).
func (s *Service) Contribute(ctx context.Context, id int, in SavingInput) (domain.Planned, ledger.Movement, error) {
	it, err := s.active(ctx, id)
	if err != nil {
		return domain.Planned{}, ledger.Movement{}, err
	}
	if err := domain.ValidateMoney("amount", in.Amount); err != nil {
		return domain.Planned{}, ledger.Movement{}, err
	}
	description := strings.TrimSpace(in.Description)
	if description == "" {
		description = "Ahorro para " + it.Name
	}
	m, err := s.savings.CreateSaving(ctx, savingsapp.Input{
		Date: s.dateOrToday(in.Date), Description: description, Category: domain.SavingsCategory, Amount: in.Amount, FutureExpenseID: id,
	})
	if err != nil {
		return domain.Planned{}, ledger.Movement{}, err
	}
	planned, err := s.Get(ctx, id)
	return planned, m, err
}

// AssignInput moves Amount (> 0) of the free balance to an item. Date defaults to today.
type AssignInput struct {
	Amount decimal.Decimal
	Date   string
}

// Assign moves part of the free balance to an item as a transfer: two Ahorro
// rows of the Gastos futuros category stored atomically, -amount linked to no
// item and +amount linked to the item, so the savings portfolio does not move.
// It is never automatic: the owner picks the item and the amount. More than the
// free balance is domain.ErrInsufficientFreeBalance.
func (s *Service) Assign(ctx context.Context, id int, in AssignInput) (domain.Planned, error) {
	it, err := s.active(ctx, id)
	if err != nil {
		return domain.Planned{}, err
	}
	if err := domain.ValidateMoney("amount", in.Amount); err != nil {
		return domain.Planned{}, err
	}
	free, err := s.repo.FreeBalance(ctx)
	if err != nil {
		return domain.Planned{}, err
	}
	if in.Amount.GreaterThan(free) {
		return domain.Planned{}, domain.ErrInsufficientFreeBalance
	}
	description := "Asignado a " + it.Name
	date := s.dateOrToday(in.Date)
	_, err = s.savings.CreateMany(ctx, []savingsapp.Input{
		{Date: date, Description: description, Category: domain.SavingsCategory, Amount: in.Amount.Neg()},
		{Date: date, Description: description, Category: domain.SavingsCategory, Amount: in.Amount, FutureExpenseID: id},
	})
	if err != nil {
		return domain.Planned{}, err
	}
	return s.Get(ctx, id)
}

// PayInput is the payment of an item. Every field is optional: Date defaults to
// today, Amount (what was actually paid, MXN) to the target and Category to the
// Gasto category inferred from the name, else domain.DefaultExpenseCategory.
type PayInput struct {
	Date     string
	Amount   *decimal.Decimal
	Category string
}

// Pay marks the item paid in one transaction: it registers the real Gasto
// (through the expenses rules), releases the saved money with a negative Ahorro
// row linked to the item (so the savings portfolio stays true, the money was
// spent) and sets status paid and paid_at. When more was saved than paid, the
// remainder goes back to the free balance as a positive unlinked row; when less,
// only what exists is released. Paying twice is domain.ErrAlreadyPaid.
func (s *Service) Pay(ctx context.Context, id int, in PayInput) (domain.Planned, ledger.Movement, error) {
	it, err := s.active(ctx, id)
	if err != nil {
		return domain.Planned{}, ledger.Movement{}, err
	}
	amount := it.Target
	if in.Amount != nil {
		amount = *in.Amount
	}
	if err := domain.ValidateMoney("amount", amount); err != nil {
		return domain.Planned{}, ledger.Movement{}, err
	}
	category := strings.TrimSpace(in.Category)
	if category == "" {
		inferred, ok, err := s.expenses.InferCategory(ctx, it.Name)
		if err != nil {
			return domain.Planned{}, ledger.Movement{}, err
		}
		category = domain.DefaultExpenseCategory
		if ok {
			category = inferred
		}
	}
	expense, err := s.expenses.Build(ctx, expensesapp.Input{
		Date: in.Date, Description: it.Name, Category: category, Amount: amount,
	})
	if err != nil {
		return domain.Planned{}, ledger.Movement{}, err
	}
	paid, err := s.repo.MarkPaid(ctx, id, func(saved decimal.Decimal) (Settlement, error) {
		return s.settlement(ctx, it, expense, saved)
	})
	if err != nil {
		return domain.Planned{}, ledger.Movement{}, err
	}
	if paid.ExpenseMovementID != nil {
		expense.ID = *paid.ExpenseMovementID
	}
	return domain.Planned{FutureExpense: paid}, expense, nil
}

// settlement builds the rows of a payment for the amount saved inside the
// transaction.
func (s *Service) settlement(ctx context.Context, it domain.FutureExpense, expense ledger.Movement, saved decimal.Decimal) (Settlement, error) {
	out := Settlement{Expense: expense}
	linked, remainder := domain.Release(saved, expense.AmountMXN)
	if linked.IsPositive() {
		m, err := s.savings.Build(ctx, savingsapp.Input{
			Date: expense.Date, Description: "Pago de " + it.Name, Category: domain.SavingsCategory,
			Amount: linked.Neg(), FutureExpenseID: it.ID,
		})
		if err != nil {
			return Settlement{}, err
		}
		out.Savings = append(out.Savings, m)
	}
	if remainder.IsPositive() {
		m, err := s.savings.Build(ctx, savingsapp.Input{
			Date: expense.Date, Description: "Sobrante de " + it.Name, Category: domain.SavingsCategory, Amount: remainder,
		})
		if err != nil {
			return Settlement{}, err
		}
		out.Savings = append(out.Savings, m)
	}
	return out, nil
}
