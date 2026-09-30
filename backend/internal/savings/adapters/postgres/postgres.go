// Package postgres implements the savings valuation repository on a pgx pool.
//
// value_mxn is a NUMERIC column. It crosses the driver as text
// (`$n::text::numeric` in, `col::text` out) so no precision is lost through a float.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
)

// Repo stores valuations in the valuations table. It is append-only.
type Repo struct{ pool *pgxpool.Pool }

// NewRepo builds a Repo.
func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// Add appends a valuation.
func (r *Repo) Add(ctx context.Context, v ledger.Valuation) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO valuations (date, instrument_id, value_mxn, note)
		VALUES ($1::date, $2, $3::text::numeric, $4)`,
		v.Date, v.Instrument, v.ValueMXN.String(), v.Note)
	return err
}

// List returns every valuation, oldest first (date asc, id asc).
func (r *Repo) List(ctx context.Context) ([]ledger.Valuation, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT date::text, instrument_id, value_mxn::text, note
		FROM valuations
		ORDER BY date ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ledger.Valuation{}
	for rows.Next() {
		var v ledger.Valuation
		var value string
		if err := rows.Scan(&v.Date, &v.Instrument, &value, &v.Note); err != nil {
			return nil, err
		}
		if v.ValueMXN, err = decimal.NewFromString(value); err != nil {
			return nil, fmt.Errorf("parse value_mxn: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
