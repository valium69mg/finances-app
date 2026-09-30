// Package postgres implements the invoice repository on a pgx pool: the
// invoices table and the metadata of their documents (the file bytes live in
// object storage).
//
// Money and rates are NUMERIC columns. They cross the driver as text
// (`$n::text::numeric` in, `col::text` out) so no precision is lost through a float.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/invoices/app"
	invoices "github.com/valium69mg/finances-app/backend/internal/invoices/domain"
)

// Repo stores invoices in the invoices and invoice_documents tables.
type Repo struct{ pool *pgxpool.Pool }

var _ app.Repo = (*Repo)(nil)

// NewRepo builds a Repo.
func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

const invoiceColumns = `id, client_id, collection_date::text, period, currency, exchange_rate::text,
	subtotal::text, subtotal_mxn::text, iva::text, isr_withheld::text, iva_withheld::text, total::text,
	expected_deposit_mxn::text, status, uuid, movement_id, declaration_period, created_at`

const documentColumns = `id, invoice_id, kind, key, name, content_type, size, sha256, uploaded_at`

const (
	pgUniqueViolation     = "23505"
	pgForeignKeyViolation = "23503"
	uuidConstraint        = "invoices_uuid_key"
)

func scanInvoice(row pgx.Row) (invoices.Invoice, error) {
	var (
		inv                                                          invoices.Invoice
		id                                                           int64
		rate, uuid, declaration                                      *string
		movementID                                                   *int64
		status                                                       string
		subtotal, subtotalMXN, iva, isr, ivaWithheld, total, deposit string
	)
	if err := row.Scan(&id, &inv.ClientID, &inv.CollectionDate, &inv.Period, &inv.Currency, &rate,
		&subtotal, &subtotalMXN, &iva, &isr, &ivaWithheld, &total, &deposit,
		&status, &uuid, &movementID, &declaration, &inv.CreatedAt); err != nil {
		return invoices.Invoice{}, err
	}
	inv.ID = int(id)
	inv.Status = invoices.Status(status)
	if uuid != nil {
		inv.UUID = *uuid
	}
	if declaration != nil {
		inv.DeclarationPeriod = *declaration
	}
	if movementID != nil {
		m := int(*movementID)
		inv.MovementID = &m
	}
	var err error
	if rate != nil {
		r, err := decimal.NewFromString(*rate)
		if err != nil {
			return invoices.Invoice{}, fmt.Errorf("parse exchange_rate: %w", err)
		}
		inv.ExchangeRate = &r
	}
	for _, f := range []struct {
		name string
		raw  string
		dst  *decimal.Decimal
	}{
		{"subtotal", subtotal, &inv.Subtotal}, {"subtotal_mxn", subtotalMXN, &inv.SubtotalMXN}, {"iva", iva, &inv.IVA},
		{"isr_withheld", isr, &inv.ISRWithheld}, {"iva_withheld", ivaWithheld, &inv.IVAWithheld},
		{"total", total, &inv.Total}, {"expected_deposit_mxn", deposit, &inv.ExpectedDepositMXN},
	} {
		if *f.dst, err = decimal.NewFromString(f.raw); err != nil {
			return invoices.Invoice{}, fmt.Errorf("parse %s: %w", f.name, err)
		}
	}
	return inv, nil
}

