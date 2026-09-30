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
	// Update replaces the fields of the movement with m.ID, provided it has
	// kind m.Kind (the kind itself is never changed). It returns
	// domain.ErrNotFound when it does not exist or has another kind.
	Update(ctx context.Context, m domain.Movement) error
	// Delete removes a movement. It returns domain.ErrNotFound when it does not exist.
	Delete(ctx context.Context, id int) error
	// GetByID returns domain.ErrNotFound when the movement does not exist.
	GetByID(ctx context.Context, id int) (domain.Movement, error)
	// ListByMonth returns the movements of a YYYY-MM month, newest first
	// (date desc, id desc). Kind filters when non-empty and limit caps the
	// result when positive.
	ListByMonth(ctx context.Context, month string, kind domain.Kind, limit int) ([]domain.Movement, error)
	// ListAllByKind returns every movement of a kind across all months, oldest
	// first (date asc, id asc). Balances such as the emergency fund need it.
	ListAllByKind(ctx context.Context, kind domain.Kind) ([]domain.Movement, error)
}

// BatchCreator stores several movements atomically. It is separate from
// MovementRepo because only the modules that need all-or-nothing writes (such
// as savings transfers) depend on it.
type BatchCreator interface {
	// CreateMany stores the movements in one transaction and returns them with
	// their generated IDs, in order. Either every movement is stored or none is.
	CreateMany(ctx context.Context, ms []domain.Movement) ([]domain.Movement, error)
}
