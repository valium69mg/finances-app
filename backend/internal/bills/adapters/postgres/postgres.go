// Package postgres implements the bills repository on a pgx pool: the bills and
// bill_occurrences tables. Money is NUMERIC and crosses the driver as text
// (`$n::text::numeric` in, `col::text` out) so no precision is lost.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/bills/app"
	bills "github.com/valium69mg/finances-app/backend/internal/bills/domain"
)

// ErrNotEmpty is returned by Import when the bills table already has rows.
var ErrNotEmpty = errors.New("bills table is not empty")

// Repo stores bills and occurrences.
type Repo struct{ pool *pgxpool.Pool }

var _ app.Repo = (*Repo)(nil)

// NewRepo builds a Repo.
func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// billSelect reads a bill with its pending occurrence (NULL columns when none).
const billSelect = `
	SELECT b.id, b.name, b.category, b.amount::text, b.currency, b.recurrence, b.next_due_date::text, b.anchor_day,
	       b.reminder_lead_days, b.active, b.notes, b.created_at,
	       o.id, o.due_date::text
	FROM bills b
	LEFT JOIN bill_occurrences o ON o.bill_id = b.id AND o.status = 'pending'`

const occurrenceColumns = `id, bill_id, due_date::text, status, paid_on::text, expense_movement_id, amount_paid::text, currency, resolved_at`

func scanBill(row pgx.Row) (bills.Bill, error) {
	var (
		b          bills.Bill
		amount     *string
		recurrence string
		anchor     int16
		occID      *int64
		occDue     *string
	)
	if err := row.Scan(&b.ID, &b.Name, &b.Category, &amount, &b.Currency, &recurrence, &b.NextDueDate, &anchor,
		&b.ReminderLeadDays, &b.Active, &b.Notes, &b.CreatedAt, &occID, &occDue); err != nil {
		return bills.Bill{}, err
	}
	b.Recurrence, b.AnchorDay = bills.Recurrence(recurrence), int(anchor)
	if amount != nil {
		a, err := decimal.NewFromString(*amount)
		if err != nil {
			return bills.Bill{}, fmt.Errorf("parse amount: %w", err)
		}
		b.Amount = &a
	}
	if occID != nil && occDue != nil {
		b.Pending = &bills.Occurrence{ID: int(*occID), BillID: b.ID, DueDate: *occDue, Status: bills.StatusPending}
	}
	return b, nil
}

func scanOccurrence(row pgx.Row) (bills.Occurrence, error) {
	var (
		o        bills.Occurrence
		id, bill int64
		status   string
		paidOn   *string
		expense  *int64
		amount   *string
		currency *string
		resolved *time.Time
	)
	if err := row.Scan(&id, &bill, &o.DueDate, &status, &paidOn, &expense, &amount, &currency, &resolved); err != nil {
		return bills.Occurrence{}, err
	}
	o.ID, o.BillID, o.Status, o.ResolvedAt = int(id), int(bill), bills.Status(status), resolved
	if paidOn != nil {
		o.PaidOn = *paidOn
	}
	if currency != nil {
		o.Currency = *currency
	}
	if expense != nil {
		e := int(*expense)
		o.ExpenseID = &e
	}
	if amount != nil {
		a, err := decimal.NewFromString(*amount)
		if err != nil {
			return bills.Occurrence{}, fmt.Errorf("parse amount_paid: %w", err)
		}
		o.AmountPaid = &a
	}
	return o, nil
}

func amountArg(a *decimal.Decimal) *string {
	if a == nil {
		return nil
	}
	s := a.String()
	return &s
}

const insertBill = `
	INSERT INTO bills (name, category, amount, currency, recurrence, next_due_date, anchor_day, reminder_lead_days, active, notes)
	VALUES ($1, $2, $3::text::numeric, $4, $5, $6::date, $7, $8, $9, $10)
	RETURNING id`

func insertBillTx(ctx context.Context, tx pgx.Tx, v bills.Validated) (int, error) {
	var id int64
	err := tx.QueryRow(ctx, insertBill, v.Name, v.Category, amountArg(v.Amount), v.Currency, string(v.Recurrence),
		v.NextDueDate, int16(v.AnchorDay), v.ReminderLeadDays, v.Active, v.Notes).Scan(&id)
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO bill_occurrences (bill_id, due_date) VALUES ($1, $2::date)`, id, v.NextDueDate); err != nil {
		return 0, err
	}
	return int(id), nil
}

// Create stores the bill and its first pending occurrence in one transaction.
func (r *Repo) Create(ctx context.Context, v bills.Validated) (bills.Bill, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return bills.Bill{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	id, err := insertBillTx(ctx, tx, v)
	if err != nil {
		return bills.Bill{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return bills.Bill{}, err
	}
	return r.Get(ctx, id)
}

// Import stores the bills, each with its first occurrence, in one transaction.
// Unless force is set it refuses (ErrNotEmpty) when the table already has rows;
// force appends, it never deletes.
func (r *Repo) Import(ctx context.Context, list []bills.Validated, force bool) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if !force {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM bills)`).Scan(&exists); err != nil {
			return 0, err
		}
		if exists {
			return 0, ErrNotEmpty
		}
	}
	for _, v := range list {
		if _, err := insertBillTx(ctx, tx, v); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(list), nil
}

