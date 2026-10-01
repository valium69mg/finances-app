package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

// ErrAlreadyImported is returned by Import when settings exist and force is off.
var ErrAlreadyImported = errors.New("settings already exist")

// Service implements the settings use cases.
type Service struct{ repo Repo }

// NewService builds a Service.
func NewService(repo Repo) *Service { return &Service{repo: repo} }

// Get returns the whole configuration.
func (s *Service) Get(ctx context.Context) (domain.Config, error) {
	return s.repo.Load(ctx)
}

// UpdateGeneral validates and replaces the general section. Every investment
// allocation key must be an existing instrument.
func (s *Service) UpdateGeneral(ctx context.Context, g domain.General) error {
	if err := g.Validate(); err != nil {
		return err
	}
	if len(g.InvestmentAllocation) > 0 {
		cfg, err := s.repo.Load(ctx)
		if err != nil {
			return err
		}
		for _, w := range g.InvestmentAllocation {
			if _, ok := cfg.FindInstrument(w.Key); !ok {
				return fmt.Errorf("%w: investment_allocation instrument %q does not exist", domain.ErrInvalid, w.Key)
			}
		}
	}
	return s.repo.SaveGeneral(ctx, g)
}

// UpdateCategories validates and replaces the categories. The budgets of the
// Gasto and Ahorro categories may not exceed the base monthly income (a rule
// that does not apply while the base is unavailable).
func (s *Service) UpdateCategories(ctx context.Context, cats []domain.Category) error {
	if err := domain.ValidateCategories(cats); err != nil {
		return err
	}
	cfg, err := s.repo.Load(ctx)
	if err != nil {
		return err
	}
	if err := domain.ValidateBudgetTotal(cats, cfg.General()); err != nil {
		return err
	}
	return s.repo.SaveCategories(ctx, cats)
}

// UpdateClients validates and replaces the clients.
func (s *Service) UpdateClients(ctx context.Context, clients []domain.Client) error {
	if err := domain.ValidateClients(clients); err != nil {
		return err
	}
	return s.repo.SaveClients(ctx, clients)
}

// UpdateInstruments validates and replaces the instruments and category defaults.
func (s *Service) UpdateInstruments(ctx context.Context, instruments []domain.Instrument, byCategory map[string]string) error {
	if err := domain.ValidateInstruments(instruments, byCategory); err != nil {
		return err
	}
	return s.repo.SaveInstruments(ctx, instruments, byCategory)
}

// UpdateBrackets validates and replaces the RESICO brackets.
func (s *Service) UpdateBrackets(ctx context.Context, brackets []domain.Bracket) error {
	if err := domain.ValidateBrackets(brackets); err != nil {
		return err
	}
	return s.repo.SaveBrackets(ctx, brackets)
}

// UpdatePaymentMethods validates and replaces the payment methods.
func (s *Service) UpdatePaymentMethods(ctx context.Context, methods []string) error {
	if err := domain.ValidatePaymentMethods(methods); err != nil {
		return err
	}
	return s.repo.SavePaymentMethods(ctx, methods)
}

// UpdateIssuer validates and replaces the issuer.
func (s *Service) UpdateIssuer(ctx context.Context, issuer domain.Issuer) error {
	if err := issuer.Validate(); err != nil {
		return err
	}
	return s.repo.SaveIssuer(ctx, issuer)
}

// UpdatePause validates and replaces the investment pause plan; nil removes it.
func (s *Service) UpdatePause(ctx context.Context, plan *domain.PausePlan) error {
	if plan != nil {
		if err := plan.Validate(); err != nil {
			return err
		}
	}
	return s.repo.SavePause(ctx, plan)
}

// CategoryBudget is the resolved budget of a category for one month.
type CategoryBudget struct {
	Name   string
	Kind   string
	Budget *decimal.Decimal
}

// MonthBudgets resolves every category budget for a YYYY-MM month: the computed
// Impuestos and Comisiones budgets and the investment pause overrides take
// precedence over the configured budget. It wraps domain.ErrMissingConfig when
// the tax parameters are incomplete.
func (s *Service) MonthBudgets(ctx context.Context, month string) ([]CategoryBudget, error) {
	if err := domain.ValidateMonth(month); err != nil {
		return nil, err
	}
	cfg, err := s.repo.Load(ctx)
	if err != nil {
		return nil, err
	}
	overrides, err := cfg.MonthBudgetOverrides(month)
	if err != nil {
		return nil, err
	}
	out := make([]CategoryBudget, len(cfg.Categories))
	for i, c := range cfg.Categories {
		out[i] = CategoryBudget{Name: c.Name, Kind: string(c.Kind), Budget: domain.CategoryBudget(c, overrides)}
	}
	return out, nil
}

// Import stores a whole configuration (the one-time load from config.json).
// It refuses to overwrite existing settings unless force is set, and validates
// every section first.
func (s *Service) Import(ctx context.Context, cfg domain.Config, force bool) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("%w: %v", domain.ErrInvalid, err)
	}
	for _, err := range []error{
		cfg.General().Validate(),
		domain.ValidateCategories(cfg.Categories),
		domain.ValidateClients(cfg.Clients),
		domain.ValidateInstruments(cfg.Instruments, cfg.InstrumentByCategory),
		domain.ValidateBrackets(cfg.Brackets),
		domain.ValidatePaymentMethods(cfg.PaymentMethods),
	} {
		if err != nil {
			return err
		}
	}
	if !cfg.Issuer.IsZero() {
		if err := cfg.Issuer.Validate(); err != nil {
			return err
		}
	}
	if cfg.Pause != nil {
		if err := cfg.Pause.Validate(); err != nil {
			return err
		}
	}
	if !force {
		empty, err := s.repo.IsEmpty(ctx)
		if err != nil {
			return err
		}
		if !empty {
			return ErrAlreadyImported
		}
	}
	return s.repo.Import(ctx, cfg)
}
