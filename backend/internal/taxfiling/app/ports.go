package app

import (
	"context"
	"io"

	expensesapp "github.com/valium69mg/finances-app/backend/internal/expenses/app"
	invoicesapp "github.com/valium69mg/finances-app/backend/internal/invoices/app"
	invoices "github.com/valium69mg/finances-app/backend/internal/invoices/domain"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
	taxfiling "github.com/valium69mg/finances-app/backend/internal/taxfiling/domain"
)

// Settings is the part of the settings module the tax filing use cases consume.
type Settings interface {
	Get(ctx context.Context) (settings.Config, error)
}

// Invoices is the read side of the invoices module.
type Invoices interface {
	// List returns the invoices, newest first, optionally filtered by
	// collection period (YYYY-MM) and status; empty filters match everything.
	List(ctx context.Context, period string, status invoices.Status) ([]invoices.Invoice, error)
}

// Expenses is the part of the expenses module used to record the payment of a
// filing as an Impuestos expense, so the USD/MXN and validation rules stay in
// one place.
type Expenses interface {
	Create(ctx context.Context, in expensesapp.Input) (expensesapp.Result, error)
	Delete(ctx context.Context, id int) error
}

// ErrObjectNotFound is returned by ObjectStore.Get when the key does not exist.
// It is the sentinel of the shared store adapter (the invoices one), so the same
// store satisfies both modules.
var ErrObjectNotFound = invoicesapp.ErrObjectNotFound

// ObjectStore is the S3-compatible storage of the attached files. It is the
// store the invoices module uses; keys are opaque to it and never exposed.
type ObjectStore interface {
	// Put stores size bytes read from r under key.
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	// Get returns the object body, or ErrObjectNotFound.
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	// Delete removes the object. Deleting a missing key is not an error.
	Delete(ctx context.Context, key string) error
}

// Repo persists the filings of the tax_filings table together with the
// declaration_period link on the invoices they include.
type Repo interface {
	// Create stores the filing and, in the same transaction, links every issued
	// invoice of its period to it. It returns taxfiling.ErrAlreadyFiled when the
	// period is filed and taxfiling.ErrInvoicesChanged when the issued invoices
	// of the period are not exactly f.InvoiceIDs.
	Create(ctx context.Context, f taxfiling.Filing) (taxfiling.Filing, error)
	// Get returns taxfiling.ErrNotFound when the period has no filing. The
	// filing carries its documents.
	Get(ctx context.Context, period string) (taxfiling.Filing, error)
	// List returns every filing, newest period first, with its documents.
	List(ctx context.Context) ([]taxfiling.Filing, error)
	// MarkPaid records the payment of a pending filing and the optional expense
	// movement. It returns taxfiling.ErrNotFound for an unknown period and
	// taxfiling.ErrAlreadyPaid when the payment is already recorded.
	MarkPaid(ctx context.Context, period string, p taxfiling.Payment, expenseID *int) (taxfiling.Filing, error)
	// Delete removes a pending filing, its document rows and clears the
	// declaration_period of its invoices in one transaction. It returns the
	// storage keys of the removed documents, and taxfiling.ErrNotFound for an
	// unknown period and taxfiling.ErrFilingPaid for a paid one.
	Delete(ctx context.Context, period string) (documentKeys []string, err error)
	// PutDocument stores or replaces the document of its kind on a filing,
	// whatever its payment state, and returns the storage key it replaced ("" when
	// there was none). It returns taxfiling.ErrNotFound for an unknown period.
	PutDocument(ctx context.Context, doc taxfiling.Document) (saved taxfiling.Document, replacedKey string, err error)
	// GetDocument returns taxfiling.ErrDocumentMissing when the filing has no
	// document of that kind (or does not exist).
	GetDocument(ctx context.Context, period string, kind taxfiling.DocumentKind) (taxfiling.Document, error)
}
