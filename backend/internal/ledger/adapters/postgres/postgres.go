// Package postgres implements the shared movement repository on a pgx pool.
//
// Money and rates are NUMERIC columns. They cross the driver as text
// (`$n::text::numeric` on the way in, `col::text` on the way out) so no
// precision is lost through a float.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/ledger/domain"
)

// Repo stores movements in the movements table.
type Repo struct{ pool *pgxpool.Pool }

// NewRepo builds a Repo.
func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

const columns = `id, date::text, description, category, instrument, kind, payment_method, currency,
	amount::text, exchange_rate::text, amount_mxn::text, transfer_id::text`

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func ratePtr(d *decimal.Decimal) *string {
	if d == nil {
		return nil
	}
	s := d.String()
	return &s
}

func scan(row pgx.Row) (domain.Movement, error) {
	var (
		m                    domain.Movement
		id                   int64
		instrument, rateText *string
		transferID           *string
		kind                 string
		amount, amountMXN    string
	)
	if err := row.Scan(&id, &m.Date, &m.Description, &m.Category, &instrument, &kind, &m.PaymentMethod,
		&m.Currency, &amount, &rateText, &amountMXN, &transferID); err != nil {
		return domain.Movement{}, err
	}
	m.ID = int(id)
	if transferID != nil {
		m.TransferID = *transferID
	}
	m.Kind = domain.Kind(kind)
	if instrument != nil {
		m.Instrument = *instrument
	}
	var err error
	if m.Amount, err = decimal.NewFromString(amount); err != nil {
		return domain.Movement{}, fmt.Errorf("parse amount: %w", err)
	}
	if m.AmountMXN, err = decimal.NewFromString(amountMXN); err != nil {
		return domain.Movement{}, fmt.Errorf("parse amount_mxn: %w", err)
	}
	if rateText != nil {
		r, err := decimal.NewFromString(*rateText)
		if err != nil {
			return domain.Movement{}, fmt.Errorf("parse exchange_rate: %w", err)
		}
		m.ExchangeRate = &r
	}
	return m, nil
}

// rowQuerier is the QueryRow half of a pool or a transaction.
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// insert stores m; a nil createdAt keeps the column default (now()).
func insert(ctx context.Context, q rowQuerier, m domain.Movement, createdAt *time.Time) (domain.Movement, error) {
	row := q.QueryRow(ctx, `
		INSERT INTO movements (date, description, category, instrument, kind, payment_method, currency,
		                       amount, exchange_rate, amount_mxn, created_at, transfer_id)
		VALUES ($1::date, $2, $3, $4, $5, $6, $7, $8::text::numeric, $9::text::numeric, $10::text::numeric,
		        COALESCE($11::timestamptz, now()), $12::text::uuid)
		RETURNING `+columns,
		m.Date, m.Description, m.Category, nullString(m.Instrument), string(m.Kind), m.PaymentMethod, m.Currency,
		m.Amount.String(), ratePtr(m.ExchangeRate), m.AmountMXN.String(), createdAt, nullString(m.TransferID))
	return scan(row)
}

// Create inserts the movement and returns it with its generated ID.
func (r *Repo) Create(ctx context.Context, m domain.Movement) (domain.Movement, error) {
	return insert(ctx, r.pool, m, nil)
}

// CreateMany inserts the movements in one transaction: either all of them are
// stored or none is. It returns them with their generated IDs, in order.
func (r *Repo) CreateMany(ctx context.Context, ms []domain.Movement) ([]domain.Movement, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	out := make([]domain.Movement, 0, len(ms))
	for i, m := range ms {
		saved, err := insert(ctx, tx, m, nil)
		if err != nil {
			return nil, fmt.Errorf("movement %d: %w", i+1, err)
		}
		out = append(out, saved)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

// ErrNotEmpty is returned by ImportMovements when the table already has rows.
var ErrNotEmpty = errors.New("movements table is not empty")

// ImportedMovement is a movement to load with its original creation time.
type ImportedMovement struct {
	Movement  domain.Movement
	CreatedAt time.Time
}

// ImportMovements inserts the rows in one transaction, keeping their creation
// times. Unless force is set it refuses (ErrNotEmpty) when the table already
// has rows; force appends without deleting anything. IDs are newly generated.
func (r *Repo) ImportMovements(ctx context.Context, rows []ImportedMovement, force bool) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if !force {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM movements)`).Scan(&exists); err != nil {
			return 0, err
		}
		if exists {
			return 0, ErrNotEmpty
		}
	}
	for i, row := range rows {
		createdAt := row.CreatedAt
		if _, err := insert(ctx, tx, row.Movement, &createdAt); err != nil {
			return 0, fmt.Errorf("row %d: %w", i+1, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(rows), nil
}

// Update replaces the fields of the movement with m.ID. The kind is a guard,
// not a value: a movement is only updated while it still has kind m.Kind, so a
// module can never overwrite (or change the kind of) a movement of another
// kind. A missing row and a kind mismatch both return domain.ErrNotFound.
func (r *Repo) Update(ctx context.Context, m domain.Movement) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE movements
		SET date = $2::date, description = $3, category = $4, instrument = $5, payment_method = $7,
		    currency = $8, amount = $9::text::numeric, exchange_rate = $10::text::numeric,
		    amount_mxn = $11::text::numeric
		WHERE id = $1 AND kind = $6`,
		int64(m.ID), m.Date, m.Description, m.Category, nullString(m.Instrument), string(m.Kind), m.PaymentMethod,
		m.Currency, m.Amount.String(), ratePtr(m.ExchangeRate), m.AmountMXN.String())
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// Delete removes the movement.
func (r *Repo) Delete(ctx context.Context, id int) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM movements WHERE id = $1`, int64(id))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// DeleteByTransfer removes every movement of a transfer (both legs) in a
// single statement, so the legs never outlive each other. It returns
// domain.ErrNotFound when no movement carries the transfer ID.
func (r *Repo) DeleteByTransfer(ctx context.Context, transferID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM movements WHERE transfer_id = $1::text::uuid`, transferID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// GetByID returns the movement or domain.ErrNotFound.
func (r *Repo) GetByID(ctx context.Context, id int) (domain.Movement, error) {
	m, err := scan(r.pool.QueryRow(ctx, `SELECT `+columns+` FROM movements WHERE id = $1`, int64(id)))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Movement{}, domain.ErrNotFound
	}
	return m, err
}

// ListByMonth returns the movements of a YYYY-MM month, newest first.
func (r *Repo) ListByMonth(ctx context.Context, month string, kind domain.Kind, limit int) ([]domain.Movement, error) {
	var limitArg *int64
	if limit > 0 {
		l := int64(limit)
		limitArg = &l
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+columns+`
		FROM movements
		WHERE date >= ($1::text || '-01')::date
		  AND date < (($1::text || '-01')::date + interval '1 month')
		  AND ($2::text = '' OR kind = $2::text)
		ORDER BY date DESC, id DESC
		LIMIT $3::bigint`,
		month, string(kind), limitArg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Movement{}
	for rows.Next() {
		m, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListAllByKind returns every movement of a kind, oldest first.
func (r *Repo) ListAllByKind(ctx context.Context, kind domain.Kind) ([]domain.Movement, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+columns+`
		FROM movements
		WHERE kind = $1::text
		ORDER BY date ASC, id ASC`, string(kind))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Movement{}
	for rows.Next() {
		m, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
