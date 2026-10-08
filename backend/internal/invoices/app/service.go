// Package app holds the invoice use cases: preparing the SAT-portal checklist,
// listing invoices, marking them issued with the stamped CFDI files (or just
// the UUID), attaching documents afterwards, downloading them and cancelling.
// The app never stamps an invoice: the stamp is done in the SAT portal.
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

	invoices "github.com/valium69mg/finances-app/backend/internal/invoices/domain"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
	taxfiling "github.com/valium69mg/finances-app/backend/internal/taxfiling/domain"
)

// ErrStorage wraps every failure of the object store, so callers can tell an
// unavailable storage apart from a rejected input.
var ErrStorage = errors.New("document storage unavailable")

// Settings is the part of the settings module the invoice use cases consume.
type Settings interface {
	Get(ctx context.Context) (settings.Config, error)
}

// PrepareInput is the user input to prepare an invoice. An empty Date is today.
// Periodicity (quincenal or mensual, default mensual) only shapes the checklist
// of a public-in-general invoice and is not stored.
type PrepareInput struct {
	ClientID     string
	Date         string
	Subtotal     *decimal.Decimal
	Amount       *decimal.Decimal
	ExchangeRate *decimal.Decimal
	MovementID   *int
	Periodicity  string
}

// Upload is a file received from the client.
type Upload struct {
	Name        string
	ContentType string
	Data        []byte
}

// IssueInput marks an invoice as issued. XML, when present, is parsed for the
// UUID; otherwise UUID is required. PDF is an optional attachment.
type IssueInput struct {
	UUID string
	XML  *Upload
	PDF  *Upload
}

// Detail is an invoice with its documents and SAT checklist.
type Detail struct {
	Invoice   invoices.Invoice
	Documents []invoices.Document
	Checklist invoices.Checklist
}

// Result is a detail plus the non-blocking warnings of the operation.
type Result struct {
	Detail
	Warnings []invoices.Warning
}

// DocumentResult is a stored document plus the warnings of its upload.
type DocumentResult struct {
	Document invoices.Document
	Warnings []invoices.Warning
}

// Service implements the invoice use cases.
type Service struct {
	repo      Repo
	store     ObjectStore
	movements Movements
	settings  Settings
	filings   Filings
	now       func() time.Time
	logger    *slog.Logger
}

// WithFilings sets the tax filing lookup used to warn when an invoice is issued
// in an already filed period. Without it the warning is never produced.
func (s *Service) WithFilings(f Filings) *Service {
	s.filings = f
	return s
}

// NewService builds a Service. A nil now selects time.Now and a nil logger
// slog.Default().
func NewService(repo Repo, store ObjectStore, movements Movements, settings Settings, now func() time.Time, logger *slog.Logger) *Service {
	if now == nil {
		now = time.Now
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{repo: repo, store: store, movements: movements, settings: settings, now: now, logger: logger}
}

func (s *Service) today() string { return s.now().Format("2006-01-02") }

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", invoices.ErrInvalidInput, fmt.Sprintf(format, args...))
}

// Prepare validates the input, stores a prepared invoice and returns it with
// its checklist and a possible-duplicate warning when the same client already
// has a non-cancelled invoice for the collection date.
func (s *Service) Prepare(ctx context.Context, in PrepareInput) (Result, error) {
	cfg, err := s.settings.Get(ctx)
	if err != nil {
		return Result{}, err
	}
	if in.Date == "" {
		in.Date = s.today()
	}
	if in.MovementID != nil {
		if err := s.checkMovement(ctx, *in.MovementID); err != nil {
			return Result{}, err
		}
	}
	// Existing invoices of the period feed the duplicate check (same date, same period).
	existing, err := s.repo.List(ctx, ListFilter{Period: ledger.MonthOf(in.Date)})
	if err != nil {
		return Result{}, err
	}
	prep, err := invoices.Prepare(cfg, existing, invoices.PrepareInput{
		ClientID: in.ClientID, Date: in.Date, Subtotal: in.Subtotal, Amount: in.Amount, ExchangeRate: in.ExchangeRate,
	})
	if err != nil {
		return Result{}, err
	}
	inv := prep.Invoice
	inv.Amounts = inv.Amounts.Rounded()
	if err := inv.Amounts.Validate(); err != nil {
		return Result{}, err
	}
	inv.MovementID = in.MovementID

	saved, err := s.repo.Create(ctx, inv)
	if err != nil {
		return Result{}, err
	}
	checklist, err := s.checklist(cfg, saved, in.Periodicity)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Detail:   Detail{Invoice: saved, Checklist: checklist},
		Warnings: invoices.DuplicateWarning(prep.Duplicates),
	}, nil
}

