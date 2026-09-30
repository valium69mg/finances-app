// Package app holds the income use cases: registering, correcting and listing
// Ingreso movements with the month total, the RESICO ISR estimate and, for
// extra contracts, the suggested (never persisted) split of the deposit.
package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	income "github.com/valium69mg/finances-app/backend/internal/income/domain"
	ledgerapp "github.com/valium69mg/finances-app/backend/internal/ledger/app"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	savings "github.com/valium69mg/finances-app/backend/internal/savings/domain"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

// DefaultListLimit is the number of incomes listed when no limit is given.
const DefaultListLimit = 20

// Settings is the part of the settings module the income use cases consume.
type Settings interface {
	Get(ctx context.Context) (settings.Config, error)
}

// Input is the data of a new or updated income. Empty optional fields take the
// defaults of ledger.NewMovement (payment method Transferencia, currency MXN,
// date today, USD rate from fx_rate_applied); an empty Category is inferred
// from the description.
type Input struct {
	Date          string
	Description   string
	Category      string
	PaymentMethod string
	Currency      string
	Amount        decimal.Decimal
	ExchangeRate  *decimal.Decimal
}

// ResicoEstimate is the RESICO ISR estimate of a month's income. RateIncreased
// is set when the movement moved the month to a higher bracket, and only then
// is PreviousRate (the rate before the movement was counted) non-nil.
type ResicoEstimate struct {
	Rate          decimal.Decimal
	EstimatedISR  decimal.Decimal
	RateIncreased bool
	PreviousRate  *decimal.Decimal
}

// Summary is the income picture of the month of a saved movement. Resico is nil
// when the tax settings are incomplete.
type Summary struct {
	Month         string
	MonthTotalMXN decimal.Decimal
	Resico        *ResicoEstimate
}

// Split is the suggested split of an extra-contract deposit. Breakdown divides
// Investments by the configured instrument weights and is empty when none apply.
type Split struct {
	income.ExtraSplit
	Breakdown []savings.Share
}

// Result is a saved income with its summary. Split is nil unless the category
// is ledger.CategoryExtraContract and the emergency fund status could be
// computed; it is a preview and is never stored.
type Result struct {
	Movement ledger.Movement
	Summary  Summary
	Split    *Split
}

// Service implements the income use cases.
type Service struct {
	repo     ledgerapp.MovementRepo
	settings Settings
	now      func() time.Time
}

// NewService builds a Service. A nil now selects time.Now.
func NewService(repo ledgerapp.MovementRepo, settings Settings, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, settings: settings, now: now}
}

// build validates the input into an Ingreso movement, inferring the category
// when it is empty.
func (s *Service) build(ctx context.Context, in Input) (ledger.Movement, settings.Config, error) {
	cfg, err := s.settings.Get(ctx)
	if err != nil {
		return ledger.Movement{}, cfg, err
	}
	category := in.Category
	if category == "" {
		inferred, ok := cfg.InferCategory(in.Description, ledger.KindIncome)
		if !ok {
			return ledger.Movement{}, cfg, fmt.Errorf("%w: could not infer the category from the description, choose one", ledger.ErrInvalid)
		}
		category = inferred
	}
	m, err := ledger.NewMovement(ledger.MovementInput{
		Date: in.Date, Description: in.Description, Category: category, Kind: ledger.KindIncome,
		PaymentMethod: in.PaymentMethod, Currency: in.Currency, Amount: in.Amount, ExchangeRate: in.ExchangeRate,
	}, cfg.Catalog(), s.now().Format("2006-01-02"))
	return m, cfg, err
}

// Create registers an income and returns it with its month summary.
func (s *Service) Create(ctx context.Context, in Input) (Result, error) {
	m, cfg, err := s.build(ctx, in)
	if err != nil {
		return Result{}, err
	}
	saved, err := s.repo.Create(ctx, m)
	if err != nil {
		return Result{}, err
	}
	return s.result(ctx, cfg, saved)
}

// Update replaces the income with the given ID. Movements of another kind are
// reported as not found: this module only manages income.
func (s *Service) Update(ctx context.Context, id int, in Input) (Result, error) {
	if _, err := s.income(ctx, id); err != nil {
		return Result{}, err
	}
	m, cfg, err := s.build(ctx, in)
	if err != nil {
		return Result{}, err
	}
	m.ID = id
	if err := s.repo.Update(ctx, m); err != nil {
		return Result{}, err
	}
	return s.result(ctx, cfg, m)
}

