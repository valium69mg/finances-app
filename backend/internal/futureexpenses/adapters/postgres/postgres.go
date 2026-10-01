// Package postgres implements the future expenses repository on a pgx pool.
//
// Money is NUMERIC. It crosses the driver as text (`$n::text::numeric` in,
// `col::text` out) so no precision is lost through a float.
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

	"github.com/valium69mg/finances-app/backend/internal/futureexpenses/app"
	domain "github.com/valium69mg/finances-app/backend/internal/futureexpenses/domain"
	ledgerpg "github.com/valium69mg/finances-app/backend/internal/ledger/adapters/postgres"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
)

// Repo stores the items in future_expenses and reads what is saved towards them
// from the movements linked through movements.future_expense_id.
type Repo struct{ pool *pgxpool.Pool }

// NewRepo builds a Repo.
func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// savedExpr is the net of the Ahorro movements linked to the item (the release
// row of a payment counts, so a paid item is back at zero).
const savedExpr = `COALESCE((SELECT SUM(m.amount_mxn) FROM movements m
	WHERE m.future_expense_id = f.id AND m.kind = 'Ahorro'), 0)::text`

const columns = `f.id, f.name, f.target_amount::text, f.due_date::text, f.status, f.paid_at::text,
	f.amount_paid::text, f.expense_movement_id, ` + savedExpr + `, f.created_at, f.updated_at`

func scan(row pgx.Row) (domain.FutureExpense, error) {
	var (
		it                   domain.FutureExpense
		id                   int64
		status               string
		target, saved        string
		paidAt, amountPaid   *string
		expenseID            *int64
		createdAt, updatedAt time.Time
	)
	if err := row.Scan(&id, &it.Name, &target, &it.DueDate, &status, &paidAt, &amountPaid, &expenseID, &saved, &createdAt, &updatedAt); err != nil {
		return domain.FutureExpense{}, err
	}
	it.ID, it.Status, it.CreatedAt, it.UpdatedAt = int(id), domain.Status(status), createdAt, updatedAt
	var err error
	if it.Target, err = decimal.NewFromString(target); err != nil {
		return domain.FutureExpense{}, fmt.Errorf("parse target_amount: %w", err)
	}
	if it.Saved, err = decimal.NewFromString(saved); err != nil {
		return domain.FutureExpense{}, fmt.Errorf("parse saved: %w", err)
	}
	if paidAt != nil {
		it.PaidAt = *paidAt
	}
	if amountPaid != nil {
		a, err := decimal.NewFromString(*amountPaid)
		if err != nil {
			return domain.FutureExpense{}, fmt.Errorf("parse amount_paid: %w", err)
		}
		it.AmountPaid = &a
	}
	if expenseID != nil {
		e := int(*expenseID)
		it.ExpenseMovementID = &e
	}
	return it, nil
}

// RowQuerier is the QueryRow half of a pool or a transaction.
type RowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Insert stores an active item through a pool or an open transaction and
// returns its id. Modules that must create an item atomically with their own
// rows (expense request approval) call it inside their transaction so the
// insert SQL stays in one place.
func Insert(ctx context.Context, q RowQuerier, v domain.Validated) (int, error) {
	var id int64
	err := q.QueryRow(ctx, `
		INSERT INTO future_expenses (name, target_amount, due_date)
		VALUES ($1, $2::text::numeric, $3::date) RETURNING id`,
		v.Name, v.Target.String(), v.DueDate).Scan(&id)
	return int(id), err
}

// Create stores an active item.
func (r *Repo) Create(ctx context.Context, v domain.Validated) (domain.FutureExpense, error) {
	id, err := Insert(ctx, r.pool, v)
	if err != nil {
		return domain.FutureExpense{}, err
	}
	return r.Get(ctx, id)
}

// Get returns the item with its saved amount.
func (r *Repo) Get(ctx context.Context, id int) (domain.FutureExpense, error) {
	it, err := scan(r.pool.QueryRow(ctx, `SELECT `+columns+` FROM future_expenses f WHERE f.id = $1`, int64(id)))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.FutureExpense{}, domain.ErrNotFound
	}
	return it, err
}

