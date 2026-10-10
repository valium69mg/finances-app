package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/personal/domain"
)

// The wishlist of a household user. Every call is scoped to the session user.

// wishCategory checks that the category is one of HER Gasto categories. An
// unknown, foreign or non-Gasto id is reported the same way, without revealing
// whether it exists.
func (s *Service) wishCategory(ctx context.Context, userID string, id int) (domain.Category, error) {
	if id <= 0 {
		return domain.Category{}, fmt.Errorf("%w: category_id is required", domain.ErrWishInvalid)
	}
	if err := s.repo.SeedDefaults(ctx, userID); err != nil {
		return domain.Category{}, err
	}
	cat, err := s.repo.GetCategory(ctx, userID, id)
	if err != nil {
		if isNotFound(err) {
			return domain.Category{}, fmt.Errorf("%w: category_id is not one of your Gasto categories", domain.ErrWishInvalid)
		}
		return domain.Category{}, err
	}
	if cat.Kind != domain.KindExpense {
		return domain.Category{}, fmt.Errorf("%w: category_id is not one of your Gasto categories", domain.ErrWishInvalid)
	}
	return cat, nil
}

// Wishlist returns every item of the caller: the active ones first (priority
// alta to baja, newest first) and the converted ones after.
func (s *Service) Wishlist(ctx context.Context, actor Identity) ([]domain.WishItem, error) {
	if err := require(actor); err != nil {
		return nil, err
	}
	items, err := s.repo.ListWish(ctx, actor.UserID)
	if err != nil {
		return nil, err
	}
	domain.SortWishlist(items)
	return items, nil
}

// CreateWish stores an item (always active: it has no link).
func (s *Service) CreateWish(ctx context.Context, actor Identity, in domain.WishInput) (domain.WishItem, error) {
	if err := require(actor); err != nil {
		return domain.WishItem{}, err
	}
	v, err := domain.ValidateWish(in)
	if err != nil {
		return domain.WishItem{}, err
	}
	if _, err := s.wishCategory(ctx, actor.UserID, v.CategoryID); err != nil {
		return domain.WishItem{}, err
	}
	return s.repo.CreateWish(ctx, actor.UserID, v)
}

// UpdateWish replaces the fields of an item of any status. The link is kept.
func (s *Service) UpdateWish(ctx context.Context, actor Identity, id int, in domain.WishInput) (domain.WishItem, error) {
	if err := require(actor); err != nil {
		return domain.WishItem{}, err
	}
	v, err := domain.ValidateWish(in)
	if err != nil {
		return domain.WishItem{}, err
	}
	if _, err := s.wishCategory(ctx, actor.UserID, v.CategoryID); err != nil {
		return domain.WishItem{}, err
	}
	return s.repo.UpdateWish(ctx, actor.UserID, id, v)
}

// DeleteWish removes the item. What it converted into is never touched.
func (s *Service) DeleteWish(ctx context.Context, actor Identity, id int) error {
	if err := require(actor); err != nil {
		return err
	}
	return s.repo.DeleteWish(ctx, actor.UserID, id)
}

// activeWish loads an item and refuses a converted one before any work starts.
// The repository checks again under a row lock.
func (s *Service) activeWish(ctx context.Context, userID string, id int) (domain.WishItem, error) {
	it, err := s.repo.GetWish(ctx, userID, id)
	if err != nil {
		return domain.WishItem{}, err
	}
	if it.Status() != domain.WishActive {
		return domain.WishItem{}, domain.ErrAlreadyConverted
	}
	return it, nil
}

// WishFutureInput converts into her future expense. TargetAmount defaults to
// the price; DueDate is required.
type WishFutureInput struct {
	TargetAmount *decimal.Decimal
	DueDate      string
}

// ConvertWishToFuture creates her future expense named after the item (through
// the same validation as a direct creation) and links the item to it, in one
// transaction.
func (s *Service) ConvertWishToFuture(ctx context.Context, actor Identity, id int, in WishFutureInput) (domain.WishItem, error) {
	if err := require(actor); err != nil {
		return domain.WishItem{}, err
	}
	if _, err := s.activeWish(ctx, actor.UserID, id); err != nil {
		return domain.WishItem{}, err
	}
	return s.repo.ConvertWishToFuture(ctx, actor.UserID, id, func(it domain.WishItem) (domain.FutureValidated, error) {
		target := it.Price
		if in.TargetAmount != nil {
			target = *in.TargetAmount
		}
		return domain.ValidateFuture(domain.FutureInput{Name: it.Name, Target: target, DueDate: in.DueDate})
	})
}

