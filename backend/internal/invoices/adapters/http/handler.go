// Package invoiceshttp exposes the invoice use cases over HTTP. Every route
// sits behind the authentication middleware passed to Register. Money and
// rates travel as decimal strings.
//
// Documents are downloaded through GET /invoices/{id}/documents/{docId}: the
// backend streams the object from the private bucket after authentication. It
// never hands out presigned URLs or bucket keys, so the object store is not
// reachable by clients and nothing can be replayed after logout.
package invoiceshttp

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"

	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/invoices/app"
	invoices "github.com/valium69mg/finances-app/backend/internal/invoices/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/httpjson"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

// maxMultipartBytes bounds an upload request: both files at their limit plus
// the small text fields and multipart framing.
const maxMultipartBytes = invoices.MaxXMLBytes + invoices.MaxPDFBytes + 1<<20

// Service is the set of use cases the handlers need.
type Service interface {
	Prepare(ctx context.Context, in app.PrepareInput) (app.Result, error)
	List(ctx context.Context, period string, status invoices.Status) ([]invoices.Invoice, error)
	Get(ctx context.Context, id int, periodicity string) (app.Detail, error)
	Issue(ctx context.Context, id int, in app.IssueInput) (app.Result, error)
	AttachDocument(ctx context.Context, id int, kind invoices.DocumentKind, up app.Upload) (app.DocumentResult, error)
	Download(ctx context.Context, invoiceID, docID int) (invoices.Document, io.ReadCloser, error)
	Cancel(ctx context.Context, id int) (invoices.Invoice, error)
}

// Handler serves the /invoices routes.
type Handler struct {
	svc    Service
	logger *slog.Logger
}

// New builds a Handler. A nil logger selects slog.Default().
func New(svc Service, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{svc: svc, logger: logger}
}

// Register mounts the invoice routes on mux, each wrapped by requireAuth.
func (h *Handler) Register(mux *http.ServeMux, requireAuth func(http.Handler) http.Handler) {
	route := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, requireAuth(fn))
	}
	route("POST /invoices", h.prepare)
	route("GET /invoices", h.list)
	route("GET /invoices/{id}", h.get)
	route("POST /invoices/{id}/issue", h.issue)
	route("POST /invoices/{id}/documents", h.attach)
	route("GET /invoices/{id}/documents/{docId}", h.download)
	route("POST /invoices/{id}/cancel", h.cancel)
}

// --- DTOs ---------------------------------------------------------------

// prepareRequest is the body of POST /invoices. Client USA takes subtotal (USD,
// default salary_usd) and exchange_rate (default fx_rate_applied); every other
// client takes amount, the total received with IVA included. date defaults to
// today; periodicity (quincenal or mensual, default mensual) shapes the
// checklist of a public-in-general invoice.
type prepareRequest struct {
	ClientID     string           `json:"client_id"`
	Date         string           `json:"date"`
	Subtotal     *decimal.Decimal `json:"subtotal"`
	Amount       *decimal.Decimal `json:"amount"`
	ExchangeRate *decimal.Decimal `json:"exchange_rate"`
	MovementID   *int             `json:"movement_id"`
	Periodicity  string           `json:"periodicity"`
}

type invoiceDTO struct {
	ID                 int              `json:"id"`
	ClientID           string           `json:"client_id"`
	CollectionDate     string           `json:"collection_date"`
	Period             string           `json:"period"`
	Currency           string           `json:"currency"`
	ExchangeRate       *decimal.Decimal `json:"exchange_rate"`
	Subtotal           decimal.Decimal  `json:"subtotal"`
	SubtotalMXN        decimal.Decimal  `json:"subtotal_mxn"`
	IVA                decimal.Decimal  `json:"iva"`
	ISRWithheld        decimal.Decimal  `json:"isr_withheld"`
	IVAWithheld        decimal.Decimal  `json:"iva_withheld"`
	Total              decimal.Decimal  `json:"total"`
	ExpectedDepositMXN decimal.Decimal  `json:"expected_deposit_mxn"`
	State              string           `json:"state"`
	UUID               *string          `json:"uuid"`
	MovementID         *int             `json:"movement_id"`
	DeclarationPeriod  *string          `json:"declaration_period"`
	CreatedAt          string           `json:"created_at"`
}

