package app

import (
	"context"
	"errors"
	"io"

	invoices "github.com/valium69mg/finances-app/backend/internal/invoices/domain"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
)

// ErrObjectNotFound is returned by ObjectStore.Get when the key does not exist.
var ErrObjectNotFound = errors.New("object not found")

// ObjectStore is the S3-compatible storage of the issued CFDI files. Keys are
// opaque to the store; the app never exposes the store to clients.
type ObjectStore interface {
	// Put stores size bytes read from r under key.
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	// Get returns the object body, or ErrObjectNotFound.
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	// Delete removes the object. Deleting a missing key is not an error.
	Delete(ctx context.Context, key string) error
}

// ListFilter narrows List. Empty fields match everything.
type ListFilter struct {
	Period string // YYYY-MM
	Status invoices.Status
}

// Repo persists invoices and the metadata of their documents.
type Repo interface {
	// Create stores a prepared invoice and returns it with its ID and creation time.
	Create(ctx context.Context, inv invoices.Invoice) (invoices.Invoice, error)
	// Get returns invoices.ErrNotFound when there is no such invoice.
	Get(ctx context.Context, id int) (invoices.Invoice, error)
	// List returns the invoices newest first (id desc).
	List(ctx context.Context, f ListFilter) ([]invoices.Invoice, error)
	// FindByUUID returns the invoice using the (normalized) UUID, if any.
	FindByUUID(ctx context.Context, uuid string) (invoices.Invoice, bool, error)
	// Issue marks a prepared invoice as issued with its UUID and stores the
	// documents (one per kind) in one transaction. It returns the storage keys
	// of documents that were replaced. It returns invoices.ErrStateChanged when
	// the invoice is no longer prepared and invoices.ErrDuplicateUUID when the
	// UUID belongs to another invoice.
	Issue(ctx context.Context, id int, uuid string, docs []invoices.Document) (replacedKeys []string, err error)
	// Cancel marks a non-cancelled invoice as cancelled, atomically refusing an
	// invoice that a tax filing includes (declaration_period set). It returns
	// invoices.ErrDeclared for such an invoice, invoices.ErrStateChanged when it
	// was already cancelled and invoices.ErrNotFound when it does not exist.
	Cancel(ctx context.Context, id int) error
	// ListDocuments returns the current documents of an invoice, xml first.
	ListDocuments(ctx context.Context, invoiceID int) ([]invoices.Document, error)
	// GetDocument returns invoices.ErrDocumentMissing when the invoice has no such document.
	GetDocument(ctx context.Context, invoiceID, docID int) (invoices.Document, error)
	// PutDocument stores or replaces the document of its kind on an issued
	// invoice and returns the storage key it replaced ("" when there was none).
	// It returns invoices.ErrStateChanged when the invoice is not issued.
	PutDocument(ctx context.Context, doc invoices.Document) (saved invoices.Document, replacedKey string, err error)
}

// Filings is the read side of the tax filing module the invoices use to warn
// when an invoice is issued in an already filed period.
type Filings interface {
	// IsFiled reports whether the YYYY-MM period has a registered tax filing.
	IsFiled(ctx context.Context, period string) (bool, error)
}

// Movements is the part of the ledger the invoices use to validate the optional
// link to an income movement.
type Movements interface {
	GetByID(ctx context.Context, id int) (ledger.Movement, error)
}
