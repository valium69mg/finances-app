package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/valium69mg/finances-app/backend/internal/invoices/app"
)

// ErrDisabled is returned by every operation of a Disabled store.
var ErrDisabled = errors.New("object storage is not configured")

// Disabled is an app.ObjectStore for a deployment without object storage
// configured. Every operation fails with ErrDisabled, which the invoices
// service reports as app.ErrStorage (503 storage_unavailable) on the file
// routes only; the rest of the API is unaffected.
type Disabled struct{ reason string }

var _ app.ObjectStore = (*Disabled)(nil)

// NewDisabled builds a Disabled store; reason says what is missing and ends up
// in the error message (never put a secret in it).
func NewDisabled(reason string) *Disabled { return &Disabled{reason: reason} }

func (d *Disabled) err() error { return fmt.Errorf("%w: %s", ErrDisabled, d.reason) }

// Put always fails with ErrDisabled.
func (d *Disabled) Put(context.Context, string, io.Reader, int64, string) error { return d.err() }

// Get always fails with ErrDisabled.
func (d *Disabled) Get(context.Context, string) (io.ReadCloser, error) { return nil, d.err() }

// Delete always fails with ErrDisabled.
func (d *Disabled) Delete(context.Context, string) error { return d.err() }

// Backoff bounds of Lazy: after a failed preparation the next attempt waits
// backoffMin, doubling up to backoffMax, so a down storage costs one quick
// attempt per window instead of one per request.
const (
	backoffMin     = time.Second
	backoffMax     = 30 * time.Second
	ensureAttempts = 10 * time.Second
)

// Lazy defers the preparation of the storage (ensuring the bucket) to the
// first use and retries it with backoff until it succeeds. The API can then
// start while the storage is down: operations fail while it is unavailable and
// work again as soon as it is back, without a restart. Once prepared it adds no
// overhead beyond an atomic-free mutex-guarded flag check.
type Lazy struct {
	inner  app.ObjectStore
	ensure func(context.Context) error
	logger *slog.Logger
	now    func() time.Time

	mu      sync.Mutex
	ready   bool
	failed  bool // at least one attempt failed since the last success
	fails   int
	retryAt time.Time
	lastErr error
}

var _ app.ObjectStore = (*Lazy)(nil)

// NewLazy wraps inner so that ensure (typically Store.EnsureBucket) runs before
// the first operation and again, with backoff, after a failure. A nil logger
// selects slog.Default().
func NewLazy(inner app.ObjectStore, ensure func(context.Context) error, logger *slog.Logger) *Lazy {
	if logger == nil {
		logger = slog.Default()
	}
	return &Lazy{inner: inner, ensure: ensure, logger: logger, now: time.Now}
}

// Prepare runs the preparation now (used at startup for an early, non-fatal
// check). The result is the same error the operations would report.
func (l *Lazy) Prepare(ctx context.Context) error { return l.prepared(ctx) }

func (l *Lazy) prepared(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.ready {
		return nil
	}
	if now := l.now(); now.Before(l.retryAt) {
		return fmt.Errorf("object storage unavailable, retrying in %s: %w", l.retryAt.Sub(now).Round(time.Second), l.lastErr)
	}

	attemptCtx, cancel := context.WithTimeout(ctx, ensureAttempts)
	defer cancel()
	if err := l.ensure(attemptCtx); err != nil {
		l.fails++
		l.failed = true
		l.lastErr = err
		wait := min(backoffMin<<min(l.fails-1, 10), backoffMax)
		l.retryAt = l.now().Add(wait)
		l.logger.Warn("object storage is not available", "error", err, "retry_in", wait.String())
		return fmt.Errorf("object storage unavailable: %w", err)
	}
	if l.failed {
		l.logger.Info("object storage is available again")
	}
	l.ready, l.failed, l.fails, l.lastErr = true, false, 0, nil
	return nil
}

// Put prepares the storage if needed and stores the object.
func (l *Lazy) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	if err := l.prepared(ctx); err != nil {
		return err
	}
	return l.inner.Put(ctx, key, r, size, contentType)
}

// Get prepares the storage if needed and returns the object body.
func (l *Lazy) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := l.prepared(ctx); err != nil {
		return nil, err
	}
	return l.inner.Get(ctx, key)
}

// Delete prepares the storage if needed and removes the object.
func (l *Lazy) Delete(ctx context.Context, key string) error {
	if err := l.prepared(ctx); err != nil {
		return err
	}
	return l.inner.Delete(ctx, key)
}
