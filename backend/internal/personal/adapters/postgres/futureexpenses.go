package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/personal/app"
	"github.com/valium69mg/finances-app/backend/internal/personal/domain"
)

// The future expenses of a household user live in personal_future_expenses;
// what is saved towards an item is read from her Ahorro rows linked through
// personal_movements.future_expense_id. Every query is scoped by user_id.

// savedExpr is the net of her Ahorro rows linked to the item (the release row
// of a payment counts, so a paid item is back at zero).
const savedExpr = `COALESCE((SELECT SUM(m.amount) FROM personal_movements m
	WHERE m.future_expense_id = f.id AND m.user_id = f.user_id AND m.kind = 'Ahorro'), 0)::text`

const futureColumns = `f.id, f.name, f.target_amount::text, f.due_date::text, f.status, f.paid_at::text,
	f.amount_paid::text, f.expense_movement_id, ` + savedExpr + `, f.created_at, f.updated_at`

func scanFuture(row pgx.Row) (domain.FutureExpense, error) {
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
	it.ID, it.Status, it.CreatedAt, it.UpdatedAt = int(id), domain.FutureStatus(status), createdAt, updatedAt
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

// CreateFutureExpense stores an active item.
func (r *Repo) CreateFutureExpense(ctx context.Context, userID string, v domain.FutureValidated) (domain.FutureExpense, error) {
	it, err := scanFuture(r.pool.QueryRow(ctx, `
		WITH f AS (
			INSERT INTO personal_future_expenses (user_id, name, target_amount, due_date)
			VALUES ($1::text::uuid, $2, $3::text::numeric, $4::date) RETURNING *)
		SELECT `+futureColumns+` FROM f`,
		userID, v.Name, v.Target.String(), v.DueDate))
	return it, err
}

// GetFutureExpense returns the item with its saved amount.
func (r *Repo) GetFutureExpense(ctx context.Context, userID string, id int) (domain.FutureExpense, error) {
	it, err := scanFuture(r.pool.QueryRow(ctx, `
		SELECT `+futureColumns+` FROM personal_future_expenses f WHERE f.id = $1 AND f.user_id = $2::text::uuid`,
		int64(id), userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.FutureExpense{}, domain.ErrNotFound
	}
	return it, err
}

// ListFutureExpenses returns every item of the user ordered by due date then id.
func (r *Repo) ListFutureExpenses(ctx context.Context, userID string) ([]domain.FutureExpense, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+futureColumns+` FROM personal_future_expenses f
		WHERE f.user_id = $1::text::uuid ORDER BY f.due_date, f.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.FutureExpense{}
	for rows.Next() {
		it, err := scanFuture(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// UpdateFutureExpense replaces the editable fields of an active item.
func (r *Repo) UpdateFutureExpense(ctx context.Context, userID string, id int, v domain.FutureValidated) (domain.FutureExpense, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE personal_future_expenses
		SET name = $3, target_amount = $4::text::numeric, due_date = $5::date, updated_at = now()
		WHERE id = $1 AND user_id = $2::text::uuid AND status = 'active'`,
		int64(id), userID, v.Name, v.Target.String(), v.DueDate)
	if err != nil {
		return domain.FutureExpense{}, err
	}
	if tag.RowsAffected() == 0 {
		// Either it is not hers or it does not exist, or it is paid: Get tells which.
		if _, err := r.GetFutureExpense(ctx, userID, id); err != nil {
			return domain.FutureExpense{}, err
		}
		return domain.FutureExpense{}, domain.ErrAlreadyPaid
	}
	return r.GetFutureExpense(ctx, userID, id)
}

// DeleteFutureExpense removes the item. The composite foreign key of
// personal_movements is ON DELETE SET NULL (future_expense_id), so its linked
// savings are unlinked (they return to the free balance) in the same
// statement, keeping user_id; nothing else is deleted.
func (r *Repo) DeleteFutureExpense(ctx context.Context, userID string, id int) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM personal_future_expenses WHERE id = $1 AND user_id = $2::text::uuid`, int64(id), userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// freeBalance is the net of the user's Gastos futuros savings linked to no item.
func freeBalance(ctx context.Context, q rowQuerier, userID string) (decimal.Decimal, error) {
	var text string
	err := q.QueryRow(ctx, `
		SELECT COALESCE(SUM(m.amount), 0)::text
		FROM personal_movements m JOIN personal_categories c ON c.id = m.category_id
		WHERE m.user_id = $1::text::uuid AND m.kind = 'Ahorro' AND c.name = $2 AND m.future_expense_id IS NULL`,
		userID, domain.SavingsCategory).Scan(&text)
	if err != nil {
		return decimal.Zero, err
	}
	return decimal.NewFromString(text)
}

// FutureFreeBalance is the net of the user's Gastos futuros savings linked to no item.
func (r *Repo) FutureFreeBalance(ctx context.Context, userID string) (decimal.Decimal, error) {
	return freeBalance(ctx, r.pool, userID)
}

// savingsCategoryID returns the id of her Gastos futuros category.
func savingsCategoryID(ctx context.Context, q rowQuerier, userID string) (int, error) {
	var id int64
	err := q.QueryRow(ctx, `
		SELECT id FROM personal_categories WHERE user_id = $1::text::uuid AND kind = 'Ahorro' AND name = $2`,
		userID, domain.SavingsCategory).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("the %s category does not exist", domain.SavingsCategory)
	}
	return int(id), err
}

// lockItem locks the user's item (mode is a row lock clause) and returns its
// status. domain.ErrNotFound when it is not hers.
func lockItem(ctx context.Context, tx pgx.Tx, userID string, id int, mode string) (domain.FutureStatus, error) {
	var status string
	err := tx.QueryRow(ctx, `SELECT status FROM personal_future_expenses WHERE id = $1 AND user_id = $2::text::uuid `+mode,
		int64(id), userID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	return domain.FutureStatus(status), err
}

func savingMovement(categoryID int, row app.SavingRow) domain.ValidMovement {
	return domain.ValidMovement{Kind: domain.KindSavings, CategoryID: categoryID, Amount: row.Amount, Description: row.Description, Date: row.Date}
}

func linkOf(id int, linked bool) *int64 {
	if !linked {
		return nil
	}
	v := int64(id)
	return &v
}

// ContributeFuture stores a linked Ahorro. The item is locked FOR SHARE, which
// waits for a payment holding FOR UPDATE, so a saving never lands on an item
// that is already paid.
func (r *Repo) ContributeFuture(ctx context.Context, userID string, id int, row app.SavingRow) (domain.Movement, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Movement{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	status, err := lockItem(ctx, tx, userID, id, "FOR SHARE")
	if err != nil {
		return domain.Movement{}, err
	}
	if status == domain.FuturePaid {
		return domain.Movement{}, domain.ErrAlreadyPaid
	}
	catID, err := savingsCategoryID(ctx, tx, userID)
	if err != nil {
		return domain.Movement{}, err
	}
	m, err := insertMovement(ctx, tx, userID, savingMovement(catID, row), linkOf(id, row.Linked))
	if err != nil {
		return domain.Movement{}, err
	}
	return m, tx.Commit(ctx)
}

// AssignFuture moves part of the free balance to an item. A per-user advisory
// lock serializes concurrent assignments, so the free balance read here is the
// one both rows are checked against.
func (r *Repo) AssignFuture(ctx context.Context, userID string, id int, amount decimal.Decimal, date, description string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('personal_free_balance:' || $1::text, 0))`, userID); err != nil {
		return err
	}
	status, err := lockItem(ctx, tx, userID, id, "FOR UPDATE")
	if err != nil {
		return err
	}
	if status == domain.FuturePaid {
		return domain.ErrAlreadyPaid
	}
	free, err := freeBalance(ctx, tx, userID)
	if err != nil {
		return err
	}
	if amount.GreaterThan(free) {
		return domain.ErrInsufficientFreeBalance
	}
	catID, err := savingsCategoryID(ctx, tx, userID)
	if err != nil {
		return err
	}
	for _, row := range []app.SavingRow{
		{Amount: amount.Neg(), Date: date, Description: description},
		{Amount: amount, Date: date, Description: description, Linked: true},
	} {
		if _, err := insertMovement(ctx, tx, userID, savingMovement(catID, row), linkOf(id, row.Linked)); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// PayFuture pays the item atomically. The row is locked FOR UPDATE, which also
// makes a concurrent saving linked to it (FOR SHARE or the foreign key check)
// wait, so the amount passed to plan is the one that is released. The Gasto,
// the Ahorro rows and the status change commit together or not at all.
func (r *Repo) PayFuture(ctx context.Context, userID string, id int, plan func(it domain.FutureExpense) (app.Settlement, error)) (domain.FutureExpense, domain.Movement, error) {
	fail := func(err error) (domain.FutureExpense, domain.Movement, error) {
		return domain.FutureExpense{}, domain.Movement{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	status, err := lockItem(ctx, tx, userID, id, "FOR UPDATE")
	if err != nil {
		return fail(err)
	}
	if status == domain.FuturePaid {
		return fail(domain.ErrAlreadyPaid)
	}
	it, err := scanFuture(tx.QueryRow(ctx, `
		SELECT `+futureColumns+` FROM personal_future_expenses f WHERE f.id = $1 AND f.user_id = $2::text::uuid`,
		int64(id), userID))
	if err != nil {
		return fail(err)
	}
	settlement, err := plan(it)
	if err != nil {
		return fail(err)
	}
	expense, err := insertMovement(ctx, tx, userID, settlement.Expense, nil)
	if err != nil {
		return fail(fmt.Errorf("register the expense: %w", err))
	}
	if len(settlement.Savings) > 0 {
		catID, err := savingsCategoryID(ctx, tx, userID)
		if err != nil {
			return fail(err)
		}
		for i, row := range settlement.Savings {
			if _, err := insertMovement(ctx, tx, userID, savingMovement(catID, row), linkOf(id, row.Linked)); err != nil {
				return fail(fmt.Errorf("release row %d: %w", i+1, err))
			}
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE personal_future_expenses
		SET status = 'paid', paid_at = $3::date, amount_paid = $4::text::numeric, expense_movement_id = $5, updated_at = now()
		WHERE id = $1 AND user_id = $2::text::uuid`,
		int64(id), userID, expense.Date, expense.Amount.String(), int64(expense.ID)); err != nil {
		return fail(err)
	}
	paid, err := scanFuture(tx.QueryRow(ctx, `
		SELECT `+futureColumns+` FROM personal_future_expenses f WHERE f.id = $1 AND f.user_id = $2::text::uuid`,
		int64(id), userID))
	if err != nil {
		return fail(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	return paid, expense, nil
}