// Get returns a bill with its pending occurrence.
func (r *Repo) Get(ctx context.Context, id int) (bills.Bill, error) {
	b, err := scanBill(r.pool.QueryRow(ctx, billSelect+` WHERE b.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return bills.Bill{}, bills.ErrNotFound
	}
	return b, err
}

// List returns the bills ordered by pending due date, then name.
func (r *Repo) List(ctx context.Context, includeInactive bool) ([]bills.Bill, error) {
	rows, err := r.pool.Query(ctx, billSelect+` WHERE ($1 OR b.active) ORDER BY b.next_due_date, lower(b.name), b.id`, includeInactive)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []bills.Bill{}
	for rows.Next() {
		b, err := scanBill(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// History returns the paid and skipped occurrences of a bill, newest first.
func (r *Repo) History(ctx context.Context, billID int) ([]bills.Occurrence, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+occurrenceColumns+` FROM bill_occurrences
		WHERE bill_id = $1 AND status <> 'pending' ORDER BY due_date DESC, id DESC`, billID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []bills.Occurrence{}
	for rows.Next() {
		o, err := scanOccurrence(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// Update replaces the editable fields of a bill and moves its pending
// occurrence to the new next due date.
func (r *Repo) Update(ctx context.Context, id int, v bills.Validated) (bills.Bill, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return bills.Bill{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `
		UPDATE bills SET name = $2, category = $3, amount = $4::text::numeric, currency = $5, recurrence = $6,
		                 next_due_date = $7::date, anchor_day = $8, reminder_lead_days = $9, active = $10, notes = $11
		WHERE id = $1`,
		id, v.Name, v.Category, amountArg(v.Amount), v.Currency, string(v.Recurrence),
		v.NextDueDate, int16(v.AnchorDay), v.ReminderLeadDays, v.Active, v.Notes)
	if err != nil {
		return bills.Bill{}, err
	}
	if tag.RowsAffected() == 0 {
		return bills.Bill{}, bills.ErrNotFound
	}
	if _, err := tx.Exec(ctx, `UPDATE bill_occurrences SET due_date = $2::date WHERE bill_id = $1 AND status = 'pending'`, id, v.NextDueDate); err != nil {
		return bills.Bill{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return bills.Bill{}, err
	}
	return r.Get(ctx, id)
}

// Deactivate sets active = false.
func (r *Repo) Deactivate(ctx context.Context, id int) error {
	tag, err := r.pool.Exec(ctx, `UPDATE bills SET active = false WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return bills.ErrNotFound
	}
	return nil
}

// Resolve closes the pending occurrence and opens the next one in one transaction.
func (r *Repo) Resolve(ctx context.Context, res app.Resolution) (bills.Bill, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return bills.Bill{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var tag interface{ RowsAffected() int64 }
	switch res.Status {
	case bills.StatusPaid:
		var expense *int64
		if res.ExpenseID != nil {
			e := int64(*res.ExpenseID)
			expense = &e
		}
		tag, err = tx.Exec(ctx, `
			UPDATE bill_occurrences
			SET status = 'paid', paid_on = $3::date, amount_paid = $4::text::numeric, currency = $5,
			    expense_movement_id = $6, resolved_at = now()
			WHERE id = $1 AND bill_id = $2 AND status = 'pending'`,
			res.OccurrenceID, res.BillID, res.PaidOn, res.AmountPaid.String(), res.Currency, expense)
	case bills.StatusSkipped:
		tag, err = tx.Exec(ctx, `
			UPDATE bill_occurrences SET status = 'skipped', resolved_at = now()
			WHERE id = $1 AND bill_id = $2 AND status = 'pending'`, res.OccurrenceID, res.BillID)
	default:
		return bills.Bill{}, fmt.Errorf("%w: cannot resolve to status %q", bills.ErrInvalidInput, res.Status)
	}
	if err != nil {
		return bills.Bill{}, err
	}
	if tag.RowsAffected() == 0 {
		var one int
		err := tx.QueryRow(ctx, `SELECT 1 FROM bills WHERE id = $1`, res.BillID).Scan(&one)
		if errors.Is(err, pgx.ErrNoRows) {
			return bills.Bill{}, bills.ErrNotFound
		}
		if err != nil {
			return bills.Bill{}, err
		}
		return bills.Bill{}, bills.ErrNotPending
	}
	if _, err := tx.Exec(ctx, `INSERT INTO bill_occurrences (bill_id, due_date) VALUES ($1, $2::date)`, res.BillID, res.NextDueDate); err != nil {
		return bills.Bill{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE bills SET next_due_date = $2::date WHERE id = $1`, res.BillID, res.NextDueDate); err != nil {
		return bills.Bill{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return bills.Bill{}, err
	}
	return r.Get(ctx, res.BillID)
}
