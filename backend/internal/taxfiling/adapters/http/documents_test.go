package taxfilinghttp_test

import (
	"bytes"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/valium69mg/finances-app/backend/internal/taxfiling/app"
	taxfiling "github.com/valium69mg/finances-app/backend/internal/taxfiling/domain"
)

func multipartBody(t *testing.T, field, filename, contentType string, data []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if field != "" {
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", `form-data; name="`+field+`"; filename="`+filename+`"`)
		h.Set("Content-Type", contentType)
		part, err := w.CreatePart(h)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write(data)
	}
	_ = w.Close()
	return &buf, w.FormDataContentType()
}

func put(h http.Handler, path string, body *bytes.Buffer, contentType string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPut, path, body)
	req.Header.Set("Authorization", "Bearer good")
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func filingWithDocument() taxfiling.Filing {
	f := sampleFiling()
	f.Documents = []taxfiling.Document{{
		Period: "2026-10", Kind: taxfiling.DocumentAcuse, Key: "tax-filings/secret-key", Name: "acuse.pdf",
		ContentType: "application/pdf", Size: 9, UploadedAt: time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC),
	}}
	return f
}

func TestAttachDocument(t *testing.T) {
	svc := &fakeService{attached: filingWithDocument()}
	body, ct := multipartBody(t, "file", "acuse.pdf", "application/pdf", []byte("%PDF-1.4 x"))
	rec := put(newServer(svc), "/tax-filing/2026-10/documents/acuse", body, ct)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if svc.gotPeriod != "2026-10" || svc.gotKind != taxfiling.DocumentAcuse || svc.gotUpload.Name != "acuse.pdf" ||
		svc.gotUpload.ContentType != "application/pdf" || string(svc.gotUpload.Data) != "%PDF-1.4 x" {
		t.Errorf("service got %q %q %+v", svc.gotPeriod, svc.gotKind, svc.gotUpload)
	}
	out := decode(t, rec)
	docs, _ := out["documents"].([]any)
	if len(docs) != 1 {
		t.Fatalf("documents = %v", out["documents"])
	}
	doc := docs[0].(map[string]any)
	if doc["kind"] != "acuse" || doc["filename"] != "acuse.pdf" || doc["content_type"] != "application/pdf" ||
		doc["size"] != float64(9) || doc["uploaded_at"] != "2026-11-06T09:00:00Z" {
		t.Errorf("document = %v", doc)
	}
	if strings.Contains(rec.Body.String(), "secret-key") {
		t.Error("the storage key must never be exposed")
	}
}

func TestAttachDocumentErrors(t *testing.T) {
	good, ct := multipartBody(t, "file", "acuse.pdf", "application/pdf", []byte("%PDF-1.4 x"))
	noFile, noFileCT := multipartBody(t, "", "", "", nil)
	big := bytes.Repeat([]byte("a"), taxfiling.MaxDocumentBytes+2<<20)
	bigBody, bigCT := multipartBody(t, "file", "acuse.pdf", "application/pdf", big)

	for _, tc := range []struct {
		name   string
		path   string
		body   *bytes.Buffer
		ct     string
		err    error
		status int
		code   string
	}{
		{"bad kind", "/tax-filing/2026-10/documents/otro", good, ct, nil, 400, "invalid_document"},
		{"no file", "/tax-filing/2026-10/documents/acuse", noFile, noFileCT, nil, 400, "invalid_document"},
		{"not multipart", "/tax-filing/2026-10/documents/acuse", bytes.NewBufferString("x"), "text/plain", nil, 400, "invalid_request"},
		{"request too large", "/tax-filing/2026-10/documents/acuse", bigBody, bigCT, nil, 413, "request_too_large"},
		{"rejected by the domain", "/tax-filing/2026-10/documents/acuse", good, ct, taxfiling.ErrInvalidDocument, 400, "invalid_document"},
		{"unknown filing", "/tax-filing/2026-10/documents/acuse", good, ct, taxfiling.ErrNotFound, 404, "not_found"},
		{"storage down", "/tax-filing/2026-10/documents/acuse", good, ct, app.ErrStorage, 503, "storage_unavailable"},
		{"bad period", "/tax-filing/x/documents/acuse", good, ct, taxfiling.ErrInvalidInput, 400, "invalid_filing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := put(newServer(&fakeService{err: tc.err}), tc.path, bytes.NewBuffer(tc.body.Bytes()), tc.ct)
			if rec.Code != tc.status || decode(t, rec)["error"] != tc.code {
				t.Errorf("= %d %s, want %d %s", rec.Code, rec.Body, tc.status, tc.code)
			}
		})
	}
}

func TestDownloadDocument(t *testing.T) {
	svc := &fakeService{
		downloadDoc:  taxfiling.Document{Kind: taxfiling.DocumentComprobante, Name: "pago 1.jpg", ContentType: "image/jpeg", Size: 5},
		downloadBody: "bytes",
	}
	rec := do(newServer(svc), "GET", "/tax-filing/2026-10/documents/comprobante", "")
	if rec.Code != http.StatusOK || rec.Body.String() != "bytes" {
		t.Fatalf("= %d %q", rec.Code, rec.Body)
	}
	if svc.gotPeriod != "2026-10" || svc.gotKind != taxfiling.DocumentComprobante {
		t.Errorf("service got %q %q", svc.gotPeriod, svc.gotKind)
	}
	for k, want := range map[string]string{
		"Content-Type":            "image/jpeg",
		"Content-Disposition":     `attachment; filename="pago 1.jpg"`,
		"Content-Length":          "5",
		"Cache-Control":           "no-store",
		"X-Content-Type-Options":  "nosniff",
		"Content-Security-Policy": "default-src 'none'; sandbox",
	} {
		if got := rec.Header().Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}

func TestDownloadDocumentErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		path   string
		err    error
		status int
		code   string
	}{
		{"bad kind", "/tax-filing/2026-10/documents/otro", nil, 400, "invalid_document"},
		{"missing document", "/tax-filing/2026-10/documents/acuse", taxfiling.ErrDocumentMissing, 404, "not_found"},
		{"unknown filing", "/tax-filing/2026-10/documents/acuse", taxfiling.ErrNotFound, 404, "not_found"},
		{"storage down", "/tax-filing/2026-10/documents/acuse", errors.Join(app.ErrStorage), 503, "storage_unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(newServer(&fakeService{err: tc.err}), "GET", tc.path, "")
			if rec.Code != tc.status || decode(t, rec)["error"] != tc.code {
				t.Errorf("= %d %s, want %d %s", rec.Code, rec.Body, tc.status, tc.code)
			}
		})
	}
}

func TestFilingJSONAlwaysHasDocuments(t *testing.T) {
	svc := &fakeService{list: []taxfiling.Filing{sampleFiling()}}
	rec := do(newServer(svc), "GET", "/tax-filing", "")
	if !strings.Contains(rec.Body.String(), `"documents":[]`) {
		t.Errorf("an undocumented filing must carry an empty documents array: %s", rec.Body)
	}
}
