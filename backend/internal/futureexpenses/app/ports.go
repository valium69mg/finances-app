package app

import (
	"context"

	"github.com/shopspring/decimal"

	expensesapp "github.com/valium69mg/finances-app/backend/internal/expenses/app"
	domain "github.com/valium69mg/finances-app/backend/internal/futureexpenses/domain"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	savingsapp "github.com/valium69mg/finances-app/backend/internal/savings/app"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

// Settings is the part of the settings module used here: the cycle start day
// for the plan.
type Settings interface {
	Get(ctx context.Context) (settings.Config, error)
}

// Savings is the part of the savings module used here. The Ahorro rows of this
// module are built and validated by it (category, instrument, amount rules).
type Savings interface {
	Build(ctx context.Context, in savingsapp.Input) (ledger.Movement, error)
	CreateSaving(ctx context.Context, in savingsapp.Input) (ledger.Movement, error)
	CreateMany(ctx context.Context, ins []savingsapp.Input) ([]ledger.Movement, error)
}

// Expenses is the part of the expenses module used to validate the Gasto of a
// payment and to infer its category from the name of the item.
type Expenses interface {
	Build(ctx context.Context, in expensesapp.Input) (ledger.Movement, error)
	InferCategory(ctx context.Context, description string) (string, bool, error)
}

// Settlement is what a payment writes besides the item itself, computed from
// the amount saved at the moment of the payment.
type Settlement struct {
	// Expense is the Gasto movement of the payment.
	Expense ledger.Movement
	// Savings are the Ahorro rows that release the saved money (see domain.Release).
	Savings []ledger.Movement
}

// Repo persists the items and performs the payment atomically.
type Repo interface {
	Create(ctx context.Context, v domain.Validated) (domain.FutureExpense, error)
	// Get returns the item with its saved amount, or domain.ErrNotFound.
	Get(ctx context.Context, id int) (domain.FutureExpense, error)
	// List returns every item (active and paid) with its saved amount, ordered
	// by due date then id.
	List(ctx context.Context) ([]domain.FutureExpense, error)
	// Update replaces an active item. domain.ErrNotFound when it does not
	// exist, domain.ErrAlreadyPaid when it is paid.
	Update(ctx context.Context, id int, v domain.Validated) (domain.FutureExpense, error)
	// Delete removes an item of any status. The Ahorro rows linked to it are
	// unlinked (they return to the free balance), never deleted.
	Delete(ctx context.Context, id int) error
	// FreeBalance is the net of the Ahorro movements of the Gastos futuros
	// category linked to no item.
	FreeBalance(ctx context.Context) (decimal.Decimal, error)
	// MarkPaid pays an item in one transaction. It locks the item, returns
	// domain.ErrNotFound or domain.ErrAlreadyPaid when it cannot be paid, reads
	// the saved amount and calls plan with it inside the transaction, stores the
	// returned movements, and sets status paid, paid_at (the date of the
	// expense), amount_paid and expense_movement_id. Any error rolls everything
	// back. The returned item is the paid one.
	MarkPaid(ctx context.Context, id int, plan func(saved decimal.Decimal) (Settlement, error)) (domain.FutureExpense, error)
}
