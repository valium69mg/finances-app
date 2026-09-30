// Package postgres implements the tax filing repository on a pgx pool: the
// tax_filings table and the declaration_period link it sets on the invoices it
// includes.
//
// Money and rates are NUMERIC columns. They cross the driver as text
// (`$n::text::numeric` in, `col::text` out) so no precision is lost through a float.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/taxfiling/app"
	taxfiling "github.com/valium69mg/finances-app/backend/internal/taxfiling/domain"
)

// Repo stores filings in the tax_filings table.
type Repo struct{ pool *pgxpool.Pool }

var _ app.Repo = (*Repo)(nil)

// NewRepo builds a Repo.
func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

const filingColumns = `period, filing_date::text, income_collected::text, isr_rate::text, isr_accrued::text,
	isr_withheld::text, isr_due::text, iva_transferred::text, iva_withheld::text, iva_creditable::text,
	iva_due::text, folio, payment_date::text, isr_paid::text, iva_paid::text, expense_movement_id, created_at`

const (
	pgUniqueViolation = "23505"
	periodConstraint  = "tax_filings_pkey"
)

func scanFiling(row pgx.Row) (taxfiling.Filing, error) {
	var (
		f                                                  taxfiling.Filing
		income, rate, accrued, isrWithheld, isrDue         string
		ivaTransferred, ivaWithheld, ivaCreditable, ivaDue string
		paymentDate, isrPaid, ivaPaid                      *string
		expense                                            *int64
	)
	if err := row.Scan(&f.Period, &f.FilingDate, &income, &rate, &accrued, &isrWithheld, &isrDue,
		&ivaTransferred, &ivaWithheld, &ivaCreditable, &ivaDue, &f.Folio,
		&paymentDate, &isrPaid, &ivaPaid, &expense, &f.CreatedAt); err != nil {
		return taxfiling.Filing{}, err
	}
	var err error
	for _, v := range []struct {
		name string
		raw  string
		dst  *decimal.Decimal
	}{
		{"income_collected", income, &f.IncomeCollected}, {"isr_rate", rate, &f.ISRRate}, {"isr_accrued", accrued, &f.ISRAccrued},
		{"isr_withheld", isrWithheld, &f.ISRWithheld}, {"isr_due", isrDue, &f.ISRDue},
		{"iva_transferred", ivaTransferred, &f.IVATransferred}, {"iva_withheld", ivaWithheld, &f.IVAWithheld},
		{"iva_creditable", ivaCreditable, &f.IVACreditable}, {"iva_due", ivaDue, &f.IVADue},
	} {
		if *v.dst, err = decimal.NewFromString(v.raw); err != nil {
			return taxfiling.Filing{}, fmt.Errorf("parse %s: %w", v.name, err)
		}
	}
	if paymentDate != nil && isrPaid != nil && ivaPaid != nil {
		p := taxfiling.Payment{Date: *paymentDate}
		if p.ISRPaid, err = decimal.NewFromString(*isrPaid); err != nil {
			return taxfiling.Filing{}, fmt.Errorf("parse isr_paid: %w", err)
		}
		if p.IVAPaid, err = decimal.NewFromString(*ivaPaid); err != nil {
			return taxfiling.Filing{}, fmt.Errorf("parse iva_paid: %w", err)
		}
		f.Payment = &p
	}
	if expense != nil {
		id := int(*expense)
		f.ExpenseMovementID = &id
	}
	return f, nil
}

func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation && pgErr.ConstraintName == constraint
}