func (s *Service) checkMovement(ctx context.Context, id int) error {
	m, err := s.movements.GetByID(ctx, id)
	if errors.Is(err, ledger.ErrNotFound) {
		return invalid("movement %d does not exist", id)
	}
	if err != nil {
		return err
	}
	if m.Kind != ledger.KindIncome {
		return invalid("movement %d is not an %s", id, ledger.KindIncome)
	}
	return nil
}

func (s *Service) checklist(cfg settings.Config, inv invoices.Invoice, periodicity string) (invoices.Checklist, error) {
	due, err := taxfiling.DueDate(inv.Period)
	if err != nil {
		return invoices.Checklist{}, err
	}
	return invoices.BuildChecklist(cfg, inv, periodicity, due)
}

// List returns the invoices, newest first, optionally filtered by period
// (YYYY-MM) and status.
func (s *Service) List(ctx context.Context, period string, status invoices.Status) ([]invoices.Invoice, error) {
	if period != "" && !ledger.IsMonth(period) {
		return nil, invalid("invalid period %q, use YYYY-MM", period)
	}
	switch status {
	case "", invoices.StatusPrepared, invoices.StatusIssued, invoices.StatusCancelled:
	default:
		return nil, invalid("invalid state %q", status)
	}
	return s.repo.List(ctx, ListFilter{Period: period, Status: status})
}

// Get returns an invoice with its documents and checklist.
func (s *Service) Get(ctx context.Context, id int, periodicity string) (Detail, error) {
	inv, err := s.repo.Get(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	return s.detail(ctx, inv, periodicity)
}

func (s *Service) detail(ctx context.Context, inv invoices.Invoice, periodicity string) (Detail, error) {
	cfg, err := s.settings.Get(ctx)
	if err != nil {
		return Detail{}, err
	}
	docs, err := s.repo.ListDocuments(ctx, inv.ID)
	if err != nil {
		return Detail{}, err
	}
	checklist, err := s.checklist(cfg, inv, periodicity)
	if err != nil {
		return Detail{}, err
	}
	return Detail{Invoice: inv, Documents: docs, Checklist: checklist}, nil
}

// Issue marks a prepared invoice as issued. The UUID comes from the XML when
// there is one (a manual UUID must then match it) or from the manual value. The
// stamped XML is the source of truth: when its currency is the invoice currency
// its amounts replace the prepared ones and a warning lists what changed; with
// another currency the differences are warnings and nothing is replaced.
// A UUID used by another invoice is rejected. The files are stored before the
// database is updated and removed again if the update fails.
func (s *Service) Issue(ctx context.Context, id int, in IssueInput) (Result, error) {
	inv, err := s.repo.Get(ctx, id)
	if err != nil {
		return Result{}, err
	}
	switch inv.Status {
	case invoices.StatusCancelled:
		return Result{}, fmt.Errorf("%w: #%d", invoices.ErrCancelled, id)
	case invoices.StatusIssued:
		return Result{}, fmt.Errorf("%w: #%d", invoices.ErrAlreadyIssued, id)
	}
	if in.XML == nil && in.UUID == "" {
		return Result{}, invoices.ErrIssueInput
	}

	var (
		uuid     string
		warnings []invoices.Warning
		amounts  *invoices.Amounts
		docs     []invoices.Document
		payloads = map[invoices.DocumentKind][]byte{}
	)
	if in.UUID != "" {
		if uuid, err = invoices.NormalizeUUID(in.UUID); err != nil {
			return Result{}, err
		}
	}
	if in.XML != nil {
		doc, cfdi, err := s.parseXML(in.XML)
		if err != nil {
			return Result{}, err
		}
		if uuid != "" && uuid != cfdi.UUID {
			return Result{}, fmt.Errorf("%w: the manual UUID differs from the XML", invoices.ErrUUIDMismatch)
		}
		uuid = cfdi.UUID
		if warnings, amounts, err = s.reconcile(inv, cfdi); err != nil {
			return Result{}, err
		}
		docs = append(docs, doc)
		payloads[invoices.DocumentXML] = in.XML.Data
	}
	if in.PDF != nil {
		doc, err := invoices.NewDocument(invoices.DocumentPDF, in.PDF.Name, in.PDF.ContentType, in.PDF.Data)
		if err != nil {
			return Result{}, err
		}
		docs = append(docs, doc)
		payloads[invoices.DocumentPDF] = in.PDF.Data
	}

	if other, found, err := s.repo.FindByUUID(ctx, uuid); err != nil {
		return Result{}, err
	} else if found && other.ID != id {
		return Result{}, fmt.Errorf("%w: invoice #%d", invoices.ErrDuplicateUUID, other.ID)
	}

	stored, err := s.putAll(ctx, id, docs, payloads)
	if err != nil {
		return Result{}, err
	}
	replaced, err := s.repo.Issue(ctx, id, uuid, stored, amounts)
	if err != nil {
		s.deleteKeys(ctx, keysOf(stored))
		return Result{}, err
	}
	s.deleteKeys(ctx, replaced)
	warnings = append(warnings, s.filedPeriodWarning(ctx, inv.Period)...)

	updated, err := s.repo.Get(ctx, id)
	if err != nil {
		return Result{}, err
	}
	detail, err := s.detail(ctx, updated, "")
	if err != nil {
		return Result{}, err
	}
	return Result{Detail: detail, Warnings: warnings}, nil
}

// reconcile compares the invoice with its XML. Same currency: the XML amounts
// and the warning listing the changes. Other currency: the mismatch warnings
// and no amounts.
func (s *Service) reconcile(inv invoices.Invoice, cfdi invoices.CFDI) ([]invoices.Warning, *invoices.Amounts, error) {
	amounts, warnings, ok, err := inv.AmountsFromCFDI(cfdi)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return inv.CompareCFDI(cfdi), nil, nil
	}
	return warnings, &amounts, nil
}

