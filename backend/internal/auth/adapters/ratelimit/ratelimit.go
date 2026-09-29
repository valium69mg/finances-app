// Package ratelimit provides an in-memory fixed-window rate limiter.
package ratelimit

import (
	"sync"
	"time"
)

type window struct {
	start time.Time
	count int
}

// Limiter counts events per key in fixed windows. State is per process, which
// is enough for a single-instance deployment.
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
// events per window.
func (l *Limiter) Allow(key string, limit int, span time.Duration) bool {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	l.prune(now, span)
	w, ok := l.windows[key]
	if !ok || !now.Before(w.start.Add(span)) {
		w = window{start: now}
	}
	if w.count >= limit {
		l.windows[key] = w
		return false
	}
	w.count++
	l.windows[key] = w
	return true
}

// prune drops expired windows at most once per span to bound memory.
func (l *Limiter) prune(now time.Time, span time.Duration) {
	if now.Sub(l.lastPrune) < span {
		return
	}
	l.lastPrune = now
	for k, w := range l.windows {
		if !now.Before(w.start.Add(span)) {
			delete(l.windows, k)
		}
	}
}
