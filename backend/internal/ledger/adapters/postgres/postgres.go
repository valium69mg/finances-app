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
	amount::text, exchange_rate::text, amount_mxn::text`

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
		kind                 string
		amount, amountMXN    string
	)
	if err := row.Scan(&id, &m.Date, &m.Description, &m.Category, &instrument, &kind, &m.PaymentMethod,
		&m.Currency, &amount, &rateText, &amountMXN); err != nil {
		return domain.Movement{}, err
	}
	m.ID = int(id)
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

// Create inserts the movement and returns it with its generated ID.
func (r *Repo) Create(ctx context.Context, m domain.Movement) (domain.Movement, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO movements (date, description, category, instrument, kind, payment_method, currency,
		                       amount, exchange_rate, amount_mxn)
		VALUES ($1::date, $2, $3, $4, $5, $6, $7, $8::text::numeric, $9::text::numeric, $10::text::numeric)
		RETURNING `+columns,
		m.Date, m.Description, m.Category, nullString(m.Instrument), string(m.Kind), m.PaymentMethod, m.Currency,
		m.Amount.String(), ratePtr(m.ExchangeRate), m.AmountMXN.String())
	return scan(row)
}

// Update replaces every field of the movement with m.ID.
func (r *Repo) Update(ctx context.Context, m domain.Movement) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE movements
		SET date = $2::date, description = $3, category = $4, instrument = $5, kind = $6, payment_method = $7,
		    currency = $8, amount = $9::text::numeric, exchange_rate = $10::text::numeric,
		    amount_mxn = $11::text::numeric
		WHERE id = $1`,
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
