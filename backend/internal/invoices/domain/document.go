package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/shopspring/decimal"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
)

// DocumentKind is the kind of a stored invoice document.
type DocumentKind string

const (
	DocumentXML DocumentKind = "xml"
	DocumentPDF DocumentKind = "pdf"
)

// Upload limits. A stamped CFDI XML is a few kilobytes; 1 MiB is generous and
// bounds the parser input.
const (
	MaxXMLBytes = 1 << 20
	MaxPDFBytes = 10 << 20
	maxNameLen  = 200
)

// Canonical content types stored for each kind; the client-declared type is
// only checked, never trusted.
const (
	contentTypeXML = "application/xml"
	contentTypePDF = "application/pdf"
)

// Errors returned by the document and CFDI rules.
var (
	ErrInvalidDocument = errors.New("invalid document")
	ErrInvalidCFDI     = errors.New("invalid CFDI XML")
	ErrNotStamped      = errors.New("CFDI XML has no TimbreFiscalDigital")
	ErrInvalidUUID     = errors.New("invalid CFDI UUID")
	ErrDuplicateUUID   = errors.New("UUID already used by another invoice")
	ErrUUIDMismatch    = errors.New("XML UUID does not match the invoice UUID")
	ErrNotFound        = errors.New("invoice not found")
	ErrDocumentMissing = errors.New("document not found")
	ErrNotIssued       = errors.New("invoice is not issued")
	ErrAlreadyIssued   = errors.New("invoice is already issued")
	ErrStateChanged    = errors.New("invoice state changed")
	// ErrDeclared is returned when cancelling an invoice that is included in a
	// registered tax filing: the saved declaration would silently go out of sync.
	ErrDeclared   = errors.New("invoice is included in a filed tax declaration")
	ErrIssueInput = errors.New("issue requires an XML or a UUID")
	// ErrNoXML is returned when an operation needs the stored XML of an invoice
	// that has none.
	ErrNoXML = errors.New("invoice has no XML document")
	// ErrCurrencyMismatch is returned when the currency of a stamped XML is not
	// the invoice currency and the amounts cannot be taken from it.
	ErrCurrencyMismatch = errors.New("XML currency does not match the invoice currency")
)

// Document is the metadata of an issued CFDI file stored in object storage.
// The bytes live in the store under Key; only the metadata is persisted in the
// database.
type Document struct {
	ID          int
	InvoiceID   int
	Kind        DocumentKind
	Key         string
	Name        string
	ContentType string
	Size        int64
	SHA256      string // lowercase hex
	UploadedAt  time.Time
}

// IsValid reports whether k is a known kind.
func (k DocumentKind) IsValid() bool { return k == DocumentXML || k == DocumentPDF }

// Extension returns the file extension (with dot) of the kind.
func (k DocumentKind) Extension() string { return "." + string(k) }

// MaxBytes returns the size limit of the kind.
func (k DocumentKind) MaxBytes() int64 {
	if k == DocumentXML {
		return MaxXMLBytes
	}
	return MaxPDFBytes
}

var declaredXMLTypes = map[string]bool{
	"":                         true,
	"application/xml":          true,
	"text/xml":                 true,
	"application/octet-stream": true,
}

var declaredPDFTypes = map[string]bool{
	"":                         true,
	"application/pdf":          true,
	"application/octet-stream": true,
}

