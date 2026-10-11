package s3_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/valium69mg/finances-app/backend/internal/invoices/adapters/s3"
)

// fakeS3 answers bucket HEAD requests and records every request. When exists
// is false the bucket is missing until a PUT creates it.
type fakeS3 struct {
	mu       sync.Mutex
	exists   bool
	requests []string
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.Method)
	switch {
	case r.Method == http.MethodHead && f.exists:
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodHead:
		w.WriteHeader(http.StatusNotFound)
	case r.Method == http.MethodPut:
		f.exists = true
		w.WriteHeader(http.StatusOK)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (f *fakeS3) sawPut() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, m := range f.requests {
		if m == http.MethodPut {
			return true
		}
	}
	return false
}

func newFakeStore(t *testing.T, exists, create bool) (*s3.Store, *fakeS3) {
	t.Helper()
	fake := &fakeS3{exists: exists}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	client, err := minio.New(strings.TrimPrefix(srv.URL, "http://"), &minio.Options{
		Creds:  credentials.NewStaticV4("k", "s", ""),
		Region: "us-east-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	return s3.NewWithClient(client, "docs", create), fake
}

func TestEnsureBucketCreatesMissingBucketWhenAllowed(t *testing.T) {
	store, fake := newFakeStore(t, false, true)
	if err := store.EnsureBucket(context.Background()); err != nil {
		t.Fatalf("EnsureBucket: %v", err)
	}
	if !fake.sawPut() {
		t.Error("the missing bucket was not created")
	}
}

func TestEnsureBucketNeverCreatesWithTheInstanceRole(t *testing.T) {
	store, fake := newFakeStore(t, false, false)
	err := store.EnsureBucket(context.Background())
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("EnsureBucket error = %v, want a missing bucket error", err)
	}
	if fake.sawPut() {
		t.Error("a store that may not create buckets sent a create request")
	}
}

func TestEnsureBucketAcceptsAnExistingBucket(t *testing.T) {
	store, fake := newFakeStore(t, true, false)
	if err := store.EnsureBucket(context.Background()); err != nil {
		t.Fatalf("EnsureBucket: %v", err)
	}
	if fake.sawPut() {
		t.Error("an existing bucket must not be created again")
	}
}