// Resync re-reads the stored XML of an issued invoice and replaces the invoice
// amounts with the ones in it. The XML UUID and currency must be the invoice's.
// A declared invoice is refused (invoices.ErrDeclared): the saved declaration
// would go out of sync; the repository re-checks this atomically in the UPDATE.
func (s *Service) Resync(ctx context.Context, id int) (Result, error) {
	inv, err := s.repo.Get(ctx, id)
	if err != nil {
		return Result{}, err
	}
	switch inv.Status {
	case invoices.StatusCancelled:
		return Result{}, fmt.Errorf("%w: #%d", invoices.ErrCancelled, id)
	case invoices.StatusPrepared:
		return Result{}, fmt.Errorf("%w: #%d", invoices.ErrNotIssued, id)
	}
	if inv.DeclarationPeriod != "" {
		return Result{}, fmt.Errorf("%w: #%d is part of the filing of %s, delete that filing first",
			invoices.ErrDeclared, id, inv.DeclarationPeriod)
	}
	docs, err := s.repo.ListDocuments(ctx, id)
	if err != nil {
		return Result{}, err
	}
	var xmlDoc *invoices.Document
	for i := range docs {
		if docs[i].Kind == invoices.DocumentXML {
			xmlDoc = &docs[i]
			break
		}
	}
	if xmlDoc == nil {
		return Result{}, fmt.Errorf("%w: #%d", invoices.ErrNoXML, id)
	}
	data, err := s.readObject(ctx, xmlDoc.Key, invoices.MaxXMLBytes)
	if err != nil {
		return Result{}, err
	}
	cfdi, err := invoices.ParseCFDI(data)
	if err != nil {
		return Result{}, err
	}
	if cfdi.UUID != inv.UUID {
		return Result{}, fmt.Errorf("%w: the XML has %s, the invoice %s", invoices.ErrUUIDMismatch, cfdi.UUID, inv.UUID)
	}
	amounts, warnings, ok, err := inv.AmountsFromCFDI(cfdi)
	if err != nil {
		return Result{}, err
	}
	if !ok {
		return Result{}, fmt.Errorf("%w: the XML has %s, the invoice %s", invoices.ErrCurrencyMismatch, cfdi.Currency, inv.Currency)
	}
	if err := s.repo.SyncAmounts(ctx, id, amounts); err != nil {
		return Result{}, err
	}
	updated, err := s.repo.Get(ctx, id)
	if err != nil {
		return Result{}, err
	}
	detail, err := s.detail(ctx, updated, "")
	if err != nil {
		return Result{}, err
	}
	return Result{Detail: detail, Warnings: warnings}, nil
}

