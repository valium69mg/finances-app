package postgres_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	taxfiling "github.com/valium69mg/finances-app/backend/internal/taxfiling/domain"
)

func document(period string, kind taxfiling.DocumentKind, key, name string) taxfiling.Document {
	return taxfiling.Document{Period: period, Kind: kind, Key: key, Name: name, ContentType: "application/pdf", Size: 12}
}

func TestPutDocumentStoresAndReplaces(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	id := insertInvoice(t, pool, "2026-10", "emitida", uuidA)
	if _, err := repo.Create(ctx, filing("2026-10", id)); err != nil {
		t.Fatal(err)
	}

	saved, replaced, err := repo.PutDocument(ctx, document("2026-10", taxfiling.DocumentAcuse, "k1", "a.pdf"))
	if err != nil || replaced != "" || saved.UploadedAt.IsZero() {
		t.Fatalf("first put = %+v, %q, %v", saved, replaced, err)
	}
	if _, _, err := repo.PutDocument(ctx, document("2026-10", taxfiling.DocumentComprobante, "k2", "p.pdf")); err != nil {
		t.Fatal(err)
	}
	saved, replaced, err = repo.PutDocument(ctx, document("2026-10", taxfiling.DocumentAcuse, "k3", "b.pdf"))
	if err != nil || replaced != "k1" || saved.Key != "k3" {
		t.Fatalf("replace = %+v, %q, %v; want the old key k1", saved, replaced, err)
	}

	got, err := repo.Get(ctx, "2026-10")
	if err != nil || len(got.Documents) != 2 {
		t.Fatalf("Get documents = %+v, %v", got.Documents, err)
	}
	if got.Documents[0].Kind != taxfiling.DocumentAcuse || got.Documents[0].Name != "b.pdf" || got.Documents[1].Kind != taxfiling.DocumentComprobante {
		t.Errorf("documents = %+v, want acuse then comprobante", got.Documents)
	}
	doc, err := repo.GetDocument(ctx, "2026-10", taxfiling.DocumentAcuse)
	if err != nil || doc.Key != "k3" || doc.ContentType != "application/pdf" || doc.Size != 12 {
		t.Errorf("GetDocument = %+v, %v", doc, err)
	}

	list, err := repo.List(ctx)
	if err != nil || len(list) != 1 || len(list[0].Documents) != 2 {
		t.Errorf("List documents = %+v, %v", list, err)
	}
}

func TestPutDocumentOnPaidFilingAndUnknownPeriod(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	id := insertInvoice(t, pool, "2026-10", "emitida", uuidA)
	if _, err := repo.Create(ctx, filing("2026-10", id)); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MarkPaid(ctx, "2026-10", taxfiling.Payment{Date: "2026-11-12"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.PutDocument(ctx, document("2026-10", taxfiling.DocumentComprobante, "k1", "p.pdf")); err != nil {
		t.Errorf("a paid filing must accept documents: %v", err)
	}
	if _, _, err := repo.PutDocument(ctx, document("2026-01", taxfiling.DocumentAcuse, "k9", "a.pdf")); !errors.Is(err, taxfiling.ErrNotFound) {
		t.Errorf("unknown period err = %v, want ErrNotFound", err)
	}
	if _, err := repo.GetDocument(ctx, "2026-10", taxfiling.DocumentAcuse); !errors.Is(err, taxfiling.ErrDocumentMissing) {
		t.Errorf("missing document err = %v, want ErrDocumentMissing", err)
	}
}

func TestDocumentConstraints(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	id := insertInvoice(t, pool, "2026-10", "emitida", uuidA)
	if _, err := repo.Create(ctx, filing("2026-10", id)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.PutDocument(ctx, document("2026-10", "otro", "k1", "a.pdf")); err == nil {
		t.Error("an unknown kind must violate the check constraint")
	}
	bad := document("2026-10", taxfiling.DocumentAcuse, "k2", "a.pdf")
	bad.Size = 0
	if _, _, err := repo.PutDocument(ctx, bad); err == nil {
		t.Error("an empty file must violate the size check")
	}
	if _, _, err := repo.PutDocument(ctx, document("2026-10", taxfiling.DocumentAcuse, "k3", "a.pdf")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.PutDocument(ctx, document("2026-10", taxfiling.DocumentComprobante, "k3", "p.pdf")); err == nil {
		t.Error("a storage key can belong to one document only")
	}
}

func TestDeletePendingFilingReturnsDocumentKeysAndCascades(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	id := insertInvoice(t, pool, "2026-09", "emitida", uuidA)
	if _, err := repo.Create(ctx, filing("2026-09", id)); err != nil {
		t.Fatal(err)
	}
	for i, k := range []taxfiling.DocumentKind{taxfiling.DocumentAcuse, taxfiling.DocumentComprobante} {
		if _, _, err := repo.PutDocument(ctx, document("2026-09", k, []string{"ka", "kb"}[i], "x.pdf")); err != nil {
			t.Fatal(err)
		}
	}
	keys, err := repo.Delete(ctx, "2026-09")
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(keys)
	if !slices.Equal(keys, []string{"ka", "kb"}) {
		t.Errorf("keys = %v", keys)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM tax_filing_documents`).Scan(&n); err != nil || n != 0 {
		t.Errorf("document rows left = %d, %v", n, err)
	}
}

func TestDeletePaidFilingKeepsItsDocuments(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	id := insertInvoice(t, pool, "2026-10", "emitida", uuidA)
	if _, err := repo.Create(ctx, filing("2026-10", id)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.PutDocument(ctx, document("2026-10", taxfiling.DocumentAcuse, "k1", "a.pdf")); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MarkPaid(ctx, "2026-10", taxfiling.Payment{Date: "2026-11-12"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Delete(ctx, "2026-10"); !errors.Is(err, taxfiling.ErrFilingPaid) {
		t.Fatalf("err = %v", err)
	}
	if _, err := repo.GetDocument(ctx, "2026-10", taxfiling.DocumentAcuse); err != nil {
		t.Errorf("the document must survive a refused delete: %v", err)
	}
}