// documentDTO never carries the storage key.
type documentDTO struct {
	ID          int    `json:"id"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
	UploadedAt  string `json:"uploaded_at"`
}

type warningDTO struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	InvoiceIDs []int  `json:"invoice_ids,omitempty"`
	Expected   string `json:"expected,omitempty"`
	Actual     string `json:"actual,omitempty"`
}

type partyDTO struct {
	RFC          string `json:"rfc"`
	Name         string `json:"name"`
	Regimen      string `json:"regimen"`
	PostalCode   string `json:"postal_code"`
	UsoCFDI      string `json:"uso_cfdi,omitempty"`
	InternalNote string `json:"internal_note,omitempty"`
}

type globalInfoDTO struct {
	Periodicity string `json:"periodicity"`
	Code        string `json:"code"`
	Months      string `json:"months"`
	Year        string `json:"year"`
}

type voucherDTO struct {
	Type          string           `json:"type"`
	Currency      string           `json:"currency"`
	ExchangeRate  *decimal.Decimal `json:"exchange_rate"`
	PaymentForm   string           `json:"payment_form"`
	PaymentMethod string           `json:"payment_method"`
	Export        bool             `json:"export"`
	Global        *globalInfoDTO   `json:"global"`
}

type conceptDTO struct {
	ProdServKey string          `json:"prod_serv_key"`
	UnitKey     string          `json:"unit_key"`
	Description string          `json:"description"`
	Quantity    int             `json:"quantity"`
	UnitValue   decimal.Decimal `json:"unit_value"`
}

type taxesDTO struct {
	IVAIncluded bool            `json:"iva_included"`
	IVA         decimal.Decimal `json:"iva"`
}

type totalsDTO struct {
	Currency           string          `json:"currency"`
	Subtotal           decimal.Decimal `json:"subtotal"`
	Total              decimal.Decimal `json:"total"`
	ExpectedDepositMXN decimal.Decimal `json:"expected_deposit_mxn"`
}

type checklistDTO struct {
	Issuer        partyDTO   `json:"issuer"`
	Receiver      partyDTO   `json:"receiver"`
	Voucher       voucherDTO `json:"voucher"`
	Concept       conceptDTO `json:"concept"`
	Taxes         taxesDTO   `json:"taxes"`
	Totals        totalsDTO  `json:"totals"`
	Period        string     `json:"period"`
	DueDate       string     `json:"due_date"`
	MissingConfig []string   `json:"missing_config"`
	ToConfirm     []string   `json:"to_confirm"`
}

// detailDTO is an invoice with its documents, checklist and the warnings of
// the operation that produced it (always an array).
type detailDTO struct {
	Invoice   invoiceDTO    `json:"invoice"`
	Documents []documentDTO `json:"documents"`
	Checklist checklistDTO  `json:"checklist"`
	Warnings  []warningDTO  `json:"warnings"`
}

type documentResultDTO struct {
	Document documentDTO  `json:"document"`
	Warnings []warningDTO `json:"warnings"`
}

func toInvoiceDTO(i invoices.Invoice) invoiceDTO {
	dto := invoiceDTO{
		ID: i.ID, ClientID: i.ClientID, CollectionDate: i.CollectionDate, Period: i.Period, Currency: i.Currency,
		ExchangeRate: i.ExchangeRate, Subtotal: i.Subtotal, SubtotalMXN: i.SubtotalMXN, IVA: i.IVA,
		ISRWithheld: i.ISRWithheld, IVAWithheld: i.IVAWithheld, Total: i.Total, ExpectedDepositMXN: i.ExpectedDepositMXN,
		State: string(i.Status), MovementID: i.MovementID,
		CreatedAt: i.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
	if i.UUID != "" {
		u := i.UUID
		dto.UUID = &u
	}
	if i.DeclarationPeriod != "" {
		p := i.DeclarationPeriod
		dto.DeclarationPeriod = &p
	}
	return dto
}

func toDocumentDTO(d invoices.Document) documentDTO {
	return documentDTO{
		ID: d.ID, Kind: string(d.Kind), Name: d.Name, ContentType: d.ContentType, Size: d.Size, SHA256: d.SHA256,
		UploadedAt: d.UploadedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

func toWarningDTOs(ws []invoices.Warning) []warningDTO {
	out := make([]warningDTO, len(ws))
	for i, w := range ws {
		out[i] = warningDTO{Code: w.Code, Message: w.Message, InvoiceIDs: w.InvoiceIDs, Expected: w.Expected, Actual: w.Actual}
	}
	return out
}

func toPartyDTO(p invoices.Party) partyDTO {
	return partyDTO{RFC: p.RFC, Name: p.Name, Regimen: p.Regimen, PostalCode: p.PostalCode, UsoCFDI: p.UsoCFDI, InternalNote: p.InternalNote}
}

func toChecklistDTO(c invoices.Checklist) checklistDTO {
	dto := checklistDTO{
		Issuer:   toPartyDTO(c.Issuer),
		Receiver: toPartyDTO(c.Receiver),
		Voucher: voucherDTO{
			Type: c.Voucher.Type, Currency: c.Voucher.Currency, ExchangeRate: c.Voucher.ExchangeRate,
			PaymentForm: c.Voucher.PaymentForm, PaymentMethod: c.Voucher.PaymentMethod, Export: c.Voucher.Export,
		},
		Concept: conceptDTO{
			ProdServKey: c.Concept.ProdServKey, UnitKey: c.Concept.UnitKey, Description: c.Concept.Description,
			Quantity: c.Concept.Quantity, UnitValue: c.Concept.UnitValue,
		},
		Taxes:         taxesDTO{IVAIncluded: c.Taxes.IVAIncluded, IVA: c.Taxes.IVA},
		Totals:        totalsDTO{Currency: c.Totals.Currency, Subtotal: c.Totals.Subtotal, Total: c.Totals.Total, ExpectedDepositMXN: c.Totals.ExpectedDepositMXN},
		Period:        c.Period,
		DueDate:       c.DueDate,
		MissingConfig: nonNil(c.MissingConfig),
		ToConfirm:     nonNil(c.ToConfirm),
	}
	if g := c.Voucher.Global; g != nil {
		dto.Voucher.Global = &globalInfoDTO{Periodicity: g.Periodicity, Code: g.Code, Months: g.Months, Year: g.Year}
	}
	return dto
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func toDetailDTO(d app.Detail, warnings []invoices.Warning) detailDTO {
	docs := make([]documentDTO, len(d.Documents))
	for i, doc := range d.Documents {
		docs[i] = toDocumentDTO(doc)
	}
	return detailDTO{
		Invoice: toInvoiceDTO(d.Invoice), Documents: docs, Checklist: toChecklistDTO(d.Checklist),
		Warnings: toWarningDTOs(warnings),
	}
}

// --- handlers -----------------------------------------------------------

func (h *Handler) prepare(w http.ResponseWriter, r *http.Request) {
	var req prepareRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	res, err := h.svc.Prepare(r.Context(), app.PrepareInput{
		ClientID: req.ClientID, Date: req.Date, Subtotal: req.Subtotal, Amount: req.Amount,
		ExchangeRate: req.ExchangeRate, MovementID: req.MovementID, Periodicity: req.Periodicity,
	})
	if err != nil {
		h.fail(w, "prepare", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, toDetailDTO(res.Detail, res.Warnings))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	list, err := h.svc.List(r.Context(), q.Get("period"), invoices.Status(q.Get("state")))
	if err != nil {
		h.fail(w, "list", err)
		return
	}
	out := make([]invoiceDTO, len(list))
	for i, inv := range list {
		out[i] = toInvoiceDTO(inv)
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	detail, err := h.svc.Get(r.Context(), id, r.URL.Query().Get("periodicity"))
	if err != nil {
		h.fail(w, "get", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toDetailDTO(detail, nil))
}

// issue takes multipart/form-data with the optional file fields xml and pdf
// and the optional text field uuid; xml or uuid is required.
func (h *Handler) issue(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	if !h.parseMultipart(w, r) {
		return
	}
	in := app.IssueInput{UUID: r.FormValue("uuid")}
	var err error
	if in.XML, err = readUpload(r, "xml", invoices.DocumentXML); err != nil {
		h.fail(w, "issue", err)
		return
	}
	if in.PDF, err = readUpload(r, "pdf", invoices.DocumentPDF); err != nil {
		h.fail(w, "issue", err)
		return
	}
	res, err := h.svc.Issue(r.Context(), id, in)
	if err != nil {
		h.fail(w, "issue", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toDetailDTO(res.Detail, res.Warnings))
}

// attach takes multipart/form-data with the text field kind (xml or pdf) and
// the file field file.
func (h *Handler) attach(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	if !h.parseMultipart(w, r) {
		return
	}
	kind := invoices.DocumentKind(r.FormValue("kind"))
	if !kind.IsValid() {
		httpjson.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_document", "message": "kind must be xml or pdf"})
		return
	}
	up, err := readUpload(r, "file", kind)
	if err != nil {
		h.fail(w, "attach", err)
		return
	}
	if up == nil {
		httpjson.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_document", "message": "the file field is required"})
		return
	}
	res, err := h.svc.AttachDocument(r.Context(), id, kind, *up)
	if err != nil {
		h.fail(w, "attach", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, documentResultDTO{Document: toDocumentDTO(res.Document), Warnings: toWarningDTOs(res.Warnings)})
}

// download streams the stored file through the API. The response is an
// attachment, is never cached and is not sniffed or sandboxed-executable.
func (h *Handler) download(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	docID, err := strconv.Atoi(r.PathValue("docId"))
	if err != nil || docID <= 0 {
		httpjson.WriteError(w, http.StatusNotFound, "not_found")
		return
	}
	doc, body, err := h.svc.Download(r.Context(), id, docID)
	if err != nil {
		h.fail(w, "download", err)
		return
	}
	defer body.Close()

	hd := w.Header()
	hd.Set("Content-Type", doc.ContentType)
	hd.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": doc.Name}))
	hd.Set("Content-Length", strconv.FormatInt(doc.Size, 10))
	hd.Set("Cache-Control", "no-store")
	hd.Set("X-Content-Type-Options", "nosniff")
	hd.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, body); err != nil {
		h.logger.Error("invoice download interrupted", "invoice", id, "document", docID, "error", err)
	}
}

func (h *Handler) cancel(w http.ResponseWriter, r *http.Request) {
	id, ok := httpjson.PathID(w, r)
	if !ok {
		return
	}
	inv, err := h.svc.Cancel(r.Context(), id)
	if err != nil {
		h.fail(w, "cancel", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toInvoiceDTO(inv))
}

// --- helpers ------------------------------------------------------------

// parseMultipart bounds the request body and parses the form. On failure it
// writes the error response and returns false.
func (h *Handler) parseMultipart(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxMultipartBytes)
	if err := r.ParseMultipartForm(maxMultipartBytes); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			httpjson.WriteError(w, http.StatusRequestEntityTooLarge, "request_too_large")
			return false
		}
		httpjson.WriteError(w, http.StatusBadRequest, "invalid_request")
		return false
	}
	return true
}

// readUpload reads the file field into memory, never more than the kind's
// limit plus one byte (the domain then rejects the oversized file). It
// returns nil when the field is absent or empty of a file.
func readUpload(r *http.Request, field string, kind invoices.DocumentKind) (*app.Upload, error) {
	files := r.MultipartForm.File[field]
	if len(files) == 0 {
		return nil, nil
	}
	fh := files[0]
	f, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, kind.MaxBytes()+1))
	if err != nil {
		return nil, err
	}
	return &app.Upload{Name: fh.Filename, ContentType: contentType(fh), Data: data}, nil
}

func contentType(fh *multipart.FileHeader) string { return fh.Header.Get("Content-Type") }

func (h *Handler) fail(w http.ResponseWriter, op string, err error) {
	reply := func(status int, code string) {
		httpjson.WriteJSON(w, status, map[string]string{"error": code, "message": err.Error()})
	}
	switch {
	case errors.Is(err, invoices.ErrUnknownClient):
		reply(http.StatusBadRequest, "unknown_client")
	case errors.Is(err, invoices.ErrInvalidInput):
		reply(http.StatusBadRequest, "invalid_invoice")
	case errors.Is(err, invoices.ErrIssueInput):
		reply(http.StatusBadRequest, "xml_or_uuid_required")
	case errors.Is(err, invoices.ErrInvalidUUID):
		reply(http.StatusBadRequest, "invalid_uuid")
	case errors.Is(err, invoices.ErrInvalidDocument):
		reply(http.StatusBadRequest, "invalid_document")
	case errors.Is(err, invoices.ErrNotStamped):
		reply(http.StatusUnprocessableEntity, "cfdi_not_stamped")
	case errors.Is(err, invoices.ErrInvalidCFDI):
		reply(http.StatusUnprocessableEntity, "invalid_cfdi")
	case errors.Is(err, invoices.ErrUUIDMismatch):
		reply(http.StatusUnprocessableEntity, "uuid_mismatch")
	case errors.Is(err, invoices.ErrDuplicateUUID):
		reply(http.StatusConflict, "duplicate_uuid")
	case errors.Is(err, invoices.ErrCancelled):
		reply(http.StatusConflict, "invoice_cancelled")
	case errors.Is(err, invoices.ErrDeclared):
		reply(http.StatusConflict, "invoice_declared")
	case errors.Is(err, invoices.ErrAlreadyIssued):
		reply(http.StatusConflict, "invoice_already_issued")
	case errors.Is(err, invoices.ErrNotIssued):
		reply(http.StatusConflict, "invoice_not_issued")
	case errors.Is(err, invoices.ErrStateChanged):
		reply(http.StatusConflict, "invoice_state_changed")
	case errors.Is(err, invoices.ErrNotFound), errors.Is(err, invoices.ErrDocumentMissing):
		httpjson.WriteError(w, http.StatusNotFound, "not_found")
	case errors.Is(err, settings.ErrMissingConfig):
		reply(http.StatusUnprocessableEntity, "settings_incomplete")
	case errors.Is(err, app.ErrStorage):
		h.logger.Error("invoice storage failed", "op", op, "error", err)
		httpjson.WriteError(w, http.StatusServiceUnavailable, "storage_unavailable")
	default:
		h.logger.Error("invoices request failed", "op", op, "error", err)
		httpjson.WriteError(w, http.StatusInternalServerError, "internal_error")
	}
}
