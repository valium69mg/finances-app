// Package app holds the savings use cases: registering, correcting and listing
// Ahorro movements, transfers between instruments, manual valuations and the
// portfolio built from them.
package app

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	ledgerapp "github.com/valium69mg/finances-app/backend/internal/ledger/app"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	savings "github.com/valium69mg/finances-app/backend/internal/savings/domain"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

// DefaultListLimit is the number of savings movements listed when no limit is given.
const DefaultListLimit = 20

// Settings is the part of the settings module the savings use cases consume.
type Settings interface {
	Get(ctx context.Context) (settings.Config, error)
}

// Movements is the movement storage the savings use cases need: the shared
// repository plus the atomic multi-insert used by transfers.
type Movements interface {
	ledgerapp.MovementRepo
	ledgerapp.BatchCreator
	ledgerapp.TransferDeleter
}

// Input is the data of a new or updated savings movement. Empty optional
// fields take the defaults of ledger.NewMovement (payment method Transferencia,
// currency MXN, date today, USD rate from fx_rate_applied). An empty Category
// is inferred from the description and an empty Instrument defaults to the one
// configured for the category. A negative Amount is a withdrawal.
type Input struct {
	Date          string
	Description   string
	Category      string
	Instrument    string
	PaymentMethod string
	Currency      string
	Amount        decimal.Decimal
	ExchangeRate  *decimal.Decimal
}

// TransferInput moves Amount (> 0, MXN) from one instrument to another. An
// empty Category is inferred from the instruments and an empty Description
// becomes "Traspaso <from> -> <to>".
type TransferInput struct {
	From        string
	To          string
	Amount      decimal.Decimal
	Date        string
	Description string
	Category    string
}

// Transfer is the pair of movements a transfer created.
type Transfer struct {
	Out ledger.Movement // negative, on the source instrument
	In  ledger.Movement // positive, on the target instrument
}

// ValuationInput is a manual valuation of an instrument. An empty Date is today.
type ValuationInput struct {
	Date       string
	Instrument string
	ValueMXN   decimal.Decimal
	Note       string
}

// Service implements the savings use cases.
type Service struct {
	movements  Movements
	valuations ValuationRepo
	settings   Settings
	now        func() time.Time
}

// NewService builds a Service. A nil now selects time.Now.
func NewService(movements Movements, valuations ValuationRepo, settings Settings, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{movements: movements, valuations: valuations, settings: settings, now: now}
}

func (s *Service) today() string { return s.now().Format("2006-01-02") }

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ledger.ErrInvalid, fmt.Sprintf(format, args...))
}

// savingsCategory resolves a category name (case and accent insensitive) to
// its canonical Ahorro category.
func savingsCategory(cfg settings.Config, name string) (string, error) {
	cat, ok := cfg.Catalog().FindCategory(name, ledger.KindSavings)
	if !ok {
		return "", invalid("%q is not a valid %s category, valid: %s",
			name, ledger.KindSavings, strings.Join(cfg.Catalog().CategoryNames(ledger.KindSavings), ", "))
	}
	return cat, nil
}

func (s *Service) build(ctx context.Context, in Input) (ledger.Movement, error) {
	cfg, err := s.settings.Get(ctx)
	if err != nil {
		return ledger.Movement{}, err
	}
	name := in.Category
	if strings.TrimSpace(name) == "" {
		inferred, ok := cfg.InferCategory(in.Description, ledger.KindSavings)
		if !ok {
			return ledger.Movement{}, invalid("could not infer the category from the description, choose one")
		}
		name = inferred
	}
	category, err := savingsCategory(cfg, name)
	if err != nil {
		return ledger.Movement{}, err
	}
	instrument, err := cfg.ResolveInstrument(category, strings.TrimSpace(in.Instrument))
	if err != nil {
		return ledger.Movement{}, fmt.Errorf("%w: %v", ledger.ErrInvalid, err)
	}
	return ledger.NewMovement(ledger.MovementInput{
		Date: in.Date, Description: in.Description, Category: category, Instrument: instrument, Kind: ledger.KindSavings,
		PaymentMethod: in.PaymentMethod, Currency: in.Currency, Amount: in.Amount, ExchangeRate: in.ExchangeRate,
	}, cfg.Catalog(), s.today())
}

// CreateSaving registers a savings movement (a withdrawal when negative).
func (s *Service) CreateSaving(ctx context.Context, in Input) (ledger.Movement, error) {
	m, err := s.build(ctx, in)
	if err != nil {
		return ledger.Movement{}, err
	}
	return s.movements.Create(ctx, m)
}