// readObject reads a stored object of at most max bytes (one more is read so an
// oversized object is noticed by the parser). A missing object is
// invoices.ErrDocumentMissing, any other failure wraps ErrStorage.
func (s *Service) readObject(ctx context.Context, key string, max int64) ([]byte, error) {
	body, err := s.store.Get(ctx, key)
	if errors.Is(err, ErrObjectNotFound) {
		s.logger.Error("stored invoice document is missing from the object store", "key", key)
		return nil, invoices.ErrDocumentMissing
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStorage, err)
	}
	defer body.Close()
	data, err := io.ReadAll(io.LimitReader(body, max+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStorage, err)
	}
	return data, nil
}

// filedPeriodWarning warns when the period of a just-issued invoice already has
// a registered filing: the invoice is never linked to it and its income stays
// undeclared. The invoice is already issued, so a failed lookup is only logged.
func (s *Service) filedPeriodWarning(ctx context.Context, period string) []invoices.Warning {
	if s.filings == nil {
		return nil
	}
	filed, err := s.filings.IsFiled(ctx, period)
	if err != nil {
		s.logger.Warn("invoice issued but its period could not be checked against the tax filings", "period", period, "error", err)
		return nil
	}
	if !filed {
		return nil
	}
	return []invoices.Warning{invoices.PeriodAlreadyFiledWarning(period)}
}

// AttachDocument stores (or replaces) the XML or PDF of an issued invoice. An
// XML must carry the UUID of the invoice; its total and currency are compared
// with the invoice as warnings.
func (s *Service) AttachDocument(ctx context.Context, id int, kind invoices.DocumentKind, up Upload) (DocumentResult, error) {
	inv, err := s.repo.Get(ctx, id)
	if err != nil {
		return DocumentResult{}, err
	}
	switch inv.Status {
	case invoices.StatusCancelled:
		return DocumentResult{}, fmt.Errorf("%w: #%d", invoices.ErrCancelled, id)
	case invoices.StatusPrepared:
		return DocumentResult{}, fmt.Errorf("%w: #%d", invoices.ErrNotIssued, id)
	}

	var (
		doc      invoices.Document
		warnings []invoices.Warning
	)
	switch kind {
	case invoices.DocumentXML:
		var cfdi invoices.CFDI
		if doc, cfdi, err = s.parseXML(&up); err != nil {
			return DocumentResult{}, err
		}
		if cfdi.UUID != inv.UUID {
			return DocumentResult{}, fmt.Errorf("%w: the XML has %s, the invoice %s", invoices.ErrUUIDMismatch, cfdi.UUID, inv.UUID)
		}
		warnings = inv.CompareCFDI(cfdi)
	case invoices.DocumentPDF:
		if doc, err = invoices.NewDocument(kind, up.Name, up.ContentType, up.Data); err != nil {
			return DocumentResult{}, err
		}
	default:
		return DocumentResult{}, fmt.Errorf("%w: unknown kind %q", invoices.ErrInvalidDocument, kind)
	}

	stored, err := s.putAll(ctx, id, []invoices.Document{doc}, map[invoices.DocumentKind][]byte{kind: up.Data})
	if err != nil {
		return DocumentResult{}, err
	}
	saved, replacedKey, err := s.repo.PutDocument(ctx, stored[0])
	if err != nil {
		s.deleteKeys(ctx, keysOf(stored))
		return DocumentResult{}, err
	}
	if replacedKey != "" {
		s.deleteKeys(ctx, []string{replacedKey})
	}
	return DocumentResult{Document: saved, Warnings: warnings}, nil
}

