// Package app holds the expense request use cases and the ports they depend
// on. The Gasto and the future expense that an approval creates are built by
// the expenses and future expenses rules, and written by the repository in the
// same transaction as the status change.
package app

import (
	"context"
	"time"

	"github.com/valium69mg/finances-app/backend/internal/expenserequests/domain"
	expensesapp "github.com/valium69mg/finances-app/backend/internal/expenses/app"
	futuredomain "github.com/valium69mg/finances-app/backend/internal/futureexpenses/domain"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/session"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

// Identity is the caller, taken from the request context by the adapters.
type Identity = session.Identity

// ListFilter narrows a listing. An empty field matches everything.
type ListFilter struct {
	RequesterID string
	Status      domain.Status
}

// Approval is what an approval writes besides the request itself. Exactly one
// of Expense and Future is set, matching Destination.
type Approval struct {
	Destination domain.Destination
	// Expense is the validated Gasto (see expenses.Build); the repository stores
	// it attributed to the requester.
	Expense *ledger.Movement
	// Future is the validated future expense item.
	Future *futuredomain.Validated
}

// Repo persists the requests.
type Repo interface {
	// Create stores a pending request of the requester.
	Create(ctx context.Context, requesterID string, v domain.Validated) (domain.Request, error)
	// Get returns domain.ErrNotFound for an unknown (or malformed) id.
	Get(ctx context.Context, id int) (domain.Request, error)
	// List returns the requests newest first.
	List(ctx context.Context, f ListFilter) ([]domain.Request, error)
	// Cancel moves a pending request of the requester to cancelada.
	// domain.ErrNotFound, domain.ErrForbidden (it belongs to someone else) or
	// domain.ErrInvalidState (not pending).
	Cancel(ctx context.Context, id int, requesterID string) (domain.Request, error)
	// Reject moves a pending request to rechazada with the comment.
	// domain.ErrNotFound or domain.ErrInvalidState.
	Reject(ctx context.Context, id int, decidedBy, comment string) (domain.Request, error)
	// Approve is one transaction: it locks the request, returns
	// domain.ErrNotFound or domain.ErrInvalidState when it is not pending,
	// stores the Gasto (created_by = the requester) or the future expense,
	// and sets status aprobada with the decision and the link to the created
	// row. Any error rolls everything back.
	Approve(ctx context.Context, id int, decidedBy string, a Approval) (domain.Request, error)
	// Revert is one transaction: it locks the request, returns
	// domain.ErrNotFound or domain.ErrInvalidState when it is not aprobada,
	// deletes the Gasto or the active future expense the approval created (a
	// link whose row is already gone is skipped), returns
	// domain.ErrFutureExpensePaid when the item is paid, and resets the request
	// to solicitada counting the revert. Any error rolls everything back.
	Revert(ctx context.Context, id int) (domain.Request, error)
	// OwnerEmails returns the email of every active owner.
	OwnerEmails(ctx context.Context) ([]string, error)
}

// Expenses is the part of the expenses module used here.
type Expenses interface {
	Build(ctx context.Context, in expensesapp.Input) (ledger.Movement, error)
	BudgetFor(ctx context.Context, category, date string) (expensesapp.BudgetFeedback, error)
}

// Settings is the part of the settings module used here: the categories.
type Settings interface {
	Get(ctx context.Context) (settings.Config, error)
}

// Mailer sends one email.
type Mailer interface {
	Send(ctx context.Context, to, subject, html, text string) error
}

// RateLimiter counts events per key in a fixed window.
type RateLimiter interface {
	Allow(key string, limit int, window time.Duration) bool
}
