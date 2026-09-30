// Package app holds the ports of the savings module.
package app

import (
	"context"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
)

// ValuationRepo persists instrument valuations. It is append-only: a
// correction is a newer valuation, never an update or a delete.
type ValuationRepo interface {
	// Add stores a valuation. Callers validate it with domain.ValidateValuation first.
	Add(ctx context.Context, v ledger.Valuation) error
	// List returns every valuation, oldest first (date asc, then insertion order).
	List(ctx context.Context) ([]ledger.Valuation, error)
}
