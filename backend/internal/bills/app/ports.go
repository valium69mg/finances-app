package app

import (
	"context"

	"github.com/shopspring/decimal"

	bills "github.com/valium69mg/finances-app/backend/internal/bills/domain"
	expensesapp "github.com/valium69mg/finances-app/backend/internal/expenses/app"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

// Settings is the part of the settings module the bills use cases consume, to
// validate the category of a bill.
type Settings interface {
	Get(ctx context.Context) (settings.Config, error)
}

// Expenses is the part of the expenses module used to register the expense of a
// payment, so the defaults, the USD rate and the category rules stay in one place.
type Expenses interface {
	Create(ctx context.Context, in expensesapp.Input) (expensesapp.Result, error)
	Delete(ctx context.Context, id int) error
}

// Resolution closes the pending occurrence of a bill and generates the next one.
type Resolution struct {
	BillID       int
	OccurrenceID int
	Status       bills.Status // paid or skipped
	// Payment fields, set only when Status is paid. ExpenseID is nil when no
	// expense is linked.
	PaidOn     string
	AmountPaid decimal.Decimal
	Currency   string
	ExpenseID  *int
	// NextDueDate is the due date of the occurrence generated in the same transaction.
	NextDueDate string
}

// Repo persists the bills and their occurrences.
type Repo interface {
	// Create stores the bill with its first pending occurrence due on
	// v.NextDueDate, in one transaction.
	Create(ctx context.Context, v bills.Validated) (bills.Bill, error)
	// Get returns the bill with its pending occurrence, or bills.ErrNotFound.
	Get(ctx context.Context, id int) (bills.Bill, error)
	// List returns the bills with their pending occurrence, ordered by due date
	// then name. Inactive bills are only included when includeInactive is set.
	List(ctx context.Context, includeInactive bool) ([]bills.Bill, error)
	// History returns the resolved (paid or skipped) occurrences of a bill,
	// newest due date first.
	History(ctx context.Context, billID int) ([]bills.Occurrence, error)
	// Update replaces the editable fields of a bill. When v.NextDueDate differs
	// from the pending occurrence's, the pending occurrence moves to it. It
	// returns bills.ErrNotFound for an unknown bill.
	Update(ctx context.Context, id int, v bills.Validated) (bills.Bill, error)
	// Deactivate sets active = false. It is idempotent and returns
	// bills.ErrNotFound for an unknown bill.
	Deactivate(ctx context.Context, id int) error
	// Resolve marks the pending occurrence paid or skipped, sets the bill's
	// next_due_date and inserts the next pending occurrence, atomically. It
	// returns bills.ErrNotPending when the occurrence is no longer pending and
	// bills.ErrNotFound for an unknown bill.
	Resolve(ctx context.Context, r Resolution) (bills.Bill, error)
}
