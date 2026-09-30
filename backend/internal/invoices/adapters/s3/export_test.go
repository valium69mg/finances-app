package s3

import "time"

// SetClock replaces the clock of a Lazy store (tests only).
func SetClock(l *Lazy, now func() time.Time) { l.now = now }

// NextAttemptIn returns how long a Lazy store waits before its next
// preparation attempt (tests only).
func NextAttemptIn(l *Lazy) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.retryAt.Sub(l.now())
}