// withExisting fills the fields an update left empty from the stored movement,
// so an edit never silently resets the date, payment method, currency or
// exchange rate to the creation defaults. The rate is only carried over while
// the movement stays in USD; a currency change without a rate takes the
// configured one, as on creation.
func withExisting(cur ledger.Movement, in Input) Input {
	if strings.TrimSpace(in.Date) == "" {
		in.Date = cur.Date
	}
	if strings.TrimSpace(in.PaymentMethod) == "" {
		in.PaymentMethod = cur.PaymentMethod
	}
	if strings.TrimSpace(in.Currency) == "" {
		in.Currency = cur.Currency
	}
	if in.ExchangeRate == nil && strings.TrimSpace(in.Currency) == ledger.CurrencyUSD && cur.Currency == ledger.CurrencyUSD {
		in.ExchangeRate = cur.ExchangeRate
	}
	return in
}

// UpdateSaving replaces the savings movement with the given ID. Fields left
// empty in the input (date, payment method, currency, exchange rate) keep their
// stored value; an empty category or instrument follows the creation rules.
// Movements of another kind are reported as not found: this module only
// manages savings.
func (s *Service) UpdateSaving(ctx context.Context, id int, in Input) (ledger.Movement, error) {
	cur, err := s.saving(ctx, id)
	if err != nil {
		return ledger.Movement{}, err
	}
	if cur.TransferID != "" {
		return ledger.Movement{}, savings.ErrTransferLegLocked
	}
	m, err := s.build(ctx, withExisting(cur, in))
	if err != nil {
		return ledger.Movement{}, err
	}
	m.ID = id
	if err := s.movements.Update(ctx, m); err != nil {
		return ledger.Movement{}, err
	}
	return m, nil
}

// DeleteSaving removes a savings movement. Deleting a leg of a transfer removes
// both legs in one statement, so a transfer is never left half there.
// Movements of another kind are reported as not found.
func (s *Service) DeleteSaving(ctx context.Context, id int) error {
	m, err := s.saving(ctx, id)
	if err != nil {
		return err
	}
	if m.TransferID != "" {
		return s.movements.DeleteByTransfer(ctx, m.TransferID)
	}
	return s.movements.Delete(ctx, id)
}

// newTransferID returns a random (version 4) UUID linking the legs of a transfer.
func newTransferID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate transfer id: %w", err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// ListSavings returns the savings of a YYYY-MM budget cycle (the current one
// when empty), newest first, at most limit of them (DefaultListLimit when not
// positive).
func (s *Service) ListSavings(ctx context.Context, month string, limit int) ([]ledger.Movement, error) {
	cfg, err := s.settings.Get(ctx)
	if err != nil {
		return nil, err
	}
	cycle := cfg.Cycle()
	if month == "" {
		month = cycle.Current(s.now())
	}
	from, to, err := cycle.Range(month)
	if err != nil {
		return nil, invalid("month %q must be YYYY-MM", month)
	}
	if limit <= 0 {
		limit = DefaultListLimit
	}
	return s.movements.ListByRange(ctx, from, to, ledger.KindSavings, limit)
}

func (s *Service) saving(ctx context.Context, id int) (ledger.Movement, error) {
	m, err := s.movements.GetByID(ctx, id)
	if err != nil {
		return ledger.Movement{}, err
	}
	if m.Kind != ledger.KindSavings {
		return ledger.Movement{}, ledger.ErrNotFound
	}
	return m, nil
}

// transferCategory returns the category of a transfer: the explicit one, else
// the category whose default instrument is the source or, failing that, the
// target (as fin.py does). When several categories share an instrument the
// last one in configuration order wins, like the reversed dict of fin.py.
func transferCategory(cfg settings.Config, in TransferInput) (string, error) {
	if strings.TrimSpace(in.Category) != "" {
		return savingsCategory(cfg, in.Category)
	}
	reverse := func(instrument string) string {
		found := ""
		for _, c := range cfg.CategoriesByKind(ledger.KindSavings) {
			if cfg.InstrumentByCategory[c.Name] == instrument {
				found = c.Name
			}
		}
		return found
	}
	if c := reverse(in.From); c != "" {
		return c, nil
	}
	if c := reverse(in.To); c != "" {
		return c, nil
	}
	return "", invalid("could not infer the savings category, specify category")
}

