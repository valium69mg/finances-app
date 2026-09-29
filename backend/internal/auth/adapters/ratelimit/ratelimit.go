// Package ratelimit provides an in-memory fixed-window rate limiter.
package ratelimit

import (
	"sync"
	"time"
)

// pruneInterval is how often expired windows are swept out to bound memory.
const pruneInterval = time.Minute

type window struct {
	start time.Time
	span  time.Duration
	count int
}

func (w window) expired(now time.Time) bool { return !now.Before(w.start.Add(w.span)) }

// Limiter counts events per key in fixed windows. State is per process, which
// is enough for a single-instance deployment. Keys are independent, and a key
// may be used with different windows over time; each stored window remembers
// the span it was opened with.
type Limiter struct {
	now func() time.Time

	mu        sync.Mutex
	windows   map[string]window
	lastPrune time.Time
}

// New builds a Limiter using now as its clock (time.Now when nil).
func New(now func() time.Time) *Limiter {
	if now == nil {
		now = time.Now
	}
	return &Limiter{now: now, windows: map[string]window{}}
}

// Allow records one event under key and reports whether it fits in limit
// events per window. A rejected event is not counted.
func (l *Limiter) Allow(key string, limit int, span time.Duration) bool {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	l.prune(now)
	w := l.current(key, now, span)
	if w.count >= limit {
		l.windows[key] = w
		return false
	}
	w.count++
	l.windows[key] = w
	return true
}

// Peek reports whether one more event under key would fit in limit events per
// window, without recording anything.
func (l *Limiter) Peek(key string, limit int, span time.Duration) bool {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	l.prune(now)
	w, ok := l.windows[key]
	if !ok || w.expired(now) {
		return limit > 0
	}
	return w.count < limit
}

// Record counts one event under key unconditionally (it may push the count past
// any limit). Use it with Peek to count only some outcomes, e.g. failures.
func (l *Limiter) Record(key string, span time.Duration) {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	l.prune(now)
	w := l.current(key, now, span)
	w.count++
	l.windows[key] = w
}

// current returns the live window for key, opening a fresh one when the key is
// unknown or its window has expired. Callers must hold l.mu.
func (l *Limiter) current(key string, now time.Time, span time.Duration) window {
	w, ok := l.windows[key]
	if !ok || w.expired(now) {
		return window{start: now, span: span}
	}
	return w
}

// prune drops expired windows at most once per pruneInterval. Callers must hold l.mu.
func (l *Limiter) prune(now time.Time) {
	if now.Sub(l.lastPrune) < pruneInterval {
		return
	}
	l.lastPrune = now
	for k, w := range l.windows {
		if w.expired(now) {
			delete(l.windows, k)
		}
	}
}
