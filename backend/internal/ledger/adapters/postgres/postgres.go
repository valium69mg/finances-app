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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/session"
)

// Repo stores movements in the movements table.
type Repo struct{ pool *pgxpool.Pool }

// NewRepo builds a Repo.
func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

const columns = `id, date::text, description, category, instrument, kind, payment_method, currency,
	amount::text, exchange_rate::text, amount_mxn::text, transfer_id::text, future_expense_id`

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nullInt(n int) *int64 {
	if n == 0 {
		return nil
	}
	v := int64(n)
	return &v
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
		futureExpenseID      *int64
		kind                 string
		amount, amountMXN    string
	)
	if err := row.Scan(&id, &m.Date, &m.Description, &m.Category, &instrument, &kind, &m.PaymentMethod,
		&m.Currency, &amount, &rateText, &amountMXN, &transferID, &futureExpenseID); err != nil {
		return domain.Movement{}, err
	}
	m.ID = int(id)
	if transferID != nil {
		m.TransferID = *transferID
	}
	if futureExpenseID != nil {
		m.FutureExpenseID = int(*futureExpenseID)
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

// RowQuerier is the QueryRow half of a pool or a transaction.
type RowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Insert stores m through a pool or an open transaction and returns it with
// its generated ID. Modules that need to write movements atomically with their
// own rows (future expense payments) call it inside their transaction so the
// insert SQL stays in one place.
func Insert(ctx context.Context, q RowQuerier, m domain.Movement) (domain.Movement, error) {
	return insert(ctx, q, m, nil)
}

// insert stores m; a nil createdAt keeps the column default (now()). The acting
// user of the request context, when there is one, is recorded in created_by;
// imports and background jobs have none and leave it NULL.
func insert(ctx context.Context, q RowQuerier, m domain.Movement, createdAt *time.Time) (domain.Movement, error) {
	row := q.QueryRow(ctx, `
		INSERT INTO movements (date, description, category, instrument, kind, payment_method, currency,
		                       amount, exchange_rate, amount_mxn, created_at, transfer_id, future_expense_id, created_by)
		VALUES ($1::date, $2, $3, $4, $5, $6, $7, $8::text::numeric, $9::text::numeric, $10::text::numeric,
		        COALESCE($11::timestamptz, now()), $12::text::uuid, $13::bigint, $14::text::uuid)
		RETURNING `+columns,
		m.Date, m.Description, m.Category, nullString(m.Instrument), string(m.Kind), m.PaymentMethod, m.Currency,
		m.Amount.String(), ratePtr(m.ExchangeRate), m.AmountMXN.String(), createdAt, nullString(m.TransferID), nullInt(m.FutureExpenseID),
		nullString(session.UserID(ctx)))
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

// Execer is the Exec half of a pool or a transaction.
type Execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// DeleteOfKind removes the movement with the id and the kind through a pool or
// an open transaction, so a module can delete a movement atomically with its
// own rows while the SQL stays in one place. It reports whether a row was
// deleted: a missing row and a kind mismatch both answer false.
func DeleteOfKind(ctx context.Context, q Execer, id int, kind domain.Kind) (bool, error) {
	tag, err := q.Exec(ctx, `DELETE FROM movements WHERE id = $1 AND kind = $2`, int64(id), string(kind))
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
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

// ListByRange returns the movements dated from..to (YYYY-MM-DD, inclusive),
// newest first.
func (r *Repo) ListByRange(ctx context.Context, from, to string, kind domain.Kind, limit int) ([]domain.Movement, error) {
	var limitArg *int64
	if limit > 0 {
		l := int64(limit)
		limitArg = &l
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+columns+`
		FROM movements
		WHERE date >= $1::text::date
		  AND date <= $2::text::date
		  AND ($3::text = '' OR kind = $3::text)
		ORDER BY date DESC, id DESC
		LIMIT $4::bigint`,
		from, to, string(kind), limitArg)
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
