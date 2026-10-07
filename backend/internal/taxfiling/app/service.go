// Package app holds the tax filing use cases: the read-only preview of the
// monthly RESICO declaration, registering the filing of a period, recording its
// payment (optionally as an Impuestos expense), the history and the periods
// still to file. The app never files anything with the SAT: the user files in
// the SAT portal and registers the result here.
package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/shopspring/decimal"

	expensesapp "github.com/valium69mg/finances-app/backend/internal/expenses/app"
	invoices "github.com/valium69mg/finances-app/backend/internal/invoices/domain"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	taxfiling "github.com/valium69mg/finances-app/backend/internal/taxfiling/domain"
)

// Warning codes of a preview or a registration.
const (
	WarningPreparedInvoices = "prepared_invoices"
	WarningAlreadyFiled     = "already_filed"
)

// ErrStorage wraps every failure of the object store, so callers can tell an
// unavailable storage apart from a rejected input.
var ErrStorage = errors.New("document storage unavailable")

const paymentMethodTransfer = "Transferencia"

// Warning is a non-blocking notice about the period.
type Warning struct {
	Code       string
	Message    string
	InvoiceIDs []int
}

// Preview is the computed declaration of a period. Nothing is stored. Filing is
// set when the period is already filed.
type Preview struct {
	Declaration taxfiling.Declaration
	Filing      *taxfiling.Filing
	Warnings    []Warning
}

// PaymentInput is the payment of a filing: the amounts paid to the SAT and,
// only when RecordExpense is true, the request to record them as an expense.
// An empty Date is the filing date when registering and today when paying later.
type PaymentInput struct {
	Date          string
	ISRPaid       decimal.Decimal
	IVAPaid       decimal.Decimal
	RecordExpense bool
}

// RegisterInput is the data of a filing to register. IVACreditable is the
// optional creditable IVA used in the computation. Payment is nil when the
// payment is still pending. An empty Date is today.
type RegisterInput struct {
	Period        string
	Date          string
	Folio         string
	IVACreditable decimal.Decimal
	Payment       *PaymentInput
}

// Result is a filing with the expense recorded for its payment (nil when none)
// and the warnings of the operation.
type Result struct {
	Filing   taxfiling.Filing
	Expense  *ledger.Movement
	Warnings []Warning
}

// Detail is a filing with the invoices it includes.
type Detail struct {
	Filing   taxfiling.Filing
	Invoices []invoices.Invoice
}

// Service implements the tax filing use cases.
type Service struct {
	repo     Repo
	invoices Invoices
	expenses Expenses
	settings Settings
	store    ObjectStore
	now      func() time.Time
	logger   *slog.Logger
}

