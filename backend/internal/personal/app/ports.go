package app

import (
	"context"

	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/personal/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/session"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

// Identity is the caller, taken from the request context by the adapters.
type Identity = session.Identity

// Settings is the part of the settings module used here: the OWNER's
// cycle_start_day. The household user cannot open /settings, so the service
// reads it internally and her periods share the owner's cut day.
type Settings interface {
	Get(ctx context.Context) (settings.Config, error)
}

// Filter narrows a movement listing. Zero values mean no filter.
type Filter struct {
	From, To   string // inclusive YYYY-MM-DD cycle range
	Kind       domain.Kind
	CategoryID int
}

// Repo persists the categories and movements of one user. Every method is
// scoped by userID: a row of another user is domain.ErrNotFound.
type Repo interface {
	// SeedDefaults inserts the default categories of every kind the user has no
	// category of, in one transaction. It is idempotent and safe under
	// concurrency.
	SeedDefaults(ctx context.Context, userID string) error
	// ListCategories returns the categories of the user (one kind when kind is
	// not empty), ordered by kind (Ingreso, Gasto, Ahorro) then id.
	ListCategories(ctx context.Context, userID string, kind domain.Kind) ([]domain.Category, error)
	GetCategory(ctx context.Context, userID string, id int) (domain.Category, error)
	// CreateCategory stores a Gasto category. domain.ErrDuplicateCategory when
	// the name exists.
	CreateCategory(ctx context.Context, userID, name string) (domain.Category, error)
	// RenameCategory renames a Gasto category. domain.ErrNotFound when it does
	// not exist (or is not a Gasto one), domain.ErrDuplicateCategory on a clash.
	RenameCategory(ctx context.Context, userID string, id int, name string) (domain.Category, error)

	// ListMovements returns the movements in the range, newest first.
	ListMovements(ctx context.Context, userID string, f Filter) ([]domain.Movement, error)
	// CreateMovement stores a movement. A category of another user or another
	// kind answers domain.ErrInvalidInput.
	CreateMovement(ctx context.Context, userID string, v domain.ValidMovement) (domain.Movement, error)
	// UpdateMovement replaces a movement, same rules as CreateMovement plus
	// domain.ErrNotFound.
	UpdateMovement(ctx context.Context, userID string, id int, v domain.ValidMovement) (domain.Movement, error)
	DeleteMovement(ctx context.Context, userID string, id int) error

	FutureRepo
	WishRepo
}

// SavingRow is one Ahorro row of the Gastos futuros category written by the
// future expenses use cases. Linked rows carry the id of the item.
type SavingRow struct {
	Amount      decimal.Decimal
	Date        string
	Description string
	Linked      bool
}

// Settlement is what a payment writes besides the item itself, computed from
// the amount saved at the moment of the payment.
type Settlement struct {
	// Expense is the Gasto of the payment, in a category of the user.
	Expense domain.ValidMovement
	// Savings are the Ahorro rows that release the saved money (domain.Release).
	Savings []SavingRow
}

// FutureRepo persists the future expenses of one user. Every method is scoped
// by userID: an item of another user is domain.ErrNotFound.
type FutureRepo interface {
	CreateFutureExpense(ctx context.Context, userID string, v domain.FutureValidated) (domain.FutureExpense, error)
	// GetFutureExpense returns the item with its saved amount.
	GetFutureExpense(ctx context.Context, userID string, id int) (domain.FutureExpense, error)
	// ListFutureExpenses returns every item (active and paid) with its saved
	// amount, ordered by due date then id.
	ListFutureExpenses(ctx context.Context, userID string) ([]domain.FutureExpense, error)
	// UpdateFutureExpense replaces an active item: domain.ErrNotFound, or
	// domain.ErrAlreadyPaid when it is paid.
	UpdateFutureExpense(ctx context.Context, userID string, id int, v domain.FutureValidated) (domain.FutureExpense, error)
	// DeleteFutureExpense removes an item of any status. The savings linked to
	// it are unlinked (they return to the free balance), never deleted.
	DeleteFutureExpense(ctx context.Context, userID string, id int) error
	// FutureFreeBalance is the net of her Ahorro rows of Gastos futuros linked
	// to no item.
	FutureFreeBalance(ctx context.Context, userID string) (decimal.Decimal, error)
	// ContributeFuture stores a linked Ahorro row while holding a share lock on
	// the item, so it cannot land on an item that is being paid.
	ContributeFuture(ctx context.Context, userID string, id int, row SavingRow) (domain.Movement, error)
	// AssignFuture stores, atomically, -amount unlinked and +amount linked.
	// domain.ErrInsufficientFreeBalance when amount exceeds the free balance
	// read inside the transaction, domain.ErrAlreadyPaid for a paid item.
	AssignFuture(ctx context.Context, userID string, id int, amount decimal.Decimal, date, description string) error
	// PayFuture pays an item in one transaction: it locks the item, returns
	// domain.ErrNotFound or domain.ErrAlreadyPaid when it cannot be paid, reads
	// the saved amount and calls plan with the item inside the transaction,
	// stores the returned rows and sets status paid, paid_at (the date of the
	// expense), amount_paid and expense_movement_id. Any error rolls everything
	// back. It returns the paid item and its Gasto.
	PayFuture(ctx context.Context, userID string, id int, plan func(it domain.FutureExpense) (Settlement, error)) (domain.FutureExpense, domain.Movement, error)
}

// WishRepo persists the wishlist of one user. Every method is scoped by userID:
// an item of another user is domain.ErrNotFound. Items come with their link and,
// for a request, the current state, so the status can be derived.
type WishRepo interface {
	CreateWish(ctx context.Context, userID string, v domain.ValidWish) (domain.WishItem, error)
	GetWish(ctx context.Context, userID string, id int) (domain.WishItem, error)
	// ListWish returns every item, newest first.
	ListWish(ctx context.Context, userID string) ([]domain.WishItem, error)
	// UpdateWish replaces the fields of an item of any status; the links are
	// never touched.
	UpdateWish(ctx context.Context, userID string, id int, v domain.ValidWish) (domain.WishItem, error)
	// DeleteWish removes the item only, never what it converted into.
	DeleteWish(ctx context.Context, userID string, id int) error
	// ConvertWishToFuture is one transaction: it locks the item, returns
	// domain.ErrAlreadyConverted unless it is active, calls plan with it, stores
	// the future expense and replaces the link of the item with it. Any error
	// rolls everything back.
	ConvertWishToFuture(ctx context.Context, userID string, id int, plan func(it domain.WishItem) (domain.FutureValidated, error)) (domain.WishItem, error)
	// ConvertWishToExpense is the same for her Gasto.
	ConvertWishToExpense(ctx context.Context, userID string, id int, plan func(it domain.WishItem) (domain.ValidMovement, error)) (domain.WishItem, error)
	// LinkWishRequest locks the item, returns domain.ErrAlreadyConverted unless
	// it is active and replaces its link with the expense request.
	LinkWishRequest(ctx context.Context, userID string, id, requestID int) (domain.WishItem, error)
}

// RequestDraft is the expense request created from a wishlist item.
type RequestDraft struct {
	Amount      decimal.Decimal
	Description string
	Category    string // an owner Gasto category name
	Date        string // empty means today
}

// ExpenseRequests is the part of the expense requests module used by the
// "ask the owner" conversion: the request is created through its service as
// the session user, so its validation, the owner notification and the hourly
// limit apply unchanged.
type ExpenseRequests interface {
	Create(ctx context.Context, actor Identity, d RequestDraft) (id int, err error)
	// Cancel withdraws a pending request of the actor (compensation when the
	// link could not be stored).
	Cancel(ctx context.Context, actor Identity, id int) error
}