// Transfer moves money between two instruments by storing two Ahorro
// movements, -amount on the source and +amount on the target, in one
// transaction so a transfer is never half recorded.
func (s *Service) Transfer(ctx context.Context, in TransferInput) (Transfer, error) {
	cfg, err := s.settings.Get(ctx)
	if err != nil {
		return Transfer{}, err
	}
	from, to := strings.TrimSpace(in.From), strings.TrimSpace(in.To)
	if from == "" || to == "" {
		return Transfer{}, invalid("from and to instruments are required")
	}
	if from == to {
		return Transfer{}, invalid("source and target instruments must differ")
	}
	if _, ok := cfg.FindInstrument(from); !ok {
		return Transfer{}, invalid("source instrument %q does not exist", from)
	}
	if _, ok := cfg.FindInstrument(to); !ok {
		return Transfer{}, invalid("target instrument %q does not exist", to)
	}
	if !in.Amount.IsPositive() {
		return Transfer{}, invalid("transfer amount must be greater than zero")
	}
	category, err := transferCategory(cfg, TransferInput{From: from, To: to, Category: in.Category})
	if err != nil {
		return Transfer{}, err
	}
	description := strings.TrimSpace(in.Description)
	if description == "" {
		description = fmt.Sprintf("Traspaso %s -> %s", from, to)
	}

	leg := func(instrument string, amount decimal.Decimal) (ledger.Movement, error) {
		return ledger.NewMovement(ledger.MovementInput{
			Date: in.Date, Description: description, Category: category, Instrument: instrument,
			Kind: ledger.KindSavings, Amount: amount,
		}, cfg.Catalog(), s.today())
	}
	transferID, err := newTransferID()
	if err != nil {
		return Transfer{}, err
	}
	out, err := leg(from, in.Amount.Neg())
	if err != nil {
		return Transfer{}, err
	}
	inMovement, err := leg(to, in.Amount)
	if err != nil {
		return Transfer{}, err
	}
	out.TransferID, inMovement.TransferID = transferID, transferID
	saved, err := s.movements.CreateMany(ctx, []ledger.Movement{out, inMovement})
	if err != nil {
		return Transfer{}, err
	}
	if len(saved) != 2 {
		return Transfer{}, fmt.Errorf("transfer stored %d movements, want 2", len(saved))
	}
	return Transfer{Out: saved[0], In: saved[1]}, nil
}

// AddValuation appends a manual valuation of a configured instrument. It never
// changes earlier valuations: a correction is a newer valuation.
func (s *Service) AddValuation(ctx context.Context, in ValuationInput) (ledger.Valuation, error) {
	cfg, err := s.settings.Get(ctx)
	if err != nil {
		return ledger.Valuation{}, err
	}
	v := ledger.Valuation{
		Date: strings.TrimSpace(in.Date), Instrument: strings.TrimSpace(in.Instrument),
		ValueMXN: in.ValueMXN, Note: strings.TrimSpace(in.Note),
	}
	if v.Date == "" {
		v.Date = s.today()
	}
	if err := savings.ValidateValuation(v); err != nil {
		return ledger.Valuation{}, err
	}
	// The table keeps cents: store (and answer with) the rounded value.
	v.ValueMXN = savings.RoundValuation(v.ValueMXN)
	if _, ok := cfg.FindInstrument(v.Instrument); !ok {
		return ledger.Valuation{}, fmt.Errorf("%w: instrument %q does not exist", savings.ErrInvalidValuation, v.Instrument)
	}
	if err := s.valuations.Add(ctx, v); err != nil {
		return ledger.Valuation{}, err
	}
	return v, nil
}

// ListValuations returns every valuation, oldest first.
func (s *Service) ListValuations(ctx context.Context) ([]ledger.Valuation, error) {
	return s.valuations.List(ctx)
}

// Portfolio builds the portfolio from all-time savings movements, the
// valuations and the settings. It fails with settings.ErrMissingConfig when
// the emergency fund cannot be computed.
func (s *Service) Portfolio(ctx context.Context) (savings.Portfolio, error) {
	cfg, err := s.settings.Get(ctx)
	if err != nil {
		return savings.Portfolio{}, err
	}
	movements, err := s.movements.ListAllByKind(ctx, ledger.KindSavings)
	if err != nil {
		return savings.Portfolio{}, err
	}
	valuations, err := s.valuations.List(ctx)
	if err != nil {
		return savings.Portfolio{}, err
	}
	return savings.ComputePortfolio(cfg, movements, valuations)
}
