// Package taxfilinghttp exposes the tax filing use cases over HTTP: the preview
// of the monthly RESICO declaration, registering a filing, recording its
// payment and the history with the periods still to file. Every route sits
// behind the authentication middleware passed to Register. Money and rates
// travel as decimal strings. "IVA acreditable" is the official SAT term and is
// kept in the field names.
package taxfilinghttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/shopspring/decimal"

	invoices "github.com/valium69mg/finances-app/backend/internal/invoices/domain"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/httpjson"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
	"github.com/valium69mg/finances-app/backend/internal/taxfiling/app"
	taxfiling "github.com/valium69mg/finances-app/backend/internal/taxfiling/domain"
)

// Service is the set of use cases the handlers need.
type Service interface {
	Preview(ctx context.Context, period string, ivaCreditable decimal.Decimal) (app.Preview, error)
	Register(ctx context.Context, in app.RegisterInput) (app.Result, error)
	Pay(ctx context.Context, period string, in app.PaymentInput) (app.Result, error)
	Get(ctx context.Context, period string) (app.Detail, error)
	List(ctx context.Context, year int, status taxfiling.PaymentStatus) ([]taxfiling.Filing, error)
	Pending(ctx context.Context) ([]taxfiling.PendingPeriod, error)
	UnfiledInvoices(ctx context.Context) ([]invoices.Invoice, error)
	Delete(ctx context.Context, period string) error
}

// Handler serves the /tax-filing routes.
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

// Register mounts the tax filing routes on mux, each wrapped by requireAuth.
func (h *Handler) Register(mux *http.ServeMux, requireAuth func(http.Handler) http.Handler) {
	route := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, requireAuth(fn))
	}
	route("GET /tax-filing/preview", h.preview)
	route("GET /tax-filing/pending-periods", h.pending)
	route("GET /tax-filing/unfiled-invoices", h.unfiled)
	route("POST /tax-filing", h.register)
	route("GET /tax-filing", h.list)
	route("GET /tax-filing/{period}", h.get)
	route("POST /tax-filing/{period}/payment", h.pay)
	route("DELETE /tax-filing/{period}", h.delete)
}

// --- DTOs ---------------------------------------------------------------

// paymentRequest is the payment part of a registration and the body of
// POST /tax-filing/{period}/payment. date defaults to the filing date when
// registering and to today when paying later; record_expense asks to record the
// total as an Impuestos expense (never done unless true).
type paymentRequest struct {
	Date          string          `json:"date"`
	ISRPaid       decimal.Decimal `json:"isr_paid"`
	IVAPaid       decimal.Decimal `json:"iva_paid"`
	RecordExpense bool            `json:"record_expense"`
}

// registerRequest is the body of POST /tax-filing. The declaration figures are
// computed by the server; only the creditable IVA, the folio and the optional
// payment come from the user. filing_date defaults to today; without payment the
// filing is registered pending.
type registerRequest struct {
	Period         string          `json:"period"`
	FilingDate     string          `json:"filing_date"`
	Folio          string          `json:"folio"`
	IVAAcreditable decimal.Decimal `json:"iva_acreditable"`
	Payment        *paymentRequest `json:"payment"`
}

type invoiceRefDTO struct {
	ID             int             `json:"id"`
	ClientID       string          `json:"client_id"`
	CollectionDate string          `json:"collection_date"`
	Currency       string          `json:"currency"`
	SubtotalMXN    decimal.Decimal `json:"subtotal_mxn"`
	State          string          `json:"state"`
	UUID           *string         `json:"uuid"`
}

type warningDTO struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	InvoiceIDs []int  `json:"invoice_ids,omitempty"`
}

type paymentDTO struct {
	Date      string          `json:"date"`
	ISRPaid   decimal.Decimal `json:"isr_paid"`
	IVAPaid   decimal.Decimal `json:"iva_paid"`
	TotalPaid decimal.Decimal `json:"total_paid"`
}

type filingDTO struct {
	Period            string          `json:"period"`
	FilingDate        string          `json:"filing_date"`
	DueDate           string          `json:"due_date"`
	IncomeCollected   decimal.Decimal `json:"income_collected"`
	ISRRate           decimal.Decimal `json:"isr_rate"`
	ISRAccrued        decimal.Decimal `json:"isr_accrued"`
	ISRWithheld       decimal.Decimal `json:"isr_withheld"`
	ISRDue            decimal.Decimal `json:"isr_due"`
	IVATransferred    decimal.Decimal `json:"iva_transferred"`
	IVAWithheld       decimal.Decimal `json:"iva_withheld"`
	IVAAcreditable    decimal.Decimal `json:"iva_acreditable"`
	IVADue            decimal.Decimal `json:"iva_due"`
	TotalToPay        decimal.Decimal `json:"total_to_pay"`
	Folio             string          `json:"folio"`
	Status            string          `json:"status"`
	Payment           *paymentDTO     `json:"payment"`
	ExpenseMovementID *int            `json:"expense_movement_id"`
	InvoiceIDs        []int           `json:"invoice_ids"`
	CreatedAt         string          `json:"created_at"`
}

