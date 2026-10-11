package s3

import (
	"time"

	"github.com/minio/minio-go/v7"
)

// SetClock replaces the clock of a Lazy store (tests only).
func SetClock(l *Lazy, now func() time.Time) { l.now = now }

// NextAttemptIn returns how long a Lazy store waits before its next
// preparation attempt (tests only).
func NextAttemptIn(l *Lazy) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.retryAt.Sub(l.now())
}

// NewWithClient builds a Store on an existing client (tests only).
func NewWithClient(client *minio.Client, bucket string, createBucket bool) *Store {
	return &Store{client: client, bucket: bucket, region: "us-east-1", createBucket: createBucket}
}
