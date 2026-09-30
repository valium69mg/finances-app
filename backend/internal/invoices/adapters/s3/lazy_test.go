package s3_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/valium69mg/finances-app/backend/internal/invoices/adapters/s3"
	"github.com/valium69mg/finances-app/backend/internal/invoices/app"
)

// memStore is an in-memory app.ObjectStore.
type memStore struct{ objects map[string][]byte }

func newMem() *memStore { return &memStore{objects: map[string][]byte{}} }

func (m *memStore) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) error {
	b, _ := io.ReadAll(r)
	m.objects[key] = b
	return nil
}
func (m *memStore) Get(_ context.Context, key string) (io.ReadCloser, error) {
	b, ok := m.objects[key]
	if !ok {
		return nil, app.ErrObjectNotFound
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}
func (m *memStore) Delete(_ context.Context, key string) error {
	delete(m.objects, key)
	return nil
}

// fakeEnsure fails while down is set and counts its calls.
type fakeEnsure struct {
	down  bool
	calls atomic.Int32
}

func (f *fakeEnsure) ensure(context.Context) error {
	f.calls.Add(1)
	if f.down {
		return errors.New("connection refused")
	}
	return nil
}

func newLazy(inner app.ObjectStore, ens *fakeEnsure, clock *time.Time) *s3.Lazy {
	l := s3.NewLazy(inner, ens.ensure, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s3.SetClock(l, func() time.Time { return *clock })
	return l
}

func TestLazyStartsWhileStorageIsDownAndRecoversWithoutRestart(t *testing.T) {
	ctx := context.Background()
	ens := &fakeEnsure{down: true}
	clock := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	store := newLazy(newMem(), ens, &clock)

	// Storage down: every operation fails, none reaches the inner store.
	if err := store.Put(ctx, "k", strings.NewReader("x"), 1, "text/plain"); err == nil {
		t.Fatal("Put with the storage down must fail")
	}
	if _, err := store.Get(ctx, "k"); err == nil {
		t.Fatal("Get with the storage down must fail")
	}
	if err := store.Delete(ctx, "k"); err == nil {
		t.Fatal("Delete with the storage down must fail")
	}

	// MinIO comes back. Within the backoff window the failure is still cached
	// (no hammering) ...
	ens.down = false
	before := ens.calls.Load()
	if err := store.Put(ctx, "k", strings.NewReader("x"), 1, "text/plain"); err == nil {
		t.Fatal("still inside the backoff window, want the cached failure")
	}
	if ens.calls.Load() != before {
		t.Error("no new preparation attempt is made inside the backoff window")
	}

	// ... and once the window passed the next use prepares the bucket and works.
	clock = clock.Add(time.Minute)
	if err := store.Put(ctx, "k", strings.NewReader("hello"), 5, "text/plain"); err != nil {
		t.Fatalf("Put after recovery: %v", err)
	}
	body, err := store.Get(ctx, "k")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := io.ReadAll(body); string(got) != "hello" {
		t.Errorf("stored %q", got)
	}
	if _, err := store.Get(ctx, "missing"); !errors.Is(err, app.ErrObjectNotFound) {
		t.Errorf("a missing key stays ErrObjectNotFound, got %v", err)
	}

	// Prepared once: no further attempts.
	calls := ens.calls.Load()
	for range 3 {
		if err := store.Delete(ctx, "k"); err != nil {
			t.Fatal(err)
		}
	}
	if ens.calls.Load() != calls {
		t.Error("a prepared storage must not be prepared again")
	}
}

func TestLazyBackoffGrowsAndIsCapped(t *testing.T) {
	ctx := context.Background()
	ens := &fakeEnsure{down: true}
	clock := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	store := newLazy(newMem(), ens, &clock)

	var last time.Duration
	for range 12 {
		if err := store.Prepare(ctx); err == nil {
			t.Fatal("storage is down")
		}
		wait := s3.NextAttemptIn(store)
		if wait < last {
			t.Fatalf("backoff shrank from %s to %s", last, wait)
		}
		last = wait
		clock = clock.Add(wait)
	}
	if last != 30*time.Second {
		t.Errorf("backoff = %s, want it capped at 30s", last)
	}
}

func TestLazyPrepareReportsTheStartupState(t *testing.T) {
	clock := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	ens := &fakeEnsure{}
	if err := newLazy(newMem(), ens, &clock).Prepare(context.Background()); err != nil {
		t.Errorf("Prepare with the storage up: %v", err)
	}
	ens = &fakeEnsure{down: true}
	if err := newLazy(newMem(), ens, &clock).Prepare(context.Background()); err == nil {
		t.Error("Prepare with the storage down must report it (non-fatal for the caller)")
	}
}

func TestDisabledFailsEveryOperation(t *testing.T) {
	ctx := context.Background()
	d := s3.NewDisabled("S3_ACCESS_KEY is not set")
	if err := d.Put(ctx, "k", strings.NewReader("x"), 1, ""); !errors.Is(err, s3.ErrDisabled) || !strings.Contains(err.Error(), "S3_ACCESS_KEY") {
		t.Errorf("Put: %v", err)
	}
	if _, err := d.Get(ctx, "k"); !errors.Is(err, s3.ErrDisabled) {
		t.Errorf("Get: %v", err)
	}
	if err := d.Delete(ctx, "k"); !errors.Is(err, s3.ErrDisabled) {
		t.Errorf("Delete: %v", err)
	}
}
