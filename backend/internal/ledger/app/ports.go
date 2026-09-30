// Package app holds the ports shared by every module that reads or writes
// ledger movements.
package app

import (
	"context"

	"github.com/valium69mg/finances-app/backend/internal/ledger/domain"
)

// MovementRepo persists movements in the shared movements table.
type MovementRepo interface {
	// Create stores a new movement and returns it with its generated ID.
	Create(ctx context.Context, m domain.Movement) (domain.Movement, error)
	// Update replaces every field of the movement with m.ID. It returns
	// domain.ErrNotFound when it does not exist.
	Update(ctx context.Context, m domain.Movement) error
	// Delete removes a movement. It returns domain.ErrNotFound when it does not exist.
	Delete(ctx context.Context, id int) error
	// GetByID returns domain.ErrNotFound when the movement does not exist.
	GetByID(ctx context.Context, id int) (domain.Movement, error)
	// ListByMonth returns the movements of a YYYY-MM month, newest first
	// (date desc, id desc). Kind filters when non-empty and limit caps the
	// result when positive.
	ListByMonth(ctx context.Context, month string, kind domain.Kind, limit int) ([]domain.Movement, error)
}