// NewDocument validates an upload and returns its metadata (without ID, key
// or invoice). The extension and the declared content type must match the kind,
// the size must be in 1..MaxBytes, and a PDF must start with the %PDF- magic.
// The stored content type is the canonical one for the kind, the name is
// reduced to a safe base name, and the checksum is the SHA-256 of data.
// XML content is validated by ParseCFDI, not here.
func NewDocument(kind DocumentKind, name, declaredType string, data []byte) (Document, error) {
	if !kind.IsValid() {
		return Document{}, fmt.Errorf("%w: unknown kind %q", ErrInvalidDocument, kind)
	}
	size := int64(len(data))
	if size == 0 {
		return Document{}, fmt.Errorf("%w: %s file is empty", ErrInvalidDocument, kind)
	}
	if size > kind.MaxBytes() {
		return Document{}, fmt.Errorf("%w: %s file exceeds %d bytes", ErrInvalidDocument, kind, kind.MaxBytes())
	}
	clean := SafeName(name)
	if !strings.EqualFold(path.Ext(clean), kind.Extension()) {
		return Document{}, fmt.Errorf("%w: a %s file must have the %s extension", ErrInvalidDocument, kind, kind.Extension())
	}
	declared := strings.ToLower(strings.TrimSpace(strings.SplitN(declaredType, ";", 2)[0]))
	contentType := contentTypeXML
	allowed := declaredXMLTypes
	if kind == DocumentPDF {
		contentType, allowed = contentTypePDF, declaredPDFTypes
		if !bytes.HasPrefix(data, []byte("%PDF-")) {
			return Document{}, fmt.Errorf("%w: the file is not a PDF", ErrInvalidDocument)
		}
	}
	if !allowed[declared] {
		return Document{}, fmt.Errorf("%w: content type %q is not valid for a %s file", ErrInvalidDocument, declared, kind)
	}
	sum := sha256.Sum256(data)
	return Document{
		Kind: kind, Name: clean, ContentType: contentType, Size: size, SHA256: hex.EncodeToString(sum[:]),
	}, nil
}

