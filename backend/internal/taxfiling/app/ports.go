package app

import (
	"context"

	expensesapp "github.com/valium69mg/finances-app/backend/internal/expenses/app"
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

// Repo persists the filings of the tax_filings table together with the
// declaration_period link on the invoices they include.
type Repo interface {
	// Create stores the filing and, in the same transaction, links every issued
	// invoice of its period to it. It returns taxfiling.ErrAlreadyFiled when the
	// period is filed and taxfiling.ErrInvoicesChanged when the issued invoices
	// of the period are not exactly f.InvoiceIDs.
	Create(ctx context.Context, f taxfiling.Filing) (taxfiling.Filing, error)
	// Get returns taxfiling.ErrNotFound when the period has no filing.
	Get(ctx context.Context, period string) (taxfiling.Filing, error)
	// List returns every filing, newest period first.
	List(ctx context.Context) ([]taxfiling.Filing, error)
	// MarkPaid records the payment of a pending filing and the optional expense
	// movement. It returns taxfiling.ErrNotFound for an unknown period and
	// taxfiling.ErrAlreadyPaid when the payment is already recorded.
	MarkPaid(ctx context.Context, period string, p taxfiling.Payment, expenseID *int) (taxfiling.Filing, error)
	// Delete removes a pending filing and clears the declaration_period of its
	// invoices in one transaction. It returns taxfiling.ErrNotFound for an
	// unknown period and taxfiling.ErrFilingPaid for a paid one.
	Delete(ctx context.Context, period string) error
}