// List returns every item ordered by due date then id.
func (r *Repo) List(ctx context.Context) ([]domain.FutureExpense, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+columns+` FROM future_expenses f ORDER BY f.due_date, f.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.FutureExpense{}
	for rows.Next() {
		it, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// Update replaces the editable fields of an active item.
func (r *Repo) Update(ctx context.Context, id int, v domain.Validated) (domain.FutureExpense, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE future_expenses
		SET name = $2, target_amount = $3::text::numeric, due_date = $4::date, updated_at = now()
		WHERE id = $1 AND status = 'active'`,
		int64(id), v.Name, v.Target.String(), v.DueDate)
	if err != nil {
		return domain.FutureExpense{}, err
	}
	if tag.RowsAffected() == 0 {
		// Either it does not exist or it is paid: Get tells which.
		if _, err := r.Get(ctx, id); err != nil {
			return domain.FutureExpense{}, err
		}
		return domain.FutureExpense{}, domain.ErrAlreadyPaid
	}
	return r.Get(ctx, id)
}

// Execer is the Exec half of a pool or a transaction.
type Execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// DeleteByID removes the item through a pool or an open transaction (the
// expense request revert deletes it atomically with the request reset). It
// reports whether a row was deleted. The foreign key of movements is ON DELETE
// SET NULL, so its linked savings are unlinked in the same statement.
func DeleteByID(ctx context.Context, q Execer, id int) (bool, error) {
	tag, err := q.Exec(ctx, `DELETE FROM future_expenses WHERE id = $1`, int64(id))
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// Delete removes the item. The foreign key of movements is ON DELETE SET NULL,
// so its linked savings are unlinked (they return to the free balance) in the
// same statement; nothing else is deleted.
func (r *Repo) Delete(ctx context.Context, id int) error {
	deleted, err := DeleteByID(ctx, r.pool, id)
	if err != nil {
		return err
	}
	if !deleted {
		return domain.ErrNotFound
	}
	return nil
}

// FreeBalance is the net of the Gastos futuros savings linked to no item.
func (r *Repo) FreeBalance(ctx context.Context) (decimal.Decimal, error) {
	var text string
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount_mxn), 0)::text FROM movements
		WHERE kind = 'Ahorro' AND category = $1 AND future_expense_id IS NULL`,
		domain.SavingsCategory).Scan(&text)
	if err != nil {
		return decimal.Zero, err
	}
	return decimal.NewFromString(text)
}

// MarkPaid pays the item atomically. The row is locked FOR UPDATE, which also
// makes a concurrent saving linked to it (its foreign key check takes a key
// share lock on the row) wait, so the amount passed to plan is the one that is
// released. The expense, the Ahorro rows and the status change commit together
// or not at all.
func (r *Repo) MarkPaid(ctx context.Context, id int, plan func(saved decimal.Decimal) (app.Settlement, error)) (domain.FutureExpense, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.FutureExpense{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM future_expenses WHERE id = $1 FOR UPDATE`, int64(id)).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.FutureExpense{}, domain.ErrNotFound
		}
		return domain.FutureExpense{}, err
	}
	if domain.Status(status) == domain.StatusPaid {
		return domain.FutureExpense{}, domain.ErrAlreadyPaid
	}
	var savedText string
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount_mxn), 0)::text FROM movements WHERE future_expense_id = $1 AND kind = 'Ahorro'`,
		int64(id)).Scan(&savedText); err != nil {
		return domain.FutureExpense{}, err
	}
	saved, err := decimal.NewFromString(savedText)
	if err != nil {
		return domain.FutureExpense{}, fmt.Errorf("parse saved: %w", err)
	}
	settlement, err := plan(saved)
	if err != nil {
		return domain.FutureExpense{}, err
	}
	expense, err := ledgerpg.Insert(ctx, tx, settlement.Expense)
	if err != nil {
		return domain.FutureExpense{}, fmt.Errorf("register the expense: %w", err)
	}
	for i, m := range settlement.Savings {
		if m.Kind != ledger.KindSavings {
			return domain.FutureExpense{}, fmt.Errorf("settlement row %d is not a saving", i+1)
		}
		if _, err := ledgerpg.Insert(ctx, tx, m); err != nil {
			return domain.FutureExpense{}, fmt.Errorf("release row %d: %w", i+1, err)
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE future_expenses
		SET status = 'paid', paid_at = $2::date, amount_paid = $3::text::numeric, expense_movement_id = $4, updated_at = now()
		WHERE id = $1`,
		int64(id), expense.Date, expense.AmountMXN.String(), int64(expense.ID)); err != nil {
		return domain.FutureExpense{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.FutureExpense{}, err
	}
	return r.Get(ctx, id)
}