func scanDocument(row pgx.Row) (invoices.Document, error) {
	var (
		doc         invoices.Document
		id, invoice int64
		kind        string
	)
	if err := row.Scan(&id, &invoice, &kind, &doc.Key, &doc.Name, &doc.ContentType, &doc.Size, &doc.SHA256, &doc.UploadedAt); err != nil {
		return invoices.Document{}, err
	}
	doc.ID, doc.InvoiceID, doc.Kind = int(id), int(invoice), invoices.DocumentKind(kind)
	return doc, nil
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Create stores a prepared invoice.
func (r *Repo) Create(ctx context.Context, inv invoices.Invoice) (invoices.Invoice, error) {
	var rate *string
	if inv.ExchangeRate != nil {
		s := inv.ExchangeRate.String()
		rate = &s
	}
	var movement *int64
	if inv.MovementID != nil {
		m := int64(*inv.MovementID)
		movement = &m
	}
	row := r.pool.QueryRow(ctx, `
		INSERT INTO invoices (client_id, collection_date, period, currency, exchange_rate, subtotal, subtotal_mxn,
		                      iva, isr_withheld, iva_withheld, total, expected_deposit_mxn, status, uuid,
		                      movement_id, declaration_period)
		VALUES ($1, $2::date, $3, $4, $5::text::numeric, $6::text::numeric, $7::text::numeric,
		        $8::text::numeric, $9::text::numeric, $10::text::numeric, $11::text::numeric, $12::text::numeric,
		        $13, $14, $15, $16)
		RETURNING `+invoiceColumns,
		inv.ClientID, inv.CollectionDate, inv.Period, inv.Currency, rate, inv.Subtotal.String(), inv.SubtotalMXN.String(),
		inv.IVA.String(), inv.ISRWithheld.String(), inv.IVAWithheld.String(), inv.Total.String(), inv.ExpectedDepositMXN.String(),
		string(inv.Status), nullable(inv.UUID), movement, nullable(inv.DeclarationPeriod))
	saved, err := scanInvoice(row)
	if isPgCode(err, pgForeignKeyViolation) {
		return invoices.Invoice{}, fmt.Errorf("%w: the linked movement does not exist", invoices.ErrInvalidInput)
	}
	return saved, err
}

// Get returns one invoice.
func (r *Repo) Get(ctx context.Context, id int) (invoices.Invoice, error) {
	inv, err := scanInvoice(r.pool.QueryRow(ctx, `SELECT `+invoiceColumns+` FROM invoices WHERE id = $1`, int64(id)))
	if errors.Is(err, pgx.ErrNoRows) {
		return invoices.Invoice{}, invoices.ErrNotFound
	}
	return inv, err
}

// List returns the invoices newest first, filtered by period and status when set.
func (r *Repo) List(ctx context.Context, f app.ListFilter) ([]invoices.Invoice, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+invoiceColumns+` FROM invoices
		WHERE ($1 = '' OR period = $1) AND ($2 = '' OR status = $2)
		ORDER BY id DESC`, f.Period, string(f.Status))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []invoices.Invoice{}
	for rows.Next() {
		inv, err := scanInvoice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

// FindByUUID returns the invoice using the UUID.
func (r *Repo) FindByUUID(ctx context.Context, uuid string) (invoices.Invoice, bool, error) {
	inv, err := scanInvoice(r.pool.QueryRow(ctx, `SELECT `+invoiceColumns+` FROM invoices WHERE uuid = $1`, uuid))
	if errors.Is(err, pgx.ErrNoRows) {
		return invoices.Invoice{}, false, nil
	}
	if err != nil {
		return invoices.Invoice{}, false, err
	}
	return inv, true, nil
}

// Issue marks a prepared invoice as issued and stores its documents in one transaction.
func (r *Repo) Issue(ctx context.Context, id int, uuid string, docs []invoices.Document) ([]string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `UPDATE invoices SET status = 'emitida', uuid = $2 WHERE id = $1 AND status = 'preparada'`, int64(id), uuid)
	if isUniqueViolation(err, uuidConstraint) {
		return nil, invoices.ErrDuplicateUUID
	}
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, invoices.ErrStateChanged
	}
	var replaced []string
	for _, doc := range docs {
		_, old, err := upsertDocument(ctx, tx, doc)
		if err != nil {
			return nil, err
		}
		if old != "" {
			replaced = append(replaced, old)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return replaced, nil
}

// Cancel marks a non-cancelled invoice as cancelled, unless a tax filing
// includes it. The declared check is part of the UPDATE itself, so it cannot
// race with a filing being registered; the follow-up read only classifies why
// nothing was updated.
func (r *Repo) Cancel(ctx context.Context, id int) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE invoices SET status = 'cancelada'
		WHERE id = $1 AND status <> 'cancelada' AND declaration_period IS NULL`, int64(id))
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 {
		return nil
	}
	var (
		status      string
		declaration *string
	)
	err = r.pool.QueryRow(ctx, `SELECT status, declaration_period FROM invoices WHERE id = $1`, int64(id)).Scan(&status, &declaration)
	if errors.Is(err, pgx.ErrNoRows) {
		return invoices.ErrNotFound
	}
	if err != nil {
		return err
	}
	if status != string(invoices.StatusCancelled) && declaration != nil {
		return fmt.Errorf("%w: it is part of the filing of %s", invoices.ErrDeclared, *declaration)
	}
	return invoices.ErrStateChanged
}

// ListDocuments returns the documents of an invoice, xml first.
func (r *Repo) ListDocuments(ctx context.Context, invoiceID int) ([]invoices.Document, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+documentColumns+` FROM invoice_documents WHERE invoice_id = $1
		ORDER BY CASE kind WHEN 'xml' THEN 0 ELSE 1 END, id`, int64(invoiceID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []invoices.Document{}
	for rows.Next() {
		doc, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, doc)
	}
	return out, rows.Err()
}

// GetDocument returns one document of an invoice.
func (r *Repo) GetDocument(ctx context.Context, invoiceID, docID int) (invoices.Document, error) {
	doc, err := scanDocument(r.pool.QueryRow(ctx,
		`SELECT `+documentColumns+` FROM invoice_documents WHERE invoice_id = $1 AND id = $2`, int64(invoiceID), int64(docID)))
	if errors.Is(err, pgx.ErrNoRows) {
		return invoices.Document{}, invoices.ErrDocumentMissing
	}
	return doc, err
}

// PutDocument stores or replaces the document of its kind on an issued invoice.
func (r *Repo) PutDocument(ctx context.Context, doc invoices.Document) (invoices.Document, string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return invoices.Document{}, "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// FOR SHARE keeps a concurrent cancel from interleaving with the check.
	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM invoices WHERE id = $1 FOR SHARE`, int64(doc.InvoiceID)).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return invoices.Document{}, "", invoices.ErrNotFound
	}
	if err != nil {
		return invoices.Document{}, "", err
	}
	if status != string(invoices.StatusIssued) {
		return invoices.Document{}, "", invoices.ErrStateChanged
	}
	saved, old, err := upsertDocument(ctx, tx, doc)
	if err != nil {
		return invoices.Document{}, "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return invoices.Document{}, "", err
	}
	return saved, old, nil
}