// WishExpenseInput converts into her Gasto ("I already bought it"). Amount
// defaults to the price, Date to today and CategoryID to the item's category.
type WishExpenseInput struct {
	Amount     *decimal.Decimal
	Date       string
	CategoryID int
}

// ConvertWishToExpense registers her Gasto and links the item to it, in one
// transaction.
func (s *Service) ConvertWishToExpense(ctx context.Context, actor Identity, id int, in WishExpenseInput) (domain.WishItem, error) {
	if err := require(actor); err != nil {
		return domain.WishItem{}, err
	}
	it, err := s.activeWish(ctx, actor.UserID, id)
	if err != nil {
		return domain.WishItem{}, err
	}
	categoryID := it.CategoryID
	if in.CategoryID != 0 {
		categoryID = in.CategoryID
	}
	cat, err := s.wishCategory(ctx, actor.UserID, categoryID)
	if err != nil {
		return domain.WishItem{}, err
	}
	date, err := s.dateOrToday(in.Date)
	if err != nil {
		return domain.WishItem{}, fmt.Errorf("%w: date must be like YYYY-MM-DD", domain.ErrWishInvalid)
	}
	return s.repo.ConvertWishToExpense(ctx, actor.UserID, id, func(it domain.WishItem) (domain.ValidMovement, error) {
		amount := it.Price
		if in.Amount != nil {
			amount = *in.Amount
		}
		if err := domain.ValidateWishPrice("amount", amount); err != nil {
			return domain.ValidMovement{}, err
		}
		return domain.ValidateMovement(domain.MovementInput{
			Kind: string(domain.KindExpense), CategoryID: cat.ID, Amount: amount,
			Description: domain.TrimDescription(it.Name), Date: date,
		})
	})
}

// WishRequestInput converts into an expense request to the owner. Category is
// an OWNER Gasto category name and is required; Amount defaults to the price
// and Date to today.
type WishRequestInput struct {
	Amount   *decimal.Decimal
	Date     string
	Category string
}

// ConvertWishToRequest creates an expense request as the caller through the
// expense requests service (same validation, owner notification and hourly
// limit) and links the item to it.
//
// The two writes live in different modules, so there is no shared transaction.
// The request is created first and linked second; the link re-checks the item
// under a row lock, so two concurrent conversions cannot both link. If the link
// fails (the item was converted meanwhile, or a database error) the request is
// withdrawn with Cancel, so a failed conversion does not leave a request the
// owner could approve behind her back; if that cancel also fails the request
// stays pending and both errors are returned.
func (s *Service) ConvertWishToRequest(ctx context.Context, actor Identity, id int, in WishRequestInput) (domain.WishItem, error) {
	if err := require(actor); err != nil {
		return domain.WishItem{}, err
	}
	if s.requests == nil {
		return domain.WishItem{}, errors.New("expense requests are not wired")
	}
	it, err := s.activeWish(ctx, actor.UserID, id)
	if err != nil {
		return domain.WishItem{}, err
	}
	if strings.TrimSpace(in.Category) == "" {
		return domain.WishItem{}, fmt.Errorf("%w: category is required", domain.ErrWishInvalid)
	}
	amount := it.Price
	if in.Amount != nil {
		amount = *in.Amount
	}
	if err := domain.ValidateWishPrice("amount", amount); err != nil {
		return domain.WishItem{}, err
	}
	reqID, err := s.requests.Create(ctx, actor, RequestDraft{
		Amount: amount, Description: it.Name, Category: in.Category, Date: strings.TrimSpace(in.Date),
	})
	if err != nil {
		return domain.WishItem{}, err
	}
	linked, err := s.repo.LinkWishRequest(ctx, actor.UserID, id, reqID)
	if err != nil {
		if cancelErr := s.requests.Cancel(ctx, actor, reqID); cancelErr != nil {
			return domain.WishItem{}, errors.Join(err, fmt.Errorf("withdraw request %d: %w", reqID, cancelErr))
		}
		return domain.WishItem{}, err
	}
	return linked, nil
}
