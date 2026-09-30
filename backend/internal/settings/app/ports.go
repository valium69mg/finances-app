// Package app holds the settings use cases and the ports they depend on.
package app

import (
	"context"

	"github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

// Repo persists the settings. Every Save* replaces its whole section
// atomically. Load returns an empty Config when nothing was saved yet.
type Repo interface {
	Load(ctx context.Context) (domain.Config, error)
	// IsEmpty reports whether no settings have been stored.
	IsEmpty(ctx context.Context) (bool, error)

	SaveGeneral(ctx context.Context, g domain.General) error
	SaveCategories(ctx context.Context, cats []domain.Category) error
	SaveClients(ctx context.Context, clients []domain.Client) error
	// SaveInstruments replaces the instruments and the per-category defaults together.
	SaveInstruments(ctx context.Context, instruments []domain.Instrument, byCategory map[string]string) error
	SaveBrackets(ctx context.Context, brackets []domain.Bracket) error
	SavePaymentMethods(ctx context.Context, methods []string) error
	SaveIssuer(ctx context.Context, issuer domain.Issuer) error
	// SavePause stores the plan; nil removes it.
	SavePause(ctx context.Context, plan *domain.PausePlan) error

	// Import replaces every section in one transaction.
	Import(ctx context.Context, cfg domain.Config) error
}
