// Package requests adapts the expense requests service to the port the
// wishlist uses to ask the owner for an item.
package requests

import (
	"context"

	erapp "github.com/valium69mg/finances-app/backend/internal/expenserequests/app"
	erdomain "github.com/valium69mg/finances-app/backend/internal/expenserequests/domain"
	"github.com/valium69mg/finances-app/backend/internal/personal/app"
)

// Adapter creates and withdraws requests as the session user through the
// expense requests service, so its validation, the owner notification and the
// hourly limit apply unchanged.
type Adapter struct{ svc *erapp.Service }

// New builds an Adapter.
func New(svc *erapp.Service) *Adapter { return &Adapter{svc: svc} }

// Create stores a pending request of the actor.
func (a *Adapter) Create(ctx context.Context, actor app.Identity, d app.RequestDraft) (int, error) {
	req, err := a.svc.Create(ctx, actor, erdomain.Input{
		Amount: d.Amount, Description: d.Description, SuggestedCategory: d.Category, Date: d.Date,
	})
	if err != nil {
		return 0, err
	}
	return req.ID, nil
}

// Cancel withdraws a pending request of the actor.
func (a *Adapter) Cancel(ctx context.Context, actor app.Identity, id int) error {
	_, err := a.svc.Cancel(ctx, actor, id)
	return err
}

var _ app.ExpenseRequests = (*Adapter)(nil)