// filingDetailDTO is a filing with the invoices it includes.
type filingDetailDTO struct {
	filingDTO
	Invoices []invoiceRefDTO `json:"invoices"`
}

// filingResultDTO is the body of a registration or a payment.
type filingResultDTO struct {
	Filing   filingDTO    `json:"filing"`
	Warnings []warningDTO `json:"warnings"`
}

// previewDTO is the computed declaration of a period. It is never stored;
// filing is set when the period is already filed.
type previewDTO struct {
	Period          string          `json:"period"`
	DueDate         string          `json:"due_date"`
	IncomeCollected decimal.Decimal `json:"income_collected"`
	ISRRate         decimal.Decimal `json:"isr_rate"`
	ISRAccrued      decimal.Decimal `json:"isr_accrued"`
	ISRWithheld     decimal.Decimal `json:"isr_withheld"`
	ISRDue          decimal.Decimal `json:"isr_due"`
	IVATransferred  decimal.Decimal `json:"iva_transferred"`
	IVAWithheld     decimal.Decimal `json:"iva_withheld"`
	IVAAcreditable  decimal.Decimal `json:"iva_acreditable"`
	IVADue          decimal.Decimal `json:"iva_due"`
	TotalToPay      decimal.Decimal `json:"total_to_pay"`
	ExportBase      decimal.Decimal `json:"export_base"`
	Invoices        []invoiceRefDTO `json:"invoices"`
	Warnings        []warningDTO    `json:"warnings"`
	Filing          *filingDTO      `json:"filing"`
}

type pendingPeriodDTO struct {
	Period  string `json:"period"`
	DueDate string `json:"due_date"`
	Overdue bool   `json:"overdue"`
}

// unfiledInvoiceDTO is an issued invoice that no filing includes although its
// period was already filed.
type unfiledInvoiceDTO struct {
	invoiceRefDTO
	Period string `json:"period"`
}

const timeFormat = "2006-01-02T15:04:05Z"

func toInvoiceRefDTO(i invoices.Invoice) invoiceRefDTO {
	dto := invoiceRefDTO{
		ID: i.ID, ClientID: i.ClientID, CollectionDate: i.CollectionDate, Currency: i.Currency,
		SubtotalMXN: i.SubtotalMXN, State: string(i.Status),
	}
	if i.UUID != "" {
		u := i.UUID
		dto.UUID = &u
	}
	return dto
}

func toInvoiceRefDTOs(list []invoices.Invoice) []invoiceRefDTO {
	out := make([]invoiceRefDTO, len(list))
	for i, inv := range list {
		out[i] = toInvoiceRefDTO(inv)
	}
	return out
}

func toWarningDTOs(ws []app.Warning) []warningDTO {
	out := make([]warningDTO, len(ws))
	for i, w := range ws {
		out[i] = warningDTO{Code: w.Code, Message: w.Message, InvoiceIDs: w.InvoiceIDs}
	}
	return out
}

func toFilingDTO(f taxfiling.Filing) filingDTO {
	due, _ := taxfiling.DueDate(f.Period) // the period was validated when it was stored
	ids := f.InvoiceIDs
	if ids == nil {
		ids = []int{}
	}
	dto := filingDTO{
		Period: f.Period, FilingDate: f.FilingDate, DueDate: due,
		IncomeCollected: f.IncomeCollected, ISRRate: f.ISRRate, ISRAccrued: f.ISRAccrued, ISRWithheld: f.ISRWithheld,
		ISRDue: f.ISRDue, IVATransferred: f.IVATransferred, IVAWithheld: f.IVAWithheld, IVAAcreditable: f.IVACreditable,
		IVADue: f.IVADue, TotalToPay: f.TotalToPay(), Folio: f.Folio, Status: string(f.PaymentStatus()),
		ExpenseMovementID: f.ExpenseMovementID, InvoiceIDs: ids, CreatedAt: f.CreatedAt.UTC().Format(timeFormat),
	}
	if p := f.Payment; p != nil {
		dto.Payment = &paymentDTO{Date: p.Date, ISRPaid: p.ISRPaid, IVAPaid: p.IVAPaid, TotalPaid: p.Total()}
	}
	return dto
}

func toPreviewDTO(p app.Preview) previewDTO {
	d := p.Declaration
	dto := previewDTO{
		Period: d.Period, DueDate: d.DueDate, IncomeCollected: d.IncomeCollected, ISRRate: d.ISRRate,
		ISRAccrued: d.ISRAccrued, ISRWithheld: d.ISRWithheld, ISRDue: d.ISRToPay, IVATransferred: d.IVATransferred,
		IVAWithheld: d.IVAWithheld, IVAAcreditable: d.IVACreditable, IVADue: d.IVAPayable, TotalToPay: d.TotalToPay(),
		ExportBase: d.ExportBase, Invoices: toInvoiceRefDTOs(d.Invoices), Warnings: toWarningDTOs(p.Warnings),
	}
	if p.Filing != nil {
		f := toFilingDTO(*p.Filing)
		dto.Filing = &f
	}
	return dto
}

