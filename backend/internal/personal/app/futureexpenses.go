package app

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/personal/domain"
)

// The future expenses of a household user. Same behaviour as the owner's module
// (internal/futureexpenses) over her own tables: the rules are the owner's pure
// domain, the rows are her Ahorro and Gasto movements.

func (s *Service) today() string { return s.now().Format(domain.DateLayout) }

func (s *Service) dateOrToday(date string) (string, error) {
	if strings.TrimSpace(date) == "" {
		return s.today(), nil
	}
	return domain.ParseDate(date)
}

// planned attaches the plan of one item (the owner's cycle start day, today in
// the configured zone).
func (s *Service) planned(ctx context.Context, it domain.FutureExpense) (domain.FuturePlanned, error) {
	cfg, err := s.settings.Get(ctx)
	if err != nil {
		return domain.FuturePlanned{}, err
	}
	return domain.PlanItem(cfg.Cycle(), s.today(), it)
}

// CreateFutureExpense stores an active item of the caller.
func (s *Service) CreateFutureExpense(ctx context.Context, actor Identity, in domain.FutureInput) (domain.FuturePlanned, error) {
	if err := require(actor); err != nil {
		return domain.FuturePlanned{}, err
	}
	v, err := domain.ValidateFuture(in)
	if err != nil {
		return domain.FuturePlanned{}, err
	}
	it, err := s.repo.CreateFutureExpense(ctx, actor.UserID, v)
	if err != nil {
		return domain.FuturePlanned{}, err
	}
	return s.planned(ctx, it)
}

// FutureExpense returns one item of the caller with its plan.
func (s *Service) FutureExpense(ctx context.Context, actor Identity, id int) (domain.FuturePlanned, error) {
	if err := require(actor); err != nil {
		return domain.FuturePlanned{}, err
	}
	it, err := s.repo.GetFutureExpense(ctx, actor.UserID, id)
	if err != nil {
		return domain.FuturePlanned{}, err
	}
	return s.planned(ctx, it)
}

// UpdateFutureExpense replaces name, target and due date of an active item. A
// paid item is a record of what happened (domain.ErrAlreadyPaid).
func (s *Service) UpdateFutureExpense(ctx context.Context, actor Identity, id int, in domain.FutureInput) (domain.FuturePlanned, error) {
	if err := require(actor); err != nil {
		return domain.FuturePlanned{}, err
	}
	v, err := domain.ValidateFuture(in)
	if err != nil {
		return domain.FuturePlanned{}, err
	}
	it, err := s.repo.UpdateFutureExpense(ctx, actor.UserID, id, v)
	if err != nil {
		return domain.FuturePlanned{}, err
	}
	return s.planned(ctx, it)
}

// DeleteFutureExpense removes an item of any status. Its savings are not
// deleted: they become free balance again. The Gasto of a paid item is never
// touched.
func (s *Service) DeleteFutureExpense(ctx context.Context, actor Identity, id int) error {
	if err := require(actor); err != nil {
		return err
	}
	return s.repo.DeleteFutureExpense(ctx, actor.UserID, id)
}

// FutureListing is every item of the caller: the plan of the active ones plus
// the paid history.
type FutureListing struct {
	Plan domain.FuturePlan
	Paid []domain.FuturePlanned
}