type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// upsertDocument inserts the document or replaces the one of the same kind and
// returns the storage key it replaced ("" for a new document). The CTE reads the
// statement snapshot, so it still sees the old key when the row is updated.
func upsertDocument(ctx context.Context, q querier, doc invoices.Document) (invoices.Document, string, error) {
	var old *string
	var id int64
	err := q.QueryRow(ctx, `
		WITH previous AS (
			SELECT key FROM invoice_documents WHERE invoice_id = $1 AND kind = $2
		)
		INSERT INTO invoice_documents (invoice_id, kind, key, name, content_type, size, sha256)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (invoice_id, kind) DO UPDATE
		SET key = EXCLUDED.key, name = EXCLUDED.name, content_type = EXCLUDED.content_type,
		    size = EXCLUDED.size, sha256 = EXCLUDED.sha256, uploaded_at = now()
		RETURNING id, uploaded_at, (SELECT key FROM previous)`,
		int64(doc.InvoiceID), string(doc.Kind), doc.Key, doc.Name, doc.ContentType, doc.Size, doc.SHA256,
	).Scan(&id, &doc.UploadedAt, &old)
	if err != nil {
		return invoices.Document{}, "", err
	}
	doc.ID = int(id)
	if old == nil {
		return doc, "", nil
	}
	return doc, *old, nil
}

func isPgCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation && pgErr.ConstraintName == constraint
}