// --- handlers -----------------------------------------------------------

func (h *Handler) preview(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	credit := decimal.Zero
	if raw := q.Get("iva_acreditable"); raw != "" {
		v, err := decimal.NewFromString(raw)
		if err != nil {
			httpjson.WriteError(w, http.StatusBadRequest, "invalid_request")
			return
		}
		credit = v
	}
	res, err := h.svc.Preview(r.Context(), q.Get("period"), credit)
	if err != nil {
		h.fail(w, "preview", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, toPreviewDTO(res))
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	in := app.RegisterInput{Period: req.Period, Date: req.FilingDate, Folio: req.Folio, IVACreditable: req.IVAAcreditable}
	if p := req.Payment; p != nil {
		in.Payment = &app.PaymentInput{Date: p.Date, ISRPaid: p.ISRPaid, IVAPaid: p.IVAPaid, RecordExpense: p.RecordExpense}
	}
	res, err := h.svc.Register(r.Context(), in)
	if err != nil {
		h.fail(w, "register", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusCreated, filingResultDTO{Filing: toFilingDTO(res.Filing), Warnings: toWarningDTOs(res.Warnings)})
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	year := 0
	if raw := q.Get("year"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 9999 {
			httpjson.WriteError(w, http.StatusBadRequest, "invalid_request")
			return
		}
		year = n
	}
	list, err := h.svc.List(r.Context(), year, taxfiling.PaymentStatus(q.Get("status")))
	if err != nil {
		h.fail(w, "list", err)
		return
	}
	out := make([]filingDTO, len(list))
	for i, f := range list {
		out[i] = toFilingDTO(f)
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	detail, err := h.svc.Get(r.Context(), r.PathValue("period"))
	if err != nil {
		h.fail(w, "get", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, filingDetailDTO{filingDTO: toFilingDTO(detail.Filing), Invoices: toInvoiceRefDTOs(detail.Invoices)})
}

func (h *Handler) pay(w http.ResponseWriter, r *http.Request) {
	var req paymentRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	res, err := h.svc.Pay(r.Context(), r.PathValue("period"), app.PaymentInput{
		Date: req.Date, ISRPaid: req.ISRPaid, IVAPaid: req.IVAPaid, RecordExpense: req.RecordExpense,
	})
	if err != nil {
		h.fail(w, "pay", err)
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, filingResultDTO{Filing: toFilingDTO(res.Filing), Warnings: toWarningDTOs(res.Warnings)})
}

func (h *Handler) pending(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.Pending(r.Context())
	if err != nil {
		h.fail(w, "pending", err)
		return
	}
	out := make([]pendingPeriodDTO, len(list))
	for i, p := range list {
		out[i] = pendingPeriodDTO{Period: p.Period, DueDate: p.DueDate, Overdue: p.Overdue}
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

// unfiled lists the issued invoices left out of an already filed period, so the
// user can declare their income with the SAT (this app builds no complementary
// declaration).
func (h *Handler) unfiled(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.UnfiledInvoices(r.Context())
	if err != nil {
		h.fail(w, "unfiled", err)
		return
	}
	out := make([]unfiledInvoiceDTO, len(list))
	for i, inv := range list {
		out[i] = unfiledInvoiceDTO{invoiceRefDTO: toInvoiceRefDTO(inv), Period: inv.Period}
	}
	httpjson.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), r.PathValue("period")); err != nil {
		h.fail(w, "delete", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) fail(w http.ResponseWriter, op string, err error) {
	reply := func(status int, code string) {
		httpjson.WriteJSON(w, status, map[string]string{"error": code, "message": err.Error()})
	}
	switch {
	case errors.Is(err, taxfiling.ErrInvalidInput):
		reply(http.StatusBadRequest, "invalid_filing")
	case errors.Is(err, ledger.ErrInvalid):
		reply(http.StatusBadRequest, "invalid_expense")
	case errors.Is(err, taxfiling.ErrAlreadyFiled):
		reply(http.StatusConflict, "already_filed")
	case errors.Is(err, taxfiling.ErrAlreadyPaid):
		reply(http.StatusConflict, "already_paid")
	case errors.Is(err, taxfiling.ErrFilingPaid):
		reply(http.StatusConflict, "filing_paid")
	case errors.Is(err, taxfiling.ErrInvoicesChanged):
		reply(http.StatusConflict, "invoices_changed")
	case errors.Is(err, taxfiling.ErrNotFound):
		httpjson.WriteError(w, http.StatusNotFound, "not_found")
	case errors.Is(err, settings.ErrMissingConfig):
		reply(http.StatusUnprocessableEntity, "settings_incomplete")
	default:
		h.logger.Error("tax filing request failed", "op", op, "error", err)
		httpjson.WriteError(w, http.StatusInternalServerError, "internal_error")
	}
}
