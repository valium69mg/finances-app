package app

import (
	"context"

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
}
