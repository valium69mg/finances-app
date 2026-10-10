// Package app holds the personal finances use cases: the categories and the
// movements of a household user. Every call is scoped to the session user and
// denied to any other role. The owner read-only view comes in a later step
// (phase 12 step 7); until then the owner is forbidden here too.
package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/valium69mg/finances-app/backend/internal/personal/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/session"
)

// Service implements the personal finances use cases.
type Service struct {
	repo     Repo
	settings Settings
	requests ExpenseRequests
	now      func() time.Time
}

// NewService builds a Service. A nil now selects time.Now; the composition root
// passes a clock in TZ_NAME so "today" and the cycles never follow UTC.
func NewService(repo Repo, settings Settings, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, settings: settings, now: now}
}

// WithRequests wires the expense requests module used by the wishlist
// conversion to a request. Without it that conversion fails.
func (s *Service) WithRequests(r ExpenseRequests) *Service {
	s.requests = r
	return s
}

// require allows only a household user with an id. Defense in depth behind the
// HTTP allowlist; the owner is denied until the read-only view (step 7).
func require(actor Identity) error {
	if actor.UserID == "" || actor.Role != session.RoleHousehold {
		return domain.ErrForbidden
	}
	return nil
}

// Categories returns the categories of the caller, seeding the defaults first.
// kind is optional ("" for every kind).
func (s *Service) Categories(ctx context.Context, actor Identity, kind string) ([]domain.Category, error) {
	if err := require(actor); err != nil {
		return nil, err
	}
	var k domain.Kind
	if strings.TrimSpace(kind) != "" {
		parsed, err := domain.ParseKind(kind)
		if err != nil {
			return nil, err
		}
		k = parsed
	}
	if err := s.repo.SeedDefaults(ctx, actor.UserID); err != nil {
		return nil, err
	}
	return s.repo.ListCategories(ctx, actor.UserID, k)
}

// CreateCategory adds a Gasto category.
func (s *Service) CreateCategory(ctx context.Context, actor Identity, name string) (domain.Category, error) {
	if err := require(actor); err != nil {
		return domain.Category{}, err
	}
	name, err := domain.ValidateCategoryName(name)
	if err != nil {
		return domain.Category{}, err
	}
	// Seed first: the defaults of a kind are only added while the user has no
	// category of it, so a first custom category must not suppress them.
	if err := s.repo.SeedDefaults(ctx, actor.UserID); err != nil {
		return domain.Category{}, err
	}
	return s.repo.CreateCategory(ctx, actor.UserID, name)
}

// RenameCategory renames a Gasto category; the fixed kinds are
// domain.ErrCategoryFixed. Movements reference the id, so they follow.
func (s *Service) RenameCategory(ctx context.Context, actor Identity, id int, name string) (domain.Category, error) {
	if err := require(actor); err != nil {
		return domain.Category{}, err
	}
	name, err := domain.ValidateCategoryName(name)
	if err != nil {
		return domain.Category{}, err
	}
	cat, err := s.repo.GetCategory(ctx, actor.UserID, id)
	if err != nil {
		return domain.Category{}, err
	}
	if !cat.Kind.Editable() {
		return domain.Category{}, domain.ErrCategoryFixed
	}
	return s.repo.RenameCategory(ctx, actor.UserID, id, name)
}

// ListInput filters a movement listing. Month is a YYYY-MM pay cycle label
// ("" for the current cycle); Kind and CategoryID are optional.
type ListInput struct {
	Month      string
	Kind       string
	CategoryID int
}

// Movements returns the caller's movements in the pay cycle, newest first. The
// cycle uses the owner's cycle_start_day.
func (s *Service) Movements(ctx context.Context, actor Identity, in ListInput) ([]domain.Movement, error) {
	if err := require(actor); err != nil {
		return nil, err
	}
	f := Filter{CategoryID: in.CategoryID}
	if strings.TrimSpace(in.Kind) != "" {
		kind, err := domain.ParseKind(in.Kind)
		if err != nil {
			return nil, err
		}
		f.Kind = kind
	}
	p, err := s.resolvePeriod(ctx, in.Month)
	if err != nil {
		return nil, err
	}
	from, to := p.Start, p.End
	f.From, f.To = from, to
	return s.repo.ListMovements(ctx, actor.UserID, f)
}