// Download returns the metadata and a reader of a stored document. The caller
// closes the reader. A document whose object is missing from the store is
// reported as invoices.ErrDocumentMissing.
func (s *Service) Download(ctx context.Context, invoiceID, docID int) (invoices.Document, io.ReadCloser, error) {
	doc, err := s.repo.GetDocument(ctx, invoiceID, docID)
	if err != nil {
		return invoices.Document{}, nil, err
	}
	body, err := s.store.Get(ctx, doc.Key)
	if errors.Is(err, ErrObjectNotFound) {
		s.logger.Error("stored invoice document is missing from the object store", "invoice", invoiceID, "document", docID)
		return invoices.Document{}, nil, invoices.ErrDocumentMissing
	}
	if err != nil {
		return invoices.Document{}, nil, fmt.Errorf("%w: %v", ErrStorage, err)
	}
	return doc, body, nil
}

// Cancel marks the invoice as cancelled. Cancelled is terminal: cancelling a
// cancelled invoice is invoices.ErrCancelled. An invoice that a tax filing
// includes cannot be cancelled (invoices.ErrDeclared): the saved declaration
// would silently go out of sync, so the filing must be deleted first. The
// repository re-checks this atomically in the UPDATE. The stored documents are kept.
func (s *Service) Cancel(ctx context.Context, id int) (invoices.Invoice, error) {
	inv, err := s.repo.Get(ctx, id)
	if err != nil {
		return invoices.Invoice{}, err
	}
	if inv.Status == invoices.StatusCancelled {
		return invoices.Invoice{}, fmt.Errorf("%w: #%d", invoices.ErrCancelled, id)
	}
	if inv.DeclarationPeriod != "" {
		return invoices.Invoice{}, fmt.Errorf("%w: #%d is part of the filing of %s, delete that filing first",
			invoices.ErrDeclared, id, inv.DeclarationPeriod)
	}
	if err := s.repo.Cancel(ctx, id); err != nil {
		return invoices.Invoice{}, err
	}
	return s.repo.Get(ctx, id)
}

// parseXML validates an XML upload as a document and parses the CFDI from it.
func (s *Service) parseXML(up *Upload) (invoices.Document, invoices.CFDI, error) {
	doc, err := invoices.NewDocument(invoices.DocumentXML, up.Name, up.ContentType, up.Data)
	if err != nil {
		return invoices.Document{}, invoices.CFDI{}, err
	}
	cfdi, err := invoices.ParseCFDI(up.Data)
	if err != nil {
		return invoices.Document{}, invoices.CFDI{}, err
	}
	return doc, cfdi, nil
}

// putAll uploads every document to the store under a fresh random key and
// returns the documents with their keys and invoice ID. On failure it removes
// what it already uploaded.
func (s *Service) putAll(ctx context.Context, invoiceID int, docs []invoices.Document, payloads map[invoices.DocumentKind][]byte) ([]invoices.Document, error) {
	out := make([]invoices.Document, 0, len(docs))
	for _, doc := range docs {
		suffix := make([]byte, 16)
		if _, err := rand.Read(suffix); err != nil {
			s.deleteKeys(ctx, keysOf(out))
			return nil, err
		}
		doc.InvoiceID = invoiceID
		doc.Key = fmt.Sprintf("invoices/%d/%s-%s%s", invoiceID, doc.Kind, hex.EncodeToString(suffix), doc.Kind.Extension())
		data := payloads[doc.Kind]
		if err := s.store.Put(ctx, doc.Key, bytes.NewReader(data), int64(len(data)), doc.ContentType); err != nil {
			s.deleteKeys(ctx, keysOf(out))
			return nil, fmt.Errorf("%w: %v", ErrStorage, err)
		}
		out = append(out, doc)
	}
	return out, nil
}

func keysOf(docs []invoices.Document) []string {
	keys := make([]string, len(docs))
	for i, d := range docs {
		keys[i] = d.Key
	}
	return keys
}

// deleteKeys removes objects on a best-effort basis: a failure leaves an
// orphan object that is logged, never an error for the caller.
func (s *Service) deleteKeys(ctx context.Context, keys []string) {
	for _, key := range keys {
		if err := s.store.Delete(context.WithoutCancel(ctx), key); err != nil {
			s.logger.Error("could not delete an invoice object", "key", key, "error", err)
		}
	}
}
