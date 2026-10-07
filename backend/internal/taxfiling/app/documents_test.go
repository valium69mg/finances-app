package app_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/valium69mg/finances-app/backend/internal/taxfiling/app"
	taxfiling "github.com/valium69mg/finances-app/backend/internal/taxfiling/domain"
)

// fakeStore is an in-memory ObjectStore that records what was deleted.
type fakeStore struct {
	objects   map[string][]byte
	deleted   []string
	putErr    error
	deleteErr error
}

func newFakeStore() *fakeStore { return &fakeStore{objects: map[string][]byte{}} }

func (f *fakeStore) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) error {
	if f.putErr != nil {
		return f.putErr
	}
	b, _ := io.ReadAll(r)
	f.objects[key] = b
	return nil
}

func (f *fakeStore) Get(_ context.Context, key string) (io.ReadCloser, error) {
	b, ok := f.objects[key]
	if !ok {
		return nil, app.ErrObjectNotFound
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func (f *fakeStore) Delete(_ context.Context, key string) error {
	f.deleted = append(f.deleted, key)
	if f.deleteErr != nil {
		return f.deleteErr
	}
	delete(f.objects, key)
	return nil
}

var pdfBytes = []byte("%PDF-1.7\nbody")

func pdfUpload(name string) app.Upload {
	return app.Upload{Name: name, ContentType: "application/pdf", Data: pdfBytes}
}

func registered(t *testing.T, fx *fixture, paid bool) {
	t.Helper()
	in := app.RegisterInput{Period: "2026-10"}
	if paid {
		in.Payment = &app.PaymentInput{ISRPaid: d("1"), IVAPaid: d("0")}
	}
	if _, err := fx.svc.Register(context.Background(), in); err != nil {
		t.Fatal(err)
	}
}

func TestAttachDocumentStoresAndReplaces(t *testing.T) {
	fx := newFixture()
	registered(t, fx, false)
	ctx := context.Background()

	f, err := fx.svc.AttachDocument(ctx, "2026-10", taxfiling.DocumentAcuse, pdfUpload("acuse.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	first, ok := f.Document(taxfiling.DocumentAcuse)
	if !ok || first.Name != "acuse.pdf" || first.ContentType != "application/pdf" || first.Size != int64(len(pdfBytes)) {
		t.Fatalf("document = %+v, %v", first, ok)
	}
	if _, in := fx.store.objects[first.Key]; !in {
		t.Fatal("the object was not stored")
	}

	f, err = fx.svc.AttachDocument(ctx, "2026-10", taxfiling.DocumentAcuse, pdfUpload("nuevo.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	second, _ := f.Document(taxfiling.DocumentAcuse)
	if second.Key == first.Key || second.Name != "nuevo.pdf" || len(f.Documents) != 1 {
		t.Fatalf("replace = %+v (docs %d)", second, len(f.Documents))
	}
	if _, in := fx.store.objects[first.Key]; in || len(fx.store.deleted) != 1 || fx.store.deleted[0] != first.Key {
		t.Errorf("the replaced object must be deleted, deleted = %v", fx.store.deleted)
	}
	if _, in := fx.store.objects[second.Key]; !in {
		t.Error("the new object must stay")
	}
}

func TestAttachDocumentKinds(t *testing.T) {
	fx := newFixture()
	registered(t, fx, false)
	ctx := context.Background()
	jpg := app.Upload{Name: "IMG_1.JPG", ContentType: "image/jpeg", Data: []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 1}}

	if _, err := fx.svc.AttachDocument(ctx, "2026-10", taxfiling.DocumentAcuse, jpg); !errors.Is(err, taxfiling.ErrInvalidDocument) {
		t.Errorf("an image acuse = %v, want ErrInvalidDocument", err)
	}
	f, err := fx.svc.AttachDocument(ctx, "2026-10", taxfiling.DocumentComprobante, jpg)
	if err != nil {
		t.Fatal(err)
	}
	if d, _ := f.Document(taxfiling.DocumentComprobante); d.ContentType != "image/jpeg" {
		t.Errorf("content type = %q", d.ContentType)
	}
	if _, err := fx.svc.AttachDocument(ctx, "2026-10", "otro", pdfUpload("a.pdf")); !errors.Is(err, taxfiling.ErrInvalidDocument) {
		t.Errorf("unknown kind = %v", err)
	}
	if len(fx.store.objects) != 1 {
		t.Errorf("rejected uploads must not reach the store, objects = %d", len(fx.store.objects))
	}
}

func TestAttachDocumentOnPaidFiling(t *testing.T) {
	fx := newFixture()
	registered(t, fx, true)
	f, err := fx.svc.AttachDocument(context.Background(), "2026-10", taxfiling.DocumentComprobante, pdfUpload("pago.pdf"))
	if err != nil || len(f.Documents) != 1 || f.Payment == nil {
		t.Fatalf("paid filing must accept documents: %+v, %v", f, err)
	}
}

func TestAttachDocumentUnknownFiling(t *testing.T) {
	fx := newFixture()
	if _, err := fx.svc.AttachDocument(context.Background(), "2026-08", taxfiling.DocumentAcuse, pdfUpload("a.pdf")); !errors.Is(err, taxfiling.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
	if _, err := fx.svc.AttachDocument(context.Background(), "x", taxfiling.DocumentAcuse, pdfUpload("a.pdf")); !errors.Is(err, taxfiling.ErrInvalidInput) {
		t.Errorf("bad period = %v", err)
	}
}

func TestAttachDocumentStorageFailure(t *testing.T) {
	fx := newFixture()
	registered(t, fx, false)
	fx.store.putErr = errors.New("minio down")
	_, err := fx.svc.AttachDocument(context.Background(), "2026-10", taxfiling.DocumentAcuse, pdfUpload("a.pdf"))
	if !errors.Is(err, app.ErrStorage) {
		t.Errorf("err = %v, want ErrStorage", err)
	}
	if len(fx.repo.filings["2026-10"].Documents) != 0 {
		t.Error("no row may be written when the store fails")
	}
}

func TestAttachDocumentRowFailureRemovesTheNewObject(t *testing.T) {
	fx := newFixture()
	registered(t, fx, false)
	fx.repo.putDocErr = errors.New("db down")
	if _, err := fx.svc.AttachDocument(context.Background(), "2026-10", taxfiling.DocumentAcuse, pdfUpload("a.pdf")); err == nil {
		t.Fatal("want an error")
	}
	if len(fx.store.objects) != 0 || len(fx.store.deleted) != 1 {
		t.Errorf("the new object must be removed, objects = %d deleted = %v", len(fx.store.objects), fx.store.deleted)
	}
}

func TestDeleteRemovesTheObjects(t *testing.T) {
	fx := newFixture()
	registered(t, fx, false)
	ctx := context.Background()
	for _, k := range []taxfiling.DocumentKind{taxfiling.DocumentAcuse, taxfiling.DocumentComprobante} {
		if _, err := fx.svc.AttachDocument(ctx, "2026-10", k, pdfUpload("a.pdf")); err != nil {
			t.Fatal(err)
		}
	}
	if err := fx.svc.Delete(ctx, "2026-10"); err != nil {
		t.Fatal(err)
	}
	if len(fx.store.objects) != 0 || len(fx.store.deleted) != 2 {
		t.Errorf("objects = %d deleted = %v, want both removed", len(fx.store.objects), fx.store.deleted)
	}
}

func TestDeleteIgnoresObjectDeletionFailures(t *testing.T) {
	fx := newFixture()
	registered(t, fx, false)
	ctx := context.Background()
	if _, err := fx.svc.AttachDocument(ctx, "2026-10", taxfiling.DocumentAcuse, pdfUpload("a.pdf")); err != nil {
		t.Fatal(err)
	}
	fx.store.deleteErr = errors.New("minio down")
	if err := fx.svc.Delete(ctx, "2026-10"); err != nil {
		t.Errorf("a best-effort object delete must not fail the request: %v", err)
	}
}

func TestDeletePaidFilingKeepsItsObjects(t *testing.T) {
	fx := newFixture()
	registered(t, fx, true)
	ctx := context.Background()
	if _, err := fx.svc.AttachDocument(ctx, "2026-10", taxfiling.DocumentAcuse, pdfUpload("a.pdf")); err != nil {
		t.Fatal(err)
	}
	if err := fx.svc.Delete(ctx, "2026-10"); !errors.Is(err, taxfiling.ErrFilingPaid) {
		t.Fatalf("err = %v", err)
	}
	if len(fx.store.objects) != 1 || len(fx.store.deleted) != 0 {
		t.Error("a refused delete must keep the objects")
	}
}

func TestDownload(t *testing.T) {
	fx := newFixture()
	registered(t, fx, false)
	ctx := context.Background()
	f, err := fx.svc.AttachDocument(ctx, "2026-10", taxfiling.DocumentAcuse, pdfUpload("acuse.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	doc, body, err := fx.svc.Download(ctx, "2026-10", taxfiling.DocumentAcuse)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(body)
	body.Close()
	if !bytes.Equal(got, pdfBytes) || doc.Name != "acuse.pdf" {
		t.Errorf("got %q / %+v", got, doc)
	}

	if _, _, err := fx.svc.Download(ctx, "2026-10", taxfiling.DocumentComprobante); !errors.Is(err, taxfiling.ErrDocumentMissing) {
		t.Errorf("missing document = %v", err)
	}
	if _, _, err := fx.svc.Download(ctx, "2026-10", "otro"); !errors.Is(err, taxfiling.ErrInvalidDocument) {
		t.Errorf("bad kind = %v", err)
	}
	stored, _ := f.Document(taxfiling.DocumentAcuse)
	delete(fx.store.objects, stored.Key)
	if _, _, err := fx.svc.Download(ctx, "2026-10", taxfiling.DocumentAcuse); !errors.Is(err, taxfiling.ErrDocumentMissing) {
		t.Errorf("object gone = %v, want ErrDocumentMissing", err)
	}
}