// NewService builds a Service. store keeps the attached files (the invoices
// store is shared). A nil now selects time.Now and a nil logger slog.Default().
func NewService(repo Repo, invs Invoices, expenses Expenses, settings Settings, store ObjectStore, now func() time.Time, logger *slog.Logger) *Service {
	if now == nil {
		now = time.Now
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{repo: repo, invoices: invs, expenses: expenses, settings: settings, store: store, now: now, logger: logger}
}

func (s *Service) today() string { return s.now().Format("2006-01-02") }

func (s *Service) existing(ctx context.Context, period string) (*taxfiling.Filing, error) {
	f, err := s.repo.Get(ctx, period)
	if errors.Is(err, taxfiling.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func preparedWarning(ids []int) []Warning {
	if len(ids) == 0 {
		return nil
	}
	return []Warning{{
		Code:       WarningPreparedInvoices,
		Message:    "the period includes invoices still in state preparada; they are not part of the declaration until they are issued",
		InvoiceIDs: ids,
	}}
}

// Preview computes the declaration of a YYYY-MM period (the previous month when
// empty) from its issued invoices, without storing anything. It warns about
// prepared invoices of the period and about a period that is already filed.
func (s *Service) Preview(ctx context.Context, period string, ivaCreditable decimal.Decimal) (Preview, error) {
	if period == "" {
		prev, err := ledger.PreviousMonth(ledger.CurrentMonth(s.now()))
		if err != nil {
			return Preview{}, err
		}
		period = prev
	}
	if err := taxfiling.ValidatePeriod(period); err != nil {
		return Preview{}, err
	}
	cfg, err := s.settings.Get(ctx)
	if err != nil {
		return Preview{}, err
	}
	list, err := s.invoices.List(ctx, period, "")
	if err != nil {
		return Preview{}, err
	}
	decl, err := taxfiling.ComputeDeclaration(cfg, list, period, ivaCreditable)
	if err != nil {
		return Preview{}, err
	}
	filed, err := s.existing(ctx, period)
	if err != nil {
		return Preview{}, err
	}
	out := Preview{Declaration: decl, Filing: filed, Warnings: preparedWarning(decl.PreparedIDs)}
	if filed != nil {
		out.Warnings = append(out.Warnings, Warning{
			Code:    WarningAlreadyFiled,
			Message: fmt.Sprintf("the period %s was already filed on %s", period, filed.FilingDate),
		})
	}
	return out, nil
}

// Register stores the filing of a period computed from its issued invoices and
// links those invoices to it. The payment is optional; when it comes with
// RecordExpense the payment is also recorded as an Impuestos expense, linked to
// the filing. The expense is only created when asked for.
func (s *Service) Register(ctx context.Context, in RegisterInput) (Result, error) {
	if err := taxfiling.ValidatePeriod(in.Period); err != nil {
		return Result{}, err
	}
	if in.Date == "" {
		in.Date = s.today()
	}
	cfg, err := s.settings.Get(ctx)
	if err != nil {
		return Result{}, err
	}
	list, err := s.invoices.List(ctx, in.Period, "")
	if err != nil {
		return Result{}, err
	}
	var filings []taxfiling.Filing
	filed, err := s.existing(ctx, in.Period)
	if err != nil {
		return Result{}, err
	}
	if filed != nil {
		filings = append(filings, *filed)
	}

	domainIn := taxfiling.FilingInput{Period: in.Period, Date: in.Date, Folio: in.Folio, IVACreditable: in.IVACreditable}
	if in.Payment != nil {
		domainIn.Payment = &taxfiling.PaymentInput{Date: in.Payment.Date, ISRPaid: in.Payment.ISRPaid, IVAPaid: in.Payment.IVAPaid}
		if domainIn.Payment.Date == "" {
			domainIn.Payment.Date = in.Date
		}
	}
	filing, err := taxfiling.NewFiling(cfg, list, filings, domainIn)
	if err != nil {
		return Result{}, err
	}
	decl, err := taxfiling.ComputeDeclaration(cfg, list, in.Period, filing.IVACreditable)
	if err != nil {
		return Result{}, err
	}

	var expense *ledger.Movement
	if in.Payment != nil && in.Payment.RecordExpense {
		if expense, err = s.recordExpense(ctx, in.Period, *filing.Payment); err != nil {
			return Result{}, err
		}
		filing.ExpenseMovementID = &expense.ID
	}
	saved, err := s.repo.Create(ctx, filing)
	if err != nil {
		s.rollbackExpense(ctx, expense)
		return Result{}, err
	}
	return Result{Filing: saved, Expense: expense, Warnings: preparedWarning(decl.PreparedIDs)}, nil
}

// Pay records the payment of a pending filing (amounts and date; an empty date
// is today). With RecordExpense it also records the total as an Impuestos
// expense on the payment date and links it to the filing; the expense is never
// created silently. A paid filing cannot be paid again.
func (s *Service) Pay(ctx context.Context, period string, in PaymentInput) (Result, error) {
	if err := taxfiling.ValidatePeriod(period); err != nil {
		return Result{}, err
	}
	if in.Date == "" {
		in.Date = s.today()
	}
	filing, err := s.repo.Get(ctx, period)
	if err != nil {
		return Result{}, err
	}
	paid, err := filing.Pay(taxfiling.PaymentInput{Date: in.Date, ISRPaid: in.ISRPaid, IVAPaid: in.IVAPaid})
	if err != nil {
		return Result{}, err
	}

	var expense *ledger.Movement
	var expenseID *int
	if in.RecordExpense {
		if expense, err = s.recordExpense(ctx, period, *paid.Payment); err != nil {
			return Result{}, err
		}
		expenseID = &expense.ID
	}
	saved, err := s.repo.MarkPaid(ctx, period, *paid.Payment, expenseID)
	if err != nil {
		s.rollbackExpense(ctx, expense)
		return Result{}, err
	}
	return Result{Filing: saved, Expense: expense}, nil
}

// recordExpense creates the Impuestos expense of a payment through the
// expenses module. An expense must be greater than zero.
func (s *Service) recordExpense(ctx context.Context, period string, p taxfiling.Payment) (*ledger.Movement, error) {
	total := p.Total()
	if !total.IsPositive() {
		return nil, fmt.Errorf("%w: the paid amounts are zero, there is no expense to record", taxfiling.ErrInvalidInput)
	}
	res, err := s.expenses.Create(ctx, expensesapp.Input{
		Date:          p.Date,
		Description:   taxfiling.PaymentExpenseDescription(period),
		Category:      ledger.CategoryTaxes,
		PaymentMethod: paymentMethodTransfer,
		Currency:      ledger.CurrencyMXN,
		Amount:        total,
	})
	if err != nil {
		return nil, err
	}
	m := res.Movement
	return &m, nil
}

// rollbackExpense removes an expense created for a filing operation that then
// failed, so no orphan movement is left behind. A failure is only logged.
func (s *Service) rollbackExpense(ctx context.Context, m *ledger.Movement) {
	if m == nil {
		return
	}
	if err := s.expenses.Delete(context.WithoutCancel(ctx), m.ID); err != nil {
		s.logger.Error("could not remove the expense of a failed tax filing operation", "movement", m.ID, "error", err)
	}
}

// Get returns the filing of a period with the invoices it includes.
func (s *Service) Get(ctx context.Context, period string) (Detail, error) {
	if err := taxfiling.ValidatePeriod(period); err != nil {
		return Detail{}, err
	}
	filing, err := s.repo.Get(ctx, period)
	if err != nil {
		return Detail{}, err
	}
	list, err := s.invoices.List(ctx, period, "")
	if err != nil {
		return Detail{}, err
	}
	linked := make([]invoices.Invoice, 0, len(filing.InvoiceIDs))
	for i := len(list) - 1; i >= 0; i-- { // List is newest first; show oldest first
		if list[i].DeclarationPeriod == period {
			linked = append(linked, list[i])
		}
	}
	return Detail{Filing: filing, Invoices: linked}, nil
}

// List returns the filings, newest period first, optionally of one year (0 is
// every year) and one payment status (empty is every status).
func (s *Service) List(ctx context.Context, year int, status taxfiling.PaymentStatus) ([]taxfiling.Filing, error) {
	if year < 0 || year > 9999 {
		return nil, fmt.Errorf("%w: invalid year %d", taxfiling.ErrInvalidInput, year)
	}
	if status != "" && !status.IsValid() {
		return nil, fmt.Errorf("%w: invalid status %q", taxfiling.ErrInvalidInput, status)
	}
	all, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	prefix := ""
	if year > 0 {
		prefix = fmt.Sprintf("%04d-", year)
	}
	out := make([]taxfiling.Filing, 0, len(all))
	for _, f := range all {
		if prefix != "" && f.Period[:len(prefix)] != prefix {
			continue
		}
		if status != "" && f.PaymentStatus() != status {
			continue
		}
		out = append(out, f)
	}
	return out, nil
}

// Pending lists the periods with issued invoices and no filing, oldest first.
func (s *Service) Pending(ctx context.Context) ([]taxfiling.PendingPeriod, error) {
	issued, err := s.invoices.List(ctx, "", invoices.StatusIssued)
	if err != nil {
		return nil, err
	}
	filings, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	return taxfiling.PendingPeriods(issued, filings, s.today())
}

// Delete removes a pending filing, so a wrong registration can be redone. A paid
// filing cannot be deleted (taxfiling.ErrFilingPaid): money moved and an expense
// may be linked to it. The attached files are removed with it; deleting their
// objects is best effort (a failure is logged, never returned).
func (s *Service) Delete(ctx context.Context, period string) error {
	if err := taxfiling.ValidatePeriod(period); err != nil {
		return err
	}
	keys, err := s.repo.Delete(ctx, period)
	if err != nil {
		return err
	}
	s.deleteKeys(ctx, keys)
	return nil
}

// Upload is a file received from the client.
type Upload struct {
	Name        string
	ContentType string
	Data        []byte
}

// AttachDocument stores (or replaces) the acuse or the comprobante of a filing
// and returns the updated filing. Documents change no figure, so they are
// accepted on pending and on paid filings alike. The new object is stored
// first, then the row is swapped, then the replaced object is removed (best
// effort); if the row write fails the new object is removed again.
func (s *Service) AttachDocument(ctx context.Context, period string, kind taxfiling.DocumentKind, up Upload) (taxfiling.Filing, error) {
	if err := taxfiling.ValidatePeriod(period); err != nil {
		return taxfiling.Filing{}, err
	}
	doc, err := taxfiling.NewDocument(kind, up.Name, up.ContentType, up.Data)
	if err != nil {
		return taxfiling.Filing{}, err
	}
	if _, err := s.repo.Get(ctx, period); err != nil {
		return taxfiling.Filing{}, err
	}
	suffix := make([]byte, 16)
	if _, err := rand.Read(suffix); err != nil {
		return taxfiling.Filing{}, err
	}
	doc.Period = period
	doc.Key = fmt.Sprintf("tax-filings/%s/%s/%s%s", period, kind, hex.EncodeToString(suffix), taxfiling.KeyExtension(doc.ContentType))
	if err := s.store.Put(ctx, doc.Key, bytes.NewReader(up.Data), doc.Size, doc.ContentType); err != nil {
		return taxfiling.Filing{}, fmt.Errorf("%w: %v", ErrStorage, err)
	}
	_, replacedKey, err := s.repo.PutDocument(ctx, doc)
	if err != nil {
		s.deleteKeys(ctx, []string{doc.Key})
		return taxfiling.Filing{}, err
	}
	if replacedKey != "" {
		s.deleteKeys(ctx, []string{replacedKey})
	}
	return s.repo.Get(ctx, period)
}

// Download returns the metadata and a reader of a stored document. The caller
// closes the reader. A document whose object is missing from the store is
// reported as taxfiling.ErrDocumentMissing.
func (s *Service) Download(ctx context.Context, period string, kind taxfiling.DocumentKind) (taxfiling.Document, io.ReadCloser, error) {
	if err := taxfiling.ValidatePeriod(period); err != nil {
		return taxfiling.Document{}, nil, err
	}
	if !kind.IsValid() {
		return taxfiling.Document{}, nil, fmt.Errorf("%w: unknown kind %q", taxfiling.ErrInvalidDocument, kind)
	}
	doc, err := s.repo.GetDocument(ctx, period, kind)
	if err != nil {
		return taxfiling.Document{}, nil, err
	}
	body, err := s.store.Get(ctx, doc.Key)
	if errors.Is(err, ErrObjectNotFound) {
		s.logger.Error("stored tax filing document is missing from the object store", "period", period, "kind", kind)
		return taxfiling.Document{}, nil, taxfiling.ErrDocumentMissing
	}
	if err != nil {
		return taxfiling.Document{}, nil, fmt.Errorf("%w: %v", ErrStorage, err)
	}
	return doc, body, nil
}

// deleteKeys removes objects on a best-effort basis: a failure leaves an
// orphan object that is logged, never an error for the caller.
func (s *Service) deleteKeys(ctx context.Context, keys []string) {
	for _, key := range keys {
		if err := s.store.Delete(context.WithoutCancel(ctx), key); err != nil {
			s.logger.Error("could not delete a tax filing object", "key", key, "error", err)
		}
	}
}

// MonthStatus is what the dashboard shows about the filings around a YYYY-MM
// month: the payment state of that period and whether the previous one needs action.
func (s *Service) MonthStatus(ctx context.Context, month string) (taxfiling.MonthStatus, error) {
	if err := taxfiling.ValidatePeriod(month); err != nil {
		return taxfiling.MonthStatus{}, err
	}
	prev, err := ledger.PreviousMonth(month)
	if err != nil {
		return taxfiling.MonthStatus{}, err
	}
	// The dashboard calls this on every load, so read only what StatusOf uses:
	// the filings of the month and of the previous period, and the previous
	// period's issued invoices, and only when that period is not filed yet.
	var filings []taxfiling.Filing
	for _, p := range []string{month, prev} {
		f, err := s.existing(ctx, p)
		if err != nil {
			return taxfiling.MonthStatus{}, err
		}
		if f != nil {
			filings = append(filings, *f)
		}
	}
	var issued []invoices.Invoice
	if _, filed := taxfiling.FindFiling(filings, prev); !filed {
		if issued, err = s.invoices.List(ctx, prev, invoices.StatusIssued); err != nil {
			return taxfiling.MonthStatus{}, err
		}
	}
	return taxfiling.StatusOf(issued, filings, month)
}

// IsFiled reports whether a YYYY-MM period has a registered filing.
func (s *Service) IsFiled(ctx context.Context, period string) (bool, error) {
	if err := taxfiling.ValidatePeriod(period); err != nil {
		return false, err
	}
	f, err := s.existing(ctx, period)
	return f != nil, err
}

// UnfiledInvoices lists the issued invoices that no filing includes although
// their period is already filed (see taxfiling.UnfiledInvoices).
func (s *Service) UnfiledInvoices(ctx context.Context) ([]invoices.Invoice, error) {
	issued, err := s.invoices.List(ctx, "", invoices.StatusIssued)
	if err != nil {
		return nil, err
	}
	filings, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	return taxfiling.UnfiledInvoices(issued, filings), nil
}