// FutureExpenses returns the plan of the active items (earliest due date first)
// and the paid items (most recently paid first).
func (s *Service) FutureExpenses(ctx context.Context, actor Identity) (FutureListing, error) {
	if err := require(actor); err != nil {
		return FutureListing{}, err
	}
	items, err := s.repo.ListFutureExpenses(ctx, actor.UserID)
	if err != nil {
		return FutureListing{}, err
	}
	cfg, err := s.settings.Get(ctx)
	if err != nil {
		return FutureListing{}, err
	}
	free, err := s.repo.FutureFreeBalance(ctx, actor.UserID)
	if err != nil {
		return FutureListing{}, err
	}
	plan, err := domain.BuildPlan(cfg.Cycle(), s.today(), items, free)
	if err != nil {
		return FutureListing{}, err
	}
	out := FutureListing{Plan: plan, Paid: []domain.FuturePlanned{}}
	for _, it := range items {
		if it.Status == domain.FuturePaid {
			out.Paid = append(out.Paid, domain.FuturePlanned{FutureExpense: it})
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

// active loads an item that can still receive savings or be paid.
func (s *Service) active(ctx context.Context, userID string, id int) (domain.FutureExpense, error) {
	it, err := s.repo.GetFutureExpense(ctx, userID, id)
	if err != nil {
		return domain.FutureExpense{}, err
	}
	if it.Status == domain.FuturePaid {
		return domain.FutureExpense{}, domain.ErrAlreadyPaid
	}
	return it, nil
}

// FutureSavingInput is a contribution: Amount is MXN and greater than zero,
// Date defaults to today and Description to "Ahorro para <name>".
type FutureSavingInput struct {
	Amount      decimal.Decimal
	Date        string
	Description string
}

// ContributeFuture registers an Ahorro in her Gastos futuros category linked to
// the item.
func (s *Service) ContributeFuture(ctx context.Context, actor Identity, id int, in FutureSavingInput) (domain.FuturePlanned, domain.Movement, error) {
	if err := require(actor); err != nil {
		return domain.FuturePlanned{}, domain.Movement{}, err
	}
	it, err := s.active(ctx, actor.UserID, id)
	if err != nil {
		return domain.FuturePlanned{}, domain.Movement{}, err
	}
	if err := domain.ValidateMoney("amount", in.Amount); err != nil {
		return domain.FuturePlanned{}, domain.Movement{}, err
	}
	description, err := domain.ValidateSavingDescription(in.Description)
	if err != nil {
		return domain.FuturePlanned{}, domain.Movement{}, err
	}
	if description == "" {
		description = domain.TrimDescription("Ahorro para " + it.Name)
	}
	date, err := s.dateOrToday(in.Date)
	if err != nil {
		return domain.FuturePlanned{}, domain.Movement{}, err
	}
	if err := s.repo.SeedDefaults(ctx, actor.UserID); err != nil {
		return domain.FuturePlanned{}, domain.Movement{}, err
	}
	m, err := s.repo.ContributeFuture(ctx, actor.UserID, id, SavingRow{Amount: in.Amount, Date: date, Description: description, Linked: true})
	if err != nil {
		return domain.FuturePlanned{}, domain.Movement{}, err
	}
	planned, err := s.FutureExpense(ctx, actor, id)
	return planned, m, err
}

// FutureAssignInput moves Amount (> 0) of the free balance to an item. Date
// defaults to today.
type FutureAssignInput struct {
	Amount decimal.Decimal
	Date   string
}

// AssignFuture moves part of the free balance to an item as a transfer: two
// Ahorro rows of Gastos futuros stored atomically, -amount linked to no item
// and +amount linked to the item. More than the free balance is
// domain.ErrInsufficientFreeBalance.
func (s *Service) AssignFuture(ctx context.Context, actor Identity, id int, in FutureAssignInput) (domain.FuturePlanned, error) {
	if err := require(actor); err != nil {
		return domain.FuturePlanned{}, err
	}
	it, err := s.active(ctx, actor.UserID, id)
	if err != nil {
		return domain.FuturePlanned{}, err
	}
	if err := domain.ValidateMoney("amount", in.Amount); err != nil {
		return domain.FuturePlanned{}, err
	}
	date, err := s.dateOrToday(in.Date)
	if err != nil {
		return domain.FuturePlanned{}, err
	}
	if err := s.repo.SeedDefaults(ctx, actor.UserID); err != nil {
		return domain.FuturePlanned{}, err
	}
	if err := s.repo.AssignFuture(ctx, actor.UserID, id, in.Amount, date, domain.TrimDescription("Asignado a "+it.Name)); err != nil {
		return domain.FuturePlanned{}, err
	}
	return s.FutureExpense(ctx, actor, id)
}

// FuturePayInput is the payment of an item. CategoryID is required and must be
// one of her Gasto categories; Date defaults to today and Amount (what was
// actually paid) to the target.
type FuturePayInput struct {
	Date       string
	Amount     *decimal.Decimal
	CategoryID int
}

// PayFuture marks the item paid in one transaction: her Gasto in the category
// she picked, a negative Ahorro row linked to the item releasing what was saved
// and, when more was saved than paid, a positive unlinked row returning the
// remainder to the free balance. When less was saved only what exists is
// released. Paying twice is domain.ErrAlreadyPaid.
func (s *Service) PayFuture(ctx context.Context, actor Identity, id int, in FuturePayInput) (domain.FuturePlanned, domain.Movement, error) {
	if err := require(actor); err != nil {
		return domain.FuturePlanned{}, domain.Movement{}, err
	}
	none := func(err error) (domain.FuturePlanned, domain.Movement, error) {
		return domain.FuturePlanned{}, domain.Movement{}, err
	}
	if _, err := s.active(ctx, actor.UserID, id); err != nil {
		return none(err)
	}
	if in.CategoryID <= 0 {
		return none(fmt.Errorf("%w: category_id is required", domain.ErrInvalidInput))
	}
	if err := s.repo.SeedDefaults(ctx, actor.UserID); err != nil {
		return none(err)
	}
	cat, err := s.repo.GetCategory(ctx, actor.UserID, in.CategoryID)
	if err != nil {
		if isNotFound(err) {
			return none(fmt.Errorf("%w: category_id does not exist", domain.ErrInvalidInput))
		}
		return none(err)
	}
	if cat.Kind != domain.KindExpense {
		return none(fmt.Errorf("%w: category %q is not a Gasto category", domain.ErrInvalidInput, cat.Name))
	}
	date, err := s.dateOrToday(in.Date)
	if err != nil {
		return none(err)
	}
	paid, expense, err := s.repo.PayFuture(ctx, actor.UserID, id, func(it domain.FutureExpense) (Settlement, error) {
		amount := it.Target
		if in.Amount != nil {
			amount = *in.Amount
		}
		if err := domain.ValidateMoney("amount", amount); err != nil {
			return Settlement{}, err
		}
		v, err := domain.ValidateMovement(domain.MovementInput{
			Kind: string(domain.KindExpense), CategoryID: cat.ID, Amount: amount,
			Description: domain.TrimDescription(it.Name), Date: date,
		})
		if err != nil {
			return Settlement{}, err
		}
		out := Settlement{Expense: v}
		linked, remainder := domain.Release(it.Saved, amount)
		if linked.IsPositive() {
			out.Savings = append(out.Savings, SavingRow{
				Amount: linked.Neg(), Date: date, Description: domain.TrimDescription("Pago de " + it.Name), Linked: true,
			})
		}
		if remainder.IsPositive() {
			out.Savings = append(out.Savings, SavingRow{
				Amount: remainder, Date: date, Description: domain.TrimDescription("Sobrante de " + it.Name),
			})
		}
		return out, nil
	})
	if err != nil {
		return none(err)
	}
	return domain.FuturePlanned{FutureExpense: paid}, expense, nil
}
