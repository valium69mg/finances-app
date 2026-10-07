package domain

import (
	"bytes"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	invoices "github.com/valium69mg/finances-app/backend/internal/invoices/domain"
)

// DocumentKind is the kind of a document attached to a filing.
type DocumentKind string

const (
	// DocumentAcuse is the SAT acknowledgement of the declaration (PDF only).
	DocumentAcuse DocumentKind = "acuse"
	// DocumentComprobante is the proof of payment (PDF or image).
	DocumentComprobante DocumentKind = "comprobante"
)

// MaxDocumentBytes is the size limit of one attached file. Phone photos are the
// common comprobante, so it matches the invoice PDF limit.
const MaxDocumentBytes = invoices.MaxPDFBytes

// Errors of the document rules.
var (
	ErrInvalidDocument = errors.New("invalid document")
	ErrDocumentMissing = errors.New("document not found")
)

// Document is the metadata of a file attached to a filing. The bytes live in
// the object store under Key, which is never exposed to clients.
type Document struct {
	Period      string
	Kind        DocumentKind
	Key         string
	Name        string
	ContentType string
	Size        int64
	UploadedAt  time.Time
}

// IsValid reports whether k is a known kind.
func (k DocumentKind) IsValid() bool { return k == DocumentAcuse || k == DocumentComprobante }

// fileType is a file format accepted for a document, recognised by its magic.
type fileType struct {
	contentType string
	extensions  []string
	// declared are the extra content types a client may declare for the format.
	declared []string
	matches  func(data []byte) bool
}

var fileTypes = []fileType{
	{"application/pdf", []string{".pdf"}, nil, func(d []byte) bool { return bytes.HasPrefix(d, []byte("%PDF-")) }},
	{"image/jpeg", []string{".jpg", ".jpeg"}, []string{"image/jpg"}, func(d []byte) bool { return bytes.HasPrefix(d, []byte{0xFF, 0xD8, 0xFF}) }},
	{"image/png", []string{".png"}, nil, func(d []byte) bool { return bytes.HasPrefix(d, []byte("\x89PNG\r\n\x1a\n")) }},
	{"image/webp", []string{".webp"}, nil, func(d []byte) bool {
		return len(d) >= 12 && bytes.Equal(d[:4], []byte("RIFF")) && bytes.Equal(d[8:12], []byte("WEBP"))
	}},
	{"image/heic", []string{".heic", ".heif"}, []string{"image/heif"}, isHEIC},
}

var heicBrands = map[string]bool{
	"heic": true, "heix": true, "hevc": true, "hevx": true, "heim": true, "heis": true, "mif1": true, "msf1": true,
}

// isHEIC reports whether data is an ISO base media file with a HEIF/HEIC brand.
func isHEIC(d []byte) bool {
	return len(d) >= 12 && bytes.Equal(d[4:8], []byte("ftyp")) && heicBrands[string(d[8:12])]
}

// NewDocument validates an upload and returns its metadata (without period,
// key or upload time). The acuse is a PDF; the comprobante is a PDF or a JPEG,
// PNG, WebP or HEIC image. The format is recognised by the file's magic bytes,
// the extension and the declared content type must agree with it, and the size
// must be in 1..MaxDocumentBytes. The stored content type is the canonical one
// of the detected format and the name is reduced to a safe base name.
func NewDocument(kind DocumentKind, name, declaredType string, data []byte) (Document, error) {
	if !kind.IsValid() {
		return Document{}, fmt.Errorf("%w: unknown kind %q", ErrInvalidDocument, kind)
	}
	size := int64(len(data))
	if size == 0 {
		return Document{}, fmt.Errorf("%w: the %s file is empty", ErrInvalidDocument, kind)
	}
	if size > MaxDocumentBytes {
		return Document{}, fmt.Errorf("%w: the %s file exceeds %d bytes", ErrInvalidDocument, kind, MaxDocumentBytes)
	}
	var detected *fileType
	for i := range fileTypes {
		if fileTypes[i].matches(data) {
			detected = &fileTypes[i]
			break
		}
	}
	if detected == nil || (kind == DocumentAcuse && detected.contentType != "application/pdf") {
		if kind == DocumentAcuse {
			return Document{}, fmt.Errorf("%w: the acuse must be a PDF", ErrInvalidDocument)
		}
		return Document{}, fmt.Errorf("%w: the comprobante must be a PDF or a JPEG, PNG, WebP or HEIC image", ErrInvalidDocument)
	}
	clean := invoices.SafeName(name)
	ext := strings.ToLower(path.Ext(clean))
	okExt := false
	for _, e := range detected.extensions {
		okExt = okExt || e == ext
	}
	if !okExt {
		return Document{}, fmt.Errorf("%w: the file extension %q does not match a %s file", ErrInvalidDocument, ext, detected.contentType)
	}
	declared := strings.ToLower(strings.TrimSpace(strings.SplitN(declaredType, ";", 2)[0]))
	okType := declared == "" || declared == "application/octet-stream" || declared == detected.contentType
	for _, t := range detected.declared {
		okType = okType || declared == t
	}
	if !okType {
		return Document{}, fmt.Errorf("%w: content type %q does not match the file, which is %s", ErrInvalidDocument, declared, detected.contentType)
	}
	return Document{Kind: kind, Name: clean, ContentType: detected.contentType, Size: size}, nil
}

// KeyExtension returns the file extension (with dot) used in the storage key of
// a document with the given canonical content type.
func KeyExtension(contentType string) string {
	for _, t := range fileTypes {
		if t.contentType == contentType {
			return t.extensions[0]
		}
	}
	return ""
}