// Create stores the filing and links the issued invoices of its period to it in
// one transaction.
func (r *Repo) Create(ctx context.Context, f taxfiling.Filing) (taxfiling.Filing, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return taxfiling.Filing{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var paymentDate, isrPaid, ivaPaid *string
	if p := f.Payment; p != nil {
		isr, iva := p.ISRPaid.String(), p.IVAPaid.String()
		paymentDate, isrPaid, ivaPaid = &p.Date, &isr, &iva
	}
	var expense *int64
	if f.ExpenseMovementID != nil {
		e := int64(*f.ExpenseMovementID)
		expense = &e
	}
	saved, err := scanFiling(tx.QueryRow(ctx, `
		INSERT INTO tax_filings (period, filing_date, income_collected, isr_rate, isr_accrued, isr_withheld, isr_due,
		                         iva_transferred, iva_withheld, iva_creditable, iva_due, folio,
		                         payment_date, isr_paid, iva_paid, expense_movement_id)
		VALUES ($1, $2::date, $3::text::numeric, $4::text::numeric, $5::text::numeric, $6::text::numeric, $7::text::numeric,
		        $8::text::numeric, $9::text::numeric, $10::text::numeric, $11::text::numeric, $12,
		        $13::date, $14::text::numeric, $15::text::numeric, $16)
		RETURNING `+filingColumns,
		f.Period, f.FilingDate, f.IncomeCollected.String(), f.ISRRate.String(), f.ISRAccrued.String(), f.ISRWithheld.String(),
		f.ISRDue.String(), f.IVATransferred.String(), f.IVAWithheld.String(), f.IVACreditable.String(), f.IVADue.String(),
		f.Folio, paymentDate, isrPaid, ivaPaid, expense))
	if isUniqueViolation(err, periodConstraint) {
		return taxfiling.Filing{}, fmt.Errorf("%w: %s", taxfiling.ErrAlreadyFiled, f.Period)
	}
	if err != nil {
		return taxfiling.Filing{}, err
	}

	// The update takes a row lock on every issued invoice of the period, so a
	// concurrent cancel waits and an invoice issued meanwhile shows up as a
	// difference from the ids the declaration was computed from.
	rows, err := tx.Query(ctx, `
		UPDATE invoices SET declaration_period = $1
		WHERE period = $1 AND status = 'emitida'
		RETURNING id`, f.Period)
	if err != nil {
		return taxfiling.Filing{}, err
	}
	ids, err := collectIDs(rows)
	if err != nil {
		return taxfiling.Filing{}, err
	}
	want := slices.Clone(f.InvoiceIDs)
	slices.Sort(want)
	if !slices.Equal(ids, want) {
		return taxfiling.Filing{}, taxfiling.ErrInvoicesChanged
	}
	if err := tx.Commit(ctx); err != nil {
		return taxfiling.Filing{}, err
	}
	saved.InvoiceIDs = ids
	return saved, nil
}

// collectIDs reads the id column of the rows, sorted ascending, and closes them.
func collectIDs(rows pgx.Rows) ([]int, error) {
	defer rows.Close()
	ids := []int{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, int(id))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	slices.Sort(ids)
	return ids, nil
}

func (r *Repo) invoiceIDs(ctx context.Context, period string) ([]int, error) {
	rows, err := r.pool.Query(ctx, `SELECT id FROM invoices WHERE declaration_period = $1 ORDER BY id`, period)
	if err != nil {
		return nil, err
	}
	return collectIDs(rows)
}

// Get returns the filing of a period with the IDs of its invoices.
func (r *Repo) Get(ctx context.Context, period string) (taxfiling.Filing, error) {
	f, err := scanFiling(r.pool.QueryRow(ctx, `SELECT `+filingColumns+` FROM tax_filings WHERE period = $1`, period))
	if errors.Is(err, pgx.ErrNoRows) {
		return taxfiling.Filing{}, taxfiling.ErrNotFound
	}
	if err != nil {
		return taxfiling.Filing{}, err
	}
	if f.InvoiceIDs, err = r.invoiceIDs(ctx, period); err != nil {
		return taxfiling.Filing{}, err
	}
	return f, nil
}

// List returns every filing, newest period first, with the IDs of its invoices.
func (r *Repo) List(ctx context.Context) ([]taxfiling.Filing, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+filingColumns+` FROM tax_filings ORDER BY period DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []taxfiling.Filing{}
	for rows.Next() {
		f, err := scanFiling(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	link, err := r.pool.Query(ctx, `SELECT declaration_period, id FROM invoices WHERE declaration_period IS NOT NULL ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer link.Close()
	byPeriod := map[string][]int{}
	for link.Next() {
		var period string
		var id int64
		if err := link.Scan(&period, &id); err != nil {
			return nil, err
		}
		byPeriod[period] = append(byPeriod[period], int(id))
	}
	if err := link.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		out[i].InvoiceIDs = byPeriod[out[i].Period]
		if out[i].InvoiceIDs == nil {
			out[i].InvoiceIDs = []int{}
		}
	}
	return out, nil
}

// MarkPaid records the payment of a pending filing.
func (r *Repo) MarkPaid(ctx context.Context, period string, p taxfiling.Payment, expenseID *int) (taxfiling.Filing, error) {
	var expense *int64
	if expenseID != nil {
		e := int64(*expenseID)
		expense = &e
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE tax_filings
		SET payment_date = $2::date, isr_paid = $3::text::numeric, iva_paid = $4::text::numeric, expense_movement_id = $5
		WHERE period = $1 AND payment_date IS NULL`,
		period, p.Date, p.ISRPaid.String(), p.IVAPaid.String(), expense)
	if err != nil {
		return taxfiling.Filing{}, err
	}
	if tag.RowsAffected() == 0 {
		if err := r.missingOrPaid(ctx, r.pool, period); err != nil {
			return taxfiling.Filing{}, err
		}
	}
	return r.Get(ctx, period)
}

type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// missingOrPaid explains why a pending filing could not be changed: it returns
// ErrNotFound when the period has no filing and ErrAlreadyPaid otherwise (Delete
// maps that one to ErrFilingPaid).
func (r *Repo) missingOrPaid(ctx context.Context, q querier, period string) error {
	var one int
	err := q.QueryRow(ctx, `SELECT 1 FROM tax_filings WHERE period = $1`, period).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return taxfiling.ErrNotFound
	}
	if err != nil {
		return err
	}
	return taxfiling.ErrAlreadyPaid
}

// Delete removes a pending filing and unlinks its invoices in one transaction.
func (r *Repo) Delete(ctx context.Context, period string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `DELETE FROM tax_filings WHERE period = $1 AND payment_date IS NULL`, period)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		err := r.missingOrPaid(ctx, tx, period)
		if errors.Is(err, taxfiling.ErrAlreadyPaid) {
			return taxfiling.ErrFilingPaid
		}
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE invoices SET declaration_period = NULL WHERE declaration_period = $1`, period); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