// check validates the input and that the category is hers and has the kind of
// the movement. An unknown or foreign category is reported the same way.
func (s *Service) check(ctx context.Context, actor Identity, in domain.MovementInput) (domain.ValidMovement, error) {
	if strings.TrimSpace(in.Date) == "" {
		in.Date = s.now().Format(domain.DateLayout)
	}
	v, err := domain.ValidateMovement(in)
	if err != nil {
		return domain.ValidMovement{}, err
	}
	cat, err := s.repo.GetCategory(ctx, actor.UserID, v.CategoryID)
	if err != nil {
		if isNotFound(err) {
			return domain.ValidMovement{}, fmt.Errorf("%w: category_id does not exist", domain.ErrInvalidInput)
		}
		return domain.ValidMovement{}, err
	}
	if cat.Kind != v.Kind {
		return domain.ValidMovement{}, fmt.Errorf("%w: category %q is not a %s category", domain.ErrInvalidInput, cat.Name, v.Kind)
	}
	return v, nil
}

// CreateMovement stores a movement of the caller.
func (s *Service) CreateMovement(ctx context.Context, actor Identity, in domain.MovementInput) (domain.Movement, error) {
	if err := require(actor); err != nil {
		return domain.Movement{}, err
	}
	v, err := s.check(ctx, actor, in)
	if err != nil {
		return domain.Movement{}, err
	}
	return s.repo.CreateMovement(ctx, actor.UserID, v)
}

// UpdateMovement replaces a movement of the caller.
func (s *Service) UpdateMovement(ctx context.Context, actor Identity, id int, in domain.MovementInput) (domain.Movement, error) {
	if err := require(actor); err != nil {
		return domain.Movement{}, err
	}
	v, err := s.check(ctx, actor, in)
	if err != nil {
		return domain.Movement{}, err
	}
	return s.repo.UpdateMovement(ctx, actor.UserID, id, v)
}

// DeleteMovement removes a movement of the caller.
func (s *Service) DeleteMovement(ctx context.Context, actor Identity, id int) error {
	if err := require(actor); err != nil {
		return err
	}
	return s.repo.DeleteMovement(ctx, actor.UserID, id)
}

func isNotFound(err error) bool { return errors.Is(err, domain.ErrNotFound) }

// resolvePeriod reads the owner's cycle_start_day and returns the cycle of the
// YYYY-MM label ("" for the current one, "today" in the clock's zone).
func (s *Service) resolvePeriod(ctx context.Context, month string) (domain.Period, error) {
	cfg, err := s.settings.Get(ctx)
	if err != nil {
		return domain.Period{}, err
	}
	cycle := cfg.Cycle()
	month = strings.TrimSpace(month)
	if month == "" {
		month = cycle.Current(s.now())
	}
	from, to, err := cycle.Range(month)
	if err != nil {
		return domain.Period{}, fmt.Errorf("%w: month %q must be YYYY-MM", domain.ErrInvalidInput, month)
	}
	return domain.Period{Month: month, Start: from, End: to}, nil
}

// Period returns the pay cycle of a month label, so her pages can show its
// range without reading the settings.
func (s *Service) Period(ctx context.Context, actor Identity, month string) (domain.Period, error) {
	if err := require(actor); err != nil {
		return domain.Period{}, err
	}
	return s.resolvePeriod(ctx, month)
}

// Dashboard returns her totals and the Gasto spending by category for a pay
// cycle. It reads only her personal rows.
func (s *Service) Dashboard(ctx context.Context, actor Identity, month string) (domain.Dashboard, error) {
	if err := require(actor); err != nil {
		return domain.Dashboard{}, err
	}
	p, err := s.resolvePeriod(ctx, month)
	if err != nil {
		return domain.Dashboard{}, err
	}
	totals, err := s.repo.KindTotals(ctx, actor.UserID, p.Start, p.End)
	if err != nil {
		return domain.Dashboard{}, err
	}
	cats, err := s.repo.SpentByCategory(ctx, actor.UserID, p.Start, p.End)
	if err != nil {
		return domain.Dashboard{}, err
	}
	return domain.Dashboard{Period: p, Totals: totals, Categories: cats}, nil
}
