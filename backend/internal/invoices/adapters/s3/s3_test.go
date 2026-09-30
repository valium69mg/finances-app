package s3_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/valium69mg/finances-app/backend/internal/invoices/adapters/s3"
	"github.com/valium69mg/finances-app/backend/internal/invoices/app"
)

// newStore returns a Store on a throwaway bucket of the S3 server named by
// TEST_S3_ENDPOINT, TEST_S3_ACCESS_KEY and TEST_S3_SECRET_KEY, and skips the
// test when they are not set. The bucket and its objects are removed afterwards.
func newStore(t *testing.T) *s3.Store {
	t.Helper()
	endpoint, access, secret := os.Getenv("TEST_S3_ENDPOINT"), os.Getenv("TEST_S3_ACCESS_KEY"), os.Getenv("TEST_S3_SECRET_KEY")
	if endpoint == "" || access == "" || secret == "" {
		t.Skip("TEST_S3_ENDPOINT, TEST_S3_ACCESS_KEY or TEST_S3_SECRET_KEY not set; skipping S3 integration test")
	}
	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	bucket := "invoices-test-" + hex.EncodeToString(suffix)

	store, err := s3.New(s3.Config{Endpoint: endpoint, AccessKey: access, SecretKey: secret, Bucket: bucket, Region: "us-east-1"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatalf("ensure bucket: %v", err)
	}
	t.Cleanup(func() {
		admin, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(access, secret, "")})
		if err != nil {
			return
		}
		for obj := range admin.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true}) {
			_ = admin.RemoveObject(ctx, bucket, obj.Key, minio.RemoveObjectOptions{})
		}
		_ = admin.RemoveBucket(ctx, bucket)
	})
	return store
}

func TestPutGetDelete(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	data := []byte("%PDF-1.7 hello")

	if err := store.Put(ctx, "invoices/1/pdf-abc.pdf", bytes.NewReader(data), int64(len(data)), "application/pdf"); err != nil {
		t.Fatal(err)
	}
	body, err := store.Get(ctx, "invoices/1/pdf-abc.pdf")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(body)
	_ = body.Close()
	if !bytes.Equal(got, data) {
		t.Errorf("read %q, want %q", got, data)
	}

	if err := store.Delete(ctx, "invoices/1/pdf-abc.pdf"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, "invoices/1/pdf-abc.pdf"); !errors.Is(err, app.ErrObjectNotFound) {
		t.Errorf("get after delete error = %v, want ErrObjectNotFound", err)
	}
	if err := store.Delete(ctx, "invoices/1/never-existed"); err != nil {
		t.Errorf("deleting a missing key must succeed, got %v", err)
	}
}

func TestEnsureBucketIsIdempotent(t *testing.T) {
	store := newStore(t)
	if err := store.EnsureBucket(context.Background()); err != nil {
		t.Errorf("second EnsureBucket: %v", err)
	}
}

func TestGetMissingKey(t *testing.T) {
	store := newStore(t)
	if _, err := store.Get(context.Background(), strings.Repeat("x", 10)); !errors.Is(err, app.ErrObjectNotFound) {
		t.Errorf("error = %v, want ErrObjectNotFound", err)
	}
}
