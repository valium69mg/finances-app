package domain_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	invoices "github.com/valium69mg/finances-app/backend/internal/invoices/domain"
)

func TestNewDocument(t *testing.T) {
	pdf := []byte("%PDF-1.7\nbody")
	sum := sha256.Sum256(pdf)

	doc, err := invoices.NewDocument(invoices.DocumentPDF, `C:\Users\me\factura 1.PDF`, "application/pdf; charset=binary", pdf)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Name != "factura 1.PDF" || doc.ContentType != "application/pdf" || doc.Size != int64(len(pdf)) ||
		doc.SHA256 != hex.EncodeToString(sum[:]) || doc.Kind != invoices.DocumentPDF {
		t.Errorf("doc = %+v", doc)
	}

	xmlDoc, err := invoices.NewDocument(invoices.DocumentXML, "cfdi.xml", "text/xml", []byte("<a/>"))
	if err != nil || xmlDoc.ContentType != "application/xml" {
		t.Errorf("xml doc = %+v, %v", xmlDoc, err)
	}
	// An empty or generic declared type is accepted; the stored type is canonical.
	if _, err := invoices.NewDocument(invoices.DocumentXML, "cfdi.xml", "", []byte("<a/>")); err != nil {
		t.Errorf("empty declared type: %v", err)
	}
}

func TestNewDocumentRejects(t *testing.T) {
	cases := []struct {
		name string
		kind invoices.DocumentKind
		file string
		ct   string
		data []byte
	}{
		{"unknown kind", "exe", "a.exe", "", []byte("x")},
		{"empty", invoices.DocumentXML, "a.xml", "", nil},
		{"wrong extension", invoices.DocumentXML, "a.txt", "", []byte("<a/>")},
		{"pdf extension on xml", invoices.DocumentXML, "a.pdf", "", []byte("<a/>")},
		{"no extension", invoices.DocumentPDF, "a", "", []byte("%PDF-")},
		{"html content type", invoices.DocumentXML, "a.xml", "text/html", []byte("<a/>")},
		{"pdf declared as image", invoices.DocumentPDF, "a.pdf", "image/png", []byte("%PDF-")},
		{"pdf without magic", invoices.DocumentPDF, "a.pdf", "application/pdf", []byte("<html>")},
		{"xml too big", invoices.DocumentXML, "a.xml", "", bytes.Repeat([]byte("a"), invoices.MaxXMLBytes+1)},
		{"pdf too big", invoices.DocumentPDF, "a.pdf", "", append([]byte("%PDF-"), make([]byte, invoices.MaxPDFBytes)...)},
	}
	for _, tc := range cases {
		if _, err := invoices.NewDocument(tc.kind, tc.file, tc.ct, tc.data); !errors.Is(err, invoices.ErrInvalidDocument) {
			t.Errorf("%s: error = %v, want ErrInvalidDocument", tc.name, err)
		}
	}
}

func TestSafeName(t *testing.T) {
	cases := map[string]string{
		"../../etc/passwd":       "passwd",
		`a\b\c.xml`:              "c.xml",
		"we\"ird\r\n.pdf":        "weird.pdf",
		"":                       "document",
		"..":                     "document",
		"   ":                    "document",
		"factura ñ.xml":          "factura ñ.xml",
		strings.Repeat("é", 300): strings.Repeat("é", 100),
	}
	for in, want := range cases {
		if got := invoices.SafeName(in); got != want {
			t.Errorf("SafeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDuplicateWarning(t *testing.T) {
	if invoices.DuplicateWarning(nil) != nil {
		t.Error("no duplicates must give no warning")
	}
	w := invoices.DuplicateWarning([]int{4, 9})
	if len(w) != 1 || w[0].Code != invoices.WarningPossibleDuplicate || len(w[0].InvoiceIDs) != 2 {
		t.Errorf("warning = %+v", w)
	}
}
