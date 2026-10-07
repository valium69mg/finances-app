package domain_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/valium69mg/finances-app/backend/internal/taxfiling/domain"
)

var (
	pdf  = []byte("%PDF-1.7\n")
	jpeg = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00}
	png  = []byte("\x89PNG\r\n\x1a\nrest")
	webp = []byte("RIFF\x00\x00\x00\x00WEBPVP8 ")
	heic = []byte("\x00\x00\x00\x18ftypheic\x00\x00")
)

func TestNewDocument(t *testing.T) {
	for _, tc := range []struct {
		name     string
		kind     domain.DocumentKind
		file     string
		declared string
		data     []byte
		wantType string // empty means the upload must be rejected
	}{
		{"acuse pdf", domain.DocumentAcuse, "acuse.pdf", "application/pdf", pdf, "application/pdf"},
		{"acuse pdf without declared type", domain.DocumentAcuse, "ACUSE.PDF", "", pdf, "application/pdf"},
		{"acuse octet-stream", domain.DocumentAcuse, "a.pdf", "application/octet-stream", pdf, "application/pdf"},
		{"acuse image", domain.DocumentAcuse, "a.jpg", "image/jpeg", jpeg, ""},
		{"acuse fake pdf", domain.DocumentAcuse, "a.pdf", "application/pdf", []byte("hello"), ""},
		{"comprobante pdf", domain.DocumentComprobante, "pago.pdf", "application/pdf", pdf, "application/pdf"},
		{"comprobante jpeg", domain.DocumentComprobante, "IMG_1.JPG", "image/jpeg; charset=x", jpeg, "image/jpeg"},
		{"comprobante jpeg alias", domain.DocumentComprobante, "x.jpeg", "image/jpg", jpeg, "image/jpeg"},
		{"comprobante png", domain.DocumentComprobante, "x.png", "image/png", png, "image/png"},
		{"comprobante webp", domain.DocumentComprobante, "x.webp", "image/webp", webp, "image/webp"},
		{"comprobante heic", domain.DocumentComprobante, "IMG_2.HEIC", "image/heic", heic, "image/heic"},
		{"comprobante heif declared", domain.DocumentComprobante, "x.heif", "image/heif", heic, "image/heic"},
		{"comprobante gif", domain.DocumentComprobante, "x.gif", "image/gif", []byte("GIF89a"), ""},
		{"comprobante text", domain.DocumentComprobante, "x.pdf", "application/pdf", []byte("plain"), ""},
		{"comprobante wrong extension", domain.DocumentComprobante, "x.png", "image/jpeg", jpeg, ""},
		{"comprobante wrong declared type", domain.DocumentComprobante, "x.jpg", "image/png", jpeg, ""},
		{"comprobante html declared", domain.DocumentComprobante, "x.jpg", "text/html", jpeg, ""},
		{"comprobante no extension", domain.DocumentComprobante, "photo", "image/jpeg", jpeg, ""},
		{"empty", domain.DocumentComprobante, "x.pdf", "application/pdf", nil, ""},
		{"unknown kind", "otro", "x.pdf", "application/pdf", pdf, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := domain.NewDocument(tc.kind, tc.file, tc.declared, tc.data)
			if tc.wantType == "" {
				if !errors.Is(err, domain.ErrInvalidDocument) {
					t.Fatalf("err = %v, want ErrInvalidDocument", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if doc.ContentType != tc.wantType || doc.Size != int64(len(tc.data)) || doc.Kind != tc.kind {
				t.Errorf("doc = %+v", doc)
			}
		})
	}
}

func TestNewDocumentSizeLimit(t *testing.T) {
	atLimit := append(bytes.Clone(pdf), make([]byte, domain.MaxDocumentBytes-len(pdf))...)
	if _, err := domain.NewDocument(domain.DocumentAcuse, "a.pdf", "application/pdf", atLimit); err != nil {
		t.Errorf("a file at the limit must be accepted: %v", err)
	}
	if _, err := domain.NewDocument(domain.DocumentAcuse, "a.pdf", "application/pdf", append(atLimit, 0)); !errors.Is(err, domain.ErrInvalidDocument) {
		t.Errorf("a file over the limit = %v, want ErrInvalidDocument", err)
	}
}

func TestNewDocumentSanitizesTheName(t *testing.T) {
	doc, err := domain.NewDocument(domain.DocumentAcuse, `C:\Users\x\..\"acuse".pdf`, "", pdf)
	if err != nil || doc.Name != "acuse.pdf" || strings.ContainsAny(doc.Name, `\/"`) {
		t.Errorf("name = %q, %v", doc.Name, err)
	}
}

func TestKeyExtension(t *testing.T) {
	for ct, want := range map[string]string{"application/pdf": ".pdf", "image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp", "image/heic": ".heic", "x": ""} {
		if got := domain.KeyExtension(ct); got != want {
			t.Errorf("KeyExtension(%q) = %q, want %q", ct, got, want)
		}
	}
}