// SafeName reduces an uploaded file name to a printable base name of at most
// 200 bytes: directories, control characters and quotes are dropped. It never
// returns an empty string.
func SafeName(name string) string {
	name = strings.ReplaceAll(name, `\`, "/")
	name = path.Base(name)
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '"' || r == utf8.RuneError {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "." || name == "/" || name == ".." || name == "" {
		return "document"
	}
	for len(name) > maxNameLen {
		_, size := utf8.DecodeLastRuneInString(name)
		name = name[:len(name)-size]
	}
	return name
}

var uuidPattern = regexp.MustCompile(`^[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}$`)

// NormalizeUUID trims and upper-cases a CFDI UUID (the SAT prints them in
// upper case) and rejects anything that is not in the 8-4-4-4-12 hex format.
func NormalizeUUID(s string) (string, error) {
	s = strings.TrimSpace(s)
	if !uuidPattern.MatchString(s) {
		return "", fmt.Errorf("%w: expected the 8-4-4-4-12 hexadecimal format", ErrInvalidUUID)
	}
	return strings.ToUpper(s), nil
}

// Warning is a non-blocking finding returned next to a successful result.
type Warning struct {
	Code       string
	Message    string
	InvoiceIDs []int    // possible_duplicate
	Expected   string   // mismatch warnings: the prepared invoice value
	Actual     string   // mismatch warnings: the XML value
	Changes    []Change // amounts_from_xml: the stored amounts that the XML replaced
}

// Change is one stored amount replaced by the value in the XML.
type Change struct {
	Field string // subtotal, iva, isr_withheld, iva_withheld or total
	From  string
	To    string
}

// Warning codes.
const (
	WarningPossibleDuplicate = "possible_duplicate"
	WarningTotalMismatch     = "total_mismatch"
	WarningSubtotalMismatch  = "subtotal_mismatch"
	WarningCurrencyMismatch  = "currency_mismatch"
	// WarningAmountsFromXML is returned when the amounts of the invoice were
	// replaced by the ones in its stamped XML (on issue or on resync).
	WarningAmountsFromXML = "amounts_from_xml"
	// WarningPeriodAlreadyFiled is returned when an invoice is issued in a
	// period whose tax declaration was already filed: the invoice is not part of
	// that declaration and its income stays undeclared.
	WarningPeriodAlreadyFiled = "period_already_filed"
)

// PeriodAlreadyFiledWarning is the warning for an invoice issued in a period
// that already has a registered tax filing.
func PeriodAlreadyFiledWarning(period string) Warning {
	return Warning{
		Code: WarningPeriodAlreadyFiled,
		Message: fmt.Sprintf("the tax declaration of %s was already filed: this invoice is not part of it and its income "+
			"is not declared; declare it with the SAT outside this app", period),
	}
}

// DuplicateWarning returns the possible-duplicate warning for the given IDs, or
// no warning when there are none.
func DuplicateWarning(ids []int) []Warning {
	if len(ids) == 0 {
		return nil
	}
	return []Warning{{
		Code:       WarningPossibleDuplicate,
		Message:    "an invoice for the same client and collection date already exists",
		InvoiceIDs: ids,
	}}
}

// CompareCFDI compares the values read from an XML with the prepared invoice.
// A mismatch is a warning, never an error: the SAT stamp is the source of truth.
// Amounts are compared at cents.
func (i Invoice) CompareCFDI(c CFDI) []Warning {
	var out []Warning
	if !c.Total.Round(2).Equal(i.Total.Round(2)) {
		out = append(out, Warning{
			Code: WarningTotalMismatch, Message: "the XML total differs from the prepared invoice total",
			Expected: i.Total.StringFixed(2), Actual: c.Total.StringFixed(2),
		})
	}
	if !c.SubTotal.Round(2).Equal(i.Subtotal.Round(2)) {
		out = append(out, Warning{
			Code: WarningSubtotalMismatch, Message: "the XML subtotal differs from the prepared invoice subtotal",
			Expected: i.Subtotal.StringFixed(2), Actual: c.SubTotal.StringFixed(2),
		})
	}
	if !strings.EqualFold(c.Currency, i.Currency) {
		out = append(out, Warning{
			Code: WarningCurrencyMismatch, Message: "the XML currency differs from the prepared invoice currency",
			Expected: i.Currency, Actual: c.Currency,
		})
	}
	return out
}

// AmountsFromCFDI returns the invoice amounts according to the stamped XML,
// which is the source of truth for an issued invoice, and a warning listing the
// stored amounts it changes (none when nothing changes). Subtotal, IVA,
// retentions and Total are the XML values in the invoice currency; the peso
// fields of a USD invoice are derived with the invoice exchange rate. It
// returns ok false, with no amounts, when the XML currency is not the invoice
// currency: such a XML only produces CompareCFDI warnings.
func (i Invoice) AmountsFromCFDI(c CFDI) (a Amounts, warnings []Warning, ok bool, err error) {
	if !strings.EqualFold(c.Currency, i.Currency) {
		return Amounts{}, nil, false, nil
	}
	a = Amounts{
		Subtotal: c.SubTotal.Round(2), IVA: c.IVATransferred.Round(2), ISRWithheld: c.ISRWithheld.Round(2),
		IVAWithheld: c.IVAWithheld.Round(2), Total: c.Total.Round(2),
	}
	a.SubtotalMXN, a.ExpectedDepositMXN = a.Subtotal, a.Total
	if i.Currency == ledger.CurrencyUSD {
		if i.ExchangeRate == nil {
			return Amounts{}, nil, false, fmt.Errorf("%w: a USD invoice has no exchange rate", ErrInvalidInput)
		}
		rate := *i.ExchangeRate
		a.SubtotalMXN = a.Subtotal.Mul(rate).Round(2)
		a.ExpectedDepositMXN = a.Total.Mul(rate).Round(2)
		a.IVA = a.IVA.Mul(rate).Round(2)
		a.ISRWithheld = a.ISRWithheld.Mul(rate).Round(2)
		a.IVAWithheld = a.IVAWithheld.Mul(rate).Round(2)
	}
	if verr := a.Validate(); verr != nil {
		return Amounts{}, nil, false, fmt.Errorf("%w: the XML amounts are not usable: %v", ErrInvalidCFDI, verr)
	}

	var changes []Change
	for _, f := range []struct {
		field    string
		old, new decimal.Decimal
	}{
		{"subtotal", i.Subtotal, a.Subtotal}, {"iva", i.IVA, a.IVA}, {"isr_withheld", i.ISRWithheld, a.ISRWithheld},
		{"iva_withheld", i.IVAWithheld, a.IVAWithheld}, {"total", i.Total, a.Total},
	} {
		if !f.old.Round(2).Equal(f.new) {
			changes = append(changes, Change{Field: f.field, From: f.old.StringFixed(2), To: f.new.StringFixed(2)})
		}
	}
	if len(changes) > 0 {
		warnings = []Warning{{
			Code:    WarningAmountsFromXML,
			Message: "the amounts of the invoice were replaced by the ones in the stamped XML",
			Changes: changes,
		}}
	}
	return a, warnings, true, nil
}