// Delete removes an income. Movements of another kind are reported as not found.
func (s *Service) Delete(ctx context.Context, id int) error {
	if _, err := s.income(ctx, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

// List returns the income of a YYYY-MM month (the current one when empty),
// newest first, at most limit of them (DefaultListLimit when not positive).
func (s *Service) List(ctx context.Context, month string, limit int) ([]ledger.Movement, error) {
	if month == "" {
		month = ledger.CurrentMonth(s.now())
	}
	if _, err := time.Parse("2006-01", month); err != nil {
		return nil, fmt.Errorf("%w: month %q must be YYYY-MM", ledger.ErrInvalid, month)
	}
	if limit <= 0 {
		limit = DefaultListLimit
	}
	return s.repo.ListByMonth(ctx, month, ledger.KindIncome, limit)
}

// InferCategory suggests the income category for a description.
func (s *Service) InferCategory(ctx context.Context, description string) (string, bool, error) {
	cfg, err := s.settings.Get(ctx)
	if err != nil {
		return "", false, err
	}
	name, ok := cfg.InferCategory(description, ledger.KindIncome)
	return name, ok, nil
}

func (s *Service) income(ctx context.Context, id int) (ledger.Movement, error) {
	m, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return ledger.Movement{}, err
	}
	if m.Kind != ledger.KindIncome {
		return ledger.Movement{}, ledger.ErrNotFound
	}
	return m, nil
}

// result attaches the month summary and, for extra contracts, the split
// preview to a saved income.
func (s *Service) result(ctx context.Context, cfg settings.Config, m ledger.Movement) (Result, error) {
	res := Result{Movement: m}

	month := ledger.MonthOf(m.Date)
	monthIncome, err := s.repo.ListByMonth(ctx, month, ledger.KindIncome, 0)
	if err != nil {
		return Result{}, err
	}
	// The month total before this movement, whether or not the listing already
	// contains it (an update may also have moved it out of another month).
	before := decimal.Zero
	for _, other := range monthIncome {
		if other.ID != m.ID {
			before = before.Add(other.AmountMXN)
		}
	}
	total := before.Add(m.AmountMXN)
	res.Summary = Summary{Month: month, MonthTotalMXN: total, Resico: resicoEstimate(cfg, before, total)}

	if m.Category == ledger.CategoryExtraContract {
		split, err := s.split(ctx, cfg, m)
		if err != nil {
			return Result{}, err
		}
		res.Split = split
	}
	return res, nil
}

// resicoEstimate mirrors fin.py: the previous rate is the one of the month
// before the movement (the first bracket when it had no income). It returns nil
// when the brackets are not configured.
func resicoEstimate(cfg settings.Config, before, total decimal.Decimal) *ResicoEstimate {
	previous, err := cfg.ResicoRateOrFirst(before)
	if err != nil {
		return nil
	}
	rate, err := cfg.ResicoRateOrFirst(total)
	if err != nil {
		return nil
	}
	est := &ResicoEstimate{Rate: rate, EstimatedISR: total.Mul(rate), RateIncreased: rate.GreaterThan(previous)}
	if est.RateIncreased {
		est.PreviousRate = &previous
	}
	return est
}

// split computes the extra-contract split from the all-time emergency fund
// status. It returns nil when the settings cannot produce that status.
func (s *Service) split(ctx context.Context, cfg settings.Config, m ledger.Movement) (*Split, error) {
	savingsMovements, err := s.repo.ListAllByKind(ctx, ledger.KindSavings)
	if err != nil {
		return nil, err
	}
	em, err := savings.EmergencyFund(cfg, savingsMovements)
	if errors.Is(err, settings.ErrMissingConfig) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := Split{ExtraSplit: income.ComputeExtraSplit(m.AmountMXN, cfg, em.Accumulated, em.Goal)}
	var weights []settings.Weight
	for _, w := range cfg.InvestmentAllocation {
		if !w.Value.IsZero() {
			weights = append(weights, w)
		}
	}
	out.Breakdown = savings.SplitByWeights(out.Investments, weights)
	return &out, nil
}
